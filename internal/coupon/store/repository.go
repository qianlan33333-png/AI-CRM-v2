package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	couponapp "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/app"
	couponport "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/port"
	coupondb "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

var _ couponapp.Store = (*Repository)(nil)
var _ couponapp.BoardStore = (*Repository)(nil)

func queries(ctx context.Context) (*coupondb.Queries, error) {
	tx, e := platformstore.TxFromContext(ctx)
	if e != nil {
		return nil, e
	}
	return coupondb.New(tx), nil
}
func (r *Repository) List(ctx context.Context, limit, offset int32, search, status string) ([]couponport.Coupon, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return nil, unavailable(e)
	}
	rows, e := q.ListCoupons(ctx, coupondb.ListCouponsParams{Search: search, StatusFilter: status, RowLimit: limit, RowOffset: offset})
	if e != nil {
		return nil, unavailable(e)
	}
	out := make([]couponport.Coupon, len(rows))
	for i, row := range rows {
		out[i], e = mapCoupon(row.ID, row.Name, row.DiscountAmountTotal, row.Currency, row.Status, row.TotalIssueLimit, row.PerUserIssueLimit, row.IssuedCount, row.ClaimStartsAt, row.ClaimEndsAt, row.ValidityMode, row.UseStartsAt, row.UseEndsAt, row.RelativeValidityDays, row.Instructions, row.CreatedBy, row.UpdatedBy, row.Version, row.CreatedAt, row.UpdatedAt, row.TargetRefs)
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (r *Repository) Count(ctx context.Context, search, status string) (int64, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return 0, unavailable(e)
	}
	n, e := q.CountCoupons(ctx, coupondb.CountCouponsParams{Search: search, StatusFilter: status})
	if e != nil || n < 0 {
		return 0, unavailable(e)
	}
	return n, nil
}
func (r *Repository) Get(ctx context.Context, id couponport.ID) (couponport.Coupon, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return couponport.Coupon{}, unavailable(e)
	}
	row, e := q.GetCoupon(ctx, int64(id))
	if e != nil {
		return couponport.Coupon{}, unavailable(e)
	}
	return mapCoupon(row.ID, row.Name, row.DiscountAmountTotal, row.Currency, row.Status, row.TotalIssueLimit, row.PerUserIssueLimit, row.IssuedCount, row.ClaimStartsAt, row.ClaimEndsAt, row.ValidityMode, row.UseStartsAt, row.UseEndsAt, row.RelativeValidityDays, row.Instructions, row.CreatedBy, row.UpdatedBy, row.Version, row.CreatedAt, row.UpdatedAt, row.TargetRefs)
}
func (r *Repository) Lock(ctx context.Context, id couponport.ID) (couponport.Coupon, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return couponport.Coupon{}, unavailable(e)
	}
	if _, e = q.LockCoupon(ctx, int64(id)); e != nil {
		return couponport.Coupon{}, unavailable(e)
	}
	return r.Get(ctx, id)
}
func (r *Repository) Create(ctx context.Context, c couponport.UpsertCommand, ids []int64, now time.Time) (couponport.Coupon, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return couponport.Coupon{}, unavailable(e)
	}
	id, e := q.CreateCoupon(ctx, createParams(c, now))
	if e != nil {
		return couponport.Coupon{}, unavailable(e)
	}
	if e = replaceTargets(ctx, q, id, c.TargetRefs, ids); e != nil {
		return couponport.Coupon{}, e
	}
	if n, countErr := q.IncrementCouponCount(ctx); countErr != nil || n < 1 {
		return couponport.Coupon{}, unavailable(countErr)
	}
	return r.Get(ctx, couponport.ID(id))
}
func (r *Repository) Update(ctx context.Context, c couponport.UpsertCommand, ids []int64, now time.Time) (couponport.Coupon, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return couponport.Coupon{}, unavailable(e)
	}
	p := updateParams(c, now)
	if e = q.UpdateCoupon(ctx, p); e != nil {
		return couponport.Coupon{}, unavailable(e)
	}
	current, currentErr := r.Get(ctx, c.ID)
	if currentErr != nil {
		return couponport.Coupon{}, currentErr
	}
	if !reflect.DeepEqual(current.TargetRefs, c.TargetRefs) {
		if e = replaceTargets(ctx, q, int64(c.ID), c.TargetRefs, ids); e != nil {
			return couponport.Coupon{}, e
		}
	}
	return r.Get(ctx, c.ID)
}
func (r *Repository) SetStatus(ctx context.Context, id couponport.ID, status string, actor int64, now time.Time) (couponport.Coupon, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return couponport.Coupon{}, unavailable(e)
	}
	if e = q.SetCouponStatus(ctx, coupondb.SetCouponStatusParams{Status: status, Actor: actor, Now: stamp(now), CouponID: int64(id)}); e != nil {
		return couponport.Coupon{}, unavailable(e)
	}
	return r.Get(ctx, id)
}
func (r *Repository) Reserve(ctx context.Context, x couponapp.Reservation) (couponapp.Receipt, bool, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return couponapp.Receipt{}, false, unavailable(e)
	}
	row, e := q.ReserveCouponReceipt(ctx, coupondb.ReserveCouponReceiptParams{Operation: x.Operation, ActorScope: x.ActorScope, KeyDigest: x.KeyDigest[:], PayloadDigest: x.PayloadDigest[:], CreatedAt: stamp(x.CreatedAt)})
	if e == nil {
		return receipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return couponapp.Receipt{}, false, unavailable(e)
	}
	old, e := q.GetCouponReceipt(ctx, coupondb.GetCouponReceiptParams{Operation: x.Operation, ActorScope: x.ActorScope, KeyDigest: x.KeyDigest[:]})
	if e != nil {
		return couponapp.Receipt{}, false, unavailable(e)
	}
	return receipt(old.ID, old.Operation, old.ActorScope, old.KeyDigest, old.PayloadDigest, old.State, old.ResultSnapshot), false, nil
}
func (r *Repository) Complete(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (couponapp.Receipt, error) {
	q, e := queries(ctx)
	if r == nil || e != nil || !json.Valid(snapshot) {
		return couponapp.Receipt{}, unavailable(e)
	}
	row, e := q.CompleteCouponReceipt(ctx, coupondb.CompleteCouponReceiptParams{ID: id, ResultSnapshot: snapshot, CompletedAt: stamp(now)})
	if e != nil {
		return couponapp.Receipt{}, unavailable(e)
	}
	return receipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), nil
}

