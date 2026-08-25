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

const wechatShopRefundPolicy = "order-wechat-shop-refund/v1"

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
	Outcome              string
	ResultDigest         [32]byte
	State                string
}

type WeChatShopRefundStore interface {
	FindWeChatShopRefundOrder(context.Context, string) (WeChatShopOrderRecord, error)
	CountWeChatShopReservedRefundAmount(context.Context, orderport.ID) (int64, error)
	GetWeChatShopRefundByCommand(context.Context, int64, [32]byte) (orderport.WeChatShopRefund, [32]byte, bool, error)
	CreateWeChatShopRefund(context.Context, WeChatShopRefundReservation) (orderport.WeChatShopRefund, bool, error)
	EnqueueWeChatShopRefund(context.Context, int64) (int64, error)
	LockWeChatShopRefundByID(context.Context, int64) (orderport.WeChatShopRefund, error)
	LockWeChatShopRefundByOutRefundNo(context.Context, string) (orderport.WeChatShopRefund, error)
	StartWeChatShopRefundExecution(context.Context, orderport.WeChatShopRefund, orderport.WeChatShopExecutionJob, time.Time) (orderport.WeChatShopRefund, WeChatShopRefundAttempt, error)
	CompleteWeChatShopRefundExecution(context.Context, orderport.WeChatShopRefund, WeChatShopRefundAttempt, orderport.WeChatShopProviderCompletion, [32]byte, time.Time) (orderport.WeChatShopRefund, error)
	ReserveWeChatShopRefundCallback(context.Context, orderport.WeChatShopRefund, orderport.WeChatShopRefundCallbackCommand) (WeChatShopCallbackReceipt, bool, error)
	CompleteWeChatShopRefundCallback(context.Context, WeChatShopCallbackReceipt, string, [32]byte, time.Time) (WeChatShopCallbackReceipt, error)
	ApplyWeChatShopRefundSettlement(context.Context, orderport.WeChatShopRefund, [32]byte, [32]byte, time.Time) (orderport.WeChatShopRefund, error)
	MarkWeChatShopRefundFinalFailed(context.Context, orderport.WeChatShopRefund, time.Time) (orderport.WeChatShopRefund, error)
	RecordWeChatShopRefundQuery(context.Context, orderport.WeChatShopRefund, orderport.WeChatShopRefundQueryResult, string, time.Time) error
}

type WeChatShopRefundService struct {
	uow      platformport.UnitOfWork
	store    WeChatShopRefundStore
	provider orderport.WeChatShopRefundProvider
	events   eventport.Appender
	now      func() time.Time
}

var _ orderport.WeChatShopRefundApplication = (*WeChatShopRefundService)(nil)

func NewWeChatShopRefundService(uow platformport.UnitOfWork, store WeChatShopRefundStore, provider orderport.WeChatShopRefundProvider, events eventport.Appender) (*WeChatShopRefundService, error) {
	if uow == nil || store == nil || provider == nil || events == nil {
		return nil, orderport.ErrCommerceRefundUnavailable
	}
	return &WeChatShopRefundService{uow: uow, store: store, provider: provider, events: events, now: time.Now}, nil
}

