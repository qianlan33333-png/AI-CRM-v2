package app

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type EffectJob struct {
	RecordID        int64
	RiverJobID      int64
	RiverGeneration int64
	RiverQueue      string
	RiverArgsDigest [32]byte
	ScheduledAt     time.Time
}

type EffectExecutionService struct {
	uow        platformport.UnitOfWork
	store      SettlementStore
	runtime    orderport.ExternalEffectRuntime
	provider   orderport.WeChatPayProvider
	settlement orderport.SettlementApplication
	now        func() time.Time
}

func NewEffectExecutionService(uow platformport.UnitOfWork, store SettlementStore, runtime orderport.ExternalEffectRuntime, provider orderport.WeChatPayProvider, settlement orderport.SettlementApplication) (*EffectExecutionService, error) {
	if uow == nil || store == nil || runtime == nil || provider == nil || settlement == nil {
		return nil, orderport.ErrSettlementUnavailable
	}
	return &EffectExecutionService{uow: uow, store: store, runtime: runtime, provider: provider, settlement: settlement, now: time.Now}, nil
}

func (service *EffectExecutionService) ExecutePayment(ctx context.Context, job EffectJob) error {
	if !validEffectJob(job) {
		return orderport.ErrInvalidSettlement
	}
	var command orderport.PaymentCommand
	var order FinancialOrderRecord
	if err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		command, err = service.store.LockPaymentCommand(tx, job.RecordID)
		if err != nil {
			return err
		}
		order, err = service.store.LockOrderByID(tx, command.OrderID)
		return err
	}); err != nil {
		return classifySettlement(err)
	}
	if effectTerminal(command.State) {
		return nil
	}
	result, err := service.runtime.Execute(ctx, effectCommand(orderport.ExternalEffectPaymentPrepay, command.SourceRefDigest, command.TargetRefDigest, command.PayloadDigest, command.PolicyVersionDigest, job), func(providerCtx context.Context) (orderport.ProviderResult, error) {
		return service.provider.CreatePrepay(providerCtx, orderport.PrepayRequest{MerchantOrderNo: order.MerchantOrderNo, AmountMinor: order.AmountMinor, Currency: order.Currency, ProductSnapshot: hex.EncodeToString(command.PayloadDigest[:]), PayerIdentityDigest: order.PaymentIdentityDigest, ProviderNotifyTarget: command.TargetRefDigest})
	})
	if err != nil && result.State == "" {
		return err
	}
	return service.uow.Within(ctx, func(tx context.Context) error {
		current, lockErr := service.store.LockPaymentCommand(tx, command.ID)
		if lockErr != nil {
			return lockErr
		}
		current, lockErr = service.bindPaymentIfNeeded(tx, current, result.EffectID)
		if lockErr != nil {
			return lockErr
		}
		if effectTerminal(current.State) {
			return nil
		}
		current, lockErr = service.store.CompletePaymentEffect(tx, current, result.State, result.ReceiptDigest, service.now().UTC())
		if lockErr != nil {
			return lockErr
		}
		if current.State == orderport.EffectExecuted {
			lockedOrder, orderErr := service.store.LockOrderByID(tx, current.OrderID)
			if orderErr != nil {
				return orderErr
			}
			if lockedOrder.State == orderport.FinancialAwaitingPrepay {
				_, orderErr = service.store.MarkOrderAwaitingPayment(tx, lockedOrder, service.now().UTC())
			}
			return orderErr
		}
		if current.State == orderport.EffectOutcomeUnknown {
			jobID, queueErr := service.store.EnqueuePaymentReconcile(tx, current.ID)
			if queueErr != nil || jobID < 1 {
				return settlementStoreError(queueErr)
			}
		}
		return nil
	})
}