func (r *Repository) DeleteDraft(ctx context.Context, id couponport.ID) error {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return unavailable(e)
	}
	if _, e = q.DeleteDraftCoupon(ctx, int64(id)); e != nil {
		return unavailable(e)
	}
	if _, e = q.DecrementCouponCount(ctx); e != nil {
		return unavailable(e)
	}
	return nil
}
func (r *Repository) ListClaims(ctx context.Context, id couponport.ID, limit, offset int32) ([]couponport.Claim, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return nil, unavailable(e)
	}
	rows, e := q.ListCouponClaims(ctx, coupondb.ListCouponClaimsParams{CouponID: int64(id), RowLimit: limit, RowOffset: offset})
	if e != nil {
		return nil, unavailable(e)
	}
	out := make([]couponport.Claim, len(rows))
	for i, x := range rows {
		if !x.ClaimedAt.Valid {
			return nil, couponapp.ErrUnavailable
		}
		out[i] = couponport.Claim{ID: x.ID, CouponID: x.CouponID, CustomerID: x.CustomerID, ClaimNumber: x.ClaimNumber, ClaimRef: x.ClaimRef, Status: x.Status, ClaimedAt: x.ClaimedAt.Time}
	}
	return out, nil
}
func (r *Repository) CountClaims(ctx context.Context, id couponport.ID) (int64, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return 0, unavailable(e)
	}
	n, e := q.CountCouponClaims(ctx, int64(id))
	if e != nil || n < 0 {
		return 0, unavailable(e)
	}
	return n, nil
}
func (r *Repository) CountCustomerClaims(ctx context.Context, id couponport.ID, customerID int64) (int64, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return 0, unavailable(e)
	}
	n, e := q.CountCustomerCouponClaims(ctx, coupondb.CountCustomerCouponClaimsParams{CouponID: int64(id), CustomerID: customerID})
	if e != nil || n < 0 {
		return 0, unavailable(e)
	}
	return n, nil
}
func (r *Repository) CreateClaim(ctx context.Context, id couponport.ID, customerID int64, number int32, ref string, now time.Time) (couponport.Claim, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return couponport.Claim{}, unavailable(e)
	}
	x, e := q.CreateCouponClaim(ctx, coupondb.CreateCouponClaimParams{CouponID: int64(id), CustomerID: customerID, ClaimNumber: number, ClaimRef: ref, ClaimedAt: stamp(now)})
	if e != nil || !x.ClaimedAt.Valid {
		return couponport.Claim{}, unavailable(e)
	}
	return couponport.Claim{ID: x.ID, CouponID: x.CouponID, CustomerID: x.CustomerID, ClaimNumber: x.ClaimNumber, ClaimRef: x.ClaimRef, Status: x.Status, ClaimedAt: x.ClaimedAt.Time}, nil
}
func (r *Repository) IncrementIssued(ctx context.Context, id couponport.ID, now time.Time) error {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return unavailable(e)
	}
	if _, e = q.IncrementCouponIssuedCount(ctx, coupondb.IncrementCouponIssuedCountParams{CouponID: int64(id), Now: stamp(now)}); e != nil {
		return unavailable(e)
	}
	return nil
}
func (r *Repository) ListAvailable(ctx context.Context, target string, customerID int64, now time.Time, limit int32) ([]couponport.Coupon, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return nil, unavailable(e)
	}
	rows, e := q.ListAvailableCoupons(ctx, coupondb.ListAvailableCouponsParams{TargetRef: target, CustomerID: customerID, Now: stamp(now), RowLimit: limit})
	if e != nil {
		return nil, unavailable(e)
	}
	out := make([]couponport.Coupon, len(rows))
	for i, x := range rows {
		out[i], e = mapCoupon(x.ID, x.Name, x.DiscountAmountTotal, x.Currency, x.Status, x.TotalIssueLimit, x.PerUserIssueLimit, x.IssuedCount, x.ClaimStartsAt, x.ClaimEndsAt, x.ValidityMode, x.UseStartsAt, x.UseEndsAt, x.RelativeValidityDays, x.Instructions, x.CreatedBy, x.UpdatedBy, x.Version, x.CreatedAt, x.UpdatedAt, x.TargetRefs)
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (r *Repository) ResolvePaymentIdentitySession(ctx context.Context, digest [32]byte, now time.Time) (int64, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return 0, unavailable(e)
	}
	id, e := q.ResolveCouponPaymentIdentitySession(ctx, coupondb.ResolveCouponPaymentIdentitySessionParams{TokenDigest: digest[:], Now: stamp(now)})
	if e != nil || id < 1 {
		return 0, unavailable(e)
	}
	return id, nil
}
func (r *Repository) ResolveSidebarGrant(ctx context.Context, digest [32]byte, now time.Time) (int64, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return 0, unavailable(e)
	}
	id, e := q.ResolveCouponSidebarGrant(ctx, coupondb.ResolveCouponSidebarGrantParams{TokenDigest: digest[:], Now: stamp(now)})
	if e != nil || id < 1 {
		return 0, unavailable(e)
	}
	return id, nil
}
func (r *Repository) ListSidebarClaims(ctx context.Context, customerID int64, limit int32) ([]couponport.SidebarCoupon, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return nil, unavailable(e)
	}
	rows, e := q.ListCouponSidebarClaims(ctx, coupondb.ListCouponSidebarClaimsParams{CustomerID: customerID, RowLimit: limit})
	if e != nil {
		return nil, unavailable(e)
	}
	out := make([]couponport.SidebarCoupon, len(rows))
	for i, row := range rows {
		if !row.ClaimedAt.Valid {
			return nil, couponapp.ErrUnavailable
		}
		out[i] = couponport.SidebarCoupon{CouponID: couponport.ID(row.CouponID), CouponName: row.CouponName, CouponStatus: row.CouponStatus, ClaimRef: row.ClaimRef, ClaimedAt: row.ClaimedAt.Time}
	}
	return out, nil
}

