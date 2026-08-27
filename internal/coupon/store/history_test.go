package store

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var couponHistoryTestURL = flag.String("coupon-history-test-database-url", "", "dedicated migrated PostgreSQL fixture")
var couponHistoryTestProduct = flag.Int64("coupon-history-test-product-id", 0, "existing fixture product; never mutated")

func TestHistoricalStoreRequiresCallerTransaction(t *testing.T) {
	r := NewRepository()
	ctx := context.Background()
	if _, err := r.CreateHistoricalDefinition(ctx, couponport.HistoricalDefinition{}); err == nil {
		t.Fatal("missing transaction accepted")
	}
	if _, err := r.CreateHistoricalClaim(ctx, couponport.HistoricalClaim{}); err == nil {
		t.Fatal("missing transaction accepted")
	}
	if _, err := r.CreateHistoricalRedemption(ctx, couponport.HistoricalRedemption{}); err == nil {
		t.Fatal("missing transaction accepted")
	}
	if _, _, err := NewHistoricalReader(nil).ListHistoricalDefinitions(ctx, 10, 0); err == nil {
		t.Fatal("missing database accepted")
	}
}

func TestCouponHistoryPostgresRoundTripAndRollback(t *testing.T) {
	if *couponHistoryTestURL == "" {
		t.Skip("dedicated database not provided")
	}
	if *couponHistoryTestProduct < 1 {
		t.Fatal("fixture product id required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *couponHistoryTestURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	r := NewRepository()
	now := time.Date(2026, 8, 28, 0, 0, 0, 123000, time.UTC)
	rollback := errors.New("rollback coupon history fixture")
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		d := couponport.HistoricalDefinition{Coupon: couponport.Coupon{Name: "UAT coupon history", DiscountAmountTotal: 1, Currency: "CNY", Status: "archived", AvailabilityStatus: "archived", TotalIssueLimit: 10, PerUserIssueLimit: 1, IssuedCount: 2, ClaimStartsAt: now, ClaimEndsAt: now.Add(time.Hour), ValidityMode: couponport.ValidityRelativeDays, RelativeValidityDays: couponHistoryInt32(1), Instructions: "history only", TargetRefs: []string{fmt.Sprintf("standard_product:%d", *couponHistoryTestProduct)}, CreatedBy: 1, UpdatedBy: 1, Version: 1, HistoryOnly: true, CreatedAt: now, UpdatedAt: now}, SourceCouponID: 900000001, OriginalStatus: "stopped", FirstClaimAt: &now}
		got, err := r.CreateHistoricalDefinition(tx, d)
		if err != nil {
			return fmt.Errorf("definition: %w", err)
		}
		d.ID = got.ID
		if !reflect.DeepEqual(got, d) {
			return fmt.Errorf("definition facts changed")
		}
		locked, err := r.Lock(tx, got.ID)
		if err != nil || !locked.HistoryOnly {
			return fmt.Errorf("history marker missing: %v", err)
		}
		claim := couponport.HistoricalClaim{SourceClaimID: 900000002, SourceCouponID: d.SourceCouponID, CouponID: int64(d.ID), ClaimNo: "legacy-claim", Status: "expired", DiscountAmountTotal: 1, Currency: "CNY", ValidFrom: now.Add(time.Hour), ValidUntil: now, ClaimedAt: now, ExpiredAt: &now, CreatedAt: now, UpdatedAt: now.Add(-time.Hour)}
		gotClaim, err := r.CreateHistoricalClaim(tx, claim)
		if err != nil {
			return fmt.Errorf("claim: %w", err)
		}
		claim.ID = gotClaim.ID
		if !reflect.DeepEqual(gotClaim, claim) {
			return fmt.Errorf("claim facts changed")
		}
		redemption := couponport.HistoricalRedemption{SourceRedemptionID: 900000003, SourceClaimID: claim.SourceClaimID, SourceOrderID: 900000004, ClaimHistoryID: claim.ID, OutTradeNo: "legacy-order", Status: "released", OriginalAmountTotal: 10, DiscountAmountTotal: 1, PayableAmountTotal: 8, Currency: "CNY", ReservedUntil: now, ReleaseReason: "original reason", ReservedAt: now, ReleasedAt: &now, CreatedAt: now, UpdatedAt: now.Add(-time.Hour)}
		gotRedemption, err := r.CreateHistoricalRedemption(tx, redemption)
		if err != nil {
			return fmt.Errorf("redemption: %w", err)
		}
		redemption.ID = gotRedemption.ID
		if !reflect.DeepEqual(gotRedemption, redemption) {
			return fmt.Errorf("redemption facts changed")
		}
		reader := NewHistoricalReader(couponHistoryInlineUOW{})
		definitions, total, err := reader.ListHistoricalDefinitions(tx, 100, 0)
		if err != nil || total < 1 || len(definitions) < 1 {
			return fmt.Errorf("definitions page: %v", err)
		}
		claims, total, err := reader.ListHistoricalClaims(tx, int64(d.ID), 100, 0)
		if err != nil || total != 1 || len(claims) != 1 || !reflect.DeepEqual(claims[0], claim) {
			return fmt.Errorf("claims page: %v", err)
		}
		redemptions, total, err := reader.ListHistoricalRedemptions(tx, int64(d.ID), 100, 0)
		if err != nil || total != 1 || len(redemptions) != 1 || !reflect.DeepEqual(redemptions[0], redemption) {
			return fmt.Errorf("redemptions page: %v", err)
		}
		return rollback
	})
	if err != rollback {
		t.Fatal(err)
	}
}

type couponHistoryInlineUOW struct{}

func (couponHistoryInlineUOW) Within(ctx context.Context, f func(context.Context) error) error {
	if _, err := platformstore.TxFromContext(ctx); err != nil {
		return err
	}
	return f(ctx)
}
func couponHistoryInt32(v int32) *int32 { return &v }
