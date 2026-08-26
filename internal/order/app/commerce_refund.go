package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const wechatShopRefundPolicy = "order-wechat-shop-refund/v2"

type PE01RefundReferenceResolver interface {
	FindPE01RefundOrderIDs(context.Context, string) ([]orderport.ID, error)
}

type WeChatPayRefundCompatibilityService struct {
	uow        platformport.UnitOfWork
	resolver   PE01RefundReferenceResolver
	settlement orderport.SettlementApplication
}

var _ orderport.WeChatPayRefundCompatibilityApplication = (*WeChatPayRefundCompatibilityService)(nil)

func NewWeChatPayRefundCompatibilityService(uow platformport.UnitOfWork, resolver PE01RefundReferenceResolver, settlement orderport.SettlementApplication) (*WeChatPayRefundCompatibilityService, error) {
	if uow == nil || resolver == nil || settlement == nil {
		return nil, orderport.ErrCommerceRefundUnavailable
	}
	return &WeChatPayRefundCompatibilityService{uow: uow, resolver: resolver, settlement: settlement}, nil
}

func (service *WeChatPayRefundCompatibilityService) RequestWeChatPayRefundV2(ctx context.Context, command orderport.WeChatPayRefundCompatibilityCommand) (orderport.RefundV2, error) {
	if service == nil || !validCompatibilityCommand(ctx, command) {
		return orderport.RefundV2{}, orderport.ErrCommerceRefundInvalid
	}
	var orderID orderport.ID
	err := service.uow.Within(ctx, func(tx context.Context) error {
		ids, findErr := service.resolver.FindPE01RefundOrderIDs(tx, command.OrderReference)
		if findErr != nil {
			return findErr
		}
		switch len(ids) {
		case 0:
			return orderport.ErrCommerceRefundNotFound
		case 1:
			orderID = ids[0]
			return nil
		default:
			return orderport.ErrCommerceRefundConflict
		}
	})
	if err != nil {
		return orderport.RefundV2{}, classifyCommerceRefund(err)
	}
	result, err := service.settlement.RequestRefundV2(ctx, orderport.RefundCommandV2{
		OrderID: orderID, AmountMinor: command.AmountMinor, Reason: command.Reason,
		TransactionIDConfirmation: command.TransactionIDConfirmation,
		Actor:                     command.Actor, IdempotencyKey: command.IdempotencyKey,
	})
	return result, classifyCommerceRefund(err)
}

type WeChatShopOrderRecord struct {
	ID                    orderport.ID
	MerchantOrderNo       string
	PlatformTransactionNo string
	AmountMinor           int64
	Currency              string
	State                 string
}

type WeChatShopRefundReservation struct {
	Order                WeChatShopOrderRecord
	Material             orderport.WeChatShopOrderMaterial
	Line                 orderport.WeChatShopOrderLine
	Command              orderport.WeChatShopRefundCommand
	OutRefundNo          string
	ReasonDigest         [32]byte
	TransactionDigest    [32]byte
	CommandKeyDigest     [32]byte
	CommandPayloadDigest [32]byte
	SourceRefDigest      [32]byte
	TargetRefDigest      [32]byte
	PayloadDigest        [32]byte
	PolicyDigest         [32]byte
	CreatedAt            time.Time
}

type WeChatShopRefundAttempt struct {
	ID             int64
	RefundID       int64
	AttemptNo      int64
	RequestDigest  [32]byte
	Outcome        orderport.WeChatShopProviderCompletion
	EvidenceDigest [32]byte
}

type WeChatShopCallbackReceipt struct {
	ID                   int64
	RefundID             int64
	ProviderEventDigest  [32]byte
	PayloadDigest        [32]byte
	ProviderRefundDigest [32]byte
	ProviderAfterSaleID  string
	ProviderStatus       string
	RiverJobID           int64
	Outcome              string
	ResultDigest         [32]byte
	State                string
}

