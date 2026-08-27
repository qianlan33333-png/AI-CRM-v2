package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	historicalOrderKind  = "orders"
	historicalRefundKind = "refunds"
)

// HistoricalImportService persists static V1 facts only. It deliberately has
// no event, effect, payment, refund-command, or Provider dependency.
type HistoricalImportService struct {
	uow     platformport.UnitOfWork
	store   orderport.HistoricalImportStore
	journal orderport.HistoricalImportJournal
}

type HistoricalImportResult struct {
	TargetID int64
	Replayed bool
}

func NewHistoricalImportService(uow platformport.UnitOfWork, store orderport.HistoricalImportStore, journal orderport.HistoricalImportJournal) (*HistoricalImportService, error) {
	if nilDependency(uow) || nilDependency(store) || nilDependency(journal) {
		return nil, orderport.ErrHistoricalUnavailable
	}
	return &HistoricalImportService{uow: uow, store: store, journal: journal}, nil
}

// ImportOrder records one V1 order projection and its migration-owned receipt
// in the same transaction.
func (service *HistoricalImportService) ImportOrder(ctx context.Context, input orderport.HistoricalOrderRecord) (result HistoricalImportResult, err error) {
	if !historicalImportReady(service, ctx) || !validHistoricalOrderInput(input) {
		return result, orderport.ErrHistoricalInput
	}
	wantDigest := HistoricalOrderTargetDigest(input.Order)
	err = service.uow.Within(ctx, func(tx context.Context) error {
		result = HistoricalImportResult{}
		receipt, found, findErr := service.journal.FindHistoricalOrderReceipt(tx, historicalOrderKind, input.Fact.SourceKeyDigest)
		if findErr != nil {
			return historicalImportError(findErr)
		}
		if found {
			if !sameHistoricalReceipt(receipt, input.Fact, wantDigest) {
				return orderport.ErrHistoricalConflict
			}
			stored, getErr := service.store.GetHistoricalOrder(tx, orderport.ID(receipt.TargetID))
			if getErr != nil {
				return historicalReplayReadError(getErr)
			}
			if !validHistoricalOrderTarget(stored) || HistoricalOrderTargetDigest(stored) != wantDigest {
				return orderport.ErrHistoricalConflict
			}
			result.TargetID, result.Replayed = receipt.TargetID, true
			return nil
		}
		stored, createErr := service.store.CreateHistoricalOrder(tx, input.Order)
		if createErr != nil {
			return historicalImportError(createErr)
		}
		if !validHistoricalOrderTarget(stored) || HistoricalOrderTargetDigest(stored) != wantDigest {
			return orderport.ErrHistoricalConflict
		}
		receipt = orderport.HistoricalImportReceipt{HistoricalFact: input.Fact, TargetID: int64(stored.ID), TargetDigest: wantDigest}
		if appendErr := service.journal.AppendHistoricalOrderReceipt(tx, historicalOrderKind, receipt); appendErr != nil {
			return historicalImportError(appendErr)
		}
		result.TargetID = int64(stored.ID)
		return nil
	})
	if err != nil {
		return HistoricalImportResult{}, err
	}
	return result, nil
}

