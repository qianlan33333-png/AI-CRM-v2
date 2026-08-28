package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	couponapp "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/app"
	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	coupondb "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/store/generated"
)

var couponReconciledTables = []string{
	couponDefinitionsTableID,
	couponBindingsTableID,
	couponClaimsTableID,
	couponRedemptionsTableID,
}

// ReconcileCoupon seals only the four immutable coupon-history source tables.
func ReconcileCoupon(ctx context.Context, pool *pgxpool.Pool, importVersion, archiveRunID string) (ReconciliationResult, error) {
	if importVersion != couponImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, importVersion, archiveRunID, couponReconciledTables)
}

func verifyCouponTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, targets map[string]map[string]struct{}) (string, error) {
	if ctx == nil || tx == nil || row.TargetTable == nil || row.TargetID == nil || row.TargetDomain == nil ||
		*row.TargetDomain != "coupon" || len(row.TargetDigest) != sha256.Size {
		return "", ErrConflict
	}
	queries := coupondb.New(tx)
	switch *row.TargetTable {
	case "coupons":
		id, err := couponDefinitionTargetID(row)
		if err != nil {
			return "", err
		}
		coupon, err := queries.GetCoupon(ctx, id)
		if err != nil {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, err)
		}
		marker, err := queries.GetHistoricalCouponMarker(ctx, id)
		if err != nil || !couponDefinitionMatches(coupon, marker, row.TargetDigest) {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, err)
		}
		return "coupons:" + *row.TargetID + ":archived:history_only:" + hex.EncodeToString(row.TargetDigest), nil
	case "coupon_targets":
		couponID, position, err := couponBindingTargetID(row)
		if err != nil {
			return "", err
		}
		target, err := queries.GetHistoricalCouponTarget(ctx, coupondb.GetHistoricalCouponTargetParams{CouponID: couponID, Position: position})
		digest := HistoricalCouponBindingTargetDigest(couponID, position, target.ProductID)
		if err != nil || target.CouponID != couponID || target.Position != position || target.ProductID < 1 ||
			target.TargetRef != "standard_product:"+strconv.FormatInt(target.ProductID, 10) ||
			!containsTarget(targets, "coupons", strconv.FormatInt(couponID, 10)) ||
			!equalBytes(digest[:], row.TargetDigest) {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, err)
		}
		return "coupon_targets:" + *row.TargetID + ":historical_binding:" + hex.EncodeToString(row.TargetDigest), nil
	case "coupon_v1_history_claims":
		id, err := couponClaimTargetID(row)
		if err != nil {
			return "", err
		}
		claim, err := queries.GetHistoricalCouponClaim(ctx, id)
		if err != nil {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, err)
		}
		parent, err := queries.GetHistoricalCouponMarker(ctx, claim.CouponID)
		if err != nil || !containsTarget(targets, "coupons", strconv.FormatInt(claim.CouponID, 10)) ||
			claim.SourceCouponID != parent.SourceCouponID || !couponClaimMatches(claim, row.TargetDigest) {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, err)
		}
		return "coupon_v1_history_claims:" + *row.TargetID + ":history:" + hex.EncodeToString(row.TargetDigest), nil
	case "coupon_v1_history_redemptions":
		id, err := couponRedemptionTargetID(row)
		if err != nil {
			return "", err
		}
		redemption, err := queries.GetHistoricalCouponRedemption(ctx, id)
		if err != nil {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, err)
		}
		claim, err := queries.GetHistoricalCouponClaim(ctx, redemption.ClaimHistoryID)
		if err != nil {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, err)
		}
		parent, err := queries.GetHistoricalCouponMarker(ctx, claim.CouponID)
		if err != nil || !containsTarget(targets, "coupon_v1_history_claims", strconv.FormatInt(redemption.ClaimHistoryID, 10)) ||
			!containsTarget(targets, "coupons", strconv.FormatInt(claim.CouponID, 10)) ||
			redemption.SourceClaimID != claim.SourceClaimID || claim.SourceCouponID != parent.SourceCouponID ||
			!couponRedemptionMatches(redemption, row.TargetDigest) {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, err)
		}
		return "coupon_v1_history_redemptions:" + *row.TargetID + ":history:" + hex.EncodeToString(row.TargetDigest), nil
	default:
		return "", ErrConflict
	}
}

func couponDefinitionTargetID(row reconciliationRow) (int64, error) {
	return couponNumericTargetID(row, couponDefinitionsTableID, "coupons")
}

func couponClaimTargetID(row reconciliationRow) (int64, error) {
	return couponNumericTargetID(row, couponClaimsTableID, "coupon_v1_history_claims")
}

