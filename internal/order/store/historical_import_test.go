package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderdb "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store/generated"
)

func TestHistoricalImportRequiresBoundTransaction(t *testing.T) {
	repository := NewRepository()
	order := historicalOrderFixture()
	refund := historicalRefundFixture()
	for name, call := range map[string]func() error{
		"create order":  func() error { _, err := repository.CreateHistoricalOrder(context.Background(), order); return err },
		"get order":     func() error { _, err := repository.GetHistoricalOrder(context.Background(), 1); return err },
		"create refund": func() error { _, err := repository.CreateHistoricalRefund(context.Background(), refund); return err },
		"get refund":    func() error { _, err := repository.GetHistoricalRefund(context.Background(), 1); return err },
		"list refunds":  func() error { _, err := repository.ListHistoricalRefunds(context.Background(), 1); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, orderport.ErrHistoricalUnavailable) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestHistoricalImportRejectsInvalidInputs(t *testing.T) {
	repository := NewRepository()
	order := historicalOrderFixture()
	for _, mutate := range []func(*orderport.Record){
		func(value *orderport.Record) { value.RecordOrigin = orderport.RecordOriginNative },
		func(value *orderport.Record) { value.Provider = "alipay" },
		func(value *orderport.Record) { value.Currency = "USD" },
	} {
		candidate := order
		mutate(&candidate)
		if _, err := repository.CreateHistoricalOrder(context.Background(), candidate); !errors.Is(err, orderport.ErrHistoricalInput) {
			t.Fatalf("order=%+v error=%v", candidate, err)
		}
	}
	if _, err := repository.CreateHistoricalRefund(context.Background(), orderport.HistoricalRefund{Currency: "CNY"}); !errors.Is(err, orderport.ErrHistoricalInput) {
		t.Fatalf("refund error=%v", err)
	}
}

func TestHistoricalImportMapsRowsAndErrors(t *testing.T) {
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	row := orderdb.CreateHistoricalOrderRow{ID: 7, RecordOrigin: orderport.RecordOriginV1History, Provider: "wechat", ProviderLabel: "微信支付", MerchantOrderNo: "M-7", PlatformTransactionNo: "T-7", CustomerID: pgtype.Int8{Int64: 8, Valid: true}, ProductID: pgtype.Int8{Int64: 9, Valid: true}, ProductCode: "sku-7", ProductNameSnapshot: "商品", AmountMinor: 123, Currency: "CNY", Status: "paid", StatusLabel: "已支付", DetailUrl: "/orders/7", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true}}
	order := historicalOrderFromCreate(row)
	if order.ID != 7 || order.RecordOrigin != orderport.RecordOriginV1History || order.Provider != "wechat" || order.Currency != "CNY" || order.CustomerID == nil || *order.CustomerID != 8 || order.ProductID == nil || *order.ProductID != 9 {
		t.Fatalf("order=%+v", order)
	}
	refund := historicalRefund(orderdb.OrderHistoricalRefund{ID: 11, OrderID: 7, SourceRefundID: 12, RefundNumber: "R-12", AmountMinor: 100, OrderAmountMinor: 123, Currency: "CNY", CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true}})
	if refund.ID != 11 || refund.OrderID != 7 || refund.SourceRefundID != 12 || refund.AmountMinor != 100 || !refund.CreatedAt.Equal(now) {
		t.Fatalf("refund=%+v", refund)
	}
	if !errors.Is(historicalCreateError(pgx.ErrNoRows), orderport.ErrHistoricalConflict) || !errors.Is(historicalCreateError(&pgconn.PgError{Code: "23505"}), orderport.ErrHistoricalConflict) || !errors.Is(historicalCreateError(errors.New("database unavailable")), orderport.ErrHistoricalUnavailable) {
		t.Fatal("historical error classification changed")
	}
}

func historicalOrderFixture() orderport.Record {
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	return orderport.Record{RecordOrigin: orderport.RecordOriginV1History, Provider: "wechat", Currency: "CNY", CreatedAt: now, UpdatedAt: now}
}

func historicalRefundFixture() orderport.HistoricalRefund {
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	return orderport.HistoricalRefund{OrderID: 1, SourceRefundID: 2, Currency: "CNY", CreatedAt: now, UpdatedAt: now}
}
