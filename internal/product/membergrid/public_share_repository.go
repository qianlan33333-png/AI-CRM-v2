package membergrid

import (
	"context"
	"errors"
)

const summarizePublicMembersSQL = `SELECT m.state, COUNT(*)::bigint
FROM public.service_period_members AS m
WHERE m.service_product_id = $1
  AND m.state IN ('active', 'expired', 'removed')
GROUP BY m.state
ORDER BY m.state ASC`

var _ PublicShareSummaryStore = (*Repository)(nil)

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