func (service *EffectExecutionService) ExecuteRefund(ctx context.Context, job EffectJob) error {
	if !validEffectJob(job) {
		return orderport.ErrInvalidSettlement
	}
	var refund orderport.RefundV2
	var order FinancialOrderRecord
	if err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		refund, err = service.store.LockRefundByID(tx, job.RecordID)
		if err != nil {
			return err
		}
		order, err = service.store.LockOrderByID(tx, refund.OrderID)
		return err
	}); err != nil {
		return classifySettlement(err)
	}
	if effectTerminal(refund.State) {
		return nil
	}
	result, err := service.runtime.Execute(ctx, effectCommand(orderport.ExternalEffectRefund, refund.SourceRefDigest, refund.TargetRefDigest, refund.PayloadDigest, refund.PolicyDigest, job), func(providerCtx context.Context) (orderport.ProviderResult, error) {
		return service.provider.RequestRefund(providerCtx, orderport.RefundRequest{MerchantOrderNo: order.MerchantOrderNo, OutRefundNo: refund.OutRefundNo, AmountMinor: refund.AmountMinor, Currency: refund.Currency, ReasonDigest: refund.ReasonDigest})
	})
	if err != nil && result.State == "" {
		return err
	}
	return service.uow.Within(ctx, func(tx context.Context) error {
		current, lockErr := service.store.LockRefundByID(tx, refund.ID)
		if lockErr != nil {
			return lockErr
		}
		current, lockErr = service.bindRefundIfNeeded(tx, current, result.EffectID)
		if lockErr != nil || effectTerminal(current.State) {
			return lockErr
		}
		current, lockErr = service.store.CompleteRefundEffect(tx, current, result.State, service.now().UTC())
		if lockErr == nil && current.State == orderport.EffectOutcomeUnknown {
			jobID, queueErr := service.store.EnqueueRefundReconcile(tx, current.ID)
			if queueErr != nil || jobID < 1 {
				return settlementStoreError(queueErr)
			}
		}
		return lockErr
	})
}

func (service *EffectExecutionService) ReconcilePayment(ctx context.Context, commandID int64) error {
	var command orderport.PaymentCommand
	var order FinancialOrderRecord
	if commandID < 1 || service == nil || service.settlement == nil {
		return orderport.ErrInvalidSettlement
	}
	if err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		command, err = service.store.LockPaymentCommand(tx, commandID)
		if err == nil {
			order, err = service.store.LockOrderByID(tx, command.OrderID)
		}
		return err
	}); err != nil {
		return classifySettlement(err)
	}
	if command.State == orderport.EffectReconciled || order.State == orderport.FinancialPaid || order.State == orderport.FinancialPartiallyRefunded || order.State == orderport.FinancialRefunded {
		return nil
	}
	if command.State != orderport.EffectOutcomeUnknown || command.ExternalEffectID == "" {
		return orderport.ErrSettlementConflict
	}
	query, err := service.provider.QueryPayment(ctx, order.MerchantOrderNo)
	if err != nil || !query.Confirmed || allZeroDigest(query.EvidenceDigest) {
		return errors.Join(orderport.ErrProviderOutcomeUnknown, err)
	}
	if query.AmountMinor != order.AmountMinor || query.Currency != order.Currency || allZeroDigest(query.ProviderTransactionDigest) || query.OccurredAt.IsZero() {
		return orderport.ErrSettlementConflict
	}
	if _, err = service.runtime.Reconcile(ctx, command.ExternalEffectID, query.EvidenceDigest); err != nil {
		return err
	}
	if err = service.uow.Within(ctx, func(tx context.Context) error {
		current, lockErr := service.store.LockPaymentCommand(tx, command.ID)
		if lockErr != nil {
			return lockErr
		}
		if current.State == orderport.EffectOutcomeUnknown {
			if _, lockErr = service.store.CompletePaymentEffect(tx, current, orderport.EffectReconciled, query.EvidenceDigest, service.now().UTC()); lockErr != nil {
				return lockErr
			}
		}
		currentOrder, lockErr := service.store.LockOrderByID(tx, order.ID)
		if lockErr == nil && currentOrder.State == orderport.FinancialAwaitingPrepay {
			_, lockErr = service.store.MarkOrderAwaitingPayment(tx, currentOrder, service.now().UTC())
		}
		return lockErr
	}); err != nil {
		return classifySettlement(err)
	}
	_, err = service.settlement.ApplyPaymentCallback(ctx, orderport.PaymentCallbackCommand{MerchantOrderNo: order.MerchantOrderNo, ProviderEventDigest: domainDigest("pe01/query-payment-event/v1", command.ExternalEffectID, hex.EncodeToString(query.EvidenceDigest[:])), PayloadDigest: query.EvidenceDigest, ProviderTransactionDigest: query.ProviderTransactionDigest, AmountMinor: query.AmountMinor, Currency: query.Currency, Succeeded: true, OccurredAt: query.OccurredAt})
	return err
}

