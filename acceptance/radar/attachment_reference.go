// Package radarfixture creates Radar-owned rows for acceptance tests.
package radarfixture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidAttachmentReference = errors.New("invalid radar attachment reference fixture")

func CreateAttachmentReference(ctx context.Context, pool *pgxpool.Pool, code, name, title string, attachmentID int64) (int64, error) {
	if pool == nil || code == "" || name == "" || title == "" || attachmentID < 1 {
		return 0, ErrInvalidAttachmentReference
	}
	var id int64
	if err := pool.QueryRow(ctx, `
INSERT INTO radar_links (public_code,name,title,destination_url,attachment_id,status,version,created_by,updated_by,created_at,updated_at)
VALUES ($1,$2,$3,'https://example.com/attachment-reference',$4,'draft',1,1,1,now(),now())
RETURNING id`, code, name, title, attachmentID).Scan(&id); err != nil {
		return 0, fmt.Errorf("create radar-owned attachment reference: %w", err)
	}
	return id, nil
}
