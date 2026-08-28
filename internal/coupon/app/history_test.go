package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
)

type couponHistoryFixture struct {
	definition couponport.HistoricalDefinition
	claim      couponport.HistoricalClaim
	redemption couponport.HistoricalRedemption
}

func newCouponHistoryFixture() couponHistoryFixture {
	at := time.Date(2026, 8, 28, 12, 0, 0, 123456789, time.FixedZone("source", 8*3600))
	c := couponTestItem(0, at)
	c.Status, c.HistoryOnly, c.IssuedCount = "archived", true, 1
	c.TargetRefs = []string{"standard_product:9", "standard_product:7"}
	return couponHistoryFixture{
		definition: couponport.HistoricalDefinition{Coupon: c, SourceCouponID: 1, OriginalStatus: "stopped", FirstClaimAt: &at},
		claim: couponport.HistoricalClaim{SourceClaimID: 2, SourceCouponID: 1, CouponID: 101, ClaimNo: " old claim ", Status: "expired",
			DiscountAmountTotal: 100, Currency: "CNY", ValidFrom: at, ValidUntil: at.Add(time.Hour), ClaimedAt: at,
			ReservedAt: &at, ExpiredAt: &at, CreatedAt: at, UpdatedAt: at},
		redemption: couponport.HistoricalRedemption{SourceRedemptionID: 3, SourceClaimID: 2, SourceOrderID: 4, ClaimHistoryID: 102,
			OutTradeNo: " old trade ", Status: "released", OriginalAmountTotal: 1000, DiscountAmountTotal: 100, PayableAmountTotal: 900,
			Currency: "CNY", ReservedUntil: at.Add(time.Hour), ReservedAt: at, ReleaseReason: " original reason ", ReleasedAt: &at, CreatedAt: at, UpdatedAt: at},
	}
}

func (f couponHistoryFixture) run(w *HistoricalWriter, ctx context.Context, kind, source string, payload [32]byte) (couponport.HistoricalReceipt, error) {
	switch kind {
	case "definitions":
		return w.ImportDefinition(ctx, source, payload, f.definition)
	case "claims":
		return w.ImportClaim(ctx, source, payload, f.claim)
	default:
		return w.ImportRedemption(ctx, source, payload, f.redemption)
	}
}

type couponHistoryMemory struct {
	ctx context.Context
	couponHistoryFixture
	receipts                               map[string]couponport.HistoricalReceipt
	creates, reads, loads, records         int
	createErr, readErr, loadErr, recordErr error
	badCreate                              bool
}

func (m *couponHistoryMemory) check(ctx context.Context) {
	if ctx != m.ctx {
		panic("coupon history lost caller transaction context")
	}
}
func (m *couponHistoryMemory) CreateHistoricalDefinition(ctx context.Context, r couponport.HistoricalDefinition) (couponport.HistoricalDefinition, error) {
	m.check(ctx)
	m.creates++
	r.ID = 101
	if m.badCreate {
		r.OriginalStatus = "changed"
	}
	m.definition = r
	return r, m.createErr
}
func (m *couponHistoryMemory) GetHistoricalDefinition(ctx context.Context, _ int64) (couponport.HistoricalDefinition, error) {
	m.check(ctx)
	m.reads++
	return m.definition, m.readErr
}
func (m *couponHistoryMemory) CreateHistoricalClaim(ctx context.Context, r couponport.HistoricalClaim) (couponport.HistoricalClaim, error) {
	m.check(ctx)
	m.creates++
	r.ID = 102
	if m.badCreate {
		r.ClaimNo = "changed"
	}
	m.claim = r
	return r, m.createErr
}
func (m *couponHistoryMemory) GetHistoricalClaim(ctx context.Context, _ int64) (couponport.HistoricalClaim, error) {
	m.check(ctx)
	m.reads++
	return m.claim, m.readErr
}
func (m *couponHistoryMemory) CreateHistoricalRedemption(ctx context.Context, r couponport.HistoricalRedemption) (couponport.HistoricalRedemption, error) {
	m.check(ctx)
	m.creates++
	r.ID = 103
	if m.badCreate {
		r.ReleaseReason = "changed"
	}
	m.redemption = r
	return r, m.createErr
}
func (m *couponHistoryMemory) GetHistoricalRedemption(ctx context.Context, _ int64) (couponport.HistoricalRedemption, error) {
	m.check(ctx)
	m.reads++
	return m.redemption, m.readErr
}
func (m *couponHistoryMemory) LoadHistoricalCoupon(ctx context.Context, kind, source string) (couponport.HistoricalReceipt, bool, error) {
	m.check(ctx)
	m.loads++
	receipt, found := m.receipts[kind+"/"+source]
	return receipt, found, m.loadErr
}
func (m *couponHistoryMemory) RecordHistoricalCoupon(ctx context.Context, kind string, receipt couponport.HistoricalReceipt) error {
	m.check(ctx)
	m.records++
	if m.recordErr != nil {
		return m.recordErr
	}
	m.receipts[kind+"/"+receipt.SourceIdentifier] = receipt
	return nil
}

