package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	hxcdb "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/store/generated"
)

var _ hxc.HXCMemberUsageHistoryStore = (*HXCHistoryStore)(nil)
var _ hxc.HXCMemberUsageHistoryReader = (*HXCHistoryReader)(nil)

func (s *HXCHistoryStore) CreateHistoricalHXCMemberUsage(ctx context.Context, value hxc.HistoricalHXCMemberUsage) (hxc.HistoricalHXCMemberUsage, error) {
	if value.ID != 0 || badHXCMemberUsage(value) {
		return hxc.HistoricalHXCMemberUsage{}, hxc.ErrHXCHistoryInvalid
	}
	queries, err := s.q(ctx)
	if err != nil {
		return hxc.HistoricalHXCMemberUsage{}, err
	}
	row, err := queries.CreateHistoricalHXCMemberUsage(ctx, hxcMemberUsageArg(value))
	if err != nil {
		return hxc.HistoricalHXCMemberUsage{}, dbErr(err)
	}
	return hxcMemberUsage(row)
}

func (s *HXCHistoryStore) GetHistoricalHXCMemberUsage(ctx context.Context, id int64) (hxc.HistoricalHXCMemberUsage, error) {
	if id < 1 {
		return hxc.HistoricalHXCMemberUsage{}, hxc.ErrHXCHistoryInvalid
	}
	queries, err := s.q(ctx)
	if err != nil {
		return hxc.HistoricalHXCMemberUsage{}, err
	}
	row, err := queries.GetHistoricalHXCMemberUsage(ctx, id)
	if err != nil {
		return hxc.HistoricalHXCMemberUsage{}, dbErr(err)
	}
	return hxcMemberUsage(row)
}

func (r *HXCHistoryReader) GetHistoricalHXCMemberUsage(ctx context.Context, id int64) (hxc.HistoricalHXCMemberUsage, error) {
	if id < 1 {
		return hxc.HistoricalHXCMemberUsage{}, hxc.ErrHXCHistoryInvalid
	}
	queries, err := r.q(ctx)
	if err != nil {
		return hxc.HistoricalHXCMemberUsage{}, err
	}
	row, err := queries.GetHistoricalHXCMemberUsage(ctx, id)
	if err != nil {
		return hxc.HistoricalHXCMemberUsage{}, dbErr(err)
	}
	return hxcMemberUsage(row)
}

func (r *HXCHistoryReader) ListHistoricalHXCMemberUsage(ctx context.Context, query hxc.HXCMemberUsageHistoryQuery) ([]hxc.HistoricalHXCMemberUsage, int64, error) {
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 {
		return nil, 0, hxc.ErrHXCHistoryInvalid
	}
	queries, err := r.q(ctx)
	if err != nil {
		return nil, 0, err
	}
	generation := pgtype.Int8{}
	if query.Generation != nil {
		generation = pgtype.Int8{Int64: *query.Generation, Valid: true}
	}
	total, err := queries.CountHistoricalHXCMemberUsage(ctx, generation)
	if err != nil {
		return nil, 0, dbErr(err)
	}
	rows, err := queries.ListHistoricalHXCMemberUsage(ctx, hxcdb.ListHistoricalHXCMemberUsageParams{Generation: generation, RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, dbErr(err)
	}
	values := make([]hxc.HistoricalHXCMemberUsage, 0, len(rows))
	for _, row := range rows {
		value, err := hxcMemberUsage(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func hxcMemberUsageArg(value hxc.HistoricalHXCMemberUsage) hxcdb.CreateHistoricalHXCMemberUsageParams {
	return hxcdb.CreateHistoricalHXCMemberUsageParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:],
		Generation: value.Generation, Unionid: value.UnionID, OwnerUserid: value.OwnerUserID, MobileHash: value.MobileHash,
		IsMember: value.IsMember, IsRegistered: value.IsRegistered, RegisteredAt: pts(value.RegisteredAt), HasRealUsage: value.HasRealUsage,
		FirstUsedAt: pts(value.FirstUsedAt), LastUsedAt: pts(value.LastUsedAt), MemberSince: pts(value.MemberSince), MembershipExpiresAt: pts(value.MembershipExpiresAt),
		MembershipTier: value.MembershipTier, MembershipStatus: value.MembershipStatus, MembershipSource: value.MembershipSource,
		RegistrationSource: value.RegistrationSource, UsageSource: value.UsageSource, UpdatedAt: pts(value.UpdatedAt),
		PayloadJson: string(value.PayloadJSON), ProjectedAt: ts(value.ProjectedAt),
	}
}

func hxcMemberUsage(row hxcdb.HxcV1MemberUsageHistory) (hxc.HistoricalHXCMemberUsage, error) {
	if row.ID < 1 || len(row.SourceKeyDigest) != 32 || len(row.SourcePayloadDigest) != 32 || len(row.SourceFieldDigest) != 32 {
		return hxc.HistoricalHXCMemberUsage{}, hxc.ErrHXCHistoryUnavailable
	}
	registeredAt, registeredOK := ptsv(row.RegisteredAt)
	firstUsedAt, firstUsedOK := ptsv(row.FirstUsedAt)
	lastUsedAt, lastUsedOK := ptsv(row.LastUsedAt)
	memberSince, memberSinceOK := ptsv(row.MemberSince)
	membershipExpiresAt, membershipExpiresOK := ptsv(row.MembershipExpiresAt)
	updatedAt, updatedOK := ptsv(row.UpdatedAt)
	projectedAt, projectedOK := tsv(row.ProjectedAt)
	if !registeredOK || !firstUsedOK || !lastUsedOK || !memberSinceOK || !membershipExpiresOK || !updatedOK || !projectedOK || !json.Valid([]byte(row.PayloadJson)) {
		return hxc.HistoricalHXCMemberUsage{}, hxc.ErrHXCHistoryUnavailable
	}
	value := hxc.HistoricalHXCMemberUsage{
		ID: row.ID, Generation: row.Generation, UnionID: row.Unionid, OwnerUserID: row.OwnerUserid, MobileHash: row.MobileHash,
		IsMember: row.IsMember, IsRegistered: row.IsRegistered, RegisteredAt: registeredAt, HasRealUsage: row.HasRealUsage,
		FirstUsedAt: firstUsedAt, LastUsedAt: lastUsedAt, MemberSince: memberSince, MembershipExpiresAt: membershipExpiresAt,
		MembershipTier: row.MembershipTier, MembershipStatus: row.MembershipStatus, MembershipSource: row.MembershipSource,
		RegistrationSource: row.RegistrationSource, UsageSource: row.UsageSource, UpdatedAt: updatedAt,
		PayloadJSON: append(json.RawMessage(nil), row.PayloadJson...), ProjectedAt: projectedAt,
	}
	copy(value.SourceKeyDigest[:], row.SourceKeyDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	copy(value.SourceFieldDigest[:], row.SourceFieldDigest)
	if badHXCMemberUsage(value) {
		return hxc.HistoricalHXCMemberUsage{}, hxc.ErrHXCHistoryUnavailable
	}
	return value, nil
}

func badHXCMemberUsage(value hxc.HistoricalHXCMemberUsage) bool {
	if value.ID == 0 {
		value.ID = 1
	}
	_, err := hxcapp.HistoricalHXCMemberUsageDigest(value)
	return err != nil
}