func couponRedemptionTargetID(row reconciliationRow) (int64, error) {
	return couponNumericTargetID(row, couponRedemptionsTableID, "coupon_v1_history_redemptions")
}

func couponNumericTargetID(row reconciliationRow, sourceTable, targetTable string) (int64, error) {
	if row.TableID != sourceTable || row.TargetDomain == nil || *row.TargetDomain != "coupon" ||
		row.TargetTable == nil || *row.TargetTable != targetTable || row.TargetID == nil || len(row.TargetDigest) != sha256.Size {
		return 0, ErrConflict
	}
	return couponCanonicalPositiveID(*row.TargetID)
}

func couponBindingTargetID(row reconciliationRow) (int64, int32, error) {
	if row.TableID != couponBindingsTableID || row.TargetDomain == nil || *row.TargetDomain != "coupon" ||
		row.TargetTable == nil || *row.TargetTable != "coupon_targets" || row.TargetID == nil || len(row.TargetDigest) != sha256.Size {
		return 0, 0, ErrConflict
	}
	couponText, positionText, found := strings.Cut(*row.TargetID, ":")
	if !found || strings.Contains(positionText, ":") {
		return 0, 0, ErrConflict
	}
	couponID, err := couponCanonicalPositiveID(couponText)
	if err != nil {
		return 0, 0, err
	}
	position64, err := strconv.ParseInt(positionText, 10, 32)
	if err != nil || position64 < 0 || strconv.FormatInt(position64, 10) != positionText ||
		HistoricalCouponBindingTargetID(couponID, int32(position64)) != *row.TargetID {
		return 0, 0, ErrConflict
	}
	return couponID, int32(position64), nil
}

func couponCanonicalPositiveID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != value {
		return 0, ErrConflict
	}
	return id, nil
}

func couponDefinitionMatches(row coupondb.GetCouponRow, marker coupondb.GetHistoricalCouponMarkerRow, expected []byte) bool {
	definition, ok := couponHistoricalDefinition(row, marker)
	if !ok || int64(definition.ID) != row.ID || marker.CouponID != row.ID || marker.SourceCouponID < 1 ||
		row.Status != "archived" || !row.HistoryOnly || row.Currency != "CNY" {
		return false
	}
	digest := couponapp.HistoricalDefinitionTargetDigest(definition)
	return equalBytes(digest[:], expected)
}

func couponClaimMatches(row coupondb.CouponV1HistoryClaim, expected []byte) bool {
	claim, ok := couponHistoricalClaim(row)
	if !ok || claim.ID != row.ID || claim.SourceClaimID < 1 || claim.SourceCouponID < 1 || claim.CouponID < 1 || claim.Currency != "CNY" {
		return false
	}
	digest := couponapp.HistoricalClaimTargetDigest(claim)
	return equalBytes(digest[:], expected)
}

func couponRedemptionMatches(row coupondb.CouponV1HistoryRedemption, expected []byte) bool {
	redemption, ok := couponHistoricalRedemption(row)
	if !ok || redemption.ID != row.ID || redemption.SourceRedemptionID < 1 || redemption.SourceClaimID < 1 ||
		redemption.SourceOrderID < 1 || redemption.ClaimHistoryID < 1 || redemption.Currency != "CNY" {
		return false
	}
	digest := couponapp.HistoricalRedemptionTargetDigest(redemption)
	return equalBytes(digest[:], expected)
}

func couponHistoricalDefinition(row coupondb.GetCouponRow, marker coupondb.GetHistoricalCouponMarkerRow) (couponport.HistoricalDefinition, bool) {
	claimStart, startOK := couponReconcileTime(row.ClaimStartsAt)
	claimEnd, endOK := couponReconcileTime(row.ClaimEndsAt)
	created, createdOK := couponReconcileTime(row.CreatedAt)
	updated, updatedOK := couponReconcileTime(row.UpdatedAt)
	firstClaim := couponReconcileTimePointer(marker.FirstClaimAt)
	rowFirstClaim := couponReconcileTimePointer(row.FirstClaimAt)
	var refs []string
	if !startOK || !endOK || !createdOK || !updatedOK || !couponTimesEqual(firstClaim, rowFirstClaim) || json.Unmarshal(row.TargetRefs, &refs) != nil {
		return couponport.HistoricalDefinition{}, false
	}
	return couponport.HistoricalDefinition{Coupon: couponport.Coupon{ID: couponport.ID(row.ID), Name: row.Name,
		DiscountAmountTotal: row.DiscountAmountTotal, Currency: row.Currency, Status: row.Status, AvailabilityStatus: "archived",
		TotalIssueLimit: row.TotalIssueLimit, PerUserIssueLimit: row.PerUserIssueLimit, IssuedCount: row.IssuedCount,
		ClaimStartsAt: claimStart, ClaimEndsAt: claimEnd, ValidityMode: couponport.ValidityMode(row.ValidityMode),
		UseStartsAt: couponReconcileTimePointer(row.UseStartsAt), UseEndsAt: couponReconcileTimePointer(row.UseEndsAt),
		RelativeValidityDays: couponReconcileIntPointer(row.RelativeValidityDays), Instructions: row.Instructions, TargetRefs: refs,
		CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy, Version: row.Version, HistoryOnly: row.HistoryOnly, CreatedAt: created, UpdatedAt: updated},
		SourceCouponID: marker.SourceCouponID, OriginalStatus: marker.OriginalStatus, FirstClaimAt: firstClaim}, true
}

