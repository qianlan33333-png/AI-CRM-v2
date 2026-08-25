package app

import (
	"context"
	"crypto/rand"
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
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

const (
	pe01PolicyVersion = "pe01-wechat-pay/v1"
	minimumKeyLength  = 16
	maximumKeyLength  = 128
)

var errSettlementRowNotFound = errors.New("PE01 settlement row not found")

func ErrSettlementRowNotFound() error { return errSettlementRowNotFound }

type FinancialOrderRecord struct {
	ID                     orderport.ID
	MerchantOrderNo        string
	ProviderTransactionRef string
	CustomerID             int64
	ProductID              int64
	ProductVersion         int64
	ProductKind            orderport.ProductKind
	AmountMinor            int64
	Currency               string
	State                  orderport.FinancialState
	PaymentIdentityDigest  [32]byte
	SettledAmountMinor     int64
	RefundedAmountMinor    int64
	SettlementDigest       [32]byte
	Version                int64
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type CallbackReceipt struct {
	ID            int64
	Kind          string
	EventDigest   [32]byte
	PayloadDigest [32]byte
	OrderID       orderport.ID
	RefundID      int64
	Outcome       string
	ResultDigest  [32]byte
	State         string
}

type SettlementStore interface {
	ReserveBoardReceipt(context.Context, BoardReservation) (BoardReceipt, bool, error)
	CompleteBoardReceipt(context.Context, int64, json.RawMessage, time.Time) (BoardReceipt, error)
	CreateCheckout(context.Context, orderport.CheckoutCommand, productport.Product, string, time.Time) (FinancialOrderRecord, error)
	CreatePaymentCommand(context.Context, FinancialOrderRecord, [32]byte, [32]byte, [32]byte, [32]byte, time.Time) (orderport.PaymentCommand, error)
	EnqueuePaymentBridge(context.Context, int64) (int64, error)
	EnqueueRefundBridge(context.Context, int64) (int64, error)
	EnqueuePaymentReconcile(context.Context, int64) (int64, error)
	EnqueueRefundReconcile(context.Context, int64) (int64, error)
	GetPaymentCommandByOrder(context.Context, orderport.ID) (orderport.PaymentCommand, error)
	LockPaymentCommand(context.Context, int64) (orderport.PaymentCommand, error)
	BindPaymentEffect(context.Context, orderport.PaymentCommand, string, time.Time) (orderport.PaymentCommand, error)
	CompletePaymentEffect(context.Context, orderport.PaymentCommand, orderport.EffectState, [32]byte, time.Time) (orderport.PaymentCommand, error)
	LockOrderByMerchantNo(context.Context, string) (FinancialOrderRecord, error)
	LockOrderByID(context.Context, orderport.ID) (FinancialOrderRecord, error)
	GetSelfScoped(context.Context, string, [32]byte) (FinancialOrderRecord, error)
	MarkOrderAwaitingPayment(context.Context, FinancialOrderRecord, time.Time) (FinancialOrderRecord, error)
	ApplyPaymentSettlement(context.Context, FinancialOrderRecord, string, [32]byte, time.Time) (FinancialOrderRecord, error)
	ReserveCallback(context.Context, string, [32]byte, [32]byte, orderport.ID, int64, time.Time) (CallbackReceipt, bool, error)
	CompleteCallback(context.Context, int64, string, [32]byte, time.Time) (CallbackReceipt, error)
	CountReservedRefundAmount(context.Context, orderport.ID) (int64, error)
	CreateRefund(context.Context, FinancialOrderRecord, orderport.RefundCommandV2, string, [32]byte, [32]byte, [32]byte, [32]byte, time.Time) (orderport.RefundV2, error)
	LockRefundByOutRefundNo(context.Context, string) (orderport.RefundV2, [32]byte, error)
	LockRefundByID(context.Context, int64) (orderport.RefundV2, error)
	BindRefundEffect(context.Context, orderport.RefundV2, string, time.Time) (orderport.RefundV2, error)
	CompleteRefundEffect(context.Context, orderport.RefundV2, orderport.EffectState, time.Time) (orderport.RefundV2, error)
	ApplyRefundSettlement(context.Context, orderport.RefundV2, [32]byte, [32]byte, time.Time) (orderport.RefundV2, error)
	AddSettledRefundToOrder(context.Context, FinancialOrderRecord, int64, time.Time) (FinancialOrderRecord, error)
}

type SettlementService struct {
	uow      platformport.UnitOfWork
	store    SettlementStore
	products productport.Reader
	benefits productport.PaidSettlementWriter
	events   eventport.Appender
	now      func() time.Time
	random   func([]byte) error
}

var _ orderport.SettlementApplication = (*SettlementService)(nil)

func NewSettlementService(uow platformport.UnitOfWork, store SettlementStore, products productport.Reader, benefits productport.PaidSettlementWriter, events eventport.Appender) (*SettlementService, error) {
	if uow == nil || store == nil || products == nil || benefits == nil || events == nil {
		return nil, orderport.ErrSettlementUnavailable
	}
	return &SettlementService{uow: uow, store: store, products: products, benefits: benefits, events: events, now: time.Now, random: func(value []byte) error { _, err := rand.Read(value); return err }}, nil
}

func (service *SettlementService) Checkout(ctx context.Context, command orderport.CheckoutCommand) (orderport.Checkout, error) {
	if !service.ready() || !validCheckoutCommand(command) {
		return orderport.Checkout{}, orderport.ErrInvalidSettlement
	}
	payloadDigest := checkoutDigest(command)
	keyDigest := sha256.Sum256([]byte(command.IdempotencyKey))
	now := service.now().UTC()
	reservation := BoardReservation{Operation: "pe01.checkout", ActorScope: command.ActorScope, KeyDigest: keyDigest, PayloadDigest: payloadDigest, CreatedAt: now}
	var result orderport.Checkout
	err := service.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, err := service.store.ReserveBoardReceipt(tx, reservation)
		if err != nil {
			return settlementStoreError(err)
		}
		if !receiptMatches(receipt, reservation) || subtle.ConstantTimeCompare(receipt.PayloadDigest[:], payloadDigest[:]) != 1 {
			return orderport.ErrSettlementConflict
		}
		if !owned {
			return replayCheckout(receipt, &result)
		}
		product, err := service.products.ReadProduct(tx, productport.ID(command.ProductID))
		if err != nil || !checkoutProductEligible(product, command.ProductKind) {
			return orderport.ErrInvalidSettlement
		}
		orderNo, err := service.orderNumber(keyDigest)
		if err != nil {
			return err
		}
		order, err := service.store.CreateCheckout(tx, command, product, orderNo, now)
		if err != nil || !validFinancialOrder(order) {
			return settlementStoreError(err)
		}
		source, target, effectPayload, policy := paymentDigests(order)
		payment, err := service.store.CreatePaymentCommand(tx, order, source, target, effectPayload, policy, now)
		if err != nil || payment.ID < 1 || payment.OrderID != order.ID || payment.State != orderport.EffectAccepted {
			return settlementStoreError(err)
		}
		if jobID, err := service.store.EnqueuePaymentBridge(tx, payment.ID); err != nil || jobID < 1 {
			return settlementStoreError(err)
		}
		result = checkoutProjection(order, payment.ID)
		if err = service.appendOrderEvent(tx, eventport.EvOrderCheckoutCreated, order, "checkout", keyDigest, now); err != nil {
			return err
		}
		return service.completeBoard(tx, receipt.ID, result, now)
	})
	if err != nil {
		return orderport.Checkout{}, classifySettlement(err)
	}
	return result, nil
}