type WeChatShopRefundStore interface {
	FindWeChatShopRefundOrder(context.Context, string) (WeChatShopOrderRecord, error)
	GetWeChatShopRefundMaterial(context.Context, string) (orderport.WeChatShopOrderMaterial, bool, error)
	EnqueueWeChatShopMaterialSync(context.Context, string, time.Time) (int64, error)
	CountWeChatShopReservedRefundAmount(context.Context, orderport.ID) (int64, error)
	CountWeChatShopReservedRefundLineCount(context.Context, orderport.ID, string, string) (int64, error)
	GetWeChatShopRefundByCommand(context.Context, int64, [32]byte) (orderport.WeChatShopRefund, [32]byte, bool, error)
	CreateWeChatShopRefund(context.Context, WeChatShopRefundReservation) (orderport.WeChatShopRefund, bool, error)
	EnqueueWeChatShopRefund(context.Context, int64) (int64, error)
	LockWeChatShopRefundByID(context.Context, int64) (orderport.WeChatShopRefund, error)
	LockWeChatShopRefundByAfterSaleID(context.Context, string) (orderport.WeChatShopRefund, error)
	StartWeChatShopRefundExecution(context.Context, orderport.WeChatShopRefund, orderport.WeChatShopExecutionJob, time.Time) (orderport.WeChatShopRefund, WeChatShopRefundAttempt, error)
	RecoverWeChatShopRefundExecution(context.Context, orderport.WeChatShopRefund, time.Time) (orderport.WeChatShopRefund, error)
	CompleteWeChatShopRefundExecution(context.Context, orderport.WeChatShopRefund, WeChatShopRefundAttempt, orderport.WeChatShopProviderCompletion, [32]byte, string, time.Time) (orderport.WeChatShopRefund, error)
	EnqueueWeChatShopRefundReconciliation(context.Context, int64) (int64, error)
	ReserveWeChatShopRefundCallback(context.Context, orderport.WeChatShopRefund, orderport.WeChatShopRefundCallbackCommand) (WeChatShopCallbackReceipt, bool, error)
	CompleteWeChatShopRefundCallback(context.Context, WeChatShopCallbackReceipt, string, [32]byte, int64, time.Time) (WeChatShopCallbackReceipt, error)
	ApplyWeChatShopRefundSettlement(context.Context, orderport.WeChatShopRefund, [32]byte, [32]byte, time.Time) (orderport.WeChatShopRefund, error)
	RecordWeChatShopRefundQuery(context.Context, orderport.WeChatShopRefund, orderport.WeChatShopRefundQueryResult, string, time.Time) error
}

type WeChatShopRefundService struct {
	uow      platformport.UnitOfWork
	store    WeChatShopRefundStore
	provider orderport.WeChatShopRefundProvider
	events   eventport.Appender
	dispatch bool
	now      func() time.Time
}

var _ orderport.WeChatShopRefundApplication = (*WeChatShopRefundService)(nil)

type WeChatShopRefundServiceOption func(*WeChatShopRefundService)

func WithWeChatShopRefundDispatch(enabled bool) WeChatShopRefundServiceOption {
	return func(service *WeChatShopRefundService) { service.dispatch = enabled }
}

func NewWeChatShopRefundService(uow platformport.UnitOfWork, store WeChatShopRefundStore, provider orderport.WeChatShopRefundProvider, events eventport.Appender, options ...WeChatShopRefundServiceOption) (*WeChatShopRefundService, error) {
	if uow == nil || store == nil || provider == nil || events == nil {
		return nil, orderport.ErrCommerceRefundUnavailable
	}
	service := &WeChatShopRefundService{uow: uow, store: store, provider: provider, events: events, dispatch: provider.Enabled(), now: time.Now}
	for _, option := range options {
		if option == nil {
			return nil, orderport.ErrCommerceRefundUnavailable
		}
		option(service)
	}
	return service, nil
}