func couponHistoryTest(t *testing.T) (*HistoricalWriter, *couponHistoryMemory, context.Context) {
	t.Helper()
	ctx := context.WithValue(context.Background(), couponTestTxKey{}, true)
	m := &couponHistoryMemory{ctx: ctx, couponHistoryFixture: newCouponHistoryFixture(), receipts: map[string]couponport.HistoricalReceipt{}}
	m.definition.ID, m.claim.ID = 101, 102
	w, err := NewHistoricalWriter(m, m)
	if err != nil {
		t.Fatal(err)
	}
	return w, m, ctx
}

func TestCouponHistoryImportAndReplayPreserveStaticFacts(t *testing.T) {
	for index, kind := range []string{"definitions", "claims", "redemptions"} {
		t.Run(kind, func(t *testing.T) {
			w, m, ctx := couponHistoryTest(t)
			f := newCouponHistoryFixture()
			first, err := f.run(w, ctx, kind, "source-row", [32]byte{1})
			if err != nil || first.Replayed || first.SourceIdentifier != "source-row" || first.PayloadDigest != [32]byte{1} || first.TargetID != int64(101+index) || first.TargetDigest == [32]byte{} {
				t.Fatalf("first=%+v err=%v", first, err)
			}
			reads := m.reads
			second, err := f.run(w, ctx, kind, "source-row", [32]byte{1})
			first.Replayed = true
			if err != nil || first != second || m.creates != 1 || m.records != 1 || m.reads <= reads {
				t.Fatalf("replay=%+v err=%v creates=%d records=%d reads=%d", second, err, m.creates, m.records, m.reads)
			}
			switch kind {
			case "definitions":
				f.definition.ID = couponport.ID(first.TargetID)
				if !reflect.DeepEqual(m.definition, normalizeHistoricalDefinition(f.definition)) || m.definition.AvailabilityStatus != "archived" {
					t.Fatal("definition changed static facts or target order")
				}
			case "claims":
				f.claim.ID = first.TargetID
				if !reflect.DeepEqual(m.claim, normalizeHistoricalClaim(f.claim)) || m.claim.CustomerID != nil || m.claim.Status != "expired" {
					t.Fatal("claim changed original facts or guessed customer")
				}
			case "redemptions":
				f.redemption.ID = first.TargetID
				if !reflect.DeepEqual(m.redemption, normalizeHistoricalRedemption(f.redemption)) || m.redemption.OrderID != nil || m.redemption.ReleaseReason != " original reason " {
					t.Fatal("redemption changed original facts or guessed order")
				}
			}
		})
	}
}

