package v1finance

import (
	"encoding/json"
	"testing"
	"time"
)

var candidateTime = time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)

func TestAdaptOrderPreservesIdentityAndDisplaySourceFields(t *testing.T) {
	for _, tc := range []struct {
		name, unionID, productName, payerNameSnapshot string
	}{
		{"exact", "union-11", "历史商品", "付款人"},
		{"untrimmed", "\t union-11 \n", " 商品名称 \t", "\n 付款人快照 "},
		{"empty_unionid", "", "历史商品", "付款人"},
		{"empty_fields", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := AdaptOrder(candidateJSON(t, map[string]any{
				"id": 11, "out_trade_no": "order-11", "product_code": "product-code-11",
				"unionid": tc.unionID, "product_name": tc.productName, "payer_name_snapshot": tc.payerNameSnapshot,
				"amount_total": 1999, "currency": "CNY", "status": "created", "trade_state": "NOTPAY",
				"created_at": candidateTime, "updated_at": candidateTime,
			}))
			if result.Disposition != DispositionCandidate || result.Fact == nil {
				t.Fatalf("order result=%#v", result)
			}
			if result.Fact.UnionID != tc.unionID || result.Fact.ProductName != tc.productName || result.Fact.PayerNameSnapshot != tc.payerNameSnapshot {
				t.Fatalf("source fields not preserved: %#v", result.Fact)
			}
			if result.Fact.Status != "created" || result.Fact.TradeState != "NOTPAY" || result.Fact.AmountMinor != 1999 || result.Fact.PaidAt != nil {
				t.Fatalf("historical order semantics changed: %#v", result.Fact)
			}
		})
	}
}

func TestAdaptHistoryPreservesPendingOrderAndRefundStatuses(t *testing.T) {
	orders := []json.RawMessage{candidateJSON(t, map[string]any{
		"id": 11, "out_trade_no": "order-11", "product_code": "product-code-11", "amount_total": 1999, "currency": "CNY",
		"status": "created", "trade_state": "NOTPAY", "refund_status": "NONE", "transaction_id": "", "created_at": candidateTime, "updated_at": candidateTime,
	})}
	refunds := []json.RawMessage{candidateJSON(t, map[string]any{
		"id": 21, "order_id": 11, "out_trade_no": "order-11", "out_refund_no": "refund-21", "refund_id": "", "reason": " 原始退款原因 ",
		"refund_amount_total": 1999, "order_amount_total": 1999, "currency": "CNY", "status": "PROCESSING",
		"transaction_id": "", "created_at": candidateTime, "updated_at": candidateTime,
	})}
	result := AdaptHistory(orders, refunds)
	if result.Orders[0].Disposition != DispositionCandidate || result.Orders[0].Fact == nil || result.Orders[0].Fact.Status != "created" ||
		result.Orders[0].Fact.TradeState != "NOTPAY" || result.Orders[0].Fact.RefundStatus != "NONE" || result.Orders[0].Fact.Product != (ProductSourceRef{Kind: "code", Value: "product-code-11"}) || result.Orders[0].Fact.AmountMinor != 1999 || result.Orders[0].Fact.PaidAt != nil {
		t.Fatalf("order fact=%#v", result.Orders[0])
	}
	if result.Refunds[0].Disposition != DispositionCandidate || result.Refunds[0].Fact == nil || result.Refunds[0].Fact.Status != "PROCESSING" ||
		result.Refunds[0].Fact.RefundNumber != "refund-21" || result.Refunds[0].Fact.OrderNumber != "order-11" || result.Refunds[0].Fact.Reason != " 原始退款原因 " || result.Refunds[0].Fact.AmountMinor != 1999 {
		t.Fatalf("refund fact=%#v", result.Refunds[0])
	}
}

