package membergrid

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

var _ PublicShareSummaryStore = (*Repository)(nil)
var _ PublicShareMemberStore = (*Repository)(nil)

func (repository *Repository) SummarizePublicMembers(ctx context.Context, serviceProductID int64) ([]PublicShareBucket, error) {
	if repository == nil || repository.shareQueries == nil || ctx == nil || serviceProductID < 1 {
		return nil, ErrUnavailable
	}
	queries, err := repository.shareQueries(ctx)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	rows, err := queries.SummarizePublicServicePeriodMembers(ctx, serviceProductID)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	buckets := make([]PublicShareBucket, len(rows))
	for index, row := range rows {
		buckets[index] = PublicShareBucket{State: row.State, Count: row.MemberCount}
	}
	return buckets, nil
}

func (repository *Repository) QueryPublicMembers(ctx context.Context, query StoreQuery) ([]PublicShareMemberRecord, error) {
	if repository == nil || repository.shareQueries == nil || ctx == nil || query.ProductID < 1 || query.State != StateAll || query.Source != SourceAny ||
		query.Limit < 1 || query.Limit > MaximumLimit+1 || (query.After != nil && (!validMemberRef(query.After.MemberRef) || query.After.UpdatedAt.IsZero())) {
		return nil, ErrUnavailable
	}
	queries, err := repository.shareQueries(ctx)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	records := make([]PublicShareMemberRecord, 0, query.Limit)
	if query.After == nil {
		rows, queryErr := queries.ListPublicServicePeriodMembersFirstPage(ctx, productdb.ListPublicServicePeriodMembersFirstPageParams{
			ServiceProductID: query.ProductID,
			RowLimit:         int32(query.Limit),
		})
		if queryErr != nil {
			return nil, errors.Join(ErrUnavailable, queryErr)
		}
		for _, row := range rows {
			record, mapErr := mapPublicMember(row.MemberRef, row.State, row.Source, row.StartsAt, row.ExpiresAt, row.UpdatedAt, row.DisplayName)
			if mapErr != nil {
				return nil, mapErr
			}
			records = append(records, record)
		}
	} else {
		rows, queryErr := queries.ListPublicServicePeriodMembersAfter(ctx, productdb.ListPublicServicePeriodMembersAfterParams{
			ServiceProductID: query.ProductID,
			AfterUpdatedAt:   pgtype.Timestamptz{Time: query.After.UpdatedAt.UTC(), Valid: true},
			AfterMemberRef:   query.After.MemberRef,
			RowLimit:         int32(query.Limit),
		})
		if queryErr != nil {
			return nil, errors.Join(ErrUnavailable, queryErr)
		}
		for _, row := range rows {
			record, mapErr := mapPublicMember(row.MemberRef, row.State, row.Source, row.StartsAt, row.ExpiresAt, row.UpdatedAt, row.DisplayName)
			if mapErr != nil {
				return nil, mapErr
			}
			records = append(records, record)
		}
	}
	return records, nil
}

func mapPublicMember(memberRef, state, source string, startsAt, expiresAt, updatedAt pgtype.Timestamptz, displayName string) (PublicShareMemberRecord, error) {
	if !startsAt.Valid || !updatedAt.Valid {
		return PublicShareMemberRecord{}, ErrUnavailable
	}
	record := PublicShareMemberRecord{
		MemberRef: memberRef, State: StateFilter(state), Source: SourceFilter(source),
		StartsAt: startsAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC(), DisplayName: displayName,
	}
	if expiresAt.Valid {
		value := expiresAt.Time.UTC()
		record.ExpiresAt = &value
	}
	return record, nil
}