func TestCouponHistoryRejectsReceiptAndTargetDrift(t *testing.T) {
	for _, kind := range []string{"definitions", "claims", "redemptions"} {
		for _, drift := range []string{"payload", "source", "receipt_id", "receipt_digest", "target_id", "target_fact", "parent"} {
			t.Run(kind+"/"+drift, func(t *testing.T) {
				w, m, ctx := couponHistoryTest(t)
				f := newCouponHistoryFixture()
				if _, err := f.run(w, ctx, kind, "row", [32]byte{1}); err != nil {
					t.Fatal(err)
				}
				payload := [32]byte{1}
				receipt := m.receipts[kind+"/row"]
				switch drift {
				case "payload":
					payload = [32]byte{2}
				case "source":
					receipt.SourceIdentifier = "other"
				case "receipt_id":
					receipt.TargetID = 0
				case "receipt_digest":
					receipt.TargetDigest = [32]byte{9}
				case "target_id":
					m.definition.ID++
					m.claim.ID++
					m.redemption.ID++
				case "target_fact":
					m.definition.FirstClaimAt = nil
					m.claim.Status = "consumed"
					m.redemption.DiscountAmountTotal++
				case "parent":
					m.definition.SourceCouponID++
					m.claim.SourceClaimID++
				}
				m.receipts[kind+"/row"] = receipt
				got, err := f.run(w, ctx, kind, "row", payload)
				if !errors.Is(err, couponport.ErrHistoryConflict) || got.TargetID != 0 || m.creates != 1 || m.records != 1 {
					t.Fatalf("drift accepted: receipt=%+v err=%v", got, err)
				}
			})
		}
	}
}

func TestCouponHistoryErrorsNeverBecomeSuccessReceipts(t *testing.T) {
	for _, kind := range []string{"definitions", "claims", "redemptions"} {
		for _, stage := range []string{"load", "create", "record", "read", "bad_create", "wrong_parent"} {
			t.Run(kind+"/"+stage, func(t *testing.T) {
				w, m, ctx := couponHistoryTest(t)
				f := newCouponHistoryFixture()
				failure := errors.New("database detail must not escape")
				want := couponport.ErrHistoryUnavailable
				switch stage {
				case "load":
					m.loadErr = failure
				case "create":
					m.createErr = failure
				case "record":
					m.recordErr = failure
				case "read":
					if _, err := f.run(w, ctx, kind, "row", [32]byte{1}); err != nil {
						t.Fatal(err)
					}
					m.readErr = failure
				case "bad_create":
					m.badCreate, want = true, couponport.ErrHistoryConflict
				case "wrong_parent":
					if kind == "definitions" {
						return
					}
					m.definition.SourceCouponID++
					m.claim.SourceClaimID++
					want = couponport.ErrHistoryConflict
				}
				got, err := f.run(w, ctx, kind, "row", [32]byte{1})
				if err != want || got != (couponport.HistoricalReceipt{}) || strings.Contains(err.Error(), "database detail") {
					t.Fatalf("failure was not closed: receipt=%+v err=%v", got, err)
				}
				if stage != "read" && len(m.receipts) != 0 {
					t.Fatal("failed import recorded success")
				}
				if stage == "wrong_parent" && m.creates != 0 {
					t.Fatal("created child for wrong parent")
				}
			})
		}
	}
}

