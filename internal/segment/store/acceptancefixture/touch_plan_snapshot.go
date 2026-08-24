// Package acceptancefixture creates Segment-owned facts for isolated
// acceptance tests without giving another domain write access to those tables.
package acceptancefixture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateTouchPlanSnapshot creates one committed Segment and its authoritative
// member and audience-package snapshots for a multi-transaction scenario.
func CreateTouchPlanSnapshot(
	ctx context.Context,
	pool *pgxpool.Pool,
	name string,
	customerIDs []int64,
	refreshedAt time.Time,
	packageVersion int64,
) (int64, error) {
	if pool == nil || name == "" || len(customerIDs) == 0 || refreshedAt.IsZero() || packageVersion < 1 {
		return 0, fmt.Errorf("valid Segment touch-plan snapshot fixture required")
	}
	for index, customerID := range customerIDs {
		if customerID < 1 || index > 0 && customerIDs[index-1] >= customerID {
			return 0, fmt.Errorf("canonical Segment customer IDs required")
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin Segment touch-plan snapshot fixture: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck -- rollback is fixture cleanup on failure.

	var segmentID int64
	if err = tx.QueryRow(ctx, `
INSERT INTO segments
  (name, definition, refresh_mode, member_count, refreshed_at, refresh_status, lifecycle_status, created_at, updated_at)
VALUES ($1::text, '{}'::jsonb, 'manual', $2::bigint, $3::timestamptz, 'idle', 'active', $3::timestamptz, $3::timestamptz)
RETURNING id`, name, len(customerIDs), refreshedAt.UTC()).Scan(&segmentID); err != nil {
		return 0, fmt.Errorf("create Segment touch-plan snapshot: %w", err)
	}
	for _, customerID := range customerIDs {
		if _, err = tx.Exec(ctx, `
INSERT INTO segment_members (segment_id, customer_id, computed_at)
VALUES ($1::bigint, $2::bigint, $3::timestamptz)`, segmentID, customerID, refreshedAt.UTC()); err != nil {
			return 0, fmt.Errorf("create Segment touch-plan snapshot member: %w", err)
		}
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO ai_audience_package_metadata
  (segment_id, lifecycle, version, created_by, updated_by, created_at, updated_at)
VALUES ($1::bigint, 'active', $2::bigint, 1, 1, $3::timestamptz, $3::timestamptz)`, segmentID, packageVersion, refreshedAt.UTC()); err != nil {
		return 0, fmt.Errorf("create Segment audience-package snapshot: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit Segment touch-plan snapshot fixture: %w", err)
	}
	return segmentID, nil
}

// DeleteTouchPlanSnapshot removes the Segment parent; its members and package
// metadata are removed by their existing foreign-key cascades.
func DeleteTouchPlanSnapshot(ctx context.Context, pool *pgxpool.Pool, segmentID int64) error {
	if pool == nil || segmentID < 1 {
		return fmt.Errorf("valid Segment touch-plan snapshot fixture required")
	}
	result, err := pool.Exec(ctx, `DELETE FROM segments WHERE id = $1::bigint`, segmentID)
	if err != nil {
		return fmt.Errorf("delete Segment touch-plan snapshot fixture: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("delete Segment touch-plan snapshot fixture: not found")
	}
	return nil
}
