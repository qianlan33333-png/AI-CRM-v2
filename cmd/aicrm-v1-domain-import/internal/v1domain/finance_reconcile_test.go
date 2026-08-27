package v1domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

func TestFinanceTargetIDUsesClosedSourceTargetPairs(t *testing.T) {
	for _, pair := range [][2]string{
		{"public/wechat_pay_orders", "order_list_projections"},
		{"public/wechat_pay_refunds", "order_historical_refunds"},
	} {
		domain, table, id := "order", pair[1], "17"
		row := reconciliationRow{TableID: pair[0], TargetDomain: &domain, TargetTable: &table, TargetID: &id, TargetDigest: make([]byte, 32)}
		if got, err := financeTargetID(row); got != 17 || err != nil {
			t.Fatalf("valid pair %v: id=%d err=%v", pair, got, err)
		}
		for _, invalidID := range []string{"", "0", "-1", "17x", " 17", "9223372036854775808"} {
			row.TargetID = &invalidID
			if _, err := financeTargetID(row); !errors.Is(err, ErrConflict) {
				t.Fatalf("invalid id %q accepted: %v", invalidID, err)
			}
		}
	}
	domain, table, id := "order", "order_list_projections", "17"
	row := reconciliationRow{TableID: "public/wechat_pay_orders", TargetDomain: &domain, TargetTable: &table, TargetID: &id, TargetDigest: make([]byte, 32)}
	for name, mutate := range map[string]func(*reconciliationRow){
		"domain missing": func(r *reconciliationRow) { r.TargetDomain = nil },
		"domain wrong":   func(r *reconciliationRow) { value := "product"; r.TargetDomain = &value },
		"table missing":  func(r *reconciliationRow) { r.TargetTable = nil },
		"table wrong":    func(r *reconciliationRow) { value := "order_refunds"; r.TargetTable = &value },
		"source wrong":   func(r *reconciliationRow) { r.TableID = "public/wechat_pay_refunds" },
		"source schema":  func(r *reconciliationRow) { r.TableID = "other/wechat_pay_orders" },
		"id missing":     func(r *reconciliationRow) { r.TargetID = nil },
		"digest length":  func(r *reconciliationRow) { r.TargetDigest = make([]byte, 31) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := row
			mutate(&changed)
			if _, err := financeTargetID(changed); !errors.Is(err, ErrConflict) {
				t.Fatalf("invalid scope accepted: %v", err)
			}
		})
	}
}

func TestFinanceOrderTargetDigestIncludesStaticFields(t *testing.T) {
	order := financeReconcileOrder()
	expected := orderapp.HistoricalOrderTargetDigest(order)
	if !financeOrderMatchesTarget(order, expected[:]) {
		t.Fatal("exact historical order rejected")
	}
	for name, mutate := range map[string]func(*orderport.Record){
		"origin":         func(r *orderport.Record) { r.RecordOrigin = orderport.RecordOriginNative },
		"provider":       func(r *orderport.Record) { r.Provider = "alipay" },
		"provider label": func(r *orderport.Record) { r.ProviderLabel += " changed" },
		"merchant":       func(r *orderport.Record) { r.MerchantOrderNo += " changed" },
		"transaction":    func(r *orderport.Record) { r.PlatformTransactionNo += " changed" },
		"customer":       func(r *orderport.Record) { r.CustomerID = nil },
		"payer":          func(r *orderport.Record) { r.PayerNameSnapshot += " changed" },
		"mobile":         func(r *orderport.Record) { r.MobileSnapshot += " changed" },
		"identity kind":  func(r *orderport.Record) { r.IdentityKind = "external_userid" },
		"identity value": func(r *orderport.Record) { r.IdentityValue += " changed" },
		"product":        func(r *orderport.Record) { r.ProductID = nil },
		"product code":   func(r *orderport.Record) { r.ProductCode += " changed" },
		"product name":   func(r *orderport.Record) { r.ProductNameSnapshot += " changed" },
		"amount":         func(r *orderport.Record) { r.AmountMinor++ },
		"currency":       func(r *orderport.Record) { r.Currency = "USD" },
		"status":         func(r *orderport.Record) { r.Status = "closed" },
		"status label":   func(r *orderport.Record) { r.StatusLabel += " changed" },
		"detail URL":     func(r *orderport.Record) { r.DetailURL += "/changed" },
		"created":        func(r *orderport.Record) { r.CreatedAt = r.CreatedAt.Add(time.Microsecond) },
		"updated":        func(r *orderport.Record) { r.UpdatedAt = r.UpdatedAt.Add(time.Microsecond) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := order
			mutate(&changed)
			if financeOrderMatchesTarget(changed, expected[:]) {
				t.Fatal("changed static field accepted")
			}
		})
	}
	order.RecordOrigin = orderport.RecordOriginNative
	nativeDigest := orderapp.HistoricalOrderTargetDigest(order)
	if financeOrderMatchesTarget(order, nativeDigest[:]) || financeOrderMatchesTarget(financeReconcileOrder(), expected[:31]) {
		t.Fatal("native origin or malformed digest accepted")
	}
}

