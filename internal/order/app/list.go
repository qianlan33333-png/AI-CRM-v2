package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

const (
	DefaultLimit  int32 = 50
	MaximumLimit  int32 = 100
	MaximumOffset int32 = 1_000_000
)

var (
	ErrInvalidArgument = errors.New("invalid order list argument")
	ErrNotFound        = errors.New("order not found")
	ErrConflict        = errors.New("order read conflict")
	ErrUnavailable     = errors.New("order list unavailable")
)

type Store interface {
	List(context.Context, orderport.Filter) ([]orderport.Record, error)
	Count(context.Context, orderport.Filter) (int64, error)
}

type Service struct {
	uow       platformport.UnitOfWork
	store     Store
	customers contactport.CustomerReader
	products  productport.Reader
}

func NewService(uow platformport.UnitOfWork, store Store, customers contactport.CustomerReader, products productport.Reader) *Service {
	return &Service{uow: uow, store: store, customers: customers, products: products}
}

func (service *Service) List(ctx context.Context, filter orderport.Filter) (orderport.Page, error) {
	filter, err := normalizeFilter(ctx, filter)
	if err != nil {
		return orderport.Page{}, err
	}
	if nilDependency(service) || nilDependency(service.uow) || nilDependency(service.store) || nilDependency(service.customers) || nilDependency(service.products) {
		return orderport.Page{}, ErrUnavailable
	}
	page := orderport.Page{Limit: filter.Limit, Items: []orderport.Item{}}
	err = service.uow.Within(ctx, func(tx context.Context) error {
		records, listErr := service.store.List(tx, filter)
		if listErr != nil {
			return listErr
		}
		page.Total, listErr = service.store.Count(tx, filter)
		if listErr != nil {
			return listErr
		}
		if page.Total < 0 || len(records) > int(filter.Limit) || int64(filter.Offset) > page.Total && len(records) != 0 || int64(filter.Offset)+int64(len(records)) > page.Total {
			return ErrUnavailable
		}
		page.Items = make([]orderport.Item, 0, len(records))
		for _, record := range records {
			item, itemErr := service.project(tx, record)
			if itemErr != nil {
				return itemErr
			}
			page.Items = append(page.Items, item)
		}
		return nil
	})
	if err != nil {
		return orderport.Page{}, classify(err)
	}
	page.HasMore = int64(filter.Offset)+int64(len(page.Items)) < page.Total
	return page, nil
}

func (service *Service) project(ctx context.Context, record orderport.Record) (orderport.Item, error) {
	if !validRecord(record) {
		return orderport.Item{}, ErrUnavailable
	}
	payerName := record.PayerNameSnapshot
	if record.CustomerID != nil {
		customer, err := service.customers.ReadCustomer(ctx, contactport.CustomerID(*record.CustomerID))
		switch {
		case err == nil && customer.ID == contactport.CustomerID(*record.CustomerID) && validText(customer.Name, 200):
			if customer.Name != "" {
				payerName = customer.Name
			}
		case errors.Is(err, contactport.ErrCustomerReadNotFound):
		case err != nil:
			return orderport.Item{}, ErrUnavailable
		default:
			return orderport.Item{}, ErrUnavailable
		}
	}
	productName := record.ProductNameSnapshot
	if record.ProductID != nil {
		product, err := service.products.ReadProduct(ctx, productport.ID(*record.ProductID))
		switch {
		case err == nil && product.ID == productport.ID(*record.ProductID) && validText(product.Name, 200):
			if product.Name != "" {
				productName = product.Name
			}
		case errors.Is(err, productport.ErrProductReadNotFound):
		case err != nil:
			return orderport.Item{}, ErrUnavailable
		default:
			return orderport.Item{}, ErrUnavailable
		}
	}
	item := orderport.Item{
		RecordOrigin: recordOrigin(record.RecordOrigin), CreatedAt: record.CreatedAt, MerchantOrderNo: record.MerchantOrderNo, OutTradeNo: record.MerchantOrderNo,
		OrderNo: record.MerchantOrderNo, PlatformTransactionNo: record.PlatformTransactionNo,
		TransactionID: record.PlatformTransactionNo, PayerName: payerName, Mobile: record.MobileSnapshot,
		ProductCode: record.ProductCode, ProductName: productName, AmountYuan: fmt.Sprintf("%d.%02d", record.AmountMinor/100, record.AmountMinor%100),
		Currency: record.Currency, Status: record.Status, StatusLabel: record.StatusLabel,
		Provider: record.Provider, ProviderLabel: record.ProviderLabel, DetailURL: record.DetailURL,
	}
	switch record.IdentityKind {
	case "userid":
		item.UserID = record.IdentityValue
	case "external_userid":
		item.ExternalUserID = record.IdentityValue
	case "unionid":
		item.UnionID = record.IdentityValue
	}
	return item, nil
}

