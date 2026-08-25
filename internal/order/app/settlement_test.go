package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type settlementTestUOW struct{ calls int }

func (uow *settlementTestUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	uow.calls++
	return fn(ctx)
}

type settlementTestStore struct {
	SettlementStore
	receipt       BoardReceipt
	completeCalls int
	events        int
	order         FinancialOrderRecord
	payment       orderport.PaymentCommand
}

func (store *settlementTestStore) ReserveBoardReceipt(_ context.Context, input BoardReservation) (BoardReceipt, bool, error) {
	if store.receipt.ID > 0 {
		return store.receipt, false, nil
	}
	store.receipt = BoardReceipt{ID: 1, Operation: input.Operation, ActorScope: input.ActorScope, KeyDigest: input.KeyDigest, PayloadDigest: input.PayloadDigest, State: "in_progress"}
	return store.receipt, true, nil
}
func (store *settlementTestStore) CompleteBoardReceipt(_ context.Context, _ int64, snapshot json.RawMessage, _ time.Time) (BoardReceipt, error) {
	store.completeCalls++
	store.receipt.State, store.receipt.ResultSnapshot = "completed", append(json.RawMessage(nil), snapshot...)
	return store.receipt, nil
}
func (store *settlementTestStore) CreateCheckout(_ context.Context, command orderport.CheckoutCommand, product productport.Product, number string, now time.Time) (FinancialOrderRecord, error) {
	store.order = FinancialOrderRecord{ID: 91, MerchantOrderNo: number, CustomerID: command.CustomerID, ProductID: int64(product.ID), ProductVersion: product.Version, ProductKind: command.ProductKind, AmountMinor: product.PriceMinor, Currency: product.Currency, State: orderport.FinancialAwaitingPrepay, PaymentIdentityDigest: command.PaymentIdentityDigest, Version: 1, CreatedAt: now, UpdatedAt: now}
	return store.order, nil
}
func (store *settlementTestStore) CreatePaymentCommand(_ context.Context, order FinancialOrderRecord, source, target, payload, policy [32]byte, now time.Time) (orderport.PaymentCommand, error) {
	store.payment = orderport.PaymentCommand{ID: 81, OrderID: order.ID, SourceRefDigest: source, TargetRefDigest: target, PayloadDigest: payload, PolicyVersionDigest: policy, State: orderport.EffectAccepted, Version: 1, CreatedAt: now, UpdatedAt: now}
	return store.payment, nil
}
func (*settlementTestStore) EnqueuePaymentBridge(context.Context, int64) (int64, error) {
	return 71, nil
}
func (store *settlementTestStore) GetPaymentCommandByOrder(context.Context, orderport.ID) (orderport.PaymentCommand, error) {
	return store.payment, nil
}

type settlementTestProducts struct{ product productport.Product }

func (reader settlementTestProducts) ReadProduct(context.Context, productport.ID) (productport.Product, error) {
	return reader.product, nil
}

type settlementTestBenefits struct{ calls int }

func (benefits *settlementTestBenefits) ApplyPaidSettlement(context.Context, productport.PaidSettlementCommand) (productport.PaidSettlementResult, error) {
	benefits.calls++
	return productport.PaidSettlementResult{EntitlementID: 1, State: "active", Version: 1}, nil
}

type settlementTestEvents struct {
	calls int
	err   error
}

func (events *settlementTestEvents) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	events.calls++
	return 1, events.err
}

func TestSettlementCheckoutExactReplayMismatchAndReceiptLast(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	store := &settlementTestStore{}
	events := &settlementTestEvents{}
	service, err := NewSettlementService(&settlementTestUOW{}, store, settlementTestProducts{product: productport.Product{ID: 7, ProductCode: "P7", Name: "Product", PriceMinor: 9900, Currency: "CNY", Version: 3, LocalLifecycle: productport.LocalProductEnabled}}, &settlementTestBenefits{}, events)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	service.random = func(value []byte) error {
		for i := range value {
			value[i] = byte(i + 1)
		}
		return nil
	}
	identity := domainDigest("test", "identity")
	command := orderport.CheckoutCommand{CustomerID: 41, ProductID: 7, ProductKind: orderport.ProductKindOrdinary, PaymentIdentityDigest: identity, ActorScope: "payment-session:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", IdempotencyKey: "checkout-key-0001"}
	first, err := service.Checkout(context.Background(), command)
	if err != nil || first.OrderID != 91 || first.PaymentCommandID != 81 || store.completeCalls != 1 || events.calls != 1 {
		t.Fatalf("first checkout = %+v err=%v complete=%d events=%d", first, err, store.completeCalls, events.calls)
	}
	replay, err := service.Checkout(context.Background(), command)
	if err != nil || replay != first || store.completeCalls != 1 || events.calls != 1 {
		t.Fatalf("replay = %+v err=%v complete=%d events=%d", replay, err, store.completeCalls, events.calls)
	}
	changed := command
	changed.ProductID = 8
	if _, err = service.Checkout(context.Background(), changed); !errors.Is(err, orderport.ErrSettlementConflict) || store.completeCalls != 1 {
		t.Fatalf("mismatch err=%v complete=%d", err, store.completeCalls)
	}

	failingStore := &settlementTestStore{}
	failingEvents := &settlementTestEvents{err: errors.New("event unavailable")}
	failing, _ := NewSettlementService(&settlementTestUOW{}, failingStore, settlementTestProducts{product: productport.Product{ID: 7, ProductCode: "P7", Name: "Product", PriceMinor: 9900, Currency: "CNY", Version: 3, LocalLifecycle: productport.LocalProductEnabled}}, &settlementTestBenefits{}, failingEvents)
	failing.now, failing.random = service.now, service.random
	if _, err = failing.Checkout(context.Background(), command); err == nil || failingStore.completeCalls != 0 {
		t.Fatalf("event failure err=%v receipt completions=%d", err, failingStore.completeCalls)
	}
}