func (service *SettlementService) ApplyPaymentCallback(ctx context.Context, command orderport.PaymentCallbackCommand) (orderport.Checkout, error) {
	if !service.ready() || !validPaymentCallback(command) {
		return orderport.Checkout{}, orderport.ErrInvalidSettlement
	}
	var result orderport.Checkout
	err := service.uow.Within(ctx, func(tx context.Context) error {
		order, err := service.store.LockOrderByMerchantNo(tx, command.MerchantOrderNo)
		if err != nil || !validFinancialOrder(order) {
			return settlementStoreError(err)
		}
		receipt, owned, err := service.store.ReserveCallback(tx, "payment", command.ProviderEventDigest, command.PayloadDigest, order.ID, 0, command.OccurredAt)
		if err != nil {
			return settlementStoreError(err)
		}
		if !owned {
			if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], command.PayloadDigest[:]) != 1 || receipt.OrderID != order.ID || receipt.Kind != "payment" || receipt.State != "completed" {
				return orderport.ErrSettlementConflict
			}
			paymentID, getErr := paymentCommandID(tx, service.store, order.ID)
			if getErr != nil {
				return getErr
			}
			result = checkoutProjection(order, paymentID)
			return nil
		}
		if order.AmountMinor != command.AmountMinor || order.Currency != command.Currency || order.State != orderport.FinancialAwaitingPayment {
			return orderport.ErrSettlementConflict
		}
		resultDigest := paymentResultDigest(command, order.ID)
		if !command.Succeeded {
			if _, err = service.store.CompleteCallback(tx, receipt.ID, "rejected", resultDigest, command.OccurredAt); err != nil {
				return settlementStoreError(err)
			}
			paymentID, getErr := paymentCommandID(tx, service.store, order.ID)
			if getErr != nil {
				return getErr
			}
			result = checkoutProjection(order, paymentID)
			return nil
		}
		providerRef := "sha256:" + hex.EncodeToString(command.ProviderTransactionDigest[:])
		order, err = service.store.ApplyPaymentSettlement(tx, order, providerRef, resultDigest, command.OccurredAt)
		if err != nil || order.State != orderport.FinancialPaid {
			return settlementStoreError(err)
		}
		if _, err = service.benefits.ApplyPaidSettlement(tx, productport.PaidSettlementCommand{Action: productport.PaidSettlementGrant, ProductKind: paidProductKind(order.ProductKind), ProductID: productport.ID(order.ProductID), ProductVersion: order.ProductVersion, OrderID: int64(order.ID), CustomerID: order.CustomerID, SettlementReceiptDigest: resultDigest, OccurredAt: command.OccurredAt}); err != nil {
			return err
		}
		if err = service.appendOrderEvent(tx, eventport.EvOrderPaymentSettled, order, "payment", resultDigest, command.OccurredAt); err != nil {
			return err
		}
		if _, err = service.store.CompleteCallback(tx, receipt.ID, "applied", resultDigest, command.OccurredAt); err != nil {
			return settlementStoreError(err)
		}
		paymentID, getErr := paymentCommandID(tx, service.store, order.ID)
		if getErr != nil {
			return getErr
		}
		result = checkoutProjection(order, paymentID)
		return nil
	})
	if err != nil {
		return orderport.Checkout{}, classifySettlement(err)
	}
	return result, nil
}

