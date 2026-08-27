// Package candidate adapts archived V1 payment rows into non-executable
// historical facts. It has no database, Provider, queue, receipt, or command
// dependency.
package v1finance

import (
	"encoding/json"
	"strings"
	"time"
)

type Disposition string

const (
	DispositionCandidate Disposition = "historical_candidate"
	DispositionPending   Disposition = "pending"
	DispositionInvalid   Disposition = "invalid"
)

type ProductSourceRef struct {
	Kind  string
	Value string
}

type OrderFact struct {
	SourceID      int64
	OrderNumber   string
	TransactionID string
	Status        string
	TradeState    string
	RefundStatus  string
	AmountMinor   int64
	RefundedMinor int64
	Currency      string
	Product       ProductSourceRef
	PaidAt        *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time

	// Unverified source values, preserved exactly for caller-side mapping.
	UnionID           string
	ProductName       string
	PayerNameSnapshot string
}

type RefundFact struct {
	SourceID       int64
	OrderSourceID  int64
	OrderNumber    string
	RefundNumber   string
	ProviderRefund string
	TransactionID  string
	Reason         string
	Status         string
	AmountMinor    int64
	OrderAmount    int64
	Currency       string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type OrderResult struct {
	Disposition Disposition
	Reason      string
	Fact        *OrderFact
}

type RefundResult struct {
	Disposition Disposition
	Reason      string
	Fact        *RefundFact
}

type History struct {
	Orders  []OrderResult
	Refunds []RefundResult
}

type orderJSON struct {
	ID             int64      `json:"id"`
	OutTradeNo     string     `json:"out_trade_no"`
	ProductCode    string     `json:"product_code"`
	AmountTotal    int64      `json:"amount_total"`
	Currency       string     `json:"currency"`
	Status         string     `json:"status"`
	TradeState     string     `json:"trade_state"`
	RefundStatus   string     `json:"refund_status"`
	TransactionID  string     `json:"transaction_id"`
	PaidAt         *time.Time `json:"paid_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	RefundedAmount int64      `json:"refunded_amount_total"`

	UnionID           string `json:"unionid"`
	ProductName       string `json:"product_name"`
	PayerNameSnapshot string `json:"payer_name_snapshot"`
}

type refundJSON struct {
	ID                int64     `json:"id"`
	OrderID           int64     `json:"order_id"`
	OutTradeNo        string    `json:"out_trade_no"`
	TransactionID     string    `json:"transaction_id"`
	OutRefundNo       string    `json:"out_refund_no"`
	RefundID          string    `json:"refund_id"`
	Reason            string    `json:"reason"`
	RefundAmountTotal int64     `json:"refund_amount_total"`
	OrderAmountTotal  int64     `json:"order_amount_total"`
	Currency          string    `json:"currency"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// AdaptHistory preserves every non-empty V1 order/refund status as a local
// historical fact. Refunds require an already-valid source-order relation;
// this function never guesses a V2 customer, product, payment, or settlement.
func AdaptHistory(orders, refunds []json.RawMessage) History {
	result := History{Orders: make([]OrderResult, len(orders)), Refunds: make([]RefundResult, len(refunds))}
	knownOrders := make(map[int64]OrderFact, len(orders))
	ambiguousOrders := make(map[int64]bool)
	for index, value := range orders {
		result.Orders[index] = AdaptOrder(value)
		if fact := result.Orders[index].Fact; result.Orders[index].Disposition == DispositionCandidate && fact != nil {
			if _, found := knownOrders[fact.SourceID]; found {
				delete(knownOrders, fact.SourceID)
				ambiguousOrders[fact.SourceID] = true
			} else if !ambiguousOrders[fact.SourceID] {
				knownOrders[fact.SourceID] = *fact
			}
		}
	}
	for index, value := range refunds {
		result.Refunds[index] = AdaptRefund(value, knownOrders)
	}
	return result
}

func AdaptOrder(value json.RawMessage) OrderResult {
	var source orderJSON
	if json.Unmarshal(value, &source) != nil {
		return invalidOrder("order_json_invalid")
	}
	if source.ID < 1 || !reference(source.OutTradeNo) || !reference(source.Status) {
		return pendingOrder("order_identity_or_status_missing")
	}
	if !reference(source.ProductCode) {
		return pendingOrder("order_product_source_unresolved")
	}
	if source.AmountTotal < 1 || source.RefundedAmount < 0 || source.RefundedAmount > source.AmountTotal {
		return invalidOrder("order_amount_invalid")
	}
	if source.Currency != "CNY" {
		return pendingOrder("order_currency_not_cny")
	}
	if !validTimes(source.CreatedAt, source.UpdatedAt) || source.PaidAt != nil && source.PaidAt.IsZero() {
		return invalidOrder("order_time_invalid")
	}
	return OrderResult{Disposition: DispositionCandidate, Fact: &OrderFact{
		SourceID: source.ID, OrderNumber: source.OutTradeNo, TransactionID: source.TransactionID, Status: source.Status, TradeState: source.TradeState, RefundStatus: source.RefundStatus,
		AmountMinor: source.AmountTotal, RefundedMinor: source.RefundedAmount, Currency: source.Currency,
		UnionID: source.UnionID, ProductName: source.ProductName, PayerNameSnapshot: source.PayerNameSnapshot,
		Product: ProductSourceRef{Kind: "code", Value: source.ProductCode}, PaidAt: source.PaidAt, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
	}}
}

func AdaptRefund(value json.RawMessage, orders map[int64]OrderFact) RefundResult {
	var source refundJSON
	if json.Unmarshal(value, &source) != nil {
		return invalidRefund("refund_json_invalid")
	}
	if source.ID < 1 || source.OrderID < 1 || !reference(source.OutTradeNo) || !reference(source.OutRefundNo) || !reference(source.Status) {
		return pendingRefund("refund_identity_or_status_missing")
	}
	if source.RefundAmountTotal < 1 || source.OrderAmountTotal < 1 || source.RefundAmountTotal > source.OrderAmountTotal {
		return invalidRefund("refund_amount_invalid")
	}
	if source.Currency != "CNY" {
		return pendingRefund("refund_currency_not_cny")
	}
	if !validTimes(source.CreatedAt, source.UpdatedAt) {
		return invalidRefund("refund_time_invalid")
	}
	order, found := orders[source.OrderID]
	if !found {
		return pendingRefund("refund_order_unresolved")
	}
	if order.OrderNumber != source.OutTradeNo || order.AmountMinor != source.OrderAmountTotal || order.Currency != source.Currency ||
		order.TransactionID != "" && source.TransactionID != "" && order.TransactionID != source.TransactionID {
		return pendingRefund("refund_order_reference_conflict")
	}
	return RefundResult{Disposition: DispositionCandidate, Fact: &RefundFact{
		SourceID: source.ID, OrderSourceID: source.OrderID, OrderNumber: source.OutTradeNo, RefundNumber: source.OutRefundNo,
		ProviderRefund: source.RefundID, TransactionID: source.TransactionID, Reason: source.Reason, Status: source.Status,
		AmountMinor: source.RefundAmountTotal, OrderAmount: source.OrderAmountTotal, Currency: source.Currency,
		CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
	}}
}

func reference(value string) bool { return value != "" && strings.TrimSpace(value) == value }

func validTimes(created, updated time.Time) bool {
	return !created.IsZero() && !updated.IsZero() && !updated.Before(created)
}

func pendingOrder(reason string) OrderResult {
	return OrderResult{Disposition: DispositionPending, Reason: reason}
}
func invalidOrder(reason string) OrderResult {
	return OrderResult{Disposition: DispositionInvalid, Reason: reason}
}
func pendingRefund(reason string) RefundResult {
	return RefundResult{Disposition: DispositionPending, Reason: reason}
}
func invalidRefund(reason string) RefundResult {
	return RefundResult{Disposition: DispositionInvalid, Reason: reason}
}
