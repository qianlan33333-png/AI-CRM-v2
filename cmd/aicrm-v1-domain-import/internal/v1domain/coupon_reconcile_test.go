package v1domain

import (
	"crypto/sha256"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	couponapp "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/app"
	coupondb "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/store/generated"
)

func TestCouponReconcileTargetIDsAreClosedAndCanonical(t *testing.T) {
	for _, pair := range []struct {
		source, target string
		parse          func(reconciliationRow) (int64, error)
	}{
		{couponDefinitionsTableID, "coupons", couponDefinitionTargetID},
		{couponClaimsTableID, "coupon_v1_history_claims", couponClaimTargetID},
		{couponRedemptionsTableID, "coupon_v1_history_redemptions", couponRedemptionTargetID},
	} {
		domain, table, id := "coupon", pair.target, "17"
		row := reconciliationRow{TableID: pair.source, TargetDomain: &domain, TargetTable: &table, TargetID: &id, TargetDigest: make([]byte, sha256.Size)}
		if actual, err := pair.parse(row); err != nil || actual != 17 {
			t.Fatalf("%s got=%d err=%v", pair.target, actual, err)
		}
		for _, invalid := range []string{"", "0", "01", "+1", "-1", "1x"} {
			row.TargetID = &invalid
			if _, err := pair.parse(row); err == nil {
				t.Fatalf("%s accepted %q", pair.target, invalid)
			}
		}
	}
	domain, table, id := "coupon", "coupon_targets", HistoricalCouponBindingTargetID(17, 0)
	row := reconciliationRow{TableID: couponBindingsTableID, TargetDomain: &domain, TargetTable: &table, TargetID: &id, TargetDigest: make([]byte, sha256.Size)}
	if couponID, position, err := couponBindingTargetID(row); err != nil || couponID != 17 || position != 0 {
		t.Fatalf("binding got=%d:%d err=%v", couponID, position, err)
	}
	for _, invalid := range []string{"17", "17:-1", "17:00", "017:0", "17:0:1"} {
		row.TargetID = &invalid
		if _, _, err := couponBindingTargetID(row); err == nil {
			t.Fatalf("binding accepted %q", invalid)
		}
	}
}

func TestCouponReconcileVerifiesAllHistoricalDigests(t *testing.T) {
	definition, marker := couponReconcileDefinition()
	definitionRecord, ok := couponHistoricalDefinition(definition, marker)
	if !ok {
		t.Fatal("definition fixture invalid")
	}
	definitionDigest := couponapp.HistoricalDefinitionTargetDigest(definitionRecord)
	if !couponDefinitionMatches(definition, marker, definitionDigest[:]) {
		t.Fatal("exact definition rejected")
	}
	definition.Name += " changed"
	if couponDefinitionMatches(definition, marker, definitionDigest[:]) {
		t.Fatal("definition field drift accepted")
	}

	claim := couponReconcileClaim()
	claimRecord, ok := couponHistoricalClaim(claim)
	if !ok {
		t.Fatal("claim fixture invalid")
	}
	claimDigest := couponapp.HistoricalClaimTargetDigest(claimRecord)
	if !couponClaimMatches(claim, claimDigest[:]) {
		t.Fatal("exact claim rejected")
	}
	claim.ClaimNo += " changed"
	if couponClaimMatches(claim, claimDigest[:]) {
		t.Fatal("claim field drift accepted")
	}

	redemption := couponReconcileRedemption()
	redemptionRecord, ok := couponHistoricalRedemption(redemption)
	if !ok {
		t.Fatal("redemption fixture invalid")
	}
	redemptionDigest := couponapp.HistoricalRedemptionTargetDigest(redemptionRecord)
	if !couponRedemptionMatches(redemption, redemptionDigest[:]) {
		t.Fatal("exact redemption rejected")
	}
	redemption.ReleaseReason += " changed"
	if couponRedemptionMatches(redemption, redemptionDigest[:]) {
		t.Fatal("redemption field drift accepted")
	}
}

func TestCouponReconcileBindingDigestCoversTuple(t *testing.T) {
	digest := HistoricalCouponBindingTargetDigest(9, 1, 41)
	if digest == [sha256.Size]byte{} || digest == HistoricalCouponBindingTargetDigest(9, 1, 42) ||
		digest == HistoricalCouponBindingTargetDigest(9, 2, 41) || digest == HistoricalCouponBindingTargetDigest(10, 1, 41) {
		t.Fatal("binding tuple digest is incomplete")
	}
}

func TestReconcileCouponOnlyAcceptsCouponVersion(t *testing.T) {
	if _, err := ReconcileCoupon(nil, nil, "v1-coupon-a2", "archive-run"); err == nil {
		t.Fatal("wrong version accepted")
	}
}

func couponReconcileDefinition() (coupondb.GetCouponRow, coupondb.GetHistoricalCouponMarkerRow) {
	at := time.Date(2026, 8, 28, 12, 0, 0, 123456000, time.UTC)
	refs, err := json.Marshal([]string{"standard_product:41"})
	if err != nil {
		panic(err)
	}
	return coupondb.GetCouponRow{ID: 17, Name: "历史优惠券", DiscountAmountTotal: 100, Currency: "CNY", Status: "archived", TotalIssueLimit: 10, PerUserIssueLimit: 1, IssuedCount: 0,
			ClaimStartsAt: reconcileCouponTimestamp(at), ClaimEndsAt: reconcileCouponTimestamp(at.Add(time.Hour)), ValidityMode: "relative_days", RelativeValidityDays: pgtype.Int4{Int32: 1, Valid: true},
			Instructions: "历史只读", CreatedBy: 1, UpdatedBy: 1, Version: 1, CreatedAt: reconcileCouponTimestamp(at), UpdatedAt: reconcileCouponTimestamp(at), TargetRefs: refs, HistoryOnly: true},
		coupondb.GetHistoricalCouponMarkerRow{CouponID: 17, SourceCouponID: 901, OriginalStatus: "stopped"}
}

func couponReconcileClaim() coupondb.CouponV1HistoryClaim {
	at := reconcileCouponTimestamp(time.Date(2026, 8, 28, 12, 0, 0, 123456000, time.UTC))
	return coupondb.CouponV1HistoryClaim{ID: 19, SourceClaimID: 902, SourceCouponID: 901, CouponID: 17, ClaimNo: "claim-902", Status: "expired", DiscountAmountTotal: 100, Currency: "CNY", ValidFrom: at, ValidUntil: reconcileCouponTimestamp(at.Time.Add(time.Hour)), ClaimedAt: at, CreatedAt: at, UpdatedAt: at}
}

func couponReconcileRedemption() coupondb.CouponV1HistoryRedemption {
	at := reconcileCouponTimestamp(time.Date(2026, 8, 28, 12, 0, 0, 123456000, time.UTC))
	return coupondb.CouponV1HistoryRedemption{ID: 23, SourceRedemptionID: 903, SourceClaimID: 902, SourceOrderID: 904, ClaimHistoryID: 19, OutTradeNo: "history-order-904", Status: "released", OriginalAmountTotal: 1000, DiscountAmountTotal: 100, PayableAmountTotal: 900, Currency: "CNY", ReservedUntil: reconcileCouponTimestamp(at.Time.Add(time.Hour)), ReleaseReason: "历史释放", ReservedAt: at, CreatedAt: at, UpdatedAt: at}
}

func reconcileCouponTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
