package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
	segmentworker "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/worker"
)

// ScheduledRefreshRepository is a read-only worker projection. RefreshOnce
// remains the only Segment member-replacement writer.
type ScheduledRefreshRepository struct {
	queries *segmentdb.Queries
}

var _ segmentworker.ScheduledRefreshFinder = (*ScheduledRefreshRepository)(nil)

func NewScheduledRefreshRepository(pool *pgxpool.Pool) (*ScheduledRefreshRepository, error) {
	if pool == nil {
		return nil, segmentapp.ErrSegmentRefreshFailed
	}
	return &ScheduledRefreshRepository{queries: segmentdb.New(pool)}, nil
}

func (repository *ScheduledRefreshRepository) ListScheduledRefreshCandidates(
	ctx context.Context,
) ([]segmentworker.ScheduledRefreshCandidate, error) {
	if repository == nil || repository.queries == nil || ctx == nil {
		return nil, segmentapp.ErrSegmentRefreshFailed
	}
	rows, err := repository.queries.ListScheduledSegmentRefreshes(ctx)
	if err != nil {
		return nil, mapRefreshDatabaseError(err)
	}
	candidates := make([]segmentworker.ScheduledRefreshCandidate, len(rows))
	for index, row := range rows {
		if row.ID <= 0 || !row.RefreshCron.Valid || row.RefreshCron.String == "" {
			return nil, segmentapp.ErrSegmentRefreshFailed
		}
		candidates[index] = segmentworker.ScheduledRefreshCandidate{
			SegmentID: segmentport.SegmentID(row.ID), RefreshCron: row.RefreshCron.String,
		}
	}
	return candidates, nil
}