func normalizeFilter(ctx context.Context, filter orderport.Filter) (orderport.Filter, error) {
	if ctx == nil || ctx.Err() != nil {
		return orderport.Filter{}, ErrInvalidArgument
	}
	filter.Provider = strings.ToLower(strings.TrimSpace(filter.Provider))
	if filter.Provider == "" {
		filter.Provider = "all"
	}
	if filter.Provider != "all" && filter.Provider != "wechat" && filter.Provider != "alipay" && filter.Provider != "wechat_shop" {
		return orderport.Filter{}, ErrInvalidArgument
	}
	for value, maximum := range map[*string]int{&filter.OrderNo: 200, &filter.Mobile: 80, &filter.ProductCode: 200, &filter.Status: 80} {
		*value = strings.TrimSpace(*value)
		if !utf8.ValidString(*value) || utf8.RuneCountInString(*value) > maximum {
			return orderport.Filter{}, ErrInvalidArgument
		}
	}
	if filter.Limit == 0 {
		filter.Limit = DefaultLimit
	}
	if filter.Limit < 1 || filter.Limit > MaximumLimit || filter.Offset < 0 || filter.Offset > MaximumOffset {
		return orderport.Filter{}, ErrInvalidArgument
	}
	if filter.CustomerID != nil && *filter.CustomerID < 1 {
		return orderport.Filter{}, ErrInvalidArgument
	}
	if invalidTime(filter.CreatedFrom) || invalidTime(filter.CreatedTo) || filter.CreatedFrom != nil && filter.CreatedTo != nil && filter.CreatedFrom.After(*filter.CreatedTo) {
		return orderport.Filter{}, ErrInvalidArgument
	}
	filter.CreatedFrom = utcTime(filter.CreatedFrom)
	filter.CreatedTo = utcTime(filter.CreatedTo)
	return filter, nil
}

func validRecord(record orderport.Record) bool {
	return validRecordOrigin(record.RecordOrigin) && record.ID > 0 && (record.Provider == "wechat" || record.Provider == "alipay" || record.Provider == "wechat_shop") &&
		validText(record.ProviderLabel, 80) && record.ProviderLabel != "" && validText(record.MerchantOrderNo, 200) && record.MerchantOrderNo != "" &&
		validText(record.PlatformTransactionNo, 200) && optionalPositive(record.CustomerID) && validText(record.PayerNameSnapshot, 200) &&
		validText(record.MobileSnapshot, 80) && validIdentity(record.IdentityKind, record.IdentityValue) && optionalPositive(record.ProductID) &&
		validText(record.ProductCode, 200) && record.ProductCode != "" && validText(record.ProductNameSnapshot, 200) && record.AmountMinor >= 0 &&
		len(record.Currency) == 3 && record.Currency == strings.ToUpper(record.Currency) && validText(record.Status, 80) && record.Status != "" &&
		validText(record.StatusLabel, 80) && record.StatusLabel != "" && strings.HasPrefix(record.DetailURL, "/") && !strings.ContainsAny(record.DetailURL, " \t\r\n") &&
		len(record.DetailURL) <= 2048 && !record.CreatedAt.IsZero() && !record.UpdatedAt.IsZero() && !record.CreatedAt.After(record.UpdatedAt)
}

func validRecordOrigin(value string) bool {
	return value == "" || value == orderport.RecordOriginNative || value == orderport.RecordOriginV1History
}

func recordOrigin(value string) string {
	if value == "" {
		return orderport.RecordOriginNative
	}
	return value
}

func validIdentity(kind, value string) bool {
	if kind == "" {
		return value == ""
	}
	return (kind == "userid" || kind == "external_userid" || kind == "unionid") && validText(value, 200) && value != ""
}
func validText(value string, maximum int) bool {
	return strings.TrimSpace(value) == value && utf8.ValidString(value) && utf8.RuneCountInString(value) <= maximum
}
func optionalPositive(value *int64) bool { return value == nil || *value > 0 }
func invalidTime(value *time.Time) bool  { return value != nil && value.IsZero() }
func utcTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}
func classify(err error) error {
	switch {
	case errors.Is(err, ErrInvalidArgument), errors.Is(err, ErrNotFound), errors.Is(err, ErrConflict), errors.Is(err, ErrUnavailable):
		return err
	default:
		return errors.Join(ErrUnavailable, err)
	}
}
func nilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
