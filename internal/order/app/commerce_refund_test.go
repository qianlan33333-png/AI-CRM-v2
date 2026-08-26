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
	material       orderport.WeChatShopOrderMaterial
	materialFound  bool
	refund         orderport.WeChatShopRefund
	commandPayload [32]byte
	enqueueCalls   int
	materialCalls  int
	reconcileCalls int
	nextAttemptID  int64
	callbacks      map[[32]byte]WeChatShopCallbackReceipt
	queryCalls     int
	reservedAmount int64
	reservedCount  int64
	lastOutcome    orderport.WeChatShopProviderCompletion
	lastEvidence   [32]byte
}

func (store *shopTestStore) FindWeChatShopRefundOrder(context.Context, string) (WeChatShopOrderRecord, error) {
	return store.order, nil
}
func (store *shopTestStore) GetWeChatShopRefundMaterial(context.Context, string) (orderport.WeChatShopOrderMaterial, bool, error) {
	return store.material, store.materialFound, nil
}
func (store *shopTestStore) EnqueueWeChatShopMaterialSync(context.Context, string, time.Time) (int64, error) {
	store.materialCalls++
	return 80, nil
}
func (store *shopTestStore) CountWeChatShopReservedRefundAmount(context.Context, orderport.ID) (int64, error) {
	return store.reservedAmount, nil
}
func (store *shopTestStore) CountWeChatShopReservedRefundLineCount(context.Context, orderport.ID, string, string) (int64, error) {
	return store.reservedCount, nil
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
		ContractVersion: "provider/v2", ProviderOrderID: reservation.Material.ProviderOrderID,
		ProductID: reservation.Command.ProductID, SKUID: reservation.Command.SKUID,
		RefundCount: reservation.Command.Count, UnitPriceMinor: reservation.Line.RealPriceMinor,
		ReasonCode: reservation.Command.ReasonCode, MaterialEvidenceDigest: reservation.Material.EvidenceDigest,
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
func (store *shopTestStore) LockWeChatShopRefundByAfterSaleID(context.Context, string) (orderport.WeChatShopRefund, error) {
	return store.refund, nil
}
func (store *shopTestStore) StartWeChatShopRefundExecution(_ context.Context, refund orderport.WeChatShopRefund, _ orderport.WeChatShopExecutionJob, at time.Time) (orderport.WeChatShopRefund, WeChatShopRefundAttempt, error) {
	store.nextAttemptID++
	refund.State, refund.AttemptCount, refund.Version, refund.UpdatedAt = orderport.WeChatShopRefundExecuting, refund.AttemptCount+1, refund.Version+1, at
	store.refund = refund
	return refund, WeChatShopRefundAttempt{ID: store.nextAttemptID, RefundID: refund.ID, AttemptNo: refund.AttemptCount, RequestDigest: refund.PayloadDigest}, nil
}
func (store *shopTestStore) RecoverWeChatShopRefundExecution(_ context.Context, refund orderport.WeChatShopRefund, at time.Time) (orderport.WeChatShopRefund, error) {
	refund.State, refund.Version, refund.UpdatedAt = orderport.WeChatShopRefundOutcomeUnknown, refund.Version+1, at
	store.refund = refund
	return refund, nil
}
func (store *shopTestStore) CompleteWeChatShopRefundExecution(_ context.Context, refund orderport.WeChatShopRefund, attempt WeChatShopRefundAttempt, outcome orderport.WeChatShopProviderCompletion, evidence [32]byte, afterSaleID string, at time.Time) (orderport.WeChatShopRefund, error) {
	if attempt.ID < 1 {
		return orderport.WeChatShopRefund{}, orderport.ErrCommerceRefundConflict
	}
	store.lastOutcome, store.lastEvidence = outcome, evidence
	switch outcome {
	case orderport.WeChatShopProviderAccepted:
		refund.State, refund.ProviderAcceptanceDigest, refund.ProviderAfterSaleID = orderport.WeChatShopRefundProviderAccepted, evidence, afterSaleID
	case orderport.WeChatShopProviderFinalFailed:
		refund.State = orderport.WeChatShopRefundFinalFailed
	default:
		refund.State = orderport.WeChatShopRefundOutcomeUnknown
	}
	refund.Version, refund.UpdatedAt = refund.Version+1, at
	store.refund = refund
	return refund, nil
}
func (store *shopTestStore) EnqueueWeChatShopRefundReconciliation(context.Context, int64) (int64, error) {
	store.reconcileCalls++
	return 82, nil
}
func (store *shopTestStore) ReserveWeChatShopRefundCallback(_ context.Context, refund orderport.WeChatShopRefund, command orderport.WeChatShopRefundCallbackCommand) (WeChatShopCallbackReceipt, bool, error) {
	if store.callbacks == nil {
		store.callbacks = map[[32]byte]WeChatShopCallbackReceipt{}
	}
	if receipt, ok := store.callbacks[command.ProviderEventDigest]; ok {
		return receipt, false, nil
	}
	receipt := WeChatShopCallbackReceipt{ID: int64(len(store.callbacks) + 1), RefundID: refund.ID, ProviderEventDigest: command.ProviderEventDigest, PayloadDigest: command.PayloadDigest, ProviderAfterSaleID: command.AfterSaleID, ProviderStatus: command.ProviderStatus, State: "reserved"}
	store.callbacks[command.ProviderEventDigest] = receipt
	return receipt, true, nil
}
func (store *shopTestStore) CompleteWeChatShopRefundCallback(_ context.Context, receipt WeChatShopCallbackReceipt, outcome string, digest [32]byte, riverJobID int64, _ time.Time) (WeChatShopCallbackReceipt, error) {
	receipt.State, receipt.Outcome, receipt.ResultDigest, receipt.RiverJobID = "completed", outcome, digest, riverJobID
	store.callbacks[receipt.ProviderEventDigest] = receipt
	return receipt, nil
}
func (store *shopTestStore) ApplyWeChatShopRefundSettlement(_ context.Context, refund orderport.WeChatShopRefund, providerDigest, settlementDigest [32]byte, at time.Time) (orderport.WeChatShopRefund, error) {
	refund.State, refund.ProviderRefundDigest, refund.SettlementDigest = orderport.WeChatShopRefundSucceeded, providerDigest, settlementDigest
	refund.SettledAt, refund.UpdatedAt, refund.Version = at, at, refund.Version+1
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

func TestWeChatShopRefundProviderAcceptanceCallbackQueuesExactQuery(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	store := readyShopTestStore(now, 51, "shop-order", "shop-transaction", 3000)
	provider := &shopTestProvider{
		enabled: true,
		request: orderport.WeChatShopProviderResult{Completion: orderport.WeChatShopProviderAccepted, EvidenceDigest: domainDigest("test", "accepted"), AfterSaleID: "9001"},
		query: orderport.WeChatShopRefundQueryResult{
			EvidenceDigest: domainDigest("test", "query"), ProviderRefundDigest: domainDigest("test", "refund"),
			AfterSaleID: "9001", ProviderOrderID: "shop-order", ProductID: "product-1", SKUID: "sku-1",
			Count: 1, AmountMinor: 1000, Currency: "CNY", Type: "REFUND", Status: "MERCHANT_REFUND_SUCCESS", OccurredAt: now.Add(2 * time.Minute),
		},
	}
	events := &shopTestEvents{}
	service, err := NewWeChatShopRefundService(commerceRefundTestUOW{}, store, provider, events)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	command := shopRefundCommand("shop-order", "shop-transaction", 1000, "shop-refund-key-1")
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
	callback := orderport.WeChatShopRefundCallbackCommand{AfterSaleID: "9001", ProviderOrderID: "shop-order", ProviderStatus: "MERCHANT_REFUND_SUCCESS", ProviderEventDigest: domainDigest("test", "event"), PayloadDigest: domainDigest("test", "payload"), OccurredAt: now.Add(time.Minute)}
	queued, err := service.ApplyRefundCallback(context.Background(), callback)
	if err != nil || queued.State != orderport.WeChatShopRefundProviderAccepted || store.reconcileCalls != 1 || events.calls != 1 {
		t.Fatalf("queued=%+v err=%v reconcile=%d events=%d", queued, err, store.reconcileCalls, events.calls)
	}
	duplicate, err := service.ApplyRefundCallback(context.Background(), callback)
	if err != nil || duplicate.State != orderport.WeChatShopRefundProviderAccepted || store.reconcileCalls != 1 || events.calls != 1 {
		t.Fatalf("duplicate=%+v err=%v reconcile=%d events=%d", duplicate, err, store.reconcileCalls, events.calls)
	}
	settled, err := service.ReconcileRefund(context.Background(), accepted.ID)
	if err != nil || settled.State != orderport.WeChatShopRefundSucceeded || provider.queryCalls != 1 || store.queryCalls != 1 || events.calls != 2 {
		t.Fatalf("settled=%+v err=%v query=%d evidence=%d events=%d", settled, err, provider.queryCalls, store.queryCalls, events.calls)
	}
}

func TestWeChatShopUnknownDoesNotRetryOrGuessAfterSaleID(t *testing.T) {
	now := time.Date(2026, 8, 25, 13, 0, 0, 0, time.UTC)
	store := readyShopTestStore(now, 52, "shop-order-2", "shop-transaction-2", 1990)
	provider := &shopTestProvider{
		enabled: true,
		request: orderport.WeChatShopProviderResult{Completion: orderport.WeChatShopProviderOutcomeUnknown, EvidenceDigest: domainDigest("test", "unknown-response")},
	}
	events := &shopTestEvents{}
	service, _ := NewWeChatShopRefundService(commerceRefundTestUOW{}, store, provider, events)
	service.now = func() time.Time { return now }
	created, err := service.RequestRefund(context.Background(), shopRefundCommand("shop-order-2", "shop-transaction-2", 1990, "shop-refund-key-2"))
	if err != nil {
		t.Fatal(err)
	}
	job := orderport.WeChatShopExecutionJob{RefundID: created.ID, RiverJobID: 92, RiverAttempt: 1, ArgsDigest: domainDigest("test", "job-2"), ScheduledAt: now}
	unknown, err := service.ExecuteRefund(context.Background(), job)
	if err != nil || unknown.State != orderport.WeChatShopRefundOutcomeUnknown || provider.requestCalls != 1 || store.lastOutcome != orderport.WeChatShopProviderOutcomeUnknown || store.lastEvidence != provider.request.EvidenceDigest {
		t.Fatalf("unknown=%+v err=%v calls=%d outcome=%s evidence=%x", unknown, err, provider.requestCalls, store.lastOutcome, store.lastEvidence)
	}
	again, err := service.ExecuteRefund(context.Background(), job)
	if err != nil || again.State != orderport.WeChatShopRefundOutcomeUnknown || provider.requestCalls != 1 {
		t.Fatalf("retry=%+v err=%v calls=%d", again, err, provider.requestCalls)
	}
	if _, err = service.QueueRefundReconciliation(context.Background(), created.ID); !errors.Is(err, orderport.ErrCommerceRefundConflict) || store.reconcileCalls != 0 {
		t.Fatalf("queue err=%v reconcile=%d", err, store.reconcileCalls)
	}
	if _, err = service.ReconcileRefund(context.Background(), created.ID); !errors.Is(err, orderport.ErrCommerceRefundConflict) || provider.queryCalls != 0 {
		t.Fatalf("reconcile err=%v query=%d", err, provider.queryCalls)
	}
}

func TestWeChatShopInterruptedExecutionBecomesUnknownWithoutProviderReplay(t *testing.T) {
	now := time.Date(2026, 8, 25, 13, 15, 0, 0, time.UTC)
	store := readyShopTestStore(now, 55, "shop-order-5", "shop-transaction-5", 1990)
	provider := &shopTestProvider{enabled: true}
	service, _ := NewWeChatShopRefundService(commerceRefundTestUOW{}, store, provider, &shopTestEvents{})
	service.now = func() time.Time { return now }
	created, err := service.RequestRefund(context.Background(), shopRefundCommand("shop-order-5", "shop-transaction-5", 1990, "shop-refund-key-5"))
	if err != nil {
		t.Fatal(err)
	}
	job := orderport.WeChatShopExecutionJob{RefundID: created.ID, RiverJobID: 94, RiverAttempt: 1, ArgsDigest: domainDigest("test", "job-5"), ScheduledAt: now}
	if _, _, err = store.StartWeChatShopRefundExecution(context.Background(), created, job, now); err != nil {
		t.Fatal(err)
	}
	recovered, err := service.ExecuteRefund(context.Background(), job)
	if err != nil || recovered.State != orderport.WeChatShopRefundOutcomeUnknown || provider.requestCalls != 0 {
		t.Fatalf("recovered=%+v err=%v calls=%d", recovered, err, provider.requestCalls)
	}
}

func TestWeChatShopFinalFailureRequiresExplicitProviderEvidence(t *testing.T) {
	result := orderport.WeChatShopProviderResult{Completion: orderport.WeChatShopProviderFinalFailed}
	if validWeChatShopProviderResult(result, nil) {
		t.Fatal("accepted final failure without Provider evidence")
	}
	result.EvidenceDigest = domainDigest("test", "explicit-rejection")
	if !validWeChatShopProviderResult(result, nil) {
		t.Fatal("rejected final failure with explicit Provider evidence")
	}
}

func TestWeChatShopMaterialUnavailableDoesNotSealIdempotencyKey(t *testing.T) {
	now := time.Date(2026, 8, 25, 13, 30, 0, 0, time.UTC)
	store := &shopTestStore{order: WeChatShopOrderRecord{ID: 54, MerchantOrderNo: "shop-order-4", PlatformTransactionNo: "shop-transaction-4", AmountMinor: 1990, Currency: "CNY", State: "paid"}}
	service, _ := NewWeChatShopRefundService(commerceRefundTestUOW{}, store, &shopTestProvider{enabled: true}, &shopTestEvents{})
	service.now = func() time.Time { return now }
	command := shopRefundCommand("shop-order-4", "shop-transaction-4", 1990, "shop-refund-key-4")
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := service.RequestRefund(context.Background(), command); !errors.Is(err, orderport.ErrWeChatShopMaterialUnavailable) || store.refund.ID != 0 {
			t.Fatalf("attempt=%d err=%v refund=%+v", attempt, err, store.refund)
		}
	}
	if store.materialCalls != 2 {
		t.Fatalf("material sync calls=%d", store.materialCalls)
	}
	store.material = readyShopMaterial(now, "shop-order-4", "shop-transaction-4", 1990)
	store.materialFound = true
	created, err := service.RequestRefund(context.Background(), command)
	if err != nil || created.ID < 1 || created.State != orderport.WeChatShopRefundAccepted {
		t.Fatalf("created=%+v err=%v", created, err)
	}
}

func TestWeChatShopDisabledNeverSchedulesOrCallsProvider(t *testing.T) {
	now := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	store := readyShopTestStore(now, 53, "shop-order-3", "shop-transaction-3", 1990)
	provider := &shopTestProvider{}
	service, _ := NewWeChatShopRefundService(commerceRefundTestUOW{}, store, provider, &shopTestEvents{})
	service.now = func() time.Time { return now }
	created, err := service.RequestRefund(context.Background(), shopRefundCommand("shop-order-3", "shop-transaction-3", 1990, "shop-refund-key-3"))
	if err != nil || created.State != orderport.WeChatShopRefundAccepted || store.enqueueCalls != 0 {
		t.Fatalf("created=%+v err=%v enqueue=%d", created, err, store.enqueueCalls)
	}
	_, err = service.ExecuteRefund(context.Background(), orderport.WeChatShopExecutionJob{RefundID: created.ID, RiverJobID: 93, RiverAttempt: 1, ArgsDigest: domainDigest("test", "job-3"), ScheduledAt: now})
	if !errors.Is(err, orderport.ErrWeChatShopRefundDisabled) || provider.requestCalls != 0 {
		t.Fatalf("disabled err=%v calls=%d", err, provider.requestCalls)
	}
}

func readyShopTestStore(now time.Time, id orderport.ID, orderNo, transactionNo string, amount int64) *shopTestStore {
	return &shopTestStore{
		order:         WeChatShopOrderRecord{ID: id, MerchantOrderNo: orderNo, PlatformTransactionNo: transactionNo, AmountMinor: amount, Currency: "CNY", State: "paid"},
		material:      readyShopMaterial(now, orderNo, transactionNo, amount),
		materialFound: true,
	}
}

func readyShopMaterial(now time.Time, orderNo, transactionNo string, amount int64) orderport.WeChatShopOrderMaterial {
	return orderport.WeChatShopOrderMaterial{
		ProviderOrderID: orderNo, DealRecorded: true, AmountMinor: amount, Currency: "CNY",
		TransactionDigest: domainDigest("wechat-shop/transaction/v1", transactionNo), EvidenceDigest: domainDigest("test", "material", orderNo),
		Source: orderport.WeChatShopMaterialProvider, Readiness: orderport.WeChatShopMaterialReady, ProviderVerified: true, SyncedAt: now,
		Lines: []orderport.WeChatShopOrderLine{{ProductID: "product-1", SKUID: "sku-1", SKUCount: 2, RealPriceMinor: amount, RemainingSKUCount: 2, AfterSaleEvidenceExact: true, Readiness: orderport.WeChatShopLineReady}},
	}
}

func shopRefundCommand(orderNo, transactionNo string, amount int64, key string) orderport.WeChatShopRefundCommand {
	return orderport.WeChatShopRefundCommand{
		OrderReference: orderNo, TransactionIDConfirmation: transactionNo, ProductID: "product-1", SKUID: "sku-1", Count: 1,
		AmountMinor: amount, ReasonCode: "10000000", Reason: "duplicate", Checked: true, Actor: 9, IdempotencyKey: key,
	}
}
