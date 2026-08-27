package store

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	coupondb "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/store/generated"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var _ couponport.HistoricalStore = (*Repository)(nil)

func (r *Repository) CreateHistoricalDefinition(ctx context.Context, v couponport.HistoricalDefinition) (couponport.HistoricalDefinition, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return couponport.HistoricalDefinition{}, unavailable(err)
	}
	id, err := q.CreateHistoricalCoupon(ctx, coupondb.CreateHistoricalCouponParams{Name: v.Name, DiscountAmountTotal: v.DiscountAmountTotal, TotalIssueLimit: v.TotalIssueLimit, PerUserIssueLimit: v.PerUserIssueLimit, ClaimStartsAt: stamp(v.ClaimStartsAt), ClaimEndsAt: stamp(v.ClaimEndsAt), ValidityMode: string(v.ValidityMode), UseStartsAt: optionalTime(v.UseStartsAt), UseEndsAt: optionalTime(v.UseEndsAt), RelativeValidityDays: optionalInt(v.RelativeValidityDays), Instructions: v.Instructions, Actor: v.CreatedBy, CreatedAt: stamp(v.CreatedAt), UpdatedAt: stamp(v.UpdatedAt)})
	if err != nil {
		return couponport.HistoricalDefinition{}, unavailable(err)
	}
	for position, ref := range v.TargetRefs {
		productID, parseErr := strconv.ParseInt(strings.TrimPrefix(ref, "standard_product:"), 10, 64)
		if parseErr != nil || productID < 1 || ref != "standard_product:"+strconv.FormatInt(productID, 10) {
			return couponport.HistoricalDefinition{}, couponport.ErrHistoryInvalid
		}
		if err = q.InsertCouponTarget(ctx, coupondb.InsertCouponTargetParams{CouponID: id, Position: int32(position), TargetRef: ref, ProductID: productID}); err != nil {
			return couponport.HistoricalDefinition{}, unavailable(err)
		}
	}
	// Existing triggers forbid inserting targets after a coupon has issued claims.
	n, err := q.RestoreHistoricalCouponClaimFacts(ctx, coupondb.RestoreHistoricalCouponClaimFactsParams{CouponID: id, IssuedCount: v.IssuedCount, FirstClaimAt: optionalTime(v.FirstClaimAt)})
	if err != nil || n != 1 {
		return couponport.HistoricalDefinition{}, unavailable(err)
	}
	if err = q.CreateHistoricalCouponMarker(ctx, coupondb.CreateHistoricalCouponMarkerParams{CouponID: id, SourceCouponID: v.SourceCouponID, OriginalStatus: v.OriginalStatus}); err != nil {
		return couponport.HistoricalDefinition{}, unavailable(err)
	}
	if n, err = q.IncrementCouponCount(ctx); err != nil || n < 1 {
		return couponport.HistoricalDefinition{}, unavailable(err)
	}
	return r.GetHistoricalDefinition(ctx, id)
}

func (r *Repository) GetHistoricalDefinition(ctx context.Context, id int64) (couponport.HistoricalDefinition, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return couponport.HistoricalDefinition{}, unavailable(err)
	}
	marker, err := q.GetHistoricalCouponMarker(ctx, id)
	if err != nil {
		return couponport.HistoricalDefinition{}, unavailable(err)
	}
	c, err := r.Get(ctx, couponport.ID(id))
	if err != nil {
		return couponport.HistoricalDefinition{}, err
	}
	c.AvailabilityStatus = "archived"
	return couponport.HistoricalDefinition{Coupon: c, SourceCouponID: marker.SourceCouponID, OriginalStatus: marker.OriginalStatus, FirstClaimAt: timePtr(marker.FirstClaimAt)}, nil
}

