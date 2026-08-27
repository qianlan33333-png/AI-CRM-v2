package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1finance"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

func TestFinanceImporterImportsOrdersBeforeRefundsAndReplays(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	archive := &financeArchiveFake{rows: map[string][]v1archive.ArchivedRow{
		financeOrdersTableID: {financeArchivedJSON(t, financeOrdersTableID, 1, map[string]any{
			"id": int64(71), "out_trade_no": "M-71", "product_code": "sku-71", "product_name": "V1商品", "payer_name_snapshot": "付款人",
			"amount_total": int64(990), "currency": "CNY", "status": "paid", "trade_state": "SUCCESS", "refund_status": "NONE", "transaction_id": "TX-71", "unionid": "union-71", "created_at": stamp, "updated_at": stamp,
		})},
		financeRefundsTableID: {financeArchivedJSON(t, financeRefundsTableID, 2, map[string]any{
			"id": int64(81), "order_id": int64(71), "out_trade_no": "M-71", "transaction_id": "TX-71", "out_refund_no": "R-81", "refund_id": "WX-R-81", "reason": "原始原因", "refund_amount_total": int64(100), "order_amount_total": int64(990), "currency": "CNY", "status": "SUCCESS", "created_at": stamp, "updated_at": stamp,
		})},
	}}
	resolver := financeResolver{customerID: financeID(91), productID: financeID(101)}
	importer, store, _ := newFinanceImporterForTest(t, archive, resolver)

	created, err := importer.Import(context.Background(), "archive-run")
	if err != nil || created != (FinanceImportResult{ImportedOrders: 1, ImportedRefunds: 1}) {
		t.Fatalf("created/error = %#v/%v", created, err)
	}
	if len(store.orders) != 1 || len(store.refunds) != 1 {
		t.Fatalf("orders/refunds = %#v/%#v", store.orders, store.refunds)
	}
	order := store.orders[1]
	if order.RecordOrigin != orderport.RecordOriginV1History || order.CustomerID == nil || *order.CustomerID != 91 || order.ProductID == nil || *order.ProductID != 101 || order.IdentityKind != "unionid" || order.Status != "paid" || order.DetailURL != "/api/admin/wechat-pay/orders/M-71" {
		t.Fatalf("order = %#v", order)
	}
	if refund := store.refunds[1]; refund.OrderID != 1 || refund.SourceRefundID != 81 || refund.Reason != "原始原因" {
		t.Fatalf("refund = %#v", refund)
	}

	replayed, err := importer.Import(context.Background(), "archive-run")
	if err != nil || replayed != (FinanceImportResult{ImportedOrders: 1, ImportedRefunds: 1, ReplayedOrders: 1, ReplayedRefunds: 1}) || len(store.orders) != 1 || len(store.refunds) != 1 {
		t.Fatalf("replayed/error/orders/refunds = %#v/%v/%#v/%#v", replayed, err, store.orders, store.refunds)
	}
}

func TestFinanceImporterQuarantinesTargetInputAndDependentRefund(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	archive := &financeArchiveFake{rows: map[string][]v1archive.ArchivedRow{
		financeOrdersTableID: {financeArchivedJSON(t, financeOrdersTableID, 1, map[string]any{
			"id": int64(71), "out_trade_no": "M-71", "product_code": string(make([]byte, 201)), "amount_total": int64(990), "currency": "CNY", "status": "paid", "created_at": stamp, "updated_at": stamp,
		})},
		financeRefundsTableID: {financeArchivedJSON(t, financeRefundsTableID, 2, map[string]any{
			"id": int64(81), "order_id": int64(71), "out_trade_no": "M-71", "out_refund_no": "R-81", "refund_amount_total": int64(100), "order_amount_total": int64(990), "currency": "CNY", "status": "SUCCESS", "created_at": stamp, "updated_at": stamp,
		})},
	}}
	importer, store, journal := newFinanceImporterForTest(t, archive, financeResolver{productID: financeID(101)})
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (FinanceImportResult{QuarantinedOrders: 1, QuarantinedRefunds: 1}) || len(store.orders) != 0 || len(store.refunds) != 0 {
		t.Fatalf("result/error/store = %#v/%v/%#v", result, err, store)
	}
	if got := journal.orders.(*financeTerminalFake).values[SourceIdentifier(archive.rows[financeOrdersTableID][0].SourceKeyHMAC)]; got.Reason != "order_target_invalid" {
		t.Fatalf("order terminal = %#v", got)
	}
	if got := journal.refunds.(*financeTerminalFake).values[SourceIdentifier(archive.rows[financeRefundsTableID][0].SourceKeyHMAC)]; got.Reason != "refund_parent_order_unavailable" {
		t.Fatalf("refund terminal = %#v", got)
	}
}

func TestFinanceImporterRejectsUnrelatedArchiveRow(t *testing.T) {
	archive := &financeArchiveFake{rows: map[string][]v1archive.ArchivedRow{
		financeOrdersTableID: {financeArchivedJSON(t, "public/commerce_coupons", 1, map[string]any{"id": 1})},
	}}
	importer, store, _ := newFinanceImporterForTest(t, archive, financeResolver{})
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || len(store.orders) != 0 {
		t.Fatalf("error/orders = %v/%#v", err, store.orders)
	}
}