func TestAdaptHistoryFailsClosedForBadAmountsAndUnresolvedRelations(t *testing.T) {
	orders := []json.RawMessage{
		candidateJSON(t, map[string]any{"id": 1, "out_trade_no": "order-1", "product_code": "product-1", "amount_total": 0, "currency": "CNY", "status": "paid", "created_at": candidateTime, "updated_at": candidateTime}),
		candidateJSON(t, map[string]any{"id": 2, "out_trade_no": "order-2", "product_code": "", "amount_total": 1, "currency": "CNY", "status": "paid", "created_at": candidateTime, "updated_at": candidateTime}),
	}
	refunds := []json.RawMessage{
		candidateJSON(t, map[string]any{"id": 3, "order_id": 99, "out_trade_no": "order-99", "out_refund_no": "refund-3", "refund_amount_total": 1, "order_amount_total": 1, "currency": "CNY", "status": "SUCCESS", "created_at": candidateTime, "updated_at": candidateTime}),
		candidateJSON(t, map[string]any{"id": 4, "order_id": 99, "out_trade_no": "order-99", "out_refund_no": "refund-4", "refund_amount_total": -1, "order_amount_total": 1, "currency": "CNY", "status": "SUCCESS", "created_at": candidateTime, "updated_at": candidateTime}),
	}
	result := AdaptHistory(orders, refunds)
	if result.Orders[0].Disposition != DispositionInvalid || result.Orders[0].Reason != "order_amount_invalid" {
		t.Fatalf("bad amount order=%#v", result.Orders[0])
	}
	if result.Orders[1].Disposition != DispositionPending || result.Orders[1].Reason != "order_product_source_unresolved" {
		t.Fatalf("missing product order=%#v", result.Orders[1])
	}
	if result.Refunds[0].Disposition != DispositionPending || result.Refunds[0].Reason != "refund_order_unresolved" {
		t.Fatalf("unresolved refund=%#v", result.Refunds[0])
	}
	if result.Refunds[1].Disposition != DispositionInvalid || result.Refunds[1].Reason != "refund_amount_invalid" {
		t.Fatalf("bad amount refund=%#v", result.Refunds[1])
	}
}

func TestAdaptRefundPreservesProviderOptionalFieldsAndRejectsConflict(t *testing.T) {
	orders := []json.RawMessage{candidateJSON(t, map[string]any{
		"id": 7, "out_trade_no": "order-7", "product_code": "product-7", "amount_total": 100, "currency": "CNY", "status": "paid",
		"transaction_id": "txn-7", "paid_at": candidateTime, "created_at": candidateTime, "updated_at": candidateTime,
	})}
	refunds := []json.RawMessage{
		candidateJSON(t, map[string]any{"id": 8, "order_id": 7, "out_trade_no": "order-7", "out_refund_no": "refund-8", "refund_id": "provider-refund-8", "transaction_id": "txn-7", "refund_amount_total": 20, "order_amount_total": 100, "currency": "CNY", "status": "CLOSED", "created_at": candidateTime, "updated_at": candidateTime}),
		candidateJSON(t, map[string]any{"id": 9, "order_id": 7, "out_trade_no": "different-order", "out_refund_no": "refund-9", "refund_amount_total": 20, "order_amount_total": 100, "currency": "CNY", "status": "UNKNOWN", "created_at": candidateTime, "updated_at": candidateTime}),
	}
	result := AdaptHistory(orders, refunds)
	if result.Refunds[0].Disposition != DispositionCandidate || result.Refunds[0].Fact == nil || result.Refunds[0].Fact.ProviderRefund != "provider-refund-8" || result.Refunds[0].Fact.Status != "CLOSED" {
		t.Fatalf("refund fact=%#v", result.Refunds[0])
	}
	if result.Refunds[1].Disposition != DispositionPending || result.Refunds[1].Reason != "refund_order_reference_conflict" {
		t.Fatalf("refund conflict=%#v", result.Refunds[1])
	}
}

func TestAdaptHistoryLeavesDuplicateSourceOrderRelationPending(t *testing.T) {
	order := map[string]any{"id": 12, "out_trade_no": "order-12", "product_code": "product-12", "amount_total": 100, "currency": "CNY", "status": "created", "created_at": candidateTime, "updated_at": candidateTime}
	result := AdaptHistory([]json.RawMessage{candidateJSON(t, order), candidateJSON(t, order)}, []json.RawMessage{candidateJSON(t, map[string]any{
		"id": 13, "order_id": 12, "out_trade_no": "order-12", "out_refund_no": "refund-13", "refund_amount_total": 10, "order_amount_total": 100,
		"currency": "CNY", "status": "PROCESSING", "created_at": candidateTime, "updated_at": candidateTime,
	})})
	if result.Refunds[0].Disposition != DispositionPending || result.Refunds[0].Reason != "refund_order_unresolved" {
		t.Fatalf("duplicate source order refund=%#v", result.Refunds[0])
	}
}

func candidateJSON(t *testing.T, value map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