func (r *Repository) CreateHistoricalClaim(ctx context.Context, v couponport.HistoricalClaim) (couponport.HistoricalClaim, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return couponport.HistoricalClaim{}, unavailable(err)
	}
	x, err := q.CreateHistoricalCouponClaim(ctx, coupondb.CreateHistoricalCouponClaimParams{SourceClaimID: v.SourceClaimID, SourceCouponID: v.SourceCouponID, CouponID: v.CouponID, CustomerID: historyInt(v.CustomerID), ClaimNo: v.ClaimNo, Status: v.Status, DiscountAmountTotal: v.DiscountAmountTotal, Currency: v.Currency, ValidFrom: stamp(v.ValidFrom), ValidUntil: stamp(v.ValidUntil), ClaimedAt: stamp(v.ClaimedAt), ReservedAt: optionalTime(v.ReservedAt), ConsumedAt: optionalTime(v.ConsumedAt), ExpiredAt: optionalTime(v.ExpiredAt), CreatedAt: stamp(v.CreatedAt), UpdatedAt: stamp(v.UpdatedAt)})
	if err != nil {
		return couponport.HistoricalClaim{}, unavailable(err)
	}
	return mapHistoricalClaim(x), nil
}
func (r *Repository) GetHistoricalClaim(ctx context.Context, id int64) (couponport.HistoricalClaim, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return couponport.HistoricalClaim{}, unavailable(err)
	}
	x, err := q.GetHistoricalCouponClaim(ctx, id)
	if err != nil {
		return couponport.HistoricalClaim{}, unavailable(err)
	}
	return mapHistoricalClaim(x), nil
}
func (r *Repository) CreateHistoricalRedemption(ctx context.Context, v couponport.HistoricalRedemption) (couponport.HistoricalRedemption, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return couponport.HistoricalRedemption{}, unavailable(err)
	}
	x, err := q.CreateHistoricalCouponRedemption(ctx, coupondb.CreateHistoricalCouponRedemptionParams{SourceRedemptionID: v.SourceRedemptionID, SourceClaimID: v.SourceClaimID, SourceOrderID: v.SourceOrderID, ClaimHistoryID: v.ClaimHistoryID, OrderID: historyInt(v.OrderID), OutTradeNo: v.OutTradeNo, Status: v.Status, OriginalAmountTotal: v.OriginalAmountTotal, DiscountAmountTotal: v.DiscountAmountTotal, PayableAmountTotal: v.PayableAmountTotal, Currency: v.Currency, ReservedUntil: stamp(v.ReservedUntil), ReleaseReason: v.ReleaseReason, ReservedAt: stamp(v.ReservedAt), ConsumedAt: optionalTime(v.ConsumedAt), ReleasedAt: optionalTime(v.ReleasedAt), CreatedAt: stamp(v.CreatedAt), UpdatedAt: stamp(v.UpdatedAt)})
	if err != nil {
		return couponport.HistoricalRedemption{}, unavailable(err)
	}
	return mapHistoricalRedemption(x), nil
}
func (r *Repository) GetHistoricalRedemption(ctx context.Context, id int64) (couponport.HistoricalRedemption, error) {
	q, err := queries(ctx)
	if r == nil || err != nil {
		return couponport.HistoricalRedemption{}, unavailable(err)
	}
	x, err := q.GetHistoricalCouponRedemption(ctx, id)
	if err != nil {
		return couponport.HistoricalRedemption{}, unavailable(err)
	}
	return mapHistoricalRedemption(x), nil
}

func mapHistoricalClaim(x coupondb.CouponV1HistoryClaim) couponport.HistoricalClaim {
	return couponport.HistoricalClaim{ID: x.ID, SourceClaimID: x.SourceClaimID, SourceCouponID: x.SourceCouponID, CouponID: x.CouponID, CustomerID: historyIntPtr(x.CustomerID), ClaimNo: x.ClaimNo, Status: x.Status, DiscountAmountTotal: x.DiscountAmountTotal, Currency: x.Currency, ValidFrom: x.ValidFrom.Time, ValidUntil: x.ValidUntil.Time, ClaimedAt: x.ClaimedAt.Time, ReservedAt: timePtr(x.ReservedAt), ConsumedAt: timePtr(x.ConsumedAt), ExpiredAt: timePtr(x.ExpiredAt), CreatedAt: x.CreatedAt.Time, UpdatedAt: x.UpdatedAt.Time}
}
func mapHistoricalRedemption(x coupondb.CouponV1HistoryRedemption) couponport.HistoricalRedemption {
	return couponport.HistoricalRedemption{ID: x.ID, SourceRedemptionID: x.SourceRedemptionID, SourceClaimID: x.SourceClaimID, SourceOrderID: x.SourceOrderID, ClaimHistoryID: x.ClaimHistoryID, OrderID: historyIntPtr(x.OrderID), OutTradeNo: x.OutTradeNo, Status: x.Status, OriginalAmountTotal: x.OriginalAmountTotal, DiscountAmountTotal: x.DiscountAmountTotal, PayableAmountTotal: x.PayableAmountTotal, Currency: x.Currency, ReservedUntil: x.ReservedUntil.Time, ReleaseReason: x.ReleaseReason, ReservedAt: x.ReservedAt.Time, ConsumedAt: timePtr(x.ConsumedAt), ReleasedAt: timePtr(x.ReleasedAt), CreatedAt: x.CreatedAt.Time, UpdatedAt: x.UpdatedAt.Time}
}
func historyInt(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
func historyIntPtr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}