func (service *WeChatShopRefundService) RequestRefund(ctx context.Context, command orderport.WeChatShopRefundCommand) (orderport.WeChatShopRefund, error) {
	if !service.ready() || !validWeChatShopCommand(ctx, command) {
		return orderport.WeChatShopRefund{}, orderport.ErrCommerceRefundInvalid
	}
	key := sha256.Sum256([]byte(command.IdempotencyKey))
	payload := wechatShopCommandDigest(command)
	now := service.now().UTC()
	var result orderport.WeChatShopRefund
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
		reserved, countErr := service.store.CountWeChatShopReservedRefundAmount(tx, order.ID)
		if countErr != nil || reserved < 0 || reserved > order.AmountMinor || command.AmountMinor > order.AmountMinor-reserved {
			return orderport.ErrCommerceRefundConflict
		}
		reservation := newWeChatShopReservation(order, command, key, payload, now)
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
		if service.provider.Enabled() {
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
		MerchantOrderNo: current.MerchantOrderNo, OutRefundNo: current.OutRefundNo,
		AmountMinor: current.AmountMinor, Currency: current.Currency, ReasonDigest: current.ReasonDigest,
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
		current, lockErr = service.store.CompleteWeChatShopRefundExecution(tx, refund, attempt, outcome, providerResult.EvidenceDigest, service.now().UTC())
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
		refund, lockErr := service.store.LockWeChatShopRefundByOutRefundNo(tx, command.OutRefundNo)
		if lockErr != nil {
			return lockErr
		}
		if refund.AmountMinor != command.AmountMinor || refund.Currency != command.Currency {
			return orderport.ErrCommerceRefundConflict
		}
		receipt, owned, reserveErr := service.store.ReserveWeChatShopRefundCallback(tx, refund, command)
		if reserveErr != nil {
			return reserveErr
		}
		if !owned {
			if receipt.State != "completed" || receipt.RefundID != refund.ID || !sameDigest(receipt.PayloadDigest, command.PayloadDigest) || !sameDigest(receipt.ProviderRefundDigest, command.ProviderRefundDigest) {
				return orderport.ErrCommerceRefundConflict
			}
			result = refund
			return nil
		}
		resultDigest := wechatShopCallbackResultDigest(command, refund.ID)
		if !command.Succeeded {
			result, lockErr = service.store.MarkWeChatShopRefundFinalFailed(tx, refund, command.OccurredAt)
			if lockErr != nil {
				return lockErr
			}
			_, lockErr = service.store.CompleteWeChatShopRefundCallback(tx, receipt, "rejected", resultDigest, command.OccurredAt)
			return lockErr
		}
		result, lockErr = service.store.ApplyWeChatShopRefundSettlement(tx, refund, command.ProviderRefundDigest, resultDigest, command.OccurredAt)
		if lockErr != nil {
			return lockErr
		}
		if appendErr := service.append(tx, eventport.EvOrderRefundSettled, result, resultDigest, command.OccurredAt); appendErr != nil {
			return appendErr
		}
		_, lockErr = service.store.CompleteWeChatShopRefundCallback(tx, receipt, "applied", resultDigest, command.OccurredAt)
		return lockErr
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
	if current.State != orderport.WeChatShopRefundOutcomeUnknown && current.State != orderport.WeChatShopRefundExecuting {
		return orderport.WeChatShopRefund{}, orderport.ErrCommerceRefundConflict
	}
	query, queryErr := service.provider.QueryRefund(ctx, current.OutRefundNo)
	if queryErr != nil || allZeroDigest(query.EvidenceDigest) {
		return orderport.WeChatShopRefund{}, errors.Join(orderport.ErrProviderOutcomeUnknown, queryErr)
	}
	outcome := "applied"
	if !query.Confirmed {
		outcome = "not_confirmed"
		query.ProviderRefundDigest, query.AmountMinor, query.Currency = [32]byte{}, 0, ""
	} else if allZeroDigest(query.ProviderRefundDigest) || query.AmountMinor != current.AmountMinor || query.Currency != current.Currency || query.OccurredAt.IsZero() {
		outcome = "conflict"
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
		if refund.State != orderport.WeChatShopRefundOutcomeUnknown && refund.State != orderport.WeChatShopRefundExecuting {
			return orderport.ErrCommerceRefundConflict
		}
		if recordErr := service.store.RecordWeChatShopRefundQuery(tx, refund, query, outcome, service.now().UTC()); recordErr != nil {
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
	if outcome == "not_confirmed" {
		return current, orderport.ErrProviderOutcomeUnknown
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

func newWeChatShopReservation(order WeChatShopOrderRecord, command orderport.WeChatShopRefundCommand, key, commandPayload [32]byte, at time.Time) WeChatShopRefundReservation {
	number := "wsr_" + hex.EncodeToString(key[:16])
	reason := domainDigest("order/wechat-shop/reason/v1", command.Reason)
	transaction := domainDigest("order/wechat-shop/transaction/v1", command.TransactionIDConfirmation)
	return WeChatShopRefundReservation{
		Order: order, Command: command, OutRefundNo: number, ReasonDigest: reason,
		TransactionDigest: transaction, CommandKeyDigest: key, CommandPayloadDigest: commandPayload,
		SourceRefDigest: domainDigest("order/wechat-shop/source/v1", number),
		TargetRefDigest: domainDigest("order/wechat-shop/target/v1", order.MerchantOrderNo),
		PayloadDigest:   domainDigest("order/wechat-shop/payload/v1", fmt.Sprint(order.ID), fmt.Sprint(command.AmountMinor), order.Currency, hex.EncodeToString(reason[:]), hex.EncodeToString(transaction[:])),
		PolicyDigest:    sha256.Sum256([]byte(wechatShopRefundPolicy)), CreatedAt: at,
	}
}

func validCompatibilityCommand(ctx context.Context, command orderport.WeChatPayRefundCompatibilityCommand) bool {
	return ctx != nil && ctx.Err() == nil && validReference(command.OrderReference) && validReference(command.TransactionIDConfirmation) && command.AmountMinor > 0 && command.AmountMinor <= 1_000_000_000 && command.Checked && command.Actor > 0 && validKey(command.IdempotencyKey) && validReason(command.Reason)
}

func validWeChatShopCommand(ctx context.Context, command orderport.WeChatShopRefundCommand) bool {
	return ctx != nil && ctx.Err() == nil && validReference(command.OrderReference) && validReference(command.TransactionIDConfirmation) && command.AmountMinor > 0 && command.AmountMinor <= 1_000_000_000 && command.Checked && command.Actor > 0 && validKey(command.IdempotencyKey) && validReason(command.Reason)
}

func validReason(reason string) bool {
	return reason != "" && len(reason) <= 500 && strings.TrimSpace(reason) == reason
}

func validWeChatShopOrder(order WeChatShopOrderRecord) bool {
	return order.ID > 0 && validReference(order.MerchantOrderNo) && validReference(order.PlatformTransactionNo) && order.AmountMinor > 0 && order.Currency == "CNY"
}

func validWeChatShopRefund(refund orderport.WeChatShopRefund) bool {
	if refund.ID < 1 || refund.OrderID < 1 || !validReference(refund.MerchantOrderNo) || !strings.HasPrefix(refund.OutRefundNo, "wsr_") || refund.AmountMinor < 1 || refund.Currency != "CNY" || refund.Version < 1 || refund.AttemptCount < 0 || refund.CreatedAt.IsZero() || refund.UpdatedAt.Before(refund.CreatedAt) || allZeroDigest(refund.ReasonDigest) || allZeroDigest(refund.TransactionDigest) || allZeroDigest(refund.SourceRefDigest) || allZeroDigest(refund.TargetRefDigest) || allZeroDigest(refund.PayloadDigest) || allZeroDigest(refund.PolicyDigest) {
		return false
	}
	switch refund.State {
	case orderport.WeChatShopRefundAccepted, orderport.WeChatShopRefundExecuting, orderport.WeChatShopRefundOutcomeUnknown, orderport.WeChatShopRefundFinalFailed:
		return allZeroDigest(refund.ProviderAcceptanceDigest) && allZeroDigest(refund.ProviderRefundDigest) && allZeroDigest(refund.SettlementDigest) && refund.SettledAt.IsZero()
	case orderport.WeChatShopRefundProviderAccepted:
		return !allZeroDigest(refund.ProviderAcceptanceDigest) && allZeroDigest(refund.ProviderRefundDigest) && allZeroDigest(refund.SettlementDigest) && refund.SettledAt.IsZero()
	case orderport.WeChatShopRefundSucceeded:
		return !allZeroDigest(refund.ProviderRefundDigest) && !allZeroDigest(refund.SettlementDigest) && !refund.SettledAt.IsZero()
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
		return err == nil && !allZeroDigest(result.EvidenceDigest)
	case orderport.WeChatShopProviderOutcomeUnknown:
		return true
	case orderport.WeChatShopProviderFinalFailed:
		return err == nil && !allZeroDigest(result.EvidenceDigest)
	default:
		return false
	}
}

func validWeChatShopCallback(command orderport.WeChatShopRefundCallbackCommand) bool {
	return validReference(command.OutRefundNo) && !allZeroDigest(command.ProviderEventDigest) && !allZeroDigest(command.PayloadDigest) && !allZeroDigest(command.ProviderRefundDigest) && command.AmountMinor > 0 && command.Currency == "CNY" && !command.OccurredAt.IsZero()
}

func wechatShopCommandDigest(command orderport.WeChatShopRefundCommand) [32]byte {
	return domainDigest("order/wechat-shop/command/v1", command.OrderReference, command.TransactionIDConfirmation, fmt.Sprint(command.AmountMinor), command.Reason, fmt.Sprint(command.Checked), fmt.Sprint(command.Actor))
}

func wechatShopCallbackResultDigest(command orderport.WeChatShopRefundCallbackCommand, refundID int64) [32]byte {
	return domainDigest("order/wechat-shop/callback-result/v1", fmt.Sprint(refundID), hex.EncodeToString(command.ProviderEventDigest[:]), hex.EncodeToString(command.PayloadDigest[:]), hex.EncodeToString(command.ProviderRefundDigest[:]), fmt.Sprint(command.AmountMinor), command.Currency, fmt.Sprint(command.Succeeded))
}

func wechatShopQueryResultDigest(query orderport.WeChatShopRefundQueryResult, refundID int64) [32]byte {
	return domainDigest("order/wechat-shop/query-result/v1", fmt.Sprint(refundID), hex.EncodeToString(query.EvidenceDigest[:]), hex.EncodeToString(query.ProviderRefundDigest[:]), fmt.Sprint(query.AmountMinor), query.Currency)
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