func TestCouponHistoryValidationAndNullableFacts(t *testing.T) {
	zero := int64(0)
	for _, tc := range []struct {
		kind, name string
		change     func(*couponHistoryFixture)
	}{
		{"definitions", "id", func(f *couponHistoryFixture) { f.definition.ID = 1 }},
		{"definitions", "source", func(f *couponHistoryFixture) { f.definition.SourceCouponID = 0 }},
		{"definitions", "active", func(f *couponHistoryFixture) { f.definition.Status = "published" }},
		{"definitions", "not_history", func(f *couponHistoryFixture) { f.definition.HistoryOnly = false }},
		{"definitions", "version", func(f *couponHistoryFixture) { f.definition.Version = 2 }},
		{"definitions", "actor", func(f *couponHistoryFixture) { f.definition.UpdatedBy = 2 }},
		{"definitions", "currency", func(f *couponHistoryFixture) { f.definition.Currency = "USD" }},
		{"definitions", "issued", func(f *couponHistoryFixture) { f.definition.IssuedCount = 3 }},
		{"definitions", "first_claim", func(f *couponHistoryFixture) { f.definition.FirstClaimAt = nil }},
		{"definitions", "name_not_trimmed", func(f *couponHistoryFixture) { f.definition.Name = " original " }},
		{"definitions", "target_kind", func(f *couponHistoryFixture) { f.definition.TargetRefs = []string{"service_period:7"} }},
		{"definitions", "target_zero", func(f *couponHistoryFixture) { f.definition.TargetRefs = []string{"standard_product:0"} }},
		{"definitions", "target_duplicate", func(f *couponHistoryFixture) {
			f.definition.TargetRefs = []string{"standard_product:7", "standard_product:7"}
		}},
		{"definitions", "claim_time", func(f *couponHistoryFixture) { f.definition.ClaimStartsAt = time.Time{} }},
		{"claims", "id", func(f *couponHistoryFixture) { f.claim.ID = 1 }},
		{"claims", "source", func(f *couponHistoryFixture) { f.claim.SourceClaimID = 0 }},
		{"claims", "customer", func(f *couponHistoryFixture) { f.claim.CustomerID = &zero }},
		{"claims", "amount", func(f *couponHistoryFixture) { f.claim.DiscountAmountTotal = -1 }},
		{"claims", "currency", func(f *couponHistoryFixture) { f.claim.Currency = "USD" }},
		{"claims", "null_byte", func(f *couponHistoryFixture) { f.claim.ClaimNo = "bad\x00" }},
		{"claims", "timestamp", func(f *couponHistoryFixture) { f.claim.ValidFrom = time.Time{} }},
		{"redemptions", "id", func(f *couponHistoryFixture) { f.redemption.ID = 1 }},
		{"redemptions", "source_order", func(f *couponHistoryFixture) { f.redemption.SourceOrderID = 0 }},
		{"redemptions", "order", func(f *couponHistoryFixture) { f.redemption.OrderID = &zero }},
		{"redemptions", "amount", func(f *couponHistoryFixture) { f.redemption.PayableAmountTotal = -1 }},
		{"redemptions", "invalid_text", func(f *couponHistoryFixture) { f.redemption.ReleaseReason = string([]byte{0xff}) }},
	} {
		t.Run(tc.kind+"/"+tc.name, func(t *testing.T) {
			w, m, ctx := couponHistoryTest(t)
			f := newCouponHistoryFixture()
			tc.change(&f)
			if _, err := f.run(w, ctx, tc.kind, "row", [32]byte{1}); err != couponport.ErrHistoryInvalid || m.loads != 0 || m.creates != 0 {
				t.Fatalf("invalid input reached storage: %v", err)
			}
		})
	}
	// PostgreSQL limits characters, not UTF-8 bytes. Do not trim source text.
	w, m, ctx := couponHistoryTest(t)
	f := newCouponHistoryFixture()
	f.definition.Name, f.definition.Instructions = strings.Repeat("券", 45), strings.Repeat("说明", 100)
	f.definition.IssuedCount, f.definition.FirstClaimAt = 0, nil
	f.definition.ValidityMode, f.definition.RelativeValidityDays = couponport.ValidityFixedRange, nil
	f.definition.UseStartsAt, f.definition.UseEndsAt = &f.definition.ClaimStartsAt, &f.definition.ClaimEndsAt
	if _, err := f.run(w, ctx, "definitions", "unicode", [32]byte{1}); err != nil || m.definition.Name != f.definition.Name || m.definition.FirstClaimAt != nil {
		t.Fatalf("valid Unicode/unissued definition rejected: %v", err)
	}
}

func TestCouponHistoryRequiresCallerContextAndDependencies(t *testing.T) {
	var missing *couponHistoryMemory
	if _, err := NewHistoricalWriter(missing, missing); err != couponport.ErrHistoryUnavailable {
		t.Fatal(err)
	}
	for _, kind := range []string{"definitions", "claims", "redemptions"} {
		w, m, ctx := couponHistoryTest(t)
		f := newCouponHistoryFixture()
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		for _, input := range []context.Context{nil, cancelled} {
			if _, err := f.run(w, input, kind, "row", [32]byte{1}); err != couponport.ErrHistoryUnavailable || m.loads != 0 {
				t.Fatalf("invalid context reached storage: %v", err)
			}
		}
		if _, err := f.run(nil, ctx, kind, "row", [32]byte{1}); err != couponport.ErrHistoryUnavailable {
			t.Fatal(err)
		}
		for _, source := range []string{"", " row ", "bad\x00"} {
			if _, err := f.run(w, ctx, kind, source, [32]byte{1}); err != couponport.ErrHistoryInvalid {
				t.Fatal(err)
			}
		}
		if _, err := f.run(w, ctx, kind, "row", [32]byte{}); err != couponport.ErrHistoryInvalid {
			t.Fatal(err)
		}
		m.loadErr = fmt.Errorf("caller transaction is required")
		if _, err := f.run(w, ctx, kind, "row", [32]byte{1}); err != couponport.ErrHistoryUnavailable || m.creates != 0 {
			t.Fatalf("missing caller transaction accepted: %v", err)
		}
	}
}