type HistoricalReader struct {
	uow        platformport.UnitOfWork
	repository *Repository
}

var _ couponport.HistoricalReader = (*HistoricalReader)(nil)

func NewHistoricalReader(uow platformport.UnitOfWork) *HistoricalReader {
	return &HistoricalReader{uow: uow, repository: NewRepository()}
}
func (r *HistoricalReader) validPage(limit, offset int32) bool {
	return r != nil && r.uow != nil && limit >= 1 && limit <= 100 && offset >= 0
}

func (r *HistoricalReader) ListHistoricalDefinitions(ctx context.Context, limit, offset int32) ([]couponport.HistoricalDefinition, int64, error) {
	items := []couponport.HistoricalDefinition{}
	var total int64
	if !r.validPage(limit, offset) {
		return nil, 0, couponport.ErrHistoryInvalid
	}
	err := r.uow.Within(ctx, func(tx context.Context) error {
		q, err := queries(tx)
		if err != nil {
			return err
		}
		ids, err := q.ListHistoricalCouponIDs(tx, coupondb.ListHistoricalCouponIDsParams{RowLimit: limit, RowOffset: offset})
		if err != nil {
			return err
		}
		for _, id := range ids {
			item, e := r.repository.GetHistoricalDefinition(tx, id)
			if e != nil {
				return e
			}
			items = append(items, item)
		}
		total, err = q.CountHistoricalCoupons(tx)
		return err
	})
	return items, total, err
}
func (r *HistoricalReader) ListHistoricalClaims(ctx context.Context, id int64, limit, offset int32) ([]couponport.HistoricalClaim, int64, error) {
	items := []couponport.HistoricalClaim{}
	var total int64
	if !r.validPage(limit, offset) || id < 1 {
		return nil, 0, couponport.ErrHistoryInvalid
	}
	err := r.uow.Within(ctx, func(tx context.Context) error {
		q, err := queries(tx)
		if err != nil {
			return err
		}
		if _, err = r.repository.GetHistoricalDefinition(tx, id); err != nil {
			return err
		}
		rows, err := q.ListHistoricalCouponClaims(tx, coupondb.ListHistoricalCouponClaimsParams{CouponID: id, RowLimit: limit, RowOffset: offset})
		if err != nil {
			return err
		}
		for _, x := range rows {
			items = append(items, mapHistoricalClaim(x))
		}
		total, err = q.CountHistoricalCouponClaims(tx, id)
		return err
	})
	return items, total, err
}
func (r *HistoricalReader) ListHistoricalRedemptions(ctx context.Context, id int64, limit, offset int32) ([]couponport.HistoricalRedemption, int64, error) {
	items := []couponport.HistoricalRedemption{}
	var total int64
	if !r.validPage(limit, offset) || id < 1 {
		return nil, 0, couponport.ErrHistoryInvalid
	}
	err := r.uow.Within(ctx, func(tx context.Context) error {
		q, err := queries(tx)
		if err != nil {
			return err
		}
		if _, err = r.repository.GetHistoricalDefinition(tx, id); err != nil {
			return err
		}
		rows, err := q.ListHistoricalCouponRedemptions(tx, coupondb.ListHistoricalCouponRedemptionsParams{CouponID: id, RowLimit: limit, RowOffset: offset})
		if err != nil {
			return err
		}
		for _, x := range rows {
			items = append(items, mapHistoricalRedemption(x))
		}
		total, err = q.CountHistoricalCouponRedemptions(tx, id)
		return err
	})
	return items, total, err
}