func (service *EffectExecutionService) ReconcileRefund(ctx context.Context, refundID int64) error {
	var refund orderport.RefundV2
	if refundID < 1 || service == nil || service.settlement == nil {
		return orderport.ErrInvalidSettlement
	}
	if err := service.uow.Within(ctx, func(tx context.Context) error {
		var err error
		refund, err = service.store.LockRefundByID(tx, refundID)
		return err
	}); err != nil {
		return classifySettlement(err)
	}
	if refund.State == "succeeded" || refund.State == orderport.EffectReconciled {
		return nil
	}
	if refund.State != orderport.EffectOutcomeUnknown || refund.ExternalEffectID == "" {
		return orderport.ErrSettlementConflict
	}
	query, err := service.provider.QueryRefund(ctx, refund.OutRefundNo)
	if err != nil || !query.Confirmed || allZeroDigest(query.EvidenceDigest) {
		return errors.Join(orderport.ErrProviderOutcomeUnknown, err)
	}
	if query.AmountMinor != refund.AmountMinor || query.Currency != refund.Currency || allZeroDigest(query.ProviderRefundDigest) || query.OccurredAt.IsZero() {
		return orderport.ErrSettlementConflict
	}
	if _, err = service.runtime.Reconcile(ctx, refund.ExternalEffectID, query.EvidenceDigest); err != nil {
		return err
	}
	if err = service.uow.Within(ctx, func(tx context.Context) error {
		current, lockErr := service.store.LockRefundByID(tx, refund.ID)
		if lockErr == nil && current.State == orderport.EffectOutcomeUnknown {
			_, lockErr = service.store.CompleteRefundEffect(tx, current, orderport.EffectReconciled, service.now().UTC())
		}
		return lockErr
	}); err != nil {
		return classifySettlement(err)
	}
	_, err = service.settlement.ApplyRefundCallback(ctx, orderport.RefundCallbackCommand{OutRefundNo: refund.OutRefundNo, ProviderEventDigest: domainDigest("pe01/query-refund-event/v1", refund.ExternalEffectID, hex.EncodeToString(query.EvidenceDigest[:])), PayloadDigest: query.EvidenceDigest, ProviderRefundDigest: query.ProviderRefundDigest, AmountMinor: query.AmountMinor, Currency: query.Currency, Succeeded: true, OccurredAt: query.OccurredAt})
	return err
}

func (service *EffectExecutionService) bindPaymentIfNeeded(ctx context.Context, command orderport.PaymentCommand, effectID string) (orderport.PaymentCommand, error) {
	if command.ExternalEffectID == effectID && effectID != "" {
		return command, nil
	}
	if command.ExternalEffectID != "" || command.State != orderport.EffectAccepted {
		return orderport.PaymentCommand{}, orderport.ErrSettlementConflict
	}
	return service.store.BindPaymentEffect(ctx, command, effectID, service.now().UTC())
}

func (service *EffectExecutionService) bindRefundIfNeeded(ctx context.Context, refund orderport.RefundV2, effectID string) (orderport.RefundV2, error) {
	if refund.ExternalEffectID == effectID && effectID != "" {
		return refund, nil
	}
	if refund.ExternalEffectID != "" || refund.State != orderport.EffectAccepted {
		return orderport.RefundV2{}, orderport.ErrSettlementConflict
	}
	return service.store.BindRefundEffect(ctx, refund, effectID, service.now().UTC())
}

func effectCommand(kind orderport.ExternalEffectKind, source, target, payload, policy [32]byte, job EffectJob) orderport.ExternalEffectCommand {
	return orderport.ExternalEffectCommand{Kind: kind, SourceRefDigest: source, TargetRefDigest: target, PayloadDigest: payload, PolicyVersionDigest: policy, RiverJobID: job.RiverJobID, RiverGeneration: job.RiverGeneration, RiverQueue: job.RiverQueue, RiverArgsDigest: job.RiverArgsDigest, RiverScheduledAt: job.ScheduledAt}
}

func validEffectJob(job EffectJob) bool {
	return job.RecordID > 0 && job.RiverJobID > 0 && job.RiverGeneration > 0 && job.RiverQueue != "" && !allZeroDigest(job.RiverArgsDigest) && !job.ScheduledAt.IsZero()
}

func effectTerminal(state orderport.EffectState) bool {
	return state == orderport.EffectExecuted || state == orderport.EffectOutcomeUnknown || state == orderport.EffectFinalFailed || state == orderport.EffectReconciled
}
