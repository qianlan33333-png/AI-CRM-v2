// Package acceptancefixture creates Segment-owned rows for cross-domain
// acceptance scenarios.
package acceptancefixture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// CreateAudienceSnapshot creates one active Segment with its current member
// set and local AI Audience package version.
func CreateAudienceSnapshot(ctx context.Context, db executor, name string, customerIDs []int64, refreshedAt time.Time, packageVersion int64) (int64, error) {
	if db == nil || name == "" || len(customerIDs) == 0 || refreshedAt.IsZero() || packageVersion <= 0 {
		return 0, fmt.Errorf("valid Segment fixture fields required")
	}
	var segmentID int64
	if err := db.QueryRow(ctx, `
INSERT INTO segments (name,definition,refresh_mode,member_count,refreshed_at,refresh_status,lifecycle_status,created_at,updated_at)
VALUES ($1,'{}','manual',$2,$3,'idle','active',$3,$3)
RETURNING id`, name, len(customerIDs), refreshedAt.UTC()).Scan(&segmentID); err != nil {
		return 0, fmt.Errorf("create Segment-owned acceptance snapshot: %w", err)
	}
	for _, customerID := range customerIDs {
		if customerID <= 0 {
			return 0, fmt.Errorf("valid Segment fixture customer required")
		}
		if _, err := db.Exec(ctx, `INSERT INTO segment_members (segment_id,customer_id,computed_at) VALUES ($1,$2,$3)`, segmentID, customerID, refreshedAt.UTC()); err != nil {
			return 0, fmt.Errorf("create Segment-owned acceptance member: %w", err)
		}
	}
	if _, err := db.Exec(ctx, `
INSERT INTO ai_audience_package_metadata (segment_id,lifecycle,version,created_by,updated_by,created_at,updated_at)
VALUES ($1,'active',$2,1,1,$3,$3)`, segmentID, packageVersion, refreshedAt.UTC()); err != nil {
		return 0, fmt.Errorf("create Segment-owned acceptance audience metadata: %w", err)
	}
	return segmentID, nil
}

func DeleteAudienceSnapshot(ctx context.Context, db executor, segmentID int64) error {
	if db == nil || segmentID <= 0 {
		return fmt.Errorf("valid Segment fixture required")
	}
	if _, err := db.Exec(ctx, `DELETE FROM segments WHERE id = $1::bigint`, segmentID); err != nil {
		return fmt.Errorf("delete Segment-owned acceptance snapshot: %w", err)
	}
	return nil
}