// ImportRefund records a V1 refund fact after verifying that its imported
// history order has the original order amount, currency, and transaction.
func (service *HistoricalImportService) ImportRefund(ctx context.Context, input orderport.HistoricalRefundRecord) (result HistoricalImportResult, err error) {
	if !historicalImportReady(service, ctx) || !validHistoricalRefundInput(input) {
		return result, orderport.ErrHistoricalInput
	}
	wantDigest := HistoricalRefundTargetDigest(input.Refund)
	err = service.uow.Within(ctx, func(tx context.Context) error {
		result = HistoricalImportResult{}
		receipt, found, findErr := service.journal.FindHistoricalOrderReceipt(tx, historicalRefundKind, input.Fact.SourceKeyDigest)
		if findErr != nil {
			return historicalImportError(findErr)
		}
		if found {
			if !sameHistoricalReceipt(receipt, input.Fact, wantDigest) {
				return orderport.ErrHistoricalConflict
			}
			stored, getErr := service.store.GetHistoricalRefund(tx, receipt.TargetID)
			if getErr != nil {
				return historicalReplayReadError(getErr)
			}
			if !validHistoricalRefundTarget(stored) || HistoricalRefundTargetDigest(stored) != wantDigest {
				return orderport.ErrHistoricalConflict
			}
			order, orderErr := service.store.GetHistoricalOrder(tx, stored.OrderID)
			if orderErr != nil {
				return historicalReplayReadError(orderErr)
			}
			if !validHistoricalOrderTarget(order) || !matchesHistoricalRefundOrder(order, stored) {
				return orderport.ErrHistoricalConflict
			}
			result.TargetID, result.Replayed = receipt.TargetID, true
			return nil
		}
		order, getErr := service.store.GetHistoricalOrder(tx, input.Refund.OrderID)
		if getErr != nil {
			return historicalReplayReadError(getErr)
		}
		if !validHistoricalOrderTarget(order) || !matchesHistoricalRefundOrder(order, input.Refund) {
			return orderport.ErrHistoricalConflict
		}
		stored, createErr := service.store.CreateHistoricalRefund(tx, input.Refund)
		if createErr != nil {
			return historicalImportError(createErr)
		}
		if !validHistoricalRefundTarget(stored) || HistoricalRefundTargetDigest(stored) != wantDigest {
			return orderport.ErrHistoricalConflict
		}
		receipt = orderport.HistoricalImportReceipt{HistoricalFact: input.Fact, TargetID: stored.ID, TargetDigest: wantDigest}
		if appendErr := service.journal.AppendHistoricalOrderReceipt(tx, historicalRefundKind, receipt); appendErr != nil {
			return historicalImportError(appendErr)
		}
		result.TargetID = stored.ID
		return nil
	})
	if err != nil {
		return HistoricalImportResult{}, err
	}
	return result, nil
}

// HistoricalOrderTargetDigest is a stable digest of only static target fields.
func HistoricalOrderTargetDigest(value orderport.Record) [32]byte {
	encoded, _ := json.Marshal(struct {
		Kind                                                                    string `json:"kind"`
		Origin, Provider, ProviderLabel, MerchantOrderNo, PlatformTransactionNo string
		CustomerID                                                              *int64
		PayerNameSnapshot, MobileSnapshot, IdentityKind, IdentityValue          string
		ProductID                                                               *int64
		ProductCode, ProductNameSnapshot                                        string
		AmountMinor                                                             int64
		Currency, Status, StatusLabel, DetailURL                                string
		CreatedAt, UpdatedAt                                                    string
	}{
		Kind: historicalOrderKind, Origin: value.RecordOrigin, Provider: value.Provider, ProviderLabel: value.ProviderLabel,
		MerchantOrderNo: value.MerchantOrderNo, PlatformTransactionNo: value.PlatformTransactionNo, CustomerID: value.CustomerID,
		PayerNameSnapshot: value.PayerNameSnapshot, MobileSnapshot: value.MobileSnapshot, IdentityKind: value.IdentityKind, IdentityValue: value.IdentityValue,
		ProductID: value.ProductID, ProductCode: value.ProductCode, ProductNameSnapshot: value.ProductNameSnapshot, AmountMinor: value.AmountMinor,
		Currency: value.Currency, Status: value.Status, StatusLabel: value.StatusLabel, DetailURL: value.DetailURL,
		CreatedAt: historicalTime(value.CreatedAt), UpdatedAt: historicalTime(value.UpdatedAt),
	})
	return sha256.Sum256(encoded)
}

// HistoricalRefundTargetDigest is a stable digest of only static target fields.
func HistoricalRefundTargetDigest(value orderport.HistoricalRefund) [32]byte {
	encoded, _ := json.Marshal(struct {
		Kind                                                  string `json:"kind"`
		OrderID                                               orderport.ID
		SourceRefundID                                        int64
		RefundNumber, ProviderRefundID, TransactionID, Status string
		AmountMinor, OrderAmountMinor                         int64
		Currency, Reason, CreatedAt, UpdatedAt                string
	}{
		Kind: historicalRefundKind, OrderID: value.OrderID, SourceRefundID: value.SourceRefundID,
		RefundNumber: value.RefundNumber, ProviderRefundID: value.ProviderRefundID, TransactionID: value.TransactionID, Status: value.Status,
		AmountMinor: value.AmountMinor, OrderAmountMinor: value.OrderAmountMinor, Currency: value.Currency, Reason: value.Reason,
		CreatedAt: historicalTime(value.CreatedAt), UpdatedAt: historicalTime(value.UpdatedAt),
	})
	return sha256.Sum256(encoded)
}

