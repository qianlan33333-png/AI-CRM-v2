// Package acceptancefixture creates Radar-owned rows for cross-domain
// acceptance scenarios.
package acceptancefixture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// CreateDraftLink creates one local Radar link referencing a Media image.
func CreateDraftLink(ctx context.Context, db queryer, publicCode, name, title, destinationURL string, coverImageID, actorID int64) (int64, error) {
	if db == nil || publicCode == "" || name == "" || title == "" || destinationURL == "" || coverImageID <= 0 || actorID <= 0 {
		return 0, fmt.Errorf("valid Radar fixture fields required")
	}
	var id int64
	if err := db.QueryRow(ctx, `
INSERT INTO radar_links (
  public_code, name, title, destination_url, cover_image_id, status,
  version, created_by, updated_by, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, 'draft', 1, $6, $6, now(), now())
RETURNING id`, publicCode, name, title, destinationURL, coverImageID, actorID).Scan(&id); err != nil {
		return 0, fmt.Errorf("create Radar-owned acceptance link: %w", err)
	}
	return id, nil
}

// CreateDraftAttachmentLink creates one local Radar link referencing a Media
// attachment.
func CreateDraftAttachmentLink(ctx context.Context, db queryer, publicCode, name, title, destinationURL string, attachmentID, actorID int64) (int64, error) {
	if db == nil || publicCode == "" || name == "" || title == "" || destinationURL == "" || attachmentID <= 0 || actorID <= 0 {
		return 0, fmt.Errorf("valid Radar attachment fixture fields required")
	}
	var id int64
	if err := db.QueryRow(ctx, `
INSERT INTO radar_links (
  public_code, name, title, destination_url, attachment_id, status,
  version, created_by, updated_by, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, 'draft', 1, $6, $6, now(), now())
RETURNING id`, publicCode, name, title, destinationURL, attachmentID, actorID).Scan(&id); err != nil {
		return 0, fmt.Errorf("create Radar-owned acceptance attachment link: %w", err)
	}
	return id, nil
}