func (service *WeChatShopRefundService) RequestRefund(ctx context.Context, command orderport.WeChatShopRefundCommand) (orderport.WeChatShopRefund, error) {
	if !service.ready() || !validWeChatShopCommand(ctx, command) {
		return orderport.WeChatShopRefund{}, orderport.ErrCommerceRefundInvalid
	}
	key := sha256.Sum256([]byte(command.IdempotencyKey))
	payload := wechatShopCommandDigest(command)
	now := service.now().UTC()
	var result orderport.WeChatShopRefund
	materialQueued := false
	err := service.uow.Within(ctx, func(tx context.Context) error {
		existing, existingPayload, found, getErr := service.store.GetWeChatShopRefundByCommand(tx, command.Actor, key)
		if getErr != nil {
			return getErr
		}
		if found {
			if !sameDigest(existingPayload, payload) || !validWeChatShopRefund(existing) {
				return orderport.ErrCommerceRefundConflict
			}
			result = existing
			return nil
		}
		order, findErr := service.store.FindWeChatShopRefundOrder(tx, command.OrderReference)
		if findErr != nil {
			return findErr
		}
		if !validWeChatShopOrder(order) || order.PlatformTransactionNo != command.TransactionIDConfirmation || (order.State != "paid" && order.State != "partially_refunded") {
			return orderport.ErrCommerceRefundConflict
		}
		material, found, materialErr := service.store.GetWeChatShopRefundMaterial(tx, order.MerchantOrderNo)
		if materialErr != nil {
			return materialErr
		}
		if !found || !refundReadyMaterial(material) {
			jobID, enqueueErr := service.store.EnqueueWeChatShopMaterialSync(tx, order.MerchantOrderNo, now)
			if enqueueErr != nil || jobID < 1 {
				return errors.Join(orderport.ErrWeChatShopMaterialUnavailable, enqueueErr)
			}
			materialQueued = true
			return nil
		}
		line, lineFound := refundMaterialLine(material, command.ProductID, command.SKUID)
		transactionDigest := domainDigest("wechat-shop/transaction/v1", command.TransactionIDConfirmation)
		if material.ProviderOrderID != order.MerchantOrderNo || material.AmountMinor != order.AmountMinor || material.Currency != order.Currency || !sameDigest(material.TransactionDigest, transactionDigest) || !lineFound || line.Readiness != orderport.WeChatShopLineReady || !line.AfterSaleEvidenceExact || line.RealPriceMinor < 0 || line.RealPriceMinor > 1_000_000_000 || line.RemainingSKUCount < command.Count || command.AmountMinor > line.RealPriceMinor*command.Count {
			return orderport.ErrCommerceRefundConflict
		}
		reserved, countErr := service.store.CountWeChatShopReservedRefundAmount(tx, order.ID)
		if countErr != nil || reserved < 0 || reserved > order.AmountMinor || command.AmountMinor > order.AmountMinor-reserved {
			return orderport.ErrCommerceRefundConflict
		}
		reservedCount, countErr := service.store.CountWeChatShopReservedRefundLineCount(tx, order.ID, command.ProductID, command.SKUID)
		if countErr != nil || reservedCount < 0 || reservedCount > line.RemainingSKUCount || command.Count > line.RemainingSKUCount-reservedCount {
			return orderport.ErrCommerceRefundConflict
		}
		reservation := newWeChatShopReservation(order, material, line, command, key, payload, now)
		created, owned, createErr := service.store.CreateWeChatShopRefund(tx, reservation)
		if createErr != nil {
			return createErr
		}
		if !validWeChatShopRefund(created) || !sameDigest(created.PayloadDigest, reservation.PayloadDigest) {
			return orderport.ErrCommerceRefundUnavailable
		}
		if !owned {
			if !sameDigest(created.PayloadDigest, reservation.PayloadDigest) {
				return orderport.ErrCommerceRefundConflict
			}
			result = created
			return nil
		}
		if service.dispatch {
			jobID, enqueueErr := service.store.EnqueueWeChatShopRefund(tx, created.ID)
			if enqueueErr != nil || jobID < 1 {
				return errors.Join(orderport.ErrCommerceRefundUnavailable, enqueueErr)
			}
		}
		if appendErr := service.append(tx, eventport.EvOrderRefundRequested, created, reservation.CommandPayloadDigest, now); appendErr != nil {
			return appendErr
		}
		result = created
		return nil
	})
	if err == nil && materialQueued {
		return orderport.WeChatShopRefund{}, orderport.ErrWeChatShopMaterialUnavailable
	}
	return result, classifyCommerceRefund(err)
}

