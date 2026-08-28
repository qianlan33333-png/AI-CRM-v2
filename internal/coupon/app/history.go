package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
)

// HistoricalWriter uses the caller transaction enforced by store and journal.
// It never issues a current claim, event, job, payment or Provider operation.
type HistoricalWriter struct {
	store   couponport.HistoricalStore
	journal couponport.HistoricalJournal
}

func NewHistoricalWriter(store couponport.HistoricalStore, journal couponport.HistoricalJournal) (*HistoricalWriter, error) {
	if nilHistoricalDependency(store) || nilHistoricalDependency(journal) {
		return nil, couponport.ErrHistoryUnavailable
	}
	return &HistoricalWriter{store: store, journal: journal}, nil
}

func (w *HistoricalWriter) ImportDefinition(ctx context.Context, source string, payload [32]byte, record couponport.HistoricalDefinition) (couponport.HistoricalReceipt, error) {
	record = normalizeHistoricalDefinition(record)
	if !validHistoricalDefinition(record) {
		return couponport.HistoricalReceipt{}, couponport.ErrHistoryInvalid
	}
	return w.importHistory(ctx, "definitions", source, payload, func(id int64) [32]byte {
		expected := record
		expected.ID = couponport.ID(id)
		return HistoricalDefinitionTargetDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		var actual couponport.HistoricalDefinition
		var err error
		if id == 0 {
			actual, err = w.store.CreateHistoricalDefinition(ctx, record)
		} else {
			actual, err = w.store.GetHistoricalDefinition(ctx, id)
		}
		return int64(actual.ID), HistoricalDefinitionTargetDigest(actual), err
	})
}

func (w *HistoricalWriter) ImportClaim(ctx context.Context, source string, payload [32]byte, record couponport.HistoricalClaim) (couponport.HistoricalReceipt, error) {
	record = normalizeHistoricalClaim(record)
	if record.ID != 0 || record.SourceClaimID < 1 || record.SourceCouponID < 1 || record.CouponID < 1 ||
		!historicalOptionalID(record.CustomerID) || record.Currency != "CNY" || record.DiscountAmountTotal < 0 ||
		!historicalText(record.ClaimNo, record.Status) ||
		!historicalTimes(record.ValidFrom, record.ValidUntil, record.ClaimedAt, record.CreatedAt, record.UpdatedAt) ||
		!historicalOptionalTimes(record.ReservedAt, record.ConsumedAt, record.ExpiredAt) {
		return couponport.HistoricalReceipt{}, couponport.ErrHistoryInvalid
	}
	return w.importHistory(ctx, "claims", source, payload, func(id int64) [32]byte {
		expected := record
		expected.ID = id
		return HistoricalClaimTargetDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		parent, err := w.store.GetHistoricalDefinition(ctx, record.CouponID)
		if err != nil {
			return 0, [32]byte{}, err
		}
		if int64(parent.ID) != record.CouponID || parent.SourceCouponID != record.SourceCouponID || !parent.HistoryOnly || parent.Status != "archived" {
			return 0, [32]byte{}, couponport.ErrHistoryConflict
		}
		var actual couponport.HistoricalClaim
		if id == 0 {
			actual, err = w.store.CreateHistoricalClaim(ctx, record)
		} else {
			actual, err = w.store.GetHistoricalClaim(ctx, id)
		}
		return actual.ID, HistoricalClaimTargetDigest(actual), err
	})
}

func (w *HistoricalWriter) ImportRedemption(ctx context.Context, source string, payload [32]byte, record couponport.HistoricalRedemption) (couponport.HistoricalReceipt, error) {
	record = normalizeHistoricalRedemption(record)
	if record.ID != 0 || record.SourceRedemptionID < 1 || record.SourceClaimID < 1 || record.SourceOrderID < 1 || record.ClaimHistoryID < 1 ||
		!historicalOptionalID(record.OrderID) || record.Currency != "CNY" || record.OriginalAmountTotal < 0 || record.DiscountAmountTotal < 0 || record.PayableAmountTotal < 0 ||
		!historicalText(record.OutTradeNo, record.Status, record.ReleaseReason) ||
		!historicalTimes(record.ReservedUntil, record.ReservedAt, record.CreatedAt, record.UpdatedAt) || !historicalOptionalTimes(record.ConsumedAt, record.ReleasedAt) {
		return couponport.HistoricalReceipt{}, couponport.ErrHistoryInvalid
	}
	return w.importHistory(ctx, "redemptions", source, payload, func(id int64) [32]byte {
		expected := record
		expected.ID = id
		return HistoricalRedemptionTargetDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		parent, err := w.store.GetHistoricalClaim(ctx, record.ClaimHistoryID)
		if err != nil {
			return 0, [32]byte{}, err
		}
		if parent.ID != record.ClaimHistoryID || parent.SourceClaimID != record.SourceClaimID {
			return 0, [32]byte{}, couponport.ErrHistoryConflict
		}
		var actual couponport.HistoricalRedemption
		if id == 0 {
			actual, err = w.store.CreateHistoricalRedemption(ctx, record)
		} else {
			actual, err = w.store.GetHistoricalRedemption(ctx, id)
		}
		return actual.ID, HistoricalRedemptionTargetDigest(actual), err
	})
}