func (service *SettlementService) RequestRefundV2(ctx context.Context, command orderport.RefundCommandV2) (orderport.RefundV2, error) {
	if !service.ready() || command.OrderID < 1 || command.AmountMinor < 1 || command.AmountMinor > 1_000_000_000 || command.Actor < 1 || !validKey(command.IdempotencyKey) || strings.TrimSpace(command.Reason) != command.Reason || command.Reason == "" || len(command.Reason) > 500 || !validReference(command.TransactionIDConfirmation) {
		return orderport.RefundV2{}, orderport.ErrInvalidSettlement
	}
	payload := refundCommandDigest(command)
	key := sha256.Sum256([]byte(command.IdempotencyKey))
	now := service.now().UTC()
	reservation := BoardReservation{Operation: "pe01.refund", ActorScope: fmt.Sprintf("admin:%d", command.Actor), KeyDigest: key, PayloadDigest: payload, CreatedAt: now}
	var result orderport.RefundV2
	err := service.uow.Within(ctx, func(tx context.Context) error {
		receipt, owned, err := service.store.ReserveBoardReceipt(tx, reservation)
		if err != nil {
			return settlementStoreError(err)
		}
		if !receiptMatches(receipt, reservation) || subtle.ConstantTimeCompare(receipt.PayloadDigest[:], payload[:]) != 1 {
			return orderport.ErrSettlementConflict
		}
		if !owned {
			if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, &result) != nil || result.ID < 1 {
				return orderport.ErrSettlementUnavailable
			}
			return nil
		}
		order, err := service.store.LockOrderByID(tx, command.OrderID)
		if err != nil || (order.State != orderport.FinancialPaid && order.State != orderport.FinancialPartiallyRefunded) || order.ProviderTransactionRef != command.TransactionIDConfirmation {
			return orderport.ErrSettlementConflict
		}
		reserved, err := service.store.CountReservedRefundAmount(tx, order.ID)
		if err != nil || reserved < 0 || command.AmountMinor > order.SettledAmountMinor-reserved {
			return orderport.ErrSettlementConflict
		}
		outRefundNo := "pe01r_" + hex.EncodeToString(key[:16])
		source, target, effectPayload, policy := refundDigests(order, command, outRefundNo)
		result, err = service.store.CreateRefund(tx, order, command, outRefundNo, source, target, effectPayload, policy, now)
		if err != nil || result.ID < 1 || result.OrderID != order.ID || result.State != orderport.EffectAccepted {
			return settlementStoreError(err)
		}
		if jobID, err := service.store.EnqueueRefundBridge(tx, result.ID); err != nil || jobID < 1 {
			return settlementStoreError(err)
		}
		if err = service.appendOrderEvent(tx, eventport.EvOrderRefundRequested, order, "refund", key, now); err != nil {
			return err
		}
		return service.completeBoard(tx, receipt.ID, result, now)
	})
	if err != nil {
		return orderport.RefundV2{}, classifySettlement(err)
	}
	return result, nil
}

