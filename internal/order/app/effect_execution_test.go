package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"testing"
	"time"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

func TestExecutePaymentRecoversStagedHandoffWithoutReplayingProvider(t *testing.T) {
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	receipt := sha256.Sum256([]byte("prepay-receipt"))
	handoff := orderport.JSAPIHandoff{AppID: "wx-app", TimeStamp: strconv.FormatInt(now.Unix(), 10), NonceStr: "nonce", Package: "prepay_id=wx-prepay", SignType: "RSA", PaySign: base64.StdEncoding.EncodeToString(make([]byte, 256)), ExpiresAt: now.Add(2 * time.Hour)}
	store := &paymentExecutionStore{
		command: orderport.PaymentCommand{ID: 11, OrderID: 21, SourceRefDigest: sha256.Sum256([]byte("source")), TargetRefDigest: sha256.Sum256([]byte("target")), PayloadDigest: sha256.Sum256([]byte("payload")), PolicyVersionDigest: sha256.Sum256([]byte("policy")), State: orderport.EffectAccepted, Version: 1, CreatedAt: now, UpdatedAt: now},
		order:   FinancialOrderRecord{ID: 21, MerchantOrderNo: "pe01_0123456789abcdef0123456789abcdef", CustomerID: 31, ProductID: 41, ProductVersion: 1, ProductKind: orderport.ProductKindOrdinary, AmountMinor: 9900, Currency: "CNY", State: orderport.FinancialAwaitingPrepay, PaymentIdentityDigest: sha256.Sum256([]byte("identity")), Version: 1, CreatedAt: now, UpdatedAt: now},
	}
	provider := &paymentExecutionProvider{result: orderport.ProviderResult{Completion: orderport.ProviderExecuted, ReceiptDigest: receipt, JSAPIHandoff: &handoff, BusinessCallDispatched: true, RealExternalCallExecuted: true}}
	runtime := &paymentExecutionRuntime{store: store, failBeforeTerminal: true}
	service, err := NewEffectExecutionService(&settlementTestUOW{}, store, runtime, provider, paymentSettlementApplicationStub{})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	job := EffectJob{RecordID: 11, RiverJobID: 51, RiverGeneration: 1, RiverQueue: "critical", RiverArgsDigest: sha256.Sum256([]byte("job")), ScheduledAt: now}
	if err = service.ExecutePayment(context.Background(), job); err == nil {
		t.Fatal("expected simulated crash before EER terminal completion")
	}
	if provider.calls != 1 || runtime.calls != 1 || !runtime.handoffObserved || store.command.State != orderport.EffectAccepted || store.order.State != orderport.FinancialAwaitingPrepay || store.command.JSAPIHandoff == nil || store.command.ProviderPrepayDigest != receipt {
		t.Fatalf("provider=%d runtime=%d observed=%t command=%+v order=%+v", provider.calls, runtime.calls, runtime.handoffObserved, store.command, store.order)
	}
	if err = service.ExecutePayment(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || runtime.calls != 2 || runtime.reconcileCalls != 1 || store.command.State != orderport.EffectExecuted || store.order.State != orderport.FinancialAwaitingPayment {
		t.Fatalf("recovery provider=%d runtime=%d reconcile=%d command=%+v order=%+v", provider.calls, runtime.calls, runtime.reconcileCalls, store.command, store.order)
	}
	if err = service.ExecutePayment(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || runtime.calls != 2 || runtime.reconcileCalls != 1 {
		t.Fatalf("terminal replay called provider/runtime/reconcile: %d/%d/%d", provider.calls, runtime.calls, runtime.reconcileCalls)
	}
}

type paymentExecutionStore struct {
	SettlementStore
	command orderport.PaymentCommand
	order   FinancialOrderRecord
}

func (store *paymentExecutionStore) LockPaymentCommand(context.Context, int64) (orderport.PaymentCommand, error) {
	return store.command, nil
}
func (store *paymentExecutionStore) LockOrderByID(context.Context, orderport.ID) (FinancialOrderRecord, error) {
	return store.order, nil
}
func (store *paymentExecutionStore) RecordPaymentHandoff(_ context.Context, command orderport.PaymentCommand, handoff orderport.JSAPIHandoff, receipt [32]byte, at time.Time) (orderport.PaymentCommand, error) {
	if command.Version != store.command.Version || store.command.State != orderport.EffectAccepted {
		return orderport.PaymentCommand{}, orderport.ErrSettlementConflict
	}
	store.command.ProviderPrepayDigest, store.command.JSAPIHandoff, store.command.Version, store.command.UpdatedAt = receipt, &handoff, store.command.Version+1, at
	return store.command, nil
}
func (store *paymentExecutionStore) BindPaymentEffect(_ context.Context, command orderport.PaymentCommand, effectID string, at time.Time) (orderport.PaymentCommand, error) {
	if command.Version != store.command.Version || store.command.State != orderport.EffectAccepted {
		return orderport.PaymentCommand{}, orderport.ErrSettlementConflict
	}
	store.command.ExternalEffectID, store.command.State, store.command.Version, store.command.UpdatedAt = effectID, orderport.EffectQueued, store.command.Version+1, at
	return store.command, nil
}
func (store *paymentExecutionStore) CompletePaymentEffect(_ context.Context, command orderport.PaymentCommand, state orderport.EffectState, receipt [32]byte, at time.Time) (orderport.PaymentCommand, error) {
	if command.Version != store.command.Version || (state != orderport.EffectExecuted && state != orderport.EffectReconciled) || store.command.JSAPIHandoff == nil || store.command.ProviderPrepayDigest != receipt {
		return orderport.PaymentCommand{}, orderport.ErrSettlementConflict
	}
	store.command.State, store.command.Version, store.command.UpdatedAt = orderport.EffectExecuted, store.command.Version+1, at
	return store.command, nil
}
func (store *paymentExecutionStore) MarkOrderAwaitingPayment(_ context.Context, order FinancialOrderRecord, at time.Time) (FinancialOrderRecord, error) {
	if order.Version != store.order.Version || store.order.State != orderport.FinancialAwaitingPrepay {
		return FinancialOrderRecord{}, orderport.ErrSettlementConflict
	}
	store.order.State, store.order.Version, store.order.UpdatedAt = orderport.FinancialAwaitingPayment, store.order.Version+1, at
	return store.order, nil
}

type paymentExecutionRuntime struct {
	store              *paymentExecutionStore
	calls              int
	reconcileCalls     int
	handoffObserved    bool
	failBeforeTerminal bool
}

func (runtime *paymentExecutionRuntime) Execute(ctx context.Context, _ orderport.ExternalEffectCommand, execute orderport.ProviderExecution) (orderport.ExternalEffectResult, error) {
	runtime.calls++
	if !runtime.failBeforeTerminal {
		return orderport.ExternalEffectResult{EffectID: "eer_1", State: orderport.EffectOutcomeUnknown, ReceiptDigest: sha256.Sum256([]byte("unknown"))}, nil
	}
	result, err := execute(ctx)
	runtime.handoffObserved = runtime.store.command.JSAPIHandoff != nil && runtime.store.command.ProviderPrepayDigest == result.ReceiptDigest
	runtime.failBeforeTerminal = false
	return orderport.ExternalEffectResult{}, errors.Join(errors.New("simulated crash before EER terminal completion"), err)
}
func (runtime *paymentExecutionRuntime) Reconcile(_ context.Context, effectID string, evidence [32]byte) (orderport.ExternalEffectResult, error) {
	runtime.reconcileCalls++
	return orderport.ExternalEffectResult{EffectID: effectID, State: orderport.EffectReconciled, ReceiptDigest: evidence}, nil
}

type paymentExecutionProvider struct {
	result orderport.ProviderResult
	calls  int
}

func (provider *paymentExecutionProvider) CreatePrepay(context.Context, orderport.PrepayRequest) (orderport.ProviderResult, error) {
	provider.calls++
	return provider.result, nil
}
func (*paymentExecutionProvider) RequestRefund(context.Context, orderport.RefundRequest) (orderport.ProviderResult, error) {
	return orderport.ProviderResult{}, errors.New("unexpected refund")
}
func (*paymentExecutionProvider) QueryPayment(context.Context, string) (orderport.PaymentQueryResult, error) {
	return orderport.PaymentQueryResult{}, errors.New("unexpected payment query")
}
func (*paymentExecutionProvider) QueryRefund(context.Context, string) (orderport.RefundQueryResult, error) {
	return orderport.RefundQueryResult{}, errors.New("unexpected refund query")
}

type paymentSettlementApplicationStub struct{}

func (paymentSettlementApplicationStub) Checkout(context.Context, orderport.CheckoutCommand) (orderport.Checkout, error) {
	return orderport.Checkout{}, nil
}
func (paymentSettlementApplicationStub) ApplyPaymentCallback(context.Context, orderport.PaymentCallbackCommand) (orderport.Checkout, error) {
	return orderport.Checkout{}, nil
}
func (paymentSettlementApplicationStub) RequestRefundV2(context.Context, orderport.RefundCommandV2) (orderport.RefundV2, error) {
	return orderport.RefundV2{}, nil
}
func (paymentSettlementApplicationStub) ApplyRefundCallback(context.Context, orderport.RefundCallbackCommand) (orderport.RefundV2, error) {
	return orderport.RefundV2{}, nil
}
func (paymentSettlementApplicationStub) GetSelfScoped(context.Context, string, [32]byte) (orderport.Checkout, error) {
	return orderport.Checkout{}, nil
}