func (service *WeChatShopRefundService) ExecuteRefund(ctx context.Context, job orderport.WeChatShopExecutionJob) (orderport.WeChatShopRefund, error) {
	if !service.ready() || !validWeChatShopJob(job) {
		return orderport.WeChatShopRefund{}, orderport.ErrCommerceRefundInvalid
	}
	if !service.provider.Enabled() {
		return orderport.WeChatShopRefund{}, orderport.ErrWeChatShopRefundDisabled
	}
	var current orderport.WeChatShopRefund
	var attempt WeChatShopRefundAttempt
	err := service.uow.Within(ctx, func(tx context.Context) error {
		refund, lockErr := service.store.LockWeChatShopRefundByID(tx, job.RefundID)
		if lockErr != nil {
			return lockErr
		}
		current = refund
		if refund.State == orderport.WeChatShopRefundExecuting {
			current, lockErr = service.store.RecoverWeChatShopRefundExecution(tx, refund, service.now().UTC())
			return lockErr
		}
		if refund.State != orderport.WeChatShopRefundAccepted {
			return nil
		}
		current, attempt, lockErr = service.store.StartWeChatShopRefundExecution(tx, refund, job, service.now().UTC())
		return lockErr
	})
	if err != nil || current.State != orderport.WeChatShopRefundExecuting || attempt.ID < 1 {
		return current, classifyCommerceRefund(err)
	}
	providerResult, providerErr := service.provider.RequestRefund(ctx, orderport.WeChatShopRefundRequest{
		ProviderOrderID: current.ProviderOrderID, ProductID: current.ProductID,
		SKUID: current.SKUID, Count: current.RefundCount, OutRefundNo: current.OutRefundNo,
		AmountMinor: current.AmountMinor, Currency: current.Currency,
		ReasonCode: current.ReasonCode, ReasonDigest: current.ReasonDigest,
	})
	outcome := providerResult.Completion
	if (providerErr != nil && outcome == "") || !validWeChatShopProviderResult(providerResult, providerErr) {
		outcome = orderport.WeChatShopProviderOutcomeUnknown
		providerResult.EvidenceDigest = [32]byte{}
	}
	err = service.uow.Within(ctx, func(tx context.Context) error {
		refund, lockErr := service.store.LockWeChatShopRefundByID(tx, current.ID)
		if lockErr != nil {
			return lockErr
		}
		if refund.State != orderport.WeChatShopRefundExecuting || refund.Version != current.Version {
			current = refund
			return nil
		}
		current, lockErr = service.store.CompleteWeChatShopRefundExecution(tx, refund, attempt, outcome, providerResult.EvidenceDigest, providerResult.AfterSaleID, service.now().UTC())
		return lockErr
	})
	return current, classifyCommerceRefund(err)
}

func (service *WeChatShopRefundService) ApplyRefundCallback(ctx context.Context, command orderport.WeChatShopRefundCallbackCommand) (orderport.WeChatShopRefund, error) {
	if !service.ready() || !validWeChatShopCallback(command) {
		return orderport.WeChatShopRefund{}, orderport.ErrCommerceRefundInvalid
	}
	var result orderport.WeChatShopRefund
	err := service.uow.Within(ctx, func(tx context.Context) error {
		refund, lockErr := service.store.LockWeChatShopRefundByAfterSaleID(tx, command.AfterSaleID)
		if lockErr != nil {
			return lockErr
		}
		if refund.ProviderOrderID != command.ProviderOrderID || refund.ProviderAfterSaleID != command.AfterSaleID {
			return orderport.ErrCommerceRefundConflict
		}
		receipt, owned, reserveErr := service.store.ReserveWeChatShopRefundCallback(tx, refund, command)
		if reserveErr != nil {
			return reserveErr
		}
		if !owned {
			if receipt.State != "completed" || receipt.Outcome != "query_queued" || receipt.RefundID != refund.ID || receipt.ProviderAfterSaleID != command.AfterSaleID || receipt.ProviderStatus != command.ProviderStatus || receipt.RiverJobID < 1 || !sameDigest(receipt.PayloadDigest, command.PayloadDigest) {
				return orderport.ErrCommerceRefundConflict
			}
			result = refund
			return nil
		}
		resultDigest := wechatShopCallbackResultDigest(command, refund.ID)
		jobID, enqueueErr := service.store.EnqueueWeChatShopRefundReconciliation(tx, refund.ID)
		if enqueueErr != nil || jobID < 1 {
			return errors.Join(orderport.ErrCommerceRefundUnavailable, enqueueErr)
		}
		_, lockErr = service.store.CompleteWeChatShopRefundCallback(tx, receipt, "query_queued", resultDigest, jobID, service.now().UTC())
		result = refund
		return lockErr
	})
	return result, classifyCommerceRefund(err)
}