func (service *SettlementService) ApplyRefundCallback(ctx context.Context, command orderport.RefundCallbackCommand) (orderport.RefundV2, error) {
	if !service.ready() || !validRefundCallback(command) {
		return orderport.RefundV2{}, orderport.ErrInvalidSettlement
	}
	var result orderport.RefundV2
	err := service.uow.Within(ctx, func(tx context.Context) error {
		refund, _, err := service.store.LockRefundByOutRefundNo(tx, command.OutRefundNo)
		if err != nil {
			return settlementStoreError(err)
		}
		order, err := service.store.LockOrderByID(tx, refund.OrderID)
		if err != nil || order.Currency != command.Currency || refund.AmountMinor != command.AmountMinor || refund.Currency != command.Currency {
			return orderport.ErrSettlementConflict
		}
		receipt, owned, err := service.store.ReserveCallback(tx, "refund", command.ProviderEventDigest, command.PayloadDigest, order.ID, refund.ID, command.OccurredAt)
		if err != nil {
			return settlementStoreError(err)
		}
		if !owned {
			if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], command.PayloadDigest[:]) != 1 || receipt.RefundID != refund.ID || receipt.State != "completed" {
				return orderport.ErrSettlementConflict
			}
			result = refund
			return nil
		}
		resultDigest := refundResultDigest(command, refund)
		if !command.Succeeded {
			if _, err = service.store.CompleteCallback(tx, receipt.ID, "rejected", resultDigest, command.OccurredAt); err != nil {
				return settlementStoreError(err)
			}
			result = refund
			return nil
		}
		result, err = service.store.ApplyRefundSettlement(tx, refund, command.ProviderRefundDigest, resultDigest, command.OccurredAt)
		if err != nil || result.State != "succeeded" {
			return settlementStoreError(err)
		}
		order, err = service.store.AddSettledRefundToOrder(tx, order, refund.AmountMinor, command.OccurredAt)
		if err != nil {
			return settlementStoreError(err)
		}
		if order.State == orderport.FinancialRefunded {
			if _, err = service.benefits.ApplyPaidSettlement(tx, productport.PaidSettlementCommand{Action: productport.PaidSettlementCompensate, ProductKind: paidProductKind(order.ProductKind), ProductID: productport.ID(order.ProductID), ProductVersion: order.ProductVersion, OrderID: int64(order.ID), CustomerID: order.CustomerID, SettlementReceiptDigest: order.SettlementDigest, OccurredAt: command.OccurredAt}); err != nil {
				return err
			}
		}
		if err = service.appendOrderEvent(tx, eventport.EvOrderRefundSettled, order, "refund", resultDigest, command.OccurredAt); err != nil {
			return err
		}
		if _, err = service.store.CompleteCallback(tx, receipt.ID, "applied", resultDigest, command.OccurredAt); err != nil {
			return settlementStoreError(err)
		}
		return nil
	})
	if err != nil {
		return orderport.RefundV2{}, classifySettlement(err)
	}
	return result, nil
}

func (service *SettlementService) GetSelfScoped(ctx context.Context, merchantOrderNo string, identity [32]byte) (orderport.Checkout, error) {
	if !service.ready() || !validReference(merchantOrderNo) || allZeroDigest(identity) {
		return orderport.Checkout{}, orderport.ErrInvalidSettlement
	}
	var result orderport.Checkout
	err := service.uow.Within(ctx, func(tx context.Context) error {
		order, err := service.store.GetSelfScoped(tx, merchantOrderNo, identity)
		if err != nil {
			return err
		}
		paymentID, getErr := paymentCommandID(tx, service.store, order.ID)
		if getErr != nil {
			return getErr
		}
		result = checkoutProjection(order, paymentID)
		return nil
	})
	return result, classifySettlement(err)
}

func (service *SettlementService) ready() bool {
	return service != nil && service.uow != nil && service.store != nil && service.products != nil && service.benefits != nil && service.events != nil && service.now != nil && service.random != nil
}