func couponHistoricalClaim(row coupondb.CouponV1HistoryClaim) (couponport.HistoricalClaim, bool) {
	validFrom, validFromOK := couponReconcileTime(row.ValidFrom)
	validUntil, validUntilOK := couponReconcileTime(row.ValidUntil)
	claimed, claimedOK := couponReconcileTime(row.ClaimedAt)
	created, createdOK := couponReconcileTime(row.CreatedAt)
	updated, updatedOK := couponReconcileTime(row.UpdatedAt)
	if !validFromOK || !validUntilOK || !claimedOK || !createdOK || !updatedOK {
		return couponport.HistoricalClaim{}, false
	}
	return couponport.HistoricalClaim{ID: row.ID, SourceClaimID: row.SourceClaimID, SourceCouponID: row.SourceCouponID, CouponID: row.CouponID,
		CustomerID: couponReconcileIDPointer(row.CustomerID), ClaimNo: row.ClaimNo, Status: row.Status, DiscountAmountTotal: row.DiscountAmountTotal,
		Currency: row.Currency, ValidFrom: validFrom, ValidUntil: validUntil, ClaimedAt: claimed,
		ReservedAt: couponReconcileTimePointer(row.ReservedAt), ConsumedAt: couponReconcileTimePointer(row.ConsumedAt), ExpiredAt: couponReconcileTimePointer(row.ExpiredAt),
		CreatedAt: created, UpdatedAt: updated}, true
}

func couponHistoricalRedemption(row coupondb.CouponV1HistoryRedemption) (couponport.HistoricalRedemption, bool) {
	reservedUntil, reservedUntilOK := couponReconcileTime(row.ReservedUntil)
	reservedAt, reservedAtOK := couponReconcileTime(row.ReservedAt)
	created, createdOK := couponReconcileTime(row.CreatedAt)
	updated, updatedOK := couponReconcileTime(row.UpdatedAt)
	if !reservedUntilOK || !reservedAtOK || !createdOK || !updatedOK {
		return couponport.HistoricalRedemption{}, false
	}
	return couponport.HistoricalRedemption{ID: row.ID, SourceRedemptionID: row.SourceRedemptionID, SourceClaimID: row.SourceClaimID,
		SourceOrderID: row.SourceOrderID, ClaimHistoryID: row.ClaimHistoryID, OrderID: couponReconcileIDPointer(row.OrderID),
		OutTradeNo: row.OutTradeNo, Status: row.Status, OriginalAmountTotal: row.OriginalAmountTotal,
		DiscountAmountTotal: row.DiscountAmountTotal, PayableAmountTotal: row.PayableAmountTotal, Currency: row.Currency,
		ReservedUntil: reservedUntil, ReleaseReason: row.ReleaseReason, ReservedAt: reservedAt,
		ConsumedAt: couponReconcileTimePointer(row.ConsumedAt), ReleasedAt: couponReconcileTimePointer(row.ReleasedAt), CreatedAt: created, UpdatedAt: updated}, true
}

func couponReconcileTime(value pgtype.Timestamptz) (time.Time, bool) {
	if !value.Valid || value.Time.IsZero() {
		return time.Time{}, false
	}
	return value.Time.UTC().Truncate(time.Microsecond), true
}

func couponReconcileTimePointer(value pgtype.Timestamptz) *time.Time {
	stamp, valid := couponReconcileTime(value)
	if !valid {
		return nil
	}
	return &stamp
}

func couponReconcileIntPointer(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	result := value.Int32
	return &result
}

func couponReconcileIDPointer(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func couponTimesEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