func TestFinanceRefundRequiresSameBatchHistoricalParentAndExactReason(t *testing.T) {
	order := financeReconcileOrder()
	refund := orderport.HistoricalRefund{ID: 23, OrderID: order.ID, SourceRefundID: 115, RefundNumber: "refund-115", ProviderRefundID: "provider-refund",
		TransactionID: order.PlatformTransactionNo, Status: "PROCESSING", AmountMinor: 100, OrderAmountMinor: order.AmountMinor,
		Currency: "CNY", Reason: " 原始退款原因\n", CreatedAt: order.CreatedAt, UpdatedAt: order.UpdatedAt}
	targets := map[string]map[string]struct{}{"order_list_projections": {"17": {}}}
	expected := orderapp.HistoricalRefundTargetDigest(refund)
	if !financeRefundMatchesTarget(refund, order, targets, expected[:]) {
		t.Fatal("exact historical refund rejected")
	}
	if financeRefundMatchesTarget(refund, order, nil, expected[:]) {
		t.Fatal("out-of-batch parent accepted")
	}
	for name, mutate := range map[string]func(*orderport.HistoricalRefund){
		"parent":          func(r *orderport.HistoricalRefund) { r.OrderID++ },
		"source refund":   func(r *orderport.HistoricalRefund) { r.SourceRefundID++ },
		"refund number":   func(r *orderport.HistoricalRefund) { r.RefundNumber += " changed" },
		"provider refund": func(r *orderport.HistoricalRefund) { r.ProviderRefundID += " changed" },
		"transaction":     func(r *orderport.HistoricalRefund) { r.TransactionID += " changed" },
		"status":          func(r *orderport.HistoricalRefund) { r.Status = "SUCCESS" },
		"amount":          func(r *orderport.HistoricalRefund) { r.AmountMinor++ },
		"order amount":    func(r *orderport.HistoricalRefund) { r.OrderAmountMinor++ },
		"currency":        func(r *orderport.HistoricalRefund) { r.Currency = "USD" },
		"reason":          func(r *orderport.HistoricalRefund) { r.Reason = strings.TrimSpace(r.Reason) },
		"created":         func(r *orderport.HistoricalRefund) { r.CreatedAt = r.CreatedAt.Add(time.Microsecond) },
		"updated":         func(r *orderport.HistoricalRefund) { r.UpdatedAt = r.UpdatedAt.Add(time.Microsecond) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := refund
			mutate(&changed)
			if financeRefundMatchesTarget(changed, order, targets, expected[:]) {
				t.Fatal("changed refund accepted")
			}
		})
	}
	for name, mutate := range map[string]func(*orderport.Record){
		"origin":      func(r *orderport.Record) { r.RecordOrigin = orderport.RecordOriginNative },
		"provider":    func(r *orderport.Record) { r.Provider = "alipay" },
		"currency":    func(r *orderport.Record) { r.Currency = "USD" },
		"amount":      func(r *orderport.Record) { r.AmountMinor++ },
		"transaction": func(r *orderport.Record) { r.PlatformTransactionNo = "" },
	} {
		t.Run("parent "+name, func(t *testing.T) {
			changed := order
			mutate(&changed)
			if financeRefundMatchesTarget(refund, changed, targets, expected[:]) {
				t.Fatal("incompatible parent accepted")
			}
		})
	}
	refund.TransactionID = ""
	emptyTransactionDigest := orderapp.HistoricalRefundTargetDigest(refund)
	if !financeRefundMatchesTarget(refund, order, targets, emptyTransactionDigest[:]) {
		t.Fatal("optional empty refund transaction rejected")
	}
}

func financeReconcileOrder() orderport.Record {
	stamp := time.Date(2026, 8, 28, 11, 0, 0, 123456000, time.UTC)
	customer, product := int64(2), int64(3)
	return orderport.Record{ID: 17, RecordOrigin: orderport.RecordOriginV1History, Provider: "wechat", ProviderLabel: "V1历史",
		MerchantOrderNo: "order-708", PlatformTransactionNo: "transaction-708", CustomerID: &customer,
		PayerNameSnapshot: "付款人", MobileSnapshot: "", IdentityKind: "unionid", IdentityValue: "source-unionid",
		ProductID: &product, ProductCode: "sku-29", ProductNameSnapshot: "历史商品", AmountMinor: 1000, Currency: "CNY",
		Status: "paid", StatusLabel: "V1历史状态", DetailURL: "/orders/17", CreatedAt: stamp, UpdatedAt: stamp}
}