func TestFinanceImporterDoesNotSealReceiptOnTargetConflict(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	archive := &financeArchiveFake{rows: map[string][]v1archive.ArchivedRow{
		financeOrdersTableID: {financeArchivedJSON(t, financeOrdersTableID, 1, map[string]any{
			"id": int64(71), "out_trade_no": "M-71", "product_code": "sku-71", "amount_total": int64(990), "currency": "CNY", "status": "paid", "created_at": stamp, "updated_at": stamp,
		})},
	}}
	importer, store, journal := newFinanceImporterForTest(t, archive, financeResolver{})
	store.orders[99] = orderport.Record{ID: 99, Provider: "wechat", MerchantOrderNo: "M-71"}
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, orderport.ErrHistoricalConflict) {
		t.Fatalf("target conflict error = %v", err)
	}
	if len(journal.orders.(*financeTerminalFake).values) != 0 {
		t.Fatalf("unexpected receipt = %#v", journal.orders)
	}
}

func TestFinanceSourceFieldDigestExcludesSourceIDs(t *testing.T) {
	left, err := financeSourceFieldDigest([]byte(`{"id":1,"order_id":2,"coupon_claim_id":3,"status":"paid"}`), "id", "order_id", "coupon_claim_id")
	if err != nil {
		t.Fatal(err)
	}
	right, err := financeSourceFieldDigest([]byte(`{"id":9,"order_id":8,"coupon_claim_id":7,"status":"paid"}`), "id", "order_id", "coupon_claim_id")
	if err != nil || left != right {
		t.Fatalf("left/right/error = %x/%x/%v", left, right, err)
	}
	if financeMappedOrderFieldDigest(left, financeID(1), nil) == financeMappedOrderFieldDigest(left, financeID(2), nil) {
		t.Fatal("resolved customer ID must affect replay digest")
	}
	if record := financeOrderRecord(v1finance.OrderFact{}, nil, nil); record.IdentityKind != "" || record.IdentityValue != "" {
		t.Fatalf("empty unionid identity = %#v", record)
	}
}

type financeArchiveFake struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive *financeArchiveFake) EachTableRow(_ context.Context, _ string, table string, callback func(v1archive.ArchivedRow) error) error {
	for _, row := range archive.rows[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type financeResolver struct {
	customerID, productID *int64
	err                   error
}

func (resolver financeResolver) ResolveHistoricalOrderReferences(context.Context, v1finance.OrderFact) (*int64, *int64, error) {
	return resolver.customerID, resolver.productID, resolver.err
}

type financeStoreFake struct {
	orders  map[orderport.ID]orderport.Record
	refunds map[int64]orderport.HistoricalRefund
}

func (store *financeStoreFake) CreateHistoricalOrder(_ context.Context, value orderport.Record) (orderport.Record, error) {
	for _, current := range store.orders {
		if current.Provider == value.Provider && current.MerchantOrderNo == value.MerchantOrderNo {
			return orderport.Record{}, orderport.ErrHistoricalConflict
		}
	}
	value.ID = orderport.ID(len(store.orders) + 1)
	store.orders[value.ID] = value
	return value, nil
}

func (store *financeStoreFake) GetHistoricalOrder(_ context.Context, id orderport.ID) (orderport.Record, error) {
	value, found := store.orders[id]
	if !found {
		return orderport.Record{}, errors.New("order absent")
	}
	return value, nil
}

func (store *financeStoreFake) CreateHistoricalRefund(_ context.Context, value orderport.HistoricalRefund) (orderport.HistoricalRefund, error) {
	for _, current := range store.refunds {
		if current.SourceRefundID == value.SourceRefundID || current.RefundNumber == value.RefundNumber {
			return orderport.HistoricalRefund{}, orderport.ErrHistoricalConflict
		}
	}
	value.ID = int64(len(store.refunds) + 1)
	store.refunds[value.ID] = value
	return value, nil
}

func (store *financeStoreFake) GetHistoricalRefund(_ context.Context, id int64) (orderport.HistoricalRefund, error) {
	value, found := store.refunds[id]
	if !found {
		return orderport.HistoricalRefund{}, errors.New("refund absent")
	}
	return value, nil
}

func newFinanceImporterForTest(t *testing.T, archive ArchiveSource, resolver FinanceReferenceResolver) (*FinanceImporter, *financeStoreFake, *FinanceJournal) {
	t.Helper()
	journal, err := newFinanceJournal(newFinanceTerminalFake(), newFinanceTerminalFake())
	if err != nil {
		t.Fatal(err)
	}
	store := &financeStoreFake{orders: map[orderport.ID]orderport.Record{}, refunds: map[int64]orderport.HistoricalRefund{}}
	writer, err := orderapp.NewHistoricalImportService(immediateUOW{}, store, journal)
	if err != nil {
		t.Fatal(err)
	}
	importer, err := NewFinanceImporter(archive, immediateUOW{}, writer, journal, resolver)
	if err != nil {
		t.Fatal(err)
	}
	return importer, store, journal
}

func financeArchivedJSON(t *testing.T, table string, ordinal int64, value any) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	source := sha256.Sum256([]byte(table + string(rune(ordinal))))
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: source, PayloadHMAC: sha256.Sum256(payload), Payload: payload}
}

func financeID(value int64) *int64 { return &value }
