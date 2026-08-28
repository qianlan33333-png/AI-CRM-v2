package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	couponDefinitionsKind = "definitions"
	couponClaimsKind      = "claims"
	couponRedemptionsKind = "redemptions"

	couponDefinitionsTableID = "public/commerce_coupons"
	couponClaimsTableID      = "public/commerce_coupon_claims"
	couponRedemptionsTableID = "public/commerce_coupon_redemptions"
	couponImportVersion      = "v1-coupon-a1"
)

type couponTerminalJournal interface {
	LoadTerminal(context.Context, string) (TerminalReceipt, bool, error)
	Record(context.Context, TerminalReceipt) error
}

// CouponHistoryJournal binds the three source tables to their separate,
// immutable V2 receipt scopes. It records no claims or redemption effects.
type CouponHistoryJournal struct {
	definitions couponTerminalJournal
	claims      couponTerminalJournal
	redemptions couponTerminalJournal
}

var _ port.HistoricalJournal = (*CouponHistoryJournal)(nil)

func NewCouponHistoryJournal(definitions, claims, redemptions *Journal) (*CouponHistoryJournal, error) {
	if !validCouponJournalScope(definitions, couponDefinitionsTableID, "coupons") ||
		!validCouponJournalScope(claims, couponClaimsTableID, "coupon_v1_history_claims") ||
		!validCouponJournalScope(redemptions, couponRedemptionsTableID, "coupon_v1_history_redemptions") ||
		definitions.scope.ArchiveRunID != claims.scope.ArchiveRunID || definitions.scope.ArchiveRunID != redemptions.scope.ArchiveRunID {
		return nil, ErrInvalidScope
	}
	return newCouponHistoryJournal(definitions, claims, redemptions)
}

func newCouponHistoryJournal(definitions, claims, redemptions couponTerminalJournal) (*CouponHistoryJournal, error) {
	if definitions == nil || claims == nil || redemptions == nil {
		return nil, ErrInvalidScope
	}
	return &CouponHistoryJournal{definitions: definitions, claims: claims, redemptions: redemptions}, nil
}

func validCouponJournalScope(journal *Journal, tableID, targetTable string) bool {
	return journal != nil && journal.tx != nil && journal.scope.valid() &&
		journal.scope.ImportVersion == couponImportVersion && journal.scope.AdapterID == v1archive.DefaultAdapterID &&
		journal.scope.TableID == tableID && journal.scope.TargetDomain == "coupon" && journal.scope.TargetTable == targetTable
}

func (journal *CouponHistoryJournal) LoadHistoricalCoupon(ctx context.Context, kind, sourceIdentifier string) (port.HistoricalReceipt, bool, error) {
	selected, err := journal.selectJournal(kind)
	if err != nil {
		return port.HistoricalReceipt{}, false, err
	}
	terminal, found, err := selected.LoadTerminal(ctx, sourceIdentifier)
	if err != nil || !found {
		return port.HistoricalReceipt{}, found, err
	}
	receipt, err := couponReceiptFromTerminal(sourceIdentifier, terminal)
	return receipt, err == nil, err
}

func (journal *CouponHistoryJournal) RecordHistoricalCoupon(ctx context.Context, kind string, receipt port.HistoricalReceipt) error {
	selected, err := journal.selectJournal(kind)
	if err != nil {
		return err
	}
	terminal, err := couponTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return selected.Record(ctx, terminal)
}

func (journal *CouponHistoryJournal) selectJournal(kind string) (couponTerminalJournal, error) {
	if journal == nil {
		return nil, ErrInvalidScope
	}
	switch kind {
	case couponDefinitionsKind:
		if journal.definitions != nil {
			return journal.definitions, nil
		}
	case couponClaimsKind:
		if journal.claims != nil {
			return journal.claims, nil
		}
	case couponRedemptionsKind:
		if journal.redemptions != nil {
			return journal.redemptions, nil
		}
	}
	return nil, ErrInvalidScope
}

func couponTerminalFromReceipt(receipt port.HistoricalReceipt) (TerminalReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || sourceKey == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(sourceKey) ||
		receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.Replayed ||
		!canonicalPositiveID(receipt.TargetID) {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{
		SourceKeyDigest: sourceKey,
		PayloadDigest:   receipt.PayloadDigest,
		Disposition:     "import",
		TargetID:        strconv.FormatInt(receipt.TargetID, 10),
		TargetDigest:    receipt.TargetDigest,
		Metadata:        map[string]any{},
	}, nil
}

func couponReceiptFromTerminal(sourceIdentifier string, terminal TerminalReceipt) (port.HistoricalReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(sourceIdentifier)
	if err != nil || sourceKey == ([sha256.Size]byte{}) || sourceIdentifier != SourceIdentifier(sourceKey) ||
		terminal.SourceKeyDigest != sourceKey || terminal.PayloadDigest == ([sha256.Size]byte{}) ||
		terminal.TargetDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" ||
		len(terminal.Metadata) != 0 || !canonicalPositiveTargetID(terminal.TargetID) {
		return port.HistoricalReceipt{}, ErrConflict
	}
	targetID, _ := strconv.ParseInt(terminal.TargetID, 10, 64)
	return port.HistoricalReceipt{SourceIdentifier: sourceIdentifier, PayloadDigest: terminal.PayloadDigest, TargetID: targetID, TargetDigest: terminal.TargetDigest}, nil
}

func canonicalPositiveID(value int64) bool {
	return value > 0
}

func canonicalPositiveTargetID(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}