func (service *WeChatShopRefundService) QueueRefundReconciliation(ctx context.Context, refundID int64) (orderport.WeChatShopRefund, error) {
	if !service.ready() || refundID < 1 {
		return orderport.WeChatShopRefund{}, orderport.ErrCommerceRefundInvalid
	}
	var result orderport.WeChatShopRefund
	err := service.uow.Within(ctx, func(tx context.Context) error {
		refund, lockErr := service.store.LockWeChatShopRefundByID(tx, refundID)
		if lockErr != nil {
			return lockErr
		}
		result = refund
		if refund.State == orderport.WeChatShopRefundSucceeded {
			return nil
		}
		if refund.State != orderport.WeChatShopRefundProviderAccepted || !validReference(refund.ProviderAfterSaleID) {
			return orderport.ErrCommerceRefundConflict
		}
		jobID, enqueueErr := service.store.EnqueueWeChatShopRefundReconciliation(tx, refund.ID)
		if enqueueErr != nil || jobID < 1 {
			return errors.Join(orderport.ErrCommerceRefundUnavailable, enqueueErr)
		}
		return nil
	})
	return result, classifyCommerceRefund(err)
}

func (service *WeChatShopRefundService) ReconcileRefund(ctx context.Context, refundID int64) (orderport.WeChatShopRefund, error) {
	if !service.ready() || refundID < 1 {
		return orderport.WeChatShopRefund{}, orderport.ErrCommerceRefundInvalid
	}
	if !service.provider.Enabled() {
		return orderport.WeChatShopRefund{}, orderport.ErrWeChatShopRefundDisabled
	}
	var current orderport.WeChatShopRefund
	err := service.uow.Within(ctx, func(tx context.Context) error {
		var lockErr error
		current, lockErr = service.store.LockWeChatShopRefundByID(tx, refundID)
		return lockErr
	})
	if err != nil {
		return orderport.WeChatShopRefund{}, classifyCommerceRefund(err)
	}
	if current.State == orderport.WeChatShopRefundSucceeded || current.State == orderport.WeChatShopRefundFinalFailed {
		return current, nil
	}
	if current.State != orderport.WeChatShopRefundProviderAccepted || !validReference(current.ProviderAfterSaleID) {
		return orderport.WeChatShopRefund{}, orderport.ErrCommerceRefundConflict
	}
	query, queryErr := service.provider.QueryRefund(ctx, current.ProviderAfterSaleID)
	if queryErr != nil || allZeroDigest(query.EvidenceDigest) {
		return orderport.WeChatShopRefund{}, errors.Join(orderport.ErrProviderOutcomeUnknown, queryErr)
	}
	outcome := "not_confirmed"
	if query.AfterSaleID != current.ProviderAfterSaleID || query.ProviderOrderID != current.ProviderOrderID || query.ProductID != current.ProductID || query.SKUID != current.SKUID || query.Count != current.RefundCount || query.AmountMinor != current.AmountMinor || query.Currency != current.Currency || query.Type != "REFUND" || allZeroDigest(query.ProviderRefundDigest) || query.OccurredAt.IsZero() {
		outcome = "conflict"
	} else if query.Status == "MERCHANT_REFUND_SUCCESS" {
		outcome = "applied"
	}
	err = service.uow.Within(ctx, func(tx context.Context) error {
		refund, lockErr := service.store.LockWeChatShopRefundByID(tx, current.ID)
		if lockErr != nil {
			return lockErr
		}
		if refund.State == orderport.WeChatShopRefundSucceeded || refund.State == orderport.WeChatShopRefundFinalFailed {
			current = refund
			return nil
		}
		if refund.State != orderport.WeChatShopRefundProviderAccepted || refund.ProviderAfterSaleID != current.ProviderAfterSaleID {
			return orderport.ErrCommerceRefundConflict
		}
		recordedQuery := query
		if outcome == "not_confirmed" {
			recordedQuery.ProviderRefundDigest, recordedQuery.AmountMinor, recordedQuery.Currency = [32]byte{}, 0, ""
		}
		if recordErr := service.store.RecordWeChatShopRefundQuery(tx, refund, recordedQuery, outcome, service.now().UTC()); recordErr != nil {
			return recordErr
		}
		if outcome != "applied" {
			return nil
		}
		resultDigest := wechatShopQueryResultDigest(query, refund.ID)
		current, lockErr = service.store.ApplyWeChatShopRefundSettlement(tx, refund, query.ProviderRefundDigest, resultDigest, query.OccurredAt)
		if lockErr != nil {
			return lockErr
		}
		return service.append(tx, eventport.EvOrderRefundSettled, current, resultDigest, query.OccurredAt)
	})
	if err != nil {
		return orderport.WeChatShopRefund{}, classifyCommerceRefund(err)
	}
	if outcome == "conflict" {
		return current, orderport.ErrCommerceRefundConflict
	}
	return current, nil
}