func (service *SettlementService) orderNumber(key [32]byte) (string, error) {
	random := make([]byte, 8)
	if err := service.random(random); err != nil {
		return "", orderport.ErrSettlementUnavailable
	}
	return "pe01_" + hex.EncodeToString(key[:8]) + hex.EncodeToString(random), nil
}

func (service *SettlementService) completeBoard(ctx context.Context, id int64, result any, now time.Time) error {
	snapshot, err := json.Marshal(result)
	if err != nil {
		return orderport.ErrSettlementUnavailable
	}
	receipt, err := service.store.CompleteBoardReceipt(ctx, id, snapshot, now)
	if err != nil || receipt.State != "completed" {
		return settlementStoreError(err)
	}
	return nil
}

func (service *SettlementService) appendOrderEvent(ctx context.Context, eventType string, order FinancialOrderRecord, operation string, digest [32]byte, at time.Time) error {
	payload, _ := json.Marshal(map[string]any{"order_id": order.ID, "merchant_order_no": order.MerchantOrderNo, "amount_minor": order.AmountMinor, "currency": order.Currency, "state": order.State})
	_, err := service.events.Append(ctx, eventport.Event{Type: eventType, CustomerID: eventport.CustomerID(order.CustomerID), Payload: payload, OccurredAt: at, IdempotencyKey: "pe01.order." + operation + ":" + hex.EncodeToString(digest[:])})
	return err
}

func validCheckoutCommand(command orderport.CheckoutCommand) bool {
	return command.CustomerID > 0 && command.ProductID > 0 && command.ProductKind.Valid() && !allZeroDigest(command.PaymentIdentityDigest) && validActorScope(command.ActorScope) && validKey(command.IdempotencyKey)
}

func checkoutProductEligible(product productport.Product, kind orderport.ProductKind) bool {
	if product.ID < 1 || product.Version < 1 || product.PriceMinor < 1 || product.Currency != "CNY" || strings.TrimSpace(product.ProductCode) == "" || strings.TrimSpace(product.Name) == "" {
		return false
	}
	if kind == orderport.ProductKindOrdinary {
		return product.LocalLifecycle == productport.LocalProductEnabled
	}
	var projection struct {
		Status  string `json:"status"`
		Enabled bool   `json:"enabled"`
	}
	return json.Unmarshal(product.LegacyAdminProjection, &projection) == nil && projection.Status == "service_period_enabled" && projection.Enabled
}

func validPaymentCallback(command orderport.PaymentCallbackCommand) bool {
	return validReference(command.MerchantOrderNo) && !allZeroDigest(command.ProviderEventDigest) && !allZeroDigest(command.PayloadDigest) && !allZeroDigest(command.ProviderTransactionDigest) && command.AmountMinor > 0 && command.Currency == "CNY" && !command.OccurredAt.IsZero()
}

func validRefundCallback(command orderport.RefundCallbackCommand) bool {
	return validReference(command.OutRefundNo) && !allZeroDigest(command.ProviderEventDigest) && !allZeroDigest(command.PayloadDigest) && !allZeroDigest(command.ProviderRefundDigest) && command.AmountMinor > 0 && command.Currency == "CNY" && !command.OccurredAt.IsZero()
}

