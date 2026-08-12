package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/compiler"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

// RefreshRepository is the only S04A writer. It can update Segment-owned
// segments/segment_members only through a caller-supplied transaction.
type RefreshRepository struct{}

var _ segmentapp.RefreshStore = (*RefreshRepository)(nil)

func NewRefreshRepository() *RefreshRepository { return &RefreshRepository{} }

func (repository *RefreshRepository) LockDefinition(
	ctx context.Context,
	segmentID segmentport.SegmentID,
) ([]byte, error) {
	if repository == nil || segmentID <= 0 {
		return nil, segmentapp.ErrInvalidSegmentRefresh
	}
	queries, err := refreshQueriesFromContext(ctx)
	if err != nil {
		return nil, err
	}
	definition, err := queries.LockSegmentDefinitionForRefresh(ctx, int64(segmentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, segmentapp.ErrSegmentNotFound
	}
	if err != nil {
		return nil, mapRefreshDatabaseError(err)
	}
	return append([]byte(nil), definition...), nil
}

func (repository *RefreshRepository) QuerySet(ctx context.Context) (compiler.QuerySet, error) {
	if repository == nil {
		return nil, segmentapp.ErrSegmentRefreshFailed
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return NewQuerySet(tx), nil
}

func (repository *RefreshRepository) ReplaceMembers(
	ctx context.Context,
	segmentID segmentport.SegmentID,
	customerIDs []int64,
	refreshedAt time.Time,
) error {
	if repository == nil || segmentID <= 0 || refreshedAt.IsZero() || refreshedAt.Location() != time.UTC || !canonicalCustomerIDs(customerIDs) {
		return segmentapp.ErrInvalidSegmentRefresh
	}
	queries, err := refreshQueriesFromContext(ctx)
	if err != nil {
		return err
	}
	if err := queries.DeleteSegmentMembersForRefresh(ctx, int64(segmentID)); err != nil {
		return mapRefreshDatabaseError(err)
	}
	if err := queries.InsertSegmentMembersForRefresh(ctx, segmentdb.InsertSegmentMembersForRefreshParams{
		SegmentID:   int64(segmentID),
		ComputedAt:  timestamp(refreshedAt),
		CustomerIds: append([]int64(nil), customerIDs...),
	}); err != nil {
		return mapRefreshDatabaseError(err)
	}
	count, err := queries.CompleteSegmentRefresh(ctx, segmentdb.CompleteSegmentRefreshParams{
		MemberCount: int64(len(customerIDs)),
		RefreshedAt: timestamp(refreshedAt),
		SegmentID:   int64(segmentID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return segmentapp.ErrSegmentNotFound
	}
	if err != nil {
		return mapRefreshDatabaseError(err)
	}
	if count != int64(len(customerIDs)) {
		return segmentapp.ErrSegmentRefreshFailed
	}
	return nil
}

func refreshQueriesFromContext(ctx context.Context) (*segmentdb.Queries, error) {
	if ctx == nil {
		return nil, platformport.ErrTransactionRequired
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return segmentdb.New(tx), nil
}

func canonicalCustomerIDs(customerIDs []int64) bool {
	for index, customerID := range customerIDs {
		if customerID <= 0 || (index > 0 && customerIDs[index-1] >= customerID) {
			return false
		}
	}
	return true
}

func mapRefreshDatabaseError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return segmentapp.ErrSegmentRefreshFailed
}