func replaceTargets(ctx context.Context, q *coupondb.Queries, id int64, refs []string, ids []int64) error {
	if len(refs) != len(ids) {
		return couponapp.ErrUnavailable
	}
	if e := q.DeleteCouponTargets(ctx, id); e != nil {
		return unavailable(e)
	}
	for i, ref := range refs {
		if e := q.InsertCouponTarget(ctx, coupondb.InsertCouponTargetParams{CouponID: id, Position: int32(i), TargetRef: ref, ProductID: ids[i]}); e != nil {
			return unavailable(e)
		}
	}
	return nil
}
func createParams(c couponport.UpsertCommand, now time.Time) coupondb.CreateCouponParams {
	return coupondb.CreateCouponParams{Name: c.Name, DiscountAmountTotal: c.DiscountAmountTotal, TotalIssueLimit: c.TotalIssueLimit, PerUserIssueLimit: c.PerUserIssueLimit, ClaimStartsAt: stamp(c.ClaimStartsAt), ClaimEndsAt: stamp(c.ClaimEndsAt), ValidityMode: string(c.ValidityMode), UseStartsAt: optionalTime(c.UseStartsAt), UseEndsAt: optionalTime(c.UseEndsAt), RelativeValidityDays: optionalInt(c.RelativeValidityDays), Instructions: c.Instructions, Actor: c.Actor, Now: stamp(now)}
}
func updateParams(c couponport.UpsertCommand, now time.Time) coupondb.UpdateCouponParams {
	return coupondb.UpdateCouponParams{Name: c.Name, DiscountAmountTotal: c.DiscountAmountTotal, TotalIssueLimit: c.TotalIssueLimit, PerUserIssueLimit: c.PerUserIssueLimit, ClaimStartsAt: stamp(c.ClaimStartsAt), ClaimEndsAt: stamp(c.ClaimEndsAt), ValidityMode: string(c.ValidityMode), UseStartsAt: optionalTime(c.UseStartsAt), UseEndsAt: optionalTime(c.UseEndsAt), RelativeValidityDays: optionalInt(c.RelativeValidityDays), Instructions: c.Instructions, Actor: c.Actor, Now: stamp(now), CouponID: int64(c.ID)}
}
func mapCoupon(id int64, name string, discount int64, currency, status string, total, perUser, issued int64, claimStart, claimEnd pgtype.Timestamptz, mode string, useStart, useEnd pgtype.Timestamptz, relative pgtype.Int4, instructions string, createdBy, updatedBy, version int64, created, updated pgtype.Timestamptz, raw []byte) (couponport.Coupon, error) {
	if !claimStart.Valid || !claimEnd.Valid || !created.Valid || !updated.Valid {
		return couponport.Coupon{}, couponapp.ErrUnavailable
	}
	var refs []string
	if json.Unmarshal(raw, &refs) != nil {
		return couponport.Coupon{}, couponapp.ErrUnavailable
	}
	return couponport.Coupon{ID: couponport.ID(id), Name: name, DiscountAmountTotal: discount, Currency: currency, Status: status, TotalIssueLimit: total, PerUserIssueLimit: perUser, IssuedCount: issued, ClaimStartsAt: claimStart.Time, ClaimEndsAt: claimEnd.Time, ValidityMode: couponport.ValidityMode(mode), UseStartsAt: timePtr(useStart), UseEndsAt: timePtr(useEnd), RelativeValidityDays: intPtr(relative), Instructions: instructions, TargetRefs: refs, CreatedBy: createdBy, UpdatedBy: updatedBy, Version: version, CreatedAt: created.Time, UpdatedAt: updated.Time}, nil
}
func receipt(id int64, op, actor string, key, payload []byte, state string, snapshot []byte) couponapp.Receipt {
	r := couponapp.Receipt{ID: id, Operation: op, ActorScope: actor, State: state, ResultSnapshot: append([]byte(nil), snapshot...)}
	copy(r.KeyDigest[:], key)
	copy(r.PayloadDigest[:], payload)
	return r
}
func stamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func optionalTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return stamp(*t)
}
func optionalInt(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}
func timePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	x := v.Time
	return &x
}
func intPtr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	x := v.Int32
	return &x
}
func unavailable(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return couponapp.ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(e, &p) {
		if p.Code == "23505" {
			return couponapp.ErrConflict
		}
		if p.Code == "55000" {
			return couponapp.ErrRulesFrozen
		}
	}
	if e != nil {
		return e
	}
	return couponapp.ErrUnavailable
}
