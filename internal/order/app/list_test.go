package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type orderTestUOW struct{ calls int }

func (u *orderTestUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	u.calls++
	return fn(ctx)
}

type orderTestStore struct {
	records  []orderport.Record
	total    int64
	listErr  error
	countErr error
	filters  []orderport.Filter
}

func (s *orderTestStore) List(_ context.Context, filter orderport.Filter) ([]orderport.Record, error) {
	s.filters = append(s.filters, filter)
	return s.records, s.listErr
}
func (s *orderTestStore) Count(_ context.Context, filter orderport.Filter) (int64, error) {
	s.filters = append(s.filters, filter)
	return s.total, s.countErr
}

type orderTestCustomers struct {
	projection contactport.CustomerProjection
	err        error
	calls      int
}

func (r *orderTestCustomers) ReadCustomer(context.Context, contactport.CustomerID) (contactport.CustomerProjection, error) {
	r.calls++
	return r.projection, r.err
}

type orderTestProducts struct {
	product productport.Product
	err     error
	calls   int
}

func (r *orderTestProducts) ReadProduct(context.Context, productport.ID) (productport.Product, error) {
	r.calls++
	return r.product, r.err
}

func validOrderRecord() orderport.Record {
	now := time.Date(2026, 8, 15, 8, 30, 0, 0, time.UTC)
	customerID, productID := int64(7), int64(9)
	return orderport.Record{
		ID: 11, Provider: "wechat", ProviderLabel: "微信支付", MerchantOrderNo: "M-11", PlatformTransactionNo: "WX-11",
		CustomerID: &customerID, PayerNameSnapshot: "旧客户", MobileSnapshot: "13800000000", IdentityKind: "external_userid", IdentityValue: "wmid-11",
		ProductID: &productID, ProductCode: "SKU-9", ProductNameSnapshot: "旧商品", AmountMinor: 9901, Currency: "CNY",
		Status: "paid", StatusLabel: "已支付", DetailURL: "/api/admin/orders/11", CreatedAt: now, UpdatedAt: now,
	}
}

func TestI03ListNormalizesProjectsAndReadsThroughPorts(t *testing.T) {
	uow := &orderTestUOW{}
	store := &orderTestStore{records: []orderport.Record{validOrderRecord()}, total: 3}
	customers := &orderTestCustomers{projection: contactport.CustomerProjection{ID: 7, Name: "新客户"}}
	products := &orderTestProducts{product: productport.Product{ID: 9, Name: "新商品"}}
	page, err := NewService(uow, store, customers, products).List(context.Background(), orderport.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if uow.calls != 1 || len(store.filters) != 2 || customers.calls != 1 || products.calls != 1 {
		t.Fatalf("collaborator calls uow/store/customer/product=%d/%d/%d/%d", uow.calls, len(store.filters), customers.calls, products.calls)
	}
	if store.filters[0].Provider != "all" || store.filters[0].Limit != 50 || store.filters[0].Offset != 0 {
		t.Fatalf("normalized filter=%+v", store.filters[0])
	}
	if page.Total != 3 || page.Limit != 50 || !page.HasMore || len(page.Items) != 1 {
		t.Fatalf("page=%+v", page)
	}
	item := page.Items[0]
	if item.MerchantOrderNo != "M-11" || item.OutTradeNo != "M-11" || item.OrderNo != "M-11" || item.TransactionID != "WX-11" || item.PlatformTransactionNo != "WX-11" {
		t.Fatalf("aliases=%+v", item)
	}
	if item.RecordOrigin != orderport.RecordOriginNative || item.PayerName != "新客户" || item.ProductName != "新商品" || item.AmountYuan != "99.01" || item.ExternalUserID != "wmid-11" || item.UserID != "" || item.UnionID != "" {
		t.Fatalf("projection=%+v", item)
	}
}

func TestI03ListProjectsV1HistoryOrigin(t *testing.T) {
	record := validOrderRecord()
	record.RecordOrigin = orderport.RecordOriginV1History
	page, err := NewService(&orderTestUOW{}, &orderTestStore{records: []orderport.Record{record}, total: 1}, &orderTestCustomers{projection: contactport.CustomerProjection{ID: 7}}, &orderTestProducts{product: productport.Product{ID: 9}}).List(context.Background(), orderport.Filter{})
	if err != nil || len(page.Items) != 1 || page.Items[0].RecordOrigin != orderport.RecordOriginV1History || page.Items[0].Status != record.Status || page.Items[0].AmountYuan != "99.01" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

func TestI03ListRejectsUnknownRecordOrigin(t *testing.T) {
	record := validOrderRecord()
	record.RecordOrigin = "unknown"
	if _, err := NewService(&orderTestUOW{}, &orderTestStore{records: []orderport.Record{record}, total: 1}, &orderTestCustomers{}, &orderTestProducts{}).List(context.Background(), orderport.Filter{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error=%v", err)
	}
}

func TestI03ListBoundaryAndSnapshotFallback(t *testing.T) {
	record := validOrderRecord()
	store := &orderTestStore{records: []orderport.Record{record}, total: 1_000_001}
	customers := &orderTestCustomers{err: contactport.ErrCustomerReadNotFound}
	products := &orderTestProducts{err: productport.ErrProductReadNotFound}
	page, err := NewService(&orderTestUOW{}, store, customers, products).List(context.Background(), orderport.Filter{Provider: " WECHAT ", Limit: 100, Offset: 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if page.HasMore || page.Items[0].PayerName != "旧客户" || page.Items[0].ProductName != "旧商品" {
		t.Fatalf("page=%+v", page)
	}
}

func TestI03ListRejectsInvalidFiltersBeforeTransaction(t *testing.T) {
	before, after := time.Now().UTC(), time.Now().UTC().Add(-time.Hour)
	for _, filter := range []orderport.Filter{
		{Provider: "stripe"}, {Limit: -1}, {Limit: 101}, {Offset: -1}, {Offset: 1_000_001},
		{OrderNo: strings.Repeat("界", 201)}, {CreatedFrom: &before, CreatedTo: &after},
	} {
		uow := &orderTestUOW{}
		_, err := NewService(uow, &orderTestStore{}, &orderTestCustomers{}, &orderTestProducts{}).List(context.Background(), filter)
		if !errors.Is(err, ErrInvalidArgument) || uow.calls != 0 {
			t.Fatalf("filter=%+v error=%v uow=%d", filter, err, uow.calls)
		}
	}
}

func TestI03ListFailsClosedOnStorePortAndProjectionErrors(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		store     *orderTestStore
		customers *orderTestCustomers
		products  *orderTestProducts
	}{
		{"list", &orderTestStore{listErr: errors.New("db")}, &orderTestCustomers{}, &orderTestProducts{}},
		{"count", &orderTestStore{records: []orderport.Record{validOrderRecord()}, countErr: errors.New("db")}, &orderTestCustomers{}, &orderTestProducts{}},
		{"customer", &orderTestStore{records: []orderport.Record{validOrderRecord()}, total: 1}, &orderTestCustomers{err: contactport.ErrCustomerReadUnavailable}, &orderTestProducts{}},
		{"product", &orderTestStore{records: []orderport.Record{validOrderRecord()}, total: 1}, &orderTestCustomers{projection: contactport.CustomerProjection{ID: 7}}, &orderTestProducts{err: productport.ErrProductReadUnavailable}},
		{"invalid record", &orderTestStore{records: []orderport.Record{{}}, total: 1}, &orderTestCustomers{}, &orderTestProducts{}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewService(&orderTestUOW{}, testCase.store, testCase.customers, testCase.products).List(context.Background(), orderport.Filter{})
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