// access creates for ID zero; replay always reads the actual target.
func (w *HistoricalWriter) importHistory(ctx context.Context, kind, source string, payload [32]byte, expected func(int64) [32]byte, access func(int64) (int64, [32]byte, error)) (couponport.HistoricalReceipt, error) {
	var empty couponport.HistoricalReceipt
	if w == nil || nilHistoricalDependency(w.store) || nilHistoricalDependency(w.journal) || ctx == nil || ctx.Err() != nil {
		return empty, couponport.ErrHistoryUnavailable
	}
	if source == "" || strings.TrimSpace(source) != source || !historicalText(source) || payload == [32]byte{} {
		return empty, couponport.ErrHistoryInvalid
	}
	receipt, found, err := w.journal.LoadHistoricalCoupon(ctx, kind, source)
	if err != nil {
		return empty, historicalError(err)
	}
	if found {
		if receipt.SourceIdentifier != source || receipt.PayloadDigest != payload || receipt.TargetID < 1 || receipt.TargetDigest != expected(receipt.TargetID) {
			return empty, couponport.ErrHistoryConflict
		}
		id, digest, err := access(receipt.TargetID)
		if err != nil {
			return empty, historicalError(err)
		}
		if id != receipt.TargetID || digest != receipt.TargetDigest {
			return empty, couponport.ErrHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	id, digest, err := access(0)
	if err != nil {
		return empty, historicalError(err)
	}
	if id < 1 || digest != expected(id) {
		return empty, couponport.ErrHistoryConflict
	}
	receipt = couponport.HistoricalReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: id, TargetDigest: digest}
	if err = w.journal.RecordHistoricalCoupon(ctx, kind, receipt); err != nil {
		return empty, historicalError(err)
	}
	return receipt, nil
}

func HistoricalDefinitionTargetDigest(record couponport.HistoricalDefinition) [32]byte {
	return historicalDigest("definitions", normalizeHistoricalDefinition(record))
}
func HistoricalClaimTargetDigest(record couponport.HistoricalClaim) [32]byte {
	return historicalDigest("claims", normalizeHistoricalClaim(record))
}
func HistoricalRedemptionTargetDigest(record couponport.HistoricalRedemption) [32]byte {
	return historicalDigest("redemptions", normalizeHistoricalRedemption(record))
}

func historicalDigest(kind string, record any) [32]byte {
	encoded, err := json.Marshal(record)
	if err != nil {
		return [32]byte{}
	}
	return sha256.Sum256(append([]byte("coupon_history\x00"+kind+"\x00"), encoded...))
}

func normalizeHistoricalDefinition(r couponport.HistoricalDefinition) couponport.HistoricalDefinition {
	r.AvailabilityStatus = "archived" // Derived display field, not a persisted state.
	r.ClaimStartsAt, r.ClaimEndsAt = canonicalTime(r.ClaimStartsAt), canonicalTime(r.ClaimEndsAt)
	r.CreatedAt, r.UpdatedAt = canonicalTime(r.CreatedAt), canonicalTime(r.UpdatedAt)
	r.UseStartsAt, r.UseEndsAt, r.FirstClaimAt = historicalTimePointer(r.UseStartsAt), historicalTimePointer(r.UseEndsAt), historicalTimePointer(r.FirstClaimAt)
	return r
}
func normalizeHistoricalClaim(r couponport.HistoricalClaim) couponport.HistoricalClaim {
	r.ValidFrom, r.ValidUntil, r.ClaimedAt = canonicalTime(r.ValidFrom), canonicalTime(r.ValidUntil), canonicalTime(r.ClaimedAt)
	r.CreatedAt, r.UpdatedAt = canonicalTime(r.CreatedAt), canonicalTime(r.UpdatedAt)
	r.ReservedAt, r.ConsumedAt, r.ExpiredAt = historicalTimePointer(r.ReservedAt), historicalTimePointer(r.ConsumedAt), historicalTimePointer(r.ExpiredAt)
	return r
}
func normalizeHistoricalRedemption(r couponport.HistoricalRedemption) couponport.HistoricalRedemption {
	r.ReservedUntil, r.ReservedAt = canonicalTime(r.ReservedUntil), canonicalTime(r.ReservedAt)
	r.CreatedAt, r.UpdatedAt = canonicalTime(r.CreatedAt), canonicalTime(r.UpdatedAt)
	r.ConsumedAt, r.ReleasedAt = historicalTimePointer(r.ConsumedAt), historicalTimePointer(r.ReleasedAt)
	return r
}

func validHistoricalDefinition(r couponport.HistoricalDefinition) bool {
	if r.ID != 0 || r.SourceCouponID < 1 || r.Status != "archived" || r.Version != 1 || !r.HistoryOnly || r.CreatedBy < 1 || r.UpdatedBy != r.CreatedBy ||
		r.Currency != "CNY" || r.Name == "" || strings.TrimSpace(r.Name) != r.Name || utf8.RuneCountInString(r.Name) > 45 ||
		strings.TrimSpace(r.Instructions) != r.Instructions || utf8.RuneCountInString(r.Instructions) > 200 || !historicalText(r.Name, r.Instructions, r.OriginalStatus) ||
		r.DiscountAmountTotal < 1 || r.TotalIssueLimit < 1 || r.PerUserIssueLimit < 1 || r.PerUserIssueLimit > r.TotalIssueLimit || r.IssuedCount < 0 || r.IssuedCount > r.TotalIssueLimit ||
		(r.IssuedCount == 0) != (r.FirstClaimAt == nil) || !historicalTimes(r.ClaimStartsAt, r.ClaimEndsAt, r.CreatedAt, r.UpdatedAt) ||
		!r.ClaimEndsAt.After(r.ClaimStartsAt) || r.UpdatedAt.Before(r.CreatedAt) || !historicalOptionalTimes(r.UseStartsAt, r.UseEndsAt, r.FirstClaimAt) || len(r.TargetRefs) < 1 || len(r.TargetRefs) > 100 {
		return false
	}
	if r.ValidityMode == couponport.ValidityFixedRange {
		if r.UseStartsAt == nil || r.UseEndsAt == nil || !r.UseEndsAt.After(*r.UseStartsAt) || r.RelativeValidityDays != nil {
			return false
		}
	} else if r.ValidityMode != couponport.ValidityRelativeDays || r.UseStartsAt != nil || r.UseEndsAt != nil || r.RelativeValidityDays == nil || *r.RelativeValidityDays < 1 {
		return false
	}
	seen := map[string]bool{}
	for _, ref := range r.TargetRefs {
		raw, ok := strings.CutPrefix(ref, "standard_product:")
		id, err := strconv.ParseInt(raw, 10, 64)
		if !ok || err != nil || id < 1 || raw != strconv.FormatInt(id, 10) || seen[ref] {
			return false
		}
		seen[ref] = true
	}
	return true
}

func historicalTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := canonicalTime(*value)
	return &normalized
}
func historicalTimes(values ...time.Time) bool {
	for _, value := range values {
		if value.IsZero() || value.Year() < 1 || value.Year() > 9999 {
			return false
		}
	}
	return true
}
func historicalOptionalTimes(values ...*time.Time) bool {
	for _, value := range values {
		if value != nil && !historicalTimes(*value) {
			return false
		}
	}
	return true
}
func historicalOptionalID(id *int64) bool { return id == nil || *id > 0 }
func historicalText(values ...string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return false
		}
	}
	return true
}
func nilHistoricalDependency(value any) bool {
	return value == nil || reflect.ValueOf(value).Kind() == reflect.Pointer && reflect.ValueOf(value).IsNil()
}
func historicalError(err error) error {
	switch {
	case errors.Is(err, couponport.ErrHistoryInvalid):
		return couponport.ErrHistoryInvalid
	case errors.Is(err, couponport.ErrHistoryConflict):
		return couponport.ErrHistoryConflict
	default:
		return couponport.ErrHistoryUnavailable
	}
}
