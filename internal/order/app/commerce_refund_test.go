package app

import (
	"context"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

type commerceRefundTestUOW struct{}

func (commerceRefundTestUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type compatibilityResolverStub struct {
	ids []orderport.ID
	err error
}

func (stub *compatibilityResolverStub) FindPE01RefundOrderIDs(context.Context, string) ([]orderport.ID, error) {
	return append([]orderport.ID(nil), stub.ids...), stub.err
}

type compatibilitySettlementStub struct {
	orderport.SettlementApplication
	command orderport.RefundCommandV2
	calls   int
}

func (stub *compatibilitySettlementStub) RequestRefundV2(_ context.Context, command orderport.RefundCommandV2) (orderport.RefundV2, error) {
	stub.calls++
	stub.command = command
	return orderport.RefundV2{ID: 91, OrderID: command.OrderID}, nil
}

func TestWeChatPayRefundCompatibilityFailsClosedAndCarriesConfirmation(t *testing.T) {
	resolver := &compatibilityResolverStub{ids: []orderport.ID{11, 12}}
	settlement := &compatibilitySettlementStub{}
	service, err := NewWeChatPayRefundCompatibilityService(commerceRefundTestUOW{}, resolver, settlement)
	if err != nil {
		t.Fatal(err)
	}
	command := orderport.WeChatPayRefundCompatibilityCommand{OrderReference: "pe01-order", TransactionIDConfirmation: "sha256:transaction", AmountMinor: 1990, Reason: "duplicate", Checked: true, Actor: 7, IdempotencyKey: "compatibility-key"}
	if _, err = service.RequestWeChatPayRefundV2(context.Background(), command); !errors.Is(err, orderport.ErrCommerceRefundConflict) || settlement.calls != 0 {
		t.Fatalf("ambiguous reference err=%v calls=%d", err, settlement.calls)
	}
	resolver.ids = []orderport.ID{11}
	result, err := service.RequestWeChatPayRefundV2(context.Background(), command)
	if err != nil || result.OrderID != 11 || settlement.calls != 1 || settlement.command.TransactionIDConfirmation != command.TransactionIDConfirmation {
		t.Fatalf("result=%+v err=%v calls=%d command=%+v", result, err, settlement.calls, settlement.command)
	}
	changed := settlement.command
	changed.TransactionIDConfirmation = "sha256:other"
	if refundCommandDigest(changed) == refundCommandDigest(settlement.command) {
		t.Fatal("transaction confirmation was omitted from PE01 idempotency payload")
	}
}

type shopTestStore struct {
	order          WeChatShopOrderRecord
	refund         orderport.WeChatShopRefund
	commandPayload [32]byte
	enqueueCalls   int
	nextAttemptID  int64
	callbacks      map[[32]byte]WeChatShopCallbackReceipt
	queryCalls     int
	reservedAmount int64
}

func (store *shopTestStore) FindWeChatShopRefundOrder(context.Context, string) (WeChatShopOrderRecord, error) {
	return store.order, nil
}
func (store *shopTestStore) CountWeChatShopReservedRefundAmount(context.Context, orderport.ID) (int64, error) {
	return store.reservedAmount, nil
}
func (store *shopTestStore) GetWeChatShopRefundByCommand(_ context.Context, actor int64, key [32]byte) (orderport.WeChatShopRefund, [32]byte, bool, error) {
	if store.refund.ID == 0 {
		return orderport.WeChatShopRefund{}, [32]byte{}, false, nil
	}
	return store.refund, store.commandPayload, true, nil
}
func (store *shopTestStore) CreateWeChatShopRefund(_ context.Context, reservation WeChatShopRefundReservation) (orderport.WeChatShopRefund, bool, error) {
	if store.refund.ID > 0 {
		if store.commandPayload != reservation.CommandPayloadDigest {
			return orderport.WeChatShopRefund{}, false, orderport.ErrCommerceRefundConflict
		}
		return store.refund, false, nil
	}
	store.commandPayload = reservation.CommandPayloadDigest
	store.refund = orderport.WeChatShopRefund{
		ID: 71, OrderID: reservation.Order.ID, MerchantOrderNo: reservation.Order.MerchantOrderNo,
		OutRefundNo: reservation.OutRefundNo, AmountMinor: reservation.Command.AmountMinor,
		Currency: reservation.Order.Currency, ReasonDigest: reservation.ReasonDigest,
		TransactionDigest: reservation.TransactionDigest, SourceRefDigest: reservation.SourceRefDigest,
		TargetRefDigest: reservation.TargetRefDigest, PayloadDigest: reservation.PayloadDigest,
		PolicyDigest: reservation.PolicyDigest, State: orderport.WeChatShopRefundAccepted,
		Version: 1, CreatedAt: reservation.CreatedAt, UpdatedAt: reservation.CreatedAt,
	}
	return store.refund, true, nil
}
func (store *shopTestStore) EnqueueWeChatShopRefund(context.Context, int64) (int64, error) {
	store.enqueueCalls++
	return 81, nil
}
func (store *shopTestStore) LockWeChatShopRefundByID(context.Context, int64) (orderport.WeChatShopRefund, error) {
	return store.refund, nil
}
func (store *shopTestStore) LockWeChatShopRefundByOutRefundNo(context.Context, string) (orderport.WeChatShopRefund, error) {
	return store.refund, nil
}
func (store *shopTestStore) StartWeChatShopRefundExecution(_ context.Context, refund orderport.WeChatShopRefund, _ orderport.WeChatShopExecutionJob, at time.Time) (orderport.WeChatShopRefund, WeChatShopRefundAttempt, error) {
	store.nextAttemptID++
	refund.State, refund.AttemptCount, refund.Version, refund.UpdatedAt = orderport.WeChatShopRefundExecuting, refund.AttemptCount+1, refund.Version+1, at
	store.refund = refund
	return refund, WeChatShopRefundAttempt{ID: store.nextAttemptID, RefundID: refund.ID, AttemptNo: refund.AttemptCount, RequestDigest: refund.PayloadDigest}, nil
}
func (store *shopTestStore) CompleteWeChatShopRefundExecution(_ context.Context, refund orderport.WeChatShopRefund, attempt WeChatShopRefundAttempt, outcome orderport.WeChatShopProviderCompletion, evidence [32]byte, at time.Time) (orderport.WeChatShopRefund, error) {
	if attempt.ID < 1 {
		return orderport.WeChatShopRefund{}, orderport.ErrCommerceRefundConflict
	}
	switch outcome {
	case orderport.WeChatShopProviderAccepted:
		refund.State, refund.ProviderAcceptanceDigest = orderport.WeChatShopRefundProviderAccepted, evidence
	case orderport.WeChatShopProviderFinalFailed:
		refund.State = orderport.WeChatShopRefundFinalFailed
	default:
		refund.State = orderport.WeChatShopRefundOutcomeUnknown
	}
	refund.Version, refund.UpdatedAt = refund.Version+1, at
	store.refund = refund
	return refund, nil
}
func (store *shopTestStore) ReserveWeChatShopRefundCallback(_ context.Context, refund orderport.WeChatShopRefund, command orderport.WeChatShopRefundCallbackCommand) (WeChatShopCallbackReceipt, bool, error) {
	if store.callbacks == nil {
		store.callbacks = map[[32]byte]WeChatShopCallbackReceipt{}
	}
	if receipt, ok := store.callbacks[command.ProviderEventDigest]; ok {
		return receipt, false, nil
	}
	receipt := WeChatShopCallbackReceipt{ID: int64(len(store.callbacks) + 1), RefundID: refund.ID, ProviderEventDigest: command.ProviderEventDigest, PayloadDigest: command.PayloadDigest, ProviderRefundDigest: command.ProviderRefundDigest, State: "reserved"}
	store.callbacks[command.ProviderEventDigest] = receipt
	return receipt, true, nil
}
func (store *shopTestStore) CompleteWeChatShopRefundCallback(_ context.Context, receipt WeChatShopCallbackReceipt, outcome string, digest [32]byte, _ time.Time) (WeChatShopCallbackReceipt, error) {
	receipt.State, receipt.Outcome, receipt.ResultDigest = "completed", outcome, digest
	store.callbacks[receipt.ProviderEventDigest] = receipt
	return receipt, nil
}
func (store *shopTestStore) ApplyWeChatShopRefundSettlement(_ context.Context, refund orderport.WeChatShopRefund, providerDigest, settlementDigest [32]byte, at time.Time) (orderport.WeChatShopRefund, error) {
	refund.State, refund.ProviderRefundDigest, refund.SettlementDigest = orderport.WeChatShopRefundSucceeded, providerDigest, settlementDigest
	refund.SettledAt, refund.UpdatedAt, refund.Version = at, at, refund.Version+1
	store.refund = refund
	return refund, nil
}
func (store *shopTestStore) MarkWeChatShopRefundFinalFailed(_ context.Context, refund orderport.WeChatShopRefund, at time.Time) (orderport.WeChatShopRefund, error) {
	refund.State, refund.ProviderAcceptanceDigest, refund.Version, refund.UpdatedAt = orderport.WeChatShopRefundFinalFailed, [32]byte{}, refund.Version+1, at
	store.refund = refund
	return refund, nil
}
func (store *shopTestStore) RecordWeChatShopRefundQuery(context.Context, orderport.WeChatShopRefund, orderport.WeChatShopRefundQueryResult, string, time.Time) error {
	store.queryCalls++
	return nil
}

type shopTestProvider struct {
	enabled      bool
	requestCalls int
	queryCalls   int
	request      orderport.WeChatShopProviderResult
	requestErr   error
	query        orderport.WeChatShopRefundQueryResult
	queryErr     error
}

func (provider *shopTestProvider) Enabled() bool { return provider.enabled }
func (provider *shopTestProvider) RequestRefund(context.Context, orderport.WeChatShopRefundRequest) (orderport.WeChatShopProviderResult, error) {
	provider.requestCalls++
	return provider.request, provider.requestErr
}
func (provider *shopTestProvider) QueryRefund(context.Context, string) (orderport.WeChatShopRefundQueryResult, error) {
	provider.queryCalls++
	return provider.query, provider.queryErr
}

type shopTestEvents struct{ calls int }

func (events *shopTestEvents) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	events.calls++
	return eventport.EventID(events.calls), nil
}

func TestWeChatShopRefundFakeProviderAcceptanceAndCallback(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	store := &shopTestStore{order: WeChatShopOrderRecord{ID: 51, MerchantOrderNo: "shop-order", PlatformTransactionNo: "shop-transaction", AmountMinor: 3000, Currency: "CNY", State: "paid"}}
	provider := &shopTestProvider{enabled: true, request: orderport.WeChatShopProviderResult{Completion: orderport.WeChatShopProviderAccepted, EvidenceDigest: domainDigest("test", "accepted")}}
	events := &shopTestEvents{}
	service, err := NewWeChatShopRefundService(commerceRefundTestUOW{}, store, provider, events)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	command := orderport.WeChatShopRefundCommand{OrderReference: "shop-order", TransactionIDConfirmation: "shop-transaction", AmountMinor: 1000, Reason: "duplicate", Checked: true, Actor: 9, IdempotencyKey: "shop-refund-key-1"}
	created, err := service.RequestRefund(context.Background(), command)
	if err != nil || created.State != orderport.WeChatShopRefundAccepted || store.enqueueCalls != 1 || events.calls != 1 {
		t.Fatalf("created=%+v err=%v enqueue=%d events=%d", created, err, store.enqueueCalls, events.calls)
	}
	replay, err := service.RequestRefund(context.Background(), command)
	if err != nil || replay.ID != created.ID || store.enqueueCalls != 1 || events.calls != 1 {
		t.Fatalf("replay=%+v err=%v enqueue=%d events=%d", replay, err, store.enqueueCalls, events.calls)
	}
	changed := command
	changed.TransactionIDConfirmation = "other-transaction"
	if _, err = service.RequestRefund(context.Background(), changed); !errors.Is(err, orderport.ErrCommerceRefundConflict) {
		t.Fatalf("changed confirmation err=%v", err)
	}
	job := orderport.WeChatShopExecutionJob{RefundID: created.ID, RiverJobID: 91, RiverAttempt: 1, ArgsDigest: domainDigest("test", "job"), ScheduledAt: now}
	accepted, err := service.ExecuteRefund(context.Background(), job)
	if err != nil || accepted.State != orderport.WeChatShopRefundProviderAccepted || provider.requestCalls != 1 {
		t.Fatalf("accepted=%+v err=%v calls=%d", accepted, err, provider.requestCalls)
	}
	callback := orderport.WeChatShopRefundCallbackCommand{OutRefundNo: accepted.OutRefundNo, ProviderEventDigest: domainDigest("test", "event"), PayloadDigest: domainDigest("test", "payload"), ProviderRefundDigest: domainDigest("test", "provider-refund"), AmountMinor: accepted.AmountMinor, Currency: "CNY", Succeeded: true, OccurredAt: now.Add(time.Minute)}
	settled, err := service.ApplyRefundCallback(context.Background(), callback)
	if err != nil || settled.State != orderport.WeChatShopRefundSucceeded || events.calls != 2 {
		t.Fatalf("settled=%+v err=%v events=%d", settled, err, events.calls)
	}
	duplicate, err := service.ApplyRefundCallback(context.Background(), callback)
	if err != nil || duplicate.State != orderport.WeChatShopRefundSucceeded || events.calls != 2 {
		t.Fatalf("duplicate=%+v err=%v events=%d", duplicate, err, events.calls)
	}
}

func TestWeChatShopUnknownDoesNotRetryAndManualQueryReconciles(t *testing.T) {
	now := time.Date(2026, 8, 25, 13, 0, 0, 0, time.UTC)
	store := &shopTestStore{order: WeChatShopOrderRecord{ID: 52, MerchantOrderNo: "shop-order-2", PlatformTransactionNo: "shop-transaction-2", AmountMinor: 1990, Currency: "CNY", State: "paid"}}
	provider := &shopTestProvider{
		enabled: true,
		request: orderport.WeChatShopProviderResult{Completion: orderport.WeChatShopProviderOutcomeUnknown},
		query:   orderport.WeChatShopRefundQueryResult{Confirmed: true, EvidenceDigest: domainDigest("test", "query"), ProviderRefundDigest: domainDigest("test", "refund"), AmountMinor: 1990, Currency: "CNY", OccurredAt: now.Add(time.Minute)},
	}
	events := &shopTestEvents{}
	service, _ := NewWeChatShopRefundService(commerceRefundTestUOW{}, store, provider, events)
	service.now = func() time.Time { return now }
	created, err := service.RequestRefund(context.Background(), orderport.WeChatShopRefundCommand{OrderReference: "shop-order-2", TransactionIDConfirmation: "shop-transaction-2", AmountMinor: 1990, Reason: "full", Checked: true, Actor: 9, IdempotencyKey: "shop-refund-key-2"})
	if err != nil {
		t.Fatal(err)
	}
	job := orderport.WeChatShopExecutionJob{RefundID: created.ID, RiverJobID: 92, RiverAttempt: 1, ArgsDigest: domainDigest("test", "job-2"), ScheduledAt: now}
	unknown, err := service.ExecuteRefund(context.Background(), job)
	if err != nil || unknown.State != orderport.WeChatShopRefundOutcomeUnknown || provider.requestCalls != 1 {
		t.Fatalf("unknown=%+v err=%v calls=%d", unknown, err, provider.requestCalls)
	}
	again, err := service.ExecuteRefund(context.Background(), job)
	if err != nil || again.State != orderport.WeChatShopRefundOutcomeUnknown || provider.requestCalls != 1 {
		t.Fatalf("retry=%+v err=%v calls=%d", again, err, provider.requestCalls)
	}
	reconciled, err := service.ReconcileRefund(context.Background(), created.ID)
	if err != nil || reconciled.State != orderport.WeChatShopRefundSucceeded || provider.queryCalls != 1 || store.queryCalls != 1 || events.calls != 2 {
		t.Fatalf("reconciled=%+v err=%v query=%d evidence=%d events=%d", reconciled, err, provider.queryCalls, store.queryCalls, events.calls)
	}
}

func TestWeChatShopDisabledNeverSchedulesOrCallsProvider(t *testing.T) {
	now := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	store := &shopTestStore{order: WeChatShopOrderRecord{ID: 53, MerchantOrderNo: "shop-order-3", PlatformTransactionNo: "shop-transaction-3", AmountMinor: 1990, Currency: "CNY", State: "paid"}}
	provider := &shopTestProvider{}
	service, _ := NewWeChatShopRefundService(commerceRefundTestUOW{}, store, provider, &shopTestEvents{})
	service.now = func() time.Time { return now }
	created, err := service.RequestRefund(context.Background(), orderport.WeChatShopRefundCommand{OrderReference: "shop-order-3", TransactionIDConfirmation: "shop-transaction-3", AmountMinor: 1990, Reason: "full", Checked: true, Actor: 9, IdempotencyKey: "shop-refund-key-3"})
	if err != nil || created.State != orderport.WeChatShopRefundAccepted || store.enqueueCalls != 0 {
		t.Fatalf("created=%+v err=%v enqueue=%d", created, err, store.enqueueCalls)
	}
	_, err = service.ExecuteRefund(context.Background(), orderport.WeChatShopExecutionJob{RefundID: created.ID, RiverJobID: 93, RiverAttempt: 1, ArgsDigest: domainDigest("test", "job-3"), ScheduledAt: now})
	if !errors.Is(err, orderport.ErrWeChatShopRefundDisabled) || provider.requestCalls != 0 {
		t.Fatalf("disabled err=%v calls=%d", err, provider.requestCalls)
	}
}