func historicalImportReady(service *HistoricalImportService, ctx context.Context) bool {
	return service != nil && ctx != nil && ctx.Err() == nil && !nilDependency(service.uow) && !nilDependency(service.store) && !nilDependency(service.journal)
}

func validHistoricalOrderInput(input orderport.HistoricalOrderRecord) bool {
	return validHistoricalFact(input.Fact) && input.Order.ID == 0 && validHistoricalOrderStatic(input.Order)
}

func validHistoricalOrderTarget(value orderport.Record) bool {
	return value.ID > 0 && validHistoricalOrderStatic(value)
}

func validHistoricalOrderStatic(value orderport.Record) bool {
	if value.RecordOrigin != orderport.RecordOriginV1History || value.Provider != "wechat" || value.Currency != "CNY" || value.AmountMinor < 0 {
		return false
	}
	value.ID = 1
	return validRecord(value)
}

func validHistoricalRefundInput(input orderport.HistoricalRefundRecord) bool {
	return validHistoricalFact(input.Fact) && input.Refund.ID == 0 && validHistoricalRefundStatic(input.Refund)
}

func validHistoricalRefundTarget(value orderport.HistoricalRefund) bool {
	return value.ID > 0 && validHistoricalRefundStatic(value)
}

func validHistoricalRefundStatic(value orderport.HistoricalRefund) bool {
	return value.OrderID > 0 && value.SourceRefundID > 0 && validHistoricalReference(value.RefundNumber, 200, true) &&
		utf8.ValidString(value.ProviderRefundID) && utf8.ValidString(value.TransactionID) &&
		validHistoricalReference(value.Status, 80, true) && utf8.ValidString(value.Reason) && value.AmountMinor > 0 && value.OrderAmountMinor >= value.AmountMinor &&
		value.Currency == "CNY" && validHistoricalTime(value.CreatedAt, value.UpdatedAt)
}

func validHistoricalFact(value orderport.HistoricalFact) bool {
	return !zeroHistoricalDigest(value.SourceKeyDigest) && !zeroHistoricalDigest(value.PayloadDigest) && !zeroHistoricalDigest(value.FieldDigest)
}

func validHistoricalReference(value string, maximum int, required bool) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= maximum && (!required || value != "")
}

func validHistoricalTime(created, updated time.Time) bool {
	return !created.IsZero() && !updated.IsZero() && !updated.Before(created) && created.Nanosecond()%1_000 == 0 && updated.Nanosecond()%1_000 == 0
}

func matchesHistoricalRefundOrder(order orderport.Record, refund orderport.HistoricalRefund) bool {
	return order.RecordOrigin == orderport.RecordOriginV1History && order.AmountMinor == refund.OrderAmountMinor && order.Currency == refund.Currency &&
		(refund.TransactionID == "" || order.PlatformTransactionNo == refund.TransactionID)
}

func sameHistoricalReceipt(receipt orderport.HistoricalImportReceipt, fact orderport.HistoricalFact, target [32]byte) bool {
	return receipt.TargetID > 0 && sameHistoricalDigest(receipt.SourceKeyDigest, fact.SourceKeyDigest) && sameHistoricalDigest(receipt.PayloadDigest, fact.PayloadDigest) &&
		sameHistoricalDigest(receipt.FieldDigest, fact.FieldDigest) && sameHistoricalDigest(receipt.TargetDigest, target)
}

func sameHistoricalDigest(left, right [32]byte) bool {
	return subtle.ConstantTimeCompare(left[:], right[:]) == 1
}
func zeroHistoricalDigest(value [32]byte) bool {
	return subtle.ConstantTimeCompare(value[:], make([]byte, len(value))) == 1
}
func historicalTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func historicalImportError(err error) error {
	if err == nil || errors.Is(err, orderport.ErrHistoricalInput) || errors.Is(err, orderport.ErrHistoricalConflict) || errors.Is(err, orderport.ErrHistoricalUnavailable) {
		return err
	}
	return errors.Join(orderport.ErrHistoricalUnavailable, err)
}

func historicalReplayReadError(err error) error {
	if errors.Is(err, ErrNotFound) || errors.Is(err, orderport.ErrHistoricalConflict) {
		return orderport.ErrHistoricalConflict
	}
	return historicalImportError(err)
}