func (service *WeChatShopRefundService) ready() bool {
	return service != nil && service.uow != nil && service.store != nil && service.provider != nil && service.events != nil && service.now != nil
}

func (service *WeChatShopRefundService) append(ctx context.Context, eventType string, refund orderport.WeChatShopRefund, digest [32]byte, at time.Time) error {
	payload, _ := json.Marshal(map[string]any{"order_id": refund.OrderID, "refund_id": refund.ID, "provider": "wechat_shop", "amount_minor": refund.AmountMinor, "currency": refund.Currency, "state": refund.State})
	_, err := service.events.Append(ctx, eventport.Event{Type: eventType, Payload: payload, OccurredAt: at.UTC(), IdempotencyKey: "order.wechat_shop.refund:" + eventType + ":" + hex.EncodeToString(digest[:])})
	return err
}

func newWeChatShopReservation(order WeChatShopOrderRecord, material orderport.WeChatShopOrderMaterial, line orderport.WeChatShopOrderLine, command orderport.WeChatShopRefundCommand, key, commandPayload [32]byte, at time.Time) WeChatShopRefundReservation {
	number := "wsr_" + hex.EncodeToString(key[:16])
	reason := domainDigest("order/wechat-shop/reason/v1", command.Reason)
	transaction := domainDigest("wechat-shop/transaction/v1", command.TransactionIDConfirmation)
	return WeChatShopRefundReservation{
		Order: order, Material: material, Line: line, Command: command, OutRefundNo: number, ReasonDigest: reason,
		TransactionDigest: transaction, CommandKeyDigest: key, CommandPayloadDigest: commandPayload,
		SourceRefDigest: domainDigest("order/wechat-shop/source/v1", number),
		TargetRefDigest: domainDigest("order/wechat-shop/target/v2", order.MerchantOrderNo, command.ProductID, command.SKUID),
		PayloadDigest:   domainDigest("order/wechat-shop/payload/v2", fmt.Sprint(order.ID), command.ProductID, command.SKUID, fmt.Sprint(command.Count), fmt.Sprint(command.AmountMinor), order.Currency, command.ReasonCode, hex.EncodeToString(reason[:]), hex.EncodeToString(transaction[:]), hex.EncodeToString(material.EvidenceDigest[:])),
		PolicyDigest:    sha256.Sum256([]byte(wechatShopRefundPolicy)), CreatedAt: at,
	}
}

func validCompatibilityCommand(ctx context.Context, command orderport.WeChatPayRefundCompatibilityCommand) bool {
	return ctx != nil && ctx.Err() == nil && validReference(command.OrderReference) && validReference(command.TransactionIDConfirmation) && command.AmountMinor > 0 && command.AmountMinor <= 1_000_000_000 && command.Checked && command.Actor > 0 && validKey(command.IdempotencyKey) && validReason(command.Reason)
}