func validActorScope(value string) bool {
	if !strings.HasPrefix(value, "payment-session:") || len(value) != len("payment-session:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "payment-session:"))
	return err == nil
}

func validKey(value string) bool {
	return len(value) >= minimumKeyLength && len(value) <= maximumKeyLength && strings.TrimSpace(value) == value
}
func validReference(value string) bool {
	return value != "" && len(value) <= 200 && strings.TrimSpace(value) == value
}

func validFinancialOrder(order FinancialOrderRecord) bool {
	return order.ID > 0 && validReference(order.MerchantOrderNo) && order.CustomerID > 0 && order.ProductID > 0 && order.ProductVersion > 0 && order.ProductKind.Valid() && order.AmountMinor > 0 && order.Currency == "CNY" && order.Version > 0 && !order.CreatedAt.IsZero()
}

func checkoutDigest(command orderport.CheckoutCommand) [32]byte {
	return domainDigest("pe01/checkout/v1", fmt.Sprint(command.CustomerID), fmt.Sprint(command.ProductID), string(command.ProductKind), hex.EncodeToString(command.PaymentIdentityDigest[:]), command.ActorScope)
}

func paymentDigests(order FinancialOrderRecord) ([32]byte, [32]byte, [32]byte, [32]byte) {
	return domainDigest("pe01/source/payment/v1", order.MerchantOrderNo), domainDigest("pe01/target/wechat/v1", order.MerchantOrderNo), domainDigest("pe01/payload/prepay/v1", fmt.Sprint(order.ID), fmt.Sprint(order.ProductID), fmt.Sprint(order.CustomerID), fmt.Sprint(order.AmountMinor), order.Currency), sha256.Sum256([]byte(pe01PolicyVersion))
}

func refundDigests(order FinancialOrderRecord, command orderport.RefundCommandV2, outRefundNo string) ([32]byte, [32]byte, [32]byte, [32]byte) {
	return domainDigest("pe01/source/refund/v1", outRefundNo), domainDigest("pe01/target/wechat-refund/v1", order.MerchantOrderNo), domainDigest("pe01/payload/refund/v2", fmt.Sprint(order.ID), fmt.Sprint(command.AmountMinor), order.Currency, command.Reason, command.TransactionIDConfirmation), sha256.Sum256([]byte(pe01PolicyVersion))
}

func refundCommandDigest(command orderport.RefundCommandV2) [32]byte {
	return domainDigest("pe01/refund-command/v2", fmt.Sprint(command.OrderID), fmt.Sprint(command.AmountMinor), command.Reason, command.TransactionIDConfirmation, fmt.Sprint(command.Actor))
}

func paymentResultDigest(command orderport.PaymentCallbackCommand, orderID orderport.ID) [32]byte {
	return domainDigest("pe01/payment-result/v1", fmt.Sprint(orderID), hex.EncodeToString(command.ProviderEventDigest[:]), hex.EncodeToString(command.PayloadDigest[:]), hex.EncodeToString(command.ProviderTransactionDigest[:]), fmt.Sprint(command.AmountMinor), command.Currency, fmt.Sprint(command.Succeeded))
}

func refundResultDigest(command orderport.RefundCallbackCommand, refund orderport.RefundV2) [32]byte {
	return domainDigest("pe01/refund-result/v1", fmt.Sprint(refund.ID), hex.EncodeToString(command.ProviderEventDigest[:]), hex.EncodeToString(command.PayloadDigest[:]), hex.EncodeToString(command.ProviderRefundDigest[:]), fmt.Sprint(command.AmountMinor), command.Currency, fmt.Sprint(command.Succeeded))
}

func domainDigest(domain string, parts ...string) [32]byte {
	return sha256.Sum256([]byte(domain + "\x00" + strings.Join(parts, "\x00")))
}

func allZeroDigest(value [32]byte) bool {
	var zero [32]byte
	return subtle.ConstantTimeCompare(value[:], zero[:]) == 1
}

func checkoutProjection(order FinancialOrderRecord, paymentID int64) orderport.Checkout {
	return orderport.Checkout{OrderID: order.ID, MerchantOrderNo: order.MerchantOrderNo, State: order.State, ProductKind: order.ProductKind, CustomerID: order.CustomerID, ProductID: order.ProductID, AmountMinor: order.AmountMinor, Currency: order.Currency, PaymentCommandID: paymentID, CreatedAt: order.CreatedAt}
}

func paymentCommandID(ctx context.Context, store SettlementStore, orderID orderport.ID) (int64, error) {
	command, err := store.GetPaymentCommandByOrder(ctx, orderID)
	if err != nil || command.ID < 1 || command.OrderID != orderID {
		return 0, settlementStoreError(err)
	}
	return command.ID, nil
}

func replayCheckout(receipt BoardReceipt, result *orderport.Checkout) error {
	if receipt.State != "completed" || json.Unmarshal(receipt.ResultSnapshot, result) != nil || result.OrderID < 1 || result.PaymentCommandID < 1 {
		return orderport.ErrSettlementUnavailable
	}
	return nil
}

func paidProductKind(kind orderport.ProductKind) productport.PaidSettlementProductKind {
	if kind == orderport.ProductKindServicePeriod {
		return productport.PaidSettlementServicePeriod
	}
	return productport.PaidSettlementOrdinary
}

func settlementStoreError(err error) error {
	if err == nil {
		return orderport.ErrSettlementUnavailable
	}
	return err
}

func classifySettlement(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, orderport.ErrInvalidSettlement), errors.Is(err, orderport.ErrSettlementConflict), errors.Is(err, orderport.ErrSettlementNotFound):
		return err
	case errors.Is(err, errSettlementRowNotFound):
		return orderport.ErrSettlementNotFound
	default:
		return errors.Join(orderport.ErrSettlementUnavailable, err)
	}
}