func TestCouponHistoryDoesNotInventSourceStateOrAmountRules(t *testing.T) {
	for _, kind := range []string{"definitions", "claims", "redemptions"} {
		t.Run(kind, func(t *testing.T) {
			w, m, ctx := couponHistoryTest(t)
			f := newCouponHistoryFixture()
			f.definition.OriginalStatus = ""
			f.claim.Status, f.claim.ClaimNo = "", ""
			f.claim.ValidUntil = f.claim.ValidFrom.Add(-time.Hour)
			f.claim.UpdatedAt = f.claim.CreatedAt.Add(-time.Hour)
			f.redemption.Status, f.redemption.OutTradeNo = "", ""
			f.redemption.ReservedUntil = f.redemption.ReservedAt.Add(-time.Hour)
			f.redemption.UpdatedAt = f.redemption.CreatedAt.Add(-time.Hour)
			f.redemption.OriginalAmountTotal, f.redemption.DiscountAmountTotal, f.redemption.PayableAmountTotal = 5, 9, 17
			if _, err := f.run(w, ctx, kind, "source-fact", [32]byte{1}); err != nil {
				t.Fatalf("source fact rejected: %v", err)
			}
			switch kind {
			case "definitions":
				if m.definition.OriginalStatus != "" {
					t.Fatal("invented original status")
				}
			case "claims":
				if m.claim.Status != "" || m.claim.ClaimNo != "" || !m.claim.ValidUntil.Before(m.claim.ValidFrom) || !m.claim.UpdatedAt.Before(m.claim.CreatedAt) {
					t.Fatal("changed claim source facts")
				}
			case "redemptions":
				if m.redemption.Status != "" || m.redemption.OutTradeNo != "" || m.redemption.OriginalAmountTotal != 5 || m.redemption.DiscountAmountTotal != 9 || m.redemption.PayableAmountTotal != 17 || !m.redemption.ReservedUntil.Before(m.redemption.ReservedAt) || !m.redemption.UpdatedAt.Before(m.redemption.CreatedAt) {
					t.Fatal("changed redemption source facts")
				}
			}
		})
	}
}

func TestCouponHistoryDigestsNormalizeTimeButPreserveReferencesAndNulls(t *testing.T) {
	f := newCouponHistoryFixture()
	definition := normalizeHistoricalDefinition(f.definition)
	if HistoricalDefinitionTargetDigest(f.definition) != HistoricalDefinitionTargetDigest(definition) || definition.ClaimStartsAt.Location() != time.UTC || definition.FirstClaimAt.Nanosecond()%1000 != 0 {
		t.Fatal("definition time not normalized to UTC microseconds")
	}
	definition.TargetRefs = []string{"standard_product:7", "standard_product:9"}
	if HistoricalDefinitionTargetDigest(f.definition) == HistoricalDefinitionTargetDigest(definition) {
		t.Fatal("definition digest lost target order")
	}
	claim := normalizeHistoricalClaim(f.claim)
	if HistoricalClaimTargetDigest(f.claim) != HistoricalClaimTargetDigest(claim) {
		t.Fatal("claim time normalization changed digest")
	}
	customer := int64(9)
	claim.CustomerID = &customer
	if HistoricalClaimTargetDigest(f.claim) == HistoricalClaimTargetDigest(claim) {
		t.Fatal("claim digest lost nullable customer")
	}
	redemption := normalizeHistoricalRedemption(f.redemption)
	if HistoricalRedemptionTargetDigest(f.redemption) != HistoricalRedemptionTargetDigest(redemption) {
		t.Fatal("redemption time normalization changed digest")
	}
	redemption.ReleaseReason = strings.TrimSpace(redemption.ReleaseReason)
	if HistoricalRedemptionTargetDigest(f.redemption) == HistoricalRedemptionTargetDigest(redemption) {
		t.Fatal("redemption digest lost original reason whitespace")
	}
}
