package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestCouponHistoryJournalRoundTripsEachImportKind(t *testing.T) {
	definitions, claims, redemptions := newCouponTerminalFake(), newCouponTerminalFake(), newCouponTerminalFake()
	journal, err := newCouponHistoryJournal(definitions, claims, redemptions)
	if err != nil {
		t.Fatal(err)
	}
	for index, kind := range []string{couponDefinitionsKind, couponClaimsKind, couponRedemptionsKind} {
		receipt := couponReceiptFixture(byte(index + 1))
		if err := journal.RecordHistoricalCoupon(context.Background(), kind, receipt); err != nil {
			t.Fatalf("record %s: %v", kind, err)
		}
		got, found, err := journal.LoadHistoricalCoupon(context.Background(), kind, receipt.SourceIdentifier)
		if err != nil || !found || got != receipt {
			t.Fatalf("load %s: got=%+v found=%v err=%v", kind, got, found, err)
		}
	}
	if len(definitions.values) != 1 || len(claims.values) != 1 || len(redemptions.values) != 1 {
		t.Fatalf("receipt scopes crossed: definitions=%d claims=%d redemptions=%d", len(definitions.values), len(claims.values), len(redemptions.values))
	}
}

func TestCouponHistoryJournalRejectsUnsafeReceiptsAndTerminals(t *testing.T) {
	journal, err := newCouponHistoryJournal(newCouponTerminalFake(), newCouponTerminalFake(), newCouponTerminalFake())
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*port.HistoricalReceipt){
		"upper-source":        func(value *port.HistoricalReceipt) { value.SourceIdentifier = "A" + value.SourceIdentifier[1:] },
		"empty-payload":       func(value *port.HistoricalReceipt) { value.PayloadDigest = [sha256.Size]byte{} },
		"empty-target-digest": func(value *port.HistoricalReceipt) { value.TargetDigest = [sha256.Size]byte{} },
		"zero-target":         func(value *port.HistoricalReceipt) { value.TargetID = 0 },
		"replayed":            func(value *port.HistoricalReceipt) { value.Replayed = true },
	} {
		t.Run(name, func(t *testing.T) {
			receipt := couponReceiptFixture(1)
			mutate(&receipt)
			if err := journal.RecordHistoricalCoupon(context.Background(), couponDefinitionsKind, receipt); !errors.Is(err, ErrInvalidScope) {
				t.Fatalf("got %v", err)
			}
		})
	}
	for name, mutate := range map[string]func(*TerminalReceipt){
		"archive": func(value *TerminalReceipt) {
			value.Disposition = "archive"
			value.Reason = "history"
			value.TargetID = ""
			value.TargetDigest = [sha256.Size]byte{}
		},
		"reason":              func(value *TerminalReceipt) { value.Reason = "unexpected" },
		"target-zero":         func(value *TerminalReceipt) { value.TargetID = "0" },
		"target-leading-zero": func(value *TerminalReceipt) { value.TargetID = "041" },
		"target-digest":       func(value *TerminalReceipt) { value.TargetDigest = [sha256.Size]byte{} },
		"metadata":            func(value *TerminalReceipt) { value.Metadata = map[string]any{"field_digest": "not-permitted"} },
	} {
		t.Run(name, func(t *testing.T) {
			receipt := couponReceiptFixture(2)
			terminal, terminalErr := couponTerminalFromReceipt(receipt)
			if terminalErr != nil {
				t.Fatal(terminalErr)
			}
			mutate(&terminal)
			if _, err := couponReceiptFromTerminal(receipt.SourceIdentifier, terminal); !errors.Is(err, ErrConflict) {
				t.Fatalf("got %v", err)
			}
		})
	}
	if _, _, err := journal.LoadHistoricalCoupon(context.Background(), "bad-kind", couponReceiptFixture(3).SourceIdentifier); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("kind error = %v", err)
	}
}

func TestNewCouponHistoryJournalPinsExactScopes(t *testing.T) {
	definitions := couponScopedJournal(couponDefinitionsTableID, "coupons", "archive-run")
	claims := couponScopedJournal(couponClaimsTableID, "coupon_v1_history_claims", "archive-run")
	redemptions := couponScopedJournal(couponRedemptionsTableID, "coupon_v1_history_redemptions", "archive-run")
	if _, err := NewCouponHistoryJournal(definitions, claims, redemptions); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Journal){
		"version": func(value *Journal) { value.scope.ImportVersion = "v1-coupon-a2" },
		"run":     func(value *Journal) { value.scope.ArchiveRunID = "other-run" },
		"adapter": func(value *Journal) { value.scope.AdapterID = "other-adapter" },
		"source":  func(value *Journal) { value.scope.TableID = couponDefinitionsTableID },
		"domain":  func(value *Journal) { value.scope.TargetDomain = "order" },
		"target":  func(value *Journal) { value.scope.TargetTable = "coupons" },
	} {
		t.Run(name, func(t *testing.T) {
			defs := couponScopedJournal(couponDefinitionsTableID, "coupons", "archive-run")
			claimRows := couponScopedJournal(couponClaimsTableID, "coupon_v1_history_claims", "archive-run")
			redemptionRows := couponScopedJournal(couponRedemptionsTableID, "coupon_v1_history_redemptions", "archive-run")
			mutate(claimRows)
			if _, err := NewCouponHistoryJournal(defs, claimRows, redemptionRows); !errors.Is(err, ErrInvalidScope) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

type couponTerminalFake struct{ values map[string]TerminalReceipt }

func newCouponTerminalFake() *couponTerminalFake {
	return &couponTerminalFake{values: map[string]TerminalReceipt{}}
}

func (fake *couponTerminalFake) LoadTerminal(_ context.Context, source string) (TerminalReceipt, bool, error) {
	value, found := fake.values[source]
	return value, found, nil
}

func (fake *couponTerminalFake) Record(_ context.Context, receipt TerminalReceipt) error {
	key := SourceIdentifier(receipt.SourceKeyDigest)
	if found, exists := fake.values[key]; exists {
		if !sameCouponTerminal(found, receipt) {
			return ErrConflict
		}
		return nil
	}
	fake.values[key] = receipt
	return nil
}

func sameCouponTerminal(left, right TerminalReceipt) bool {
	return left.SourceKeyDigest == right.SourceKeyDigest && left.PayloadDigest == right.PayloadDigest &&
		left.Disposition == right.Disposition && left.Reason == right.Reason && left.TargetID == right.TargetID &&
		left.TargetDigest == right.TargetDigest && len(left.Metadata) == 0 && len(right.Metadata) == 0
}

func couponReceiptFixture(first byte) port.HistoricalReceipt {
	source, payload, target := [sha256.Size]byte{}, [sha256.Size]byte{}, [sha256.Size]byte{}
	source[0], payload[0], target[0] = first, first+10, first+20
	return port.HistoricalReceipt{SourceIdentifier: SourceIdentifier(source), PayloadDigest: payload, TargetID: int64(first) + 40, TargetDigest: target}
}

func couponScopedJournal(tableID, targetTable, archiveRun string) *Journal {
	return &Journal{scope: Scope{ImportVersion: couponImportVersion, ArchiveRunID: archiveRun, AdapterID: v1archive.DefaultAdapterID,
		TableID: tableID, TargetDomain: "coupon", TargetTable: targetTable}, tx: func(context.Context) (pgx.Tx, error) { return nil, nil }}
}
