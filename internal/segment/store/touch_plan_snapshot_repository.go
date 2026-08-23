package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

// TouchPlanSnapshotRepository is a transaction-bound, Segment-owned reader
// for the Campaign initiation source seam. It never starts a UoW itself.
type TouchPlanSnapshotRepository struct{}

var _ segmentport.TouchPlanSnapshotReader = (*TouchPlanSnapshotRepository)(nil)

func NewTouchPlanSnapshotRepository() *TouchPlanSnapshotRepository {
	return &TouchPlanSnapshotRepository{}
}

func (repository *TouchPlanSnapshotRepository) ReadSegmentTouchPlanSnapshot(ctx context.Context, segmentID segmentport.SegmentID) (segmentport.SegmentTouchPlanSnapshot, error) {
	if repository == nil || segmentID < 1 {
		return segmentport.SegmentTouchPlanSnapshot{}, segmentport.ErrTouchPlanSnapshotUnavailable
	}
	queries, err := touchPlanSnapshotQueries(ctx)
	if err != nil {
		return segmentport.SegmentTouchPlanSnapshot{}, err
	}
	row, err := queries.LockSegmentTouchPlanSnapshot(ctx, int64(segmentID))
	if err != nil || row.ID != int64(segmentID) || !touchPlanSnapshotReady(row.MemberCount, row.RefreshedAt, row.RefreshStatus, row.LifecycleStatus) {
		return segmentport.SegmentTouchPlanSnapshot{}, snapshotError(err)
	}
	members, err := queries.ListTouchPlanSnapshotMembers(ctx, int64(segmentID))
	if err != nil || int64(len(members)) != row.MemberCount {
		return segmentport.SegmentTouchPlanSnapshot{}, snapshotError(err)
	}
	snapshot := segmentport.SegmentTouchPlanSnapshot{
		SegmentID: segmentID, RefreshedAt: row.RefreshedAt.Time.UTC(), CustomerIDs: customerIDs(members),
	}
	snapshot.Digest = segmentport.CanonicalSegmentTouchPlanSnapshotDigest(snapshot.SegmentID, snapshot.RefreshedAt, snapshot.CustomerIDs)
	if !snapshot.Valid() {
		return segmentport.SegmentTouchPlanSnapshot{}, segmentport.ErrTouchPlanSnapshotUnavailable
	}
	return snapshot, nil
}

func (repository *TouchPlanSnapshotRepository) ReadAudiencePackageTouchPlanSnapshot(ctx context.Context, packageID segmentport.SegmentID) (segmentport.AudiencePackageTouchPlanSnapshot, error) {
	if repository == nil || packageID < 1 {
		return segmentport.AudiencePackageTouchPlanSnapshot{}, segmentport.ErrTouchPlanSnapshotUnavailable
	}
	queries, err := touchPlanSnapshotQueries(ctx)
	if err != nil {
		return segmentport.AudiencePackageTouchPlanSnapshot{}, err
	}
	row, err := queries.LockAudiencePackageTouchPlanSnapshot(ctx, int64(packageID))
	if err != nil || row.PackageID != int64(packageID) || row.PackageLifecycle != "active" ||
		!touchPlanSnapshotReady(row.MemberCount, row.RefreshedAt, row.RefreshStatus, row.LifecycleStatus) {
		return segmentport.AudiencePackageTouchPlanSnapshot{}, snapshotError(err)
	}
	members, err := queries.ListTouchPlanSnapshotMembers(ctx, int64(packageID))
	if err != nil || int64(len(members)) != row.MemberCount {
		return segmentport.AudiencePackageTouchPlanSnapshot{}, snapshotError(err)
	}
	snapshot := segmentport.AudiencePackageTouchPlanSnapshot{
		PackageID: packageID, PackageVersion: row.PackageVersion, RefreshedAt: row.RefreshedAt.Time.UTC(), CustomerIDs: customerIDs(members),
	}
	snapshot.Digest = segmentport.CanonicalAudiencePackageTouchPlanSnapshotDigest(snapshot.PackageID, snapshot.PackageVersion, snapshot.RefreshedAt, snapshot.CustomerIDs)
	if !snapshot.Valid() {
		return segmentport.AudiencePackageTouchPlanSnapshot{}, segmentport.ErrTouchPlanSnapshotUnavailable
	}
	return snapshot, nil
}

func touchPlanSnapshotQueries(ctx context.Context) (*segmentdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return segmentdb.New(tx), nil
}

func touchPlanSnapshotReady(memberCount int64, refreshedAt pgtype.Timestamptz, refreshStatus, lifecycle string) bool {
	return memberCount >= 0 && memberCount <= segmentport.MaximumTouchPlanSnapshotMembers && refreshedAt.Valid &&
		!refreshedAt.Time.IsZero() && refreshStatus == string(segmentport.RefreshStatusIdle) && lifecycle == string(segmentport.LifecycleStatusActive)
}

func customerIDs(rows []int64) []segmentport.CustomerID {
	result := make([]segmentport.CustomerID, len(rows))
	for index, customerID := range rows {
		result[index] = segmentport.CustomerID(customerID)
	}
	return result
}

func snapshotError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return segmentport.ErrTouchPlanSnapshotUnavailable
	}
	return segmentport.ErrTouchPlanSnapshotUnavailable
}
