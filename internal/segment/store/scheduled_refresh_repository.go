package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentworker "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/worker"
)

// ScheduledRefreshRepository is a read-only worker projection. RefreshOnce
// remains the only Segment member-replacement writer.
type ScheduledRefreshRepository struct {
	pool *pgxpool.Pool
}

var _ segmentworker.ScheduledRefreshFinder = (*ScheduledRefreshRepository)(nil)

func NewScheduledRefreshRepository(pool *pgxpool.Pool) (*ScheduledRefreshRepository, error) {
	if pool == nil {
		return nil, segmentapp.ErrSegmentRefreshFailed
	}
	return &ScheduledRefreshRepository{pool: pool}, nil
}

func (repository *ScheduledRefreshRepository) ListScheduledRefreshCandidates(
	ctx context.Context,
) ([]segmentworker.ScheduledRefreshCandidate, error) {
	if repository == nil || repository.pool == nil || ctx == nil {
		return nil, segmentapp.ErrSegmentRefreshFailed
	}
	rows, err := repository.pool.Query(ctx, `
		SELECT id, refresh_cron FROM segments
		WHERE refresh_mode = 'scheduled' AND refresh_cron IS NOT NULL AND lifecycle_status = 'active'
		ORDER BY id`)
	if err != nil {
		return nil, mapRefreshDatabaseError(err)
	}
	defer rows.Close()
	candidates := make([]segmentworker.ScheduledRefreshCandidate, 0)
	for rows.Next() {
		var id int64
		var cron string
		if err := rows.Scan(&id, &cron); err != nil || id <= 0 || cron == "" {
			return nil, segmentapp.ErrSegmentRefreshFailed
		}
		candidates = append(candidates, segmentworker.ScheduledRefreshCandidate{
			SegmentID: segmentport.SegmentID(id), RefreshCron: cron,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, mapRefreshDatabaseError(err)
	}
	return candidates, nil
}
