package membergrid

import (
	"context"
	"errors"
	"time"
)

const summarizePublicMembersSQL = `SELECT m.state, COUNT(*)::bigint
FROM public.service_period_members AS m
WHERE m.service_product_id = $1
  AND m.state IN ('active', 'expired', 'removed')
GROUP BY m.state
ORDER BY m.state ASC`

const publicMemberProjection = `SELECT
  m.member_ref,
  m.state,
  m.source,
  m.starts_at,
  COALESCE(m.expires_at, TIMESTAMPTZ 'epoch') AS expires_at_value,
  (m.expires_at IS NOT NULL) AS has_expires_at,
  m.updated_at,
  c.name AS display_name
FROM public.service_period_members AS m
JOIN public.customers AS c ON c.id = m.customer_id`

const firstPublicMembersPageSQL = publicMemberProjection + `
WHERE m.service_product_id = $1
ORDER BY m.updated_at DESC, m.member_ref DESC
LIMIT $2`

const afterPublicMembersPageSQL = publicMemberProjection + `
WHERE m.service_product_id = $1
  AND (m.updated_at, m.member_ref) < ($2::timestamptz, $3::text)
ORDER BY m.updated_at DESC, m.member_ref DESC
LIMIT $4`

var _ PublicShareSummaryStore = (*Repository)(nil)
var _ PublicShareMemberStore = (*Repository)(nil)

func (repository *Repository) SummarizePublicMembers(ctx context.Context, serviceProductID int64) ([]PublicShareBucket, error) {
	if repository == nil || repository.executor == nil || ctx == nil || serviceProductID < 1 {
		return nil, ErrUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	rows, err := executor.Query(ctx, summarizePublicMembersSQL, serviceProductID)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	defer rows.Close()
	buckets := make([]PublicShareBucket, 0, 3)
	for rows.Next() {
		var bucket PublicShareBucket
		if err = rows.Scan(&bucket.State, &bucket.Count); err != nil {
			return nil, errors.Join(ErrUnavailable, err)
		}
		buckets = append(buckets, bucket)
	}
	if err = rows.Err(); err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	return buckets, nil
}

func (repository *Repository) QueryPublicMembers(ctx context.Context, query StoreQuery) ([]PublicShareMemberRecord, error) {
	if repository == nil || repository.executor == nil || ctx == nil || query.ProductID < 1 || query.State != StateAll || query.Source != SourceAny ||
		query.Limit < 1 || query.Limit > MaximumLimit+1 || (query.After != nil && (!validMemberRef(query.After.MemberRef) || query.After.UpdatedAt.IsZero())) {
		return nil, ErrUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	var rows sqlRows
	if query.After == nil {
		rows, err = executor.Query(ctx, firstPublicMembersPageSQL, query.ProductID, query.Limit)
	} else {
		rows, err = executor.Query(ctx, afterPublicMembersPageSQL, query.ProductID, query.After.UpdatedAt.UTC(), query.After.MemberRef, query.Limit)
	}
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	defer rows.Close()

	records := make([]PublicShareMemberRecord, 0, query.Limit)
	for rows.Next() {
		var record PublicShareMemberRecord
		var state, source string
		var expiresValue time.Time
		var hasExpires bool
		if err = rows.Scan(&record.MemberRef, &state, &source, &record.StartsAt, &expiresValue, &hasExpires, &record.UpdatedAt, &record.DisplayName); err != nil {
			return nil, errors.Join(ErrUnavailable, err)
		}
		record.State = StateFilter(state)
		record.Source = SourceFilter(source)
		record.StartsAt = record.StartsAt.UTC()
		record.UpdatedAt = record.UpdatedAt.UTC()
		if hasExpires {
			value := expiresValue.UTC()
			record.ExpiresAt = &value
		}
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	return records, nil
}