func validWeChatShopCommand(ctx context.Context, command orderport.WeChatShopRefundCommand) bool {
	return ctx != nil && ctx.Err() == nil && validReference(command.OrderReference) && validReference(command.TransactionIDConfirmation) && validReference(command.ProductID) && validReference(command.SKUID) && command.Count > 0 && command.Count <= 1_000_000 && command.AmountMinor > 0 && command.AmountMinor <= 1_000_000_000 && validWeChatShopReasonCode(command.ReasonCode) && command.Checked && command.Actor > 0 && validKey(command.IdempotencyKey) && validReason(command.Reason)
}

func validReason(reason string) bool {
	return reason != "" && len(reason) <= 500 && strings.TrimSpace(reason) == reason
}

func validWeChatShopOrder(order WeChatShopOrderRecord) bool {
	return order.ID > 0 && validReference(order.MerchantOrderNo) && validReference(order.PlatformTransactionNo) && order.AmountMinor > 0 && order.Currency == "CNY"
}

func validWeChatShopRefund(refund orderport.WeChatShopRefund) bool {
	if refund.ID < 1 || refund.OrderID < 1 || refund.ContractVersion != "provider/v2" || !validReference(refund.MerchantOrderNo) || !validReference(refund.ProviderOrderID) || !validReference(refund.ProductID) || !validReference(refund.SKUID) || refund.RefundCount < 1 || refund.UnitPriceMinor < 0 || refund.UnitPriceMinor > 1_000_000_000 || !validWeChatShopReasonCode(refund.ReasonCode) || allZeroDigest(refund.MaterialEvidenceDigest) || !strings.HasPrefix(refund.OutRefundNo, "wsr_") || refund.AmountMinor < 1 || refund.AmountMinor > refund.UnitPriceMinor*refund.RefundCount || refund.Currency != "CNY" || refund.Version < 1 || refund.AttemptCount < 0 || refund.CreatedAt.IsZero() || refund.UpdatedAt.Before(refund.CreatedAt) || allZeroDigest(refund.ReasonDigest) || allZeroDigest(refund.TransactionDigest) || allZeroDigest(refund.SourceRefDigest) || allZeroDigest(refund.TargetRefDigest) || allZeroDigest(refund.PayloadDigest) || allZeroDigest(refund.PolicyDigest) {
		return false
	}
	switch refund.State {
	case orderport.WeChatShopRefundAccepted, orderport.WeChatShopRefundExecuting, orderport.WeChatShopRefundOutcomeUnknown, orderport.WeChatShopRefundFinalFailed:
		return refund.ProviderAfterSaleID == "" && allZeroDigest(refund.ProviderAcceptanceDigest) && allZeroDigest(refund.ProviderRefundDigest) && allZeroDigest(refund.SettlementDigest) && refund.SettledAt.IsZero()
	case orderport.WeChatShopRefundProviderAccepted:
		return validReference(refund.ProviderAfterSaleID) && !allZeroDigest(refund.ProviderAcceptanceDigest) && allZeroDigest(refund.ProviderRefundDigest) && allZeroDigest(refund.SettlementDigest) && refund.SettledAt.IsZero()
	case orderport.WeChatShopRefundSucceeded:
		return validReference(refund.ProviderAfterSaleID) && !allZeroDigest(refund.ProviderAcceptanceDigest) && !allZeroDigest(refund.ProviderRefundDigest) && !allZeroDigest(refund.SettlementDigest) && !refund.SettledAt.IsZero()
	default:
		return false
	}
}

func validWeChatShopJob(job orderport.WeChatShopExecutionJob) bool {
	return job.RefundID > 0 && job.RiverJobID > 0 && job.RiverAttempt > 0 && !allZeroDigest(job.ArgsDigest) && !job.ScheduledAt.IsZero()
}

