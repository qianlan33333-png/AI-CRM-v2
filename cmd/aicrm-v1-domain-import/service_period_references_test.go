package main

import (
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

func TestServicePeriodSourceReferenceUsesExactRetainedFields(t *testing.T) {
	row := v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: "public/wechat_pay_products", Payload: []byte(`{"id":12,"product_code":"period"}`)}
	if id, value, err := servicePeriodSourceReferenceFields(row, row.TableID, "product_code"); err != nil || id != 12 || value != "period" {
		t.Fatalf("reference=%d/%s err=%v", id, value, err)
	}
	for _, payload := range []string{`{"id":null,"product_code":"period"}`, `{"id":12}`, `{"id":12,"product_code":null}`, `{"id":"12","product_code":"period"}`} {
		invalid := row
		invalid.Payload = []byte(payload)
		if _, _, err := servicePeriodSourceReferenceFields(invalid, row.TableID, "product_code"); err == nil {
			t.Fatal("invalid source reference accepted")
		}
	}
	row.RedactedFields = []string{"product_code"}
	if _, _, err := servicePeriodSourceReferenceFields(row, row.TableID, "product_code"); err == nil {
		t.Fatal("redacted source used for reference")
	}
}

func TestServicePeriodOrderReferencePreservesSourceIdentityNotCurrentProduct(t *testing.T) {
	product := int64(7)
	order := orderport.Record{ID: 9, RecordOrigin: orderport.RecordOriginV1History, ProductID: &product, MerchantOrderNo: "source-order"}
	receipt := v1domain.TerminalReceipt{TargetID: "9", TargetDigest: orderapp.HistoricalOrderTargetDigest(order)}
	if !servicePeriodOrderReferenceMatches(order, receipt, "source-order", "source-order") || !servicePeriodOrderReferenceMatches(order, receipt, "source-order", "") {
		t.Fatal("verified history reference rejected")
	}
	if servicePeriodOrderReferenceMatches(order, receipt, "source-order", "other-order") {
		t.Fatal("cross-order reference accepted")
	}
	order.RecordOrigin = orderport.RecordOriginNative
	if servicePeriodOrderReferenceMatches(order, receipt, "source-order", "") {
		t.Fatal("native order linked as history")
	}
	order.RecordOrigin = orderport.RecordOriginV1History
	order.ProductID = nil
	if servicePeriodOrderReferenceMatches(order, receipt, "source-order", "") {
		t.Fatal("changed order accepted against the old digest")
	}
	receipt.TargetDigest = orderapp.HistoricalOrderTargetDigest(order)
	if !servicePeriodOrderReferenceMatches(order, receipt, "source-order", "") {
		t.Fatal("verified historical order with unresolved product rejected")
	}
}