func validWeChatShopProviderResult(result orderport.WeChatShopProviderResult, err error) bool {
	switch result.Completion {
	case orderport.WeChatShopProviderAccepted:
		return err == nil && !allZeroDigest(result.EvidenceDigest) && validReference(result.AfterSaleID)
	case orderport.WeChatShopProviderOutcomeUnknown:
		return result.AfterSaleID == ""
	case orderport.WeChatShopProviderFinalFailed:
		return err == nil && !allZeroDigest(result.EvidenceDigest) && result.AfterSaleID == ""
	default:
		return false
	}
}

func validWeChatShopCallback(command orderport.WeChatShopRefundCallbackCommand) bool {
	return validReference(command.AfterSaleID) && validReference(command.ProviderOrderID) && validProviderStatus(command.ProviderStatus) && !allZeroDigest(command.ProviderEventDigest) && !allZeroDigest(command.PayloadDigest) && !command.OccurredAt.IsZero()
}

func wechatShopCommandDigest(command orderport.WeChatShopRefundCommand) [32]byte {
	return domainDigest("order/wechat-shop/command/v2", command.OrderReference, command.TransactionIDConfirmation, command.ProductID, command.SKUID, fmt.Sprint(command.Count), fmt.Sprint(command.AmountMinor), command.ReasonCode, command.Reason, fmt.Sprint(command.Checked), fmt.Sprint(command.Actor))
}

func wechatShopCallbackResultDigest(command orderport.WeChatShopRefundCallbackCommand, refundID int64) [32]byte {
	return domainDigest("order/wechat-shop/callback-result/v2", fmt.Sprint(refundID), command.AfterSaleID, command.ProviderOrderID, command.ProviderStatus, hex.EncodeToString(command.ProviderEventDigest[:]), hex.EncodeToString(command.PayloadDigest[:]))
}

func wechatShopQueryResultDigest(query orderport.WeChatShopRefundQueryResult, refundID int64) [32]byte {
	return domainDigest("order/wechat-shop/query-result/v1", fmt.Sprint(refundID), hex.EncodeToString(query.EvidenceDigest[:]), hex.EncodeToString(query.ProviderRefundDigest[:]), fmt.Sprint(query.AmountMinor), query.Currency)
}

func refundReadyMaterial(material orderport.WeChatShopOrderMaterial) bool {
	return material.Source == orderport.WeChatShopMaterialProvider && material.ProviderVerified && material.Readiness == orderport.WeChatShopMaterialReady && material.DealRecorded && material.Currency == "CNY" && material.AmountMinor > 0 && !allZeroDigest(material.TransactionDigest) && !allZeroDigest(material.EvidenceDigest) && !material.SyncedAt.IsZero()
}

func refundMaterialLine(material orderport.WeChatShopOrderMaterial, productID, skuID string) (orderport.WeChatShopOrderLine, bool) {
	for _, line := range material.Lines {
		if line.ProductID == productID && line.SKUID == skuID {
			return line, true
		}
	}
	return orderport.WeChatShopOrderLine{}, false
}

func validWeChatShopReasonCode(value string) bool {
	switch value {
	case "10000000", "10000001", "10000002", "10000006", "10000007", "10000008", "10000014", "10000015", "10000017", "10000021":
		return true
	default:
		return false
	}
}

func validProviderStatus(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if character != '_' && (character < 'A' || character > 'Z') {
			return false
		}
	}
	return true
}

func sameDigest(left, right [32]byte) bool {
	return subtle.ConstantTimeCompare(left[:], right[:]) == 1
}

func classifyCommerceRefund(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, orderport.ErrCommerceRefundInvalid), errors.Is(err, orderport.ErrInvalidSettlement):
		return orderport.ErrCommerceRefundInvalid
	case errors.Is(err, orderport.ErrCommerceRefundConflict), errors.Is(err, orderport.ErrSettlementConflict):
		return orderport.ErrCommerceRefundConflict
	case errors.Is(err, orderport.ErrCommerceRefundNotFound), errors.Is(err, orderport.ErrSettlementNotFound):
		return orderport.ErrCommerceRefundNotFound
	case errors.Is(err, orderport.ErrProviderOutcomeUnknown), errors.Is(err, orderport.ErrWeChatShopRefundDisabled):
		return err
	default:
		return errors.Join(orderport.ErrCommerceRefundUnavailable, err)
	}
}
