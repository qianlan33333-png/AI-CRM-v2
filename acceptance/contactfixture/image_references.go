package contactfixture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidImageReference = errors.New("invalid contact image reference fixture")

func CreateImageReference(ctx context.Context, pool *pgxpool.Pool, name, code string, imageID int64) (int64, error) {
	if pool == nil || name == "" || code == "" || imageID < 1 {
		return 0, ErrInvalidImageReference
	}
	var id int64
	if err := pool.QueryRow(ctx, `
INSERT INTO channels (name,code,status,config,created_by,updated_by,created_at,updated_at)
VALUES ($1,$2,'archived',jsonb_build_object('schema_version',1,'welcome_image_library_ids',jsonb_build_array($3::bigint)),1,1,now(),now())
RETURNING id`, name, code, imageID).Scan(&id); err != nil {
		return 0, fmt.Errorf("create contact-owned image reference: %w", err)
	}
	return id, nil
}

func CreateImageReferenceWithRawIDs(ctx context.Context, pool *pgxpool.Pool, name, code, rawIDs string) (int64, error) {
	if pool == nil || name == "" || code == "" || rawIDs == "" {
		return 0, ErrInvalidImageReference
	}
	var id int64
	if err := pool.QueryRow(ctx, `
INSERT INTO channels (name,code,status,config,created_by,updated_by,created_at,updated_at)
VALUES ($1,$2,'archived',jsonb_build_object('schema_version',1,'welcome_image_library_ids',$3::jsonb),1,1,now(),now())
RETURNING id`, name, code, rawIDs).Scan(&id); err != nil {
		return 0, fmt.Errorf("create contact-owned malformed image reference: %w", err)
	}
	return id, nil
}

func CreateAttachmentReference(ctx context.Context, pool *pgxpool.Pool, name, code string, attachmentID int64) (int64, error) {
	if pool == nil || name == "" || code == "" || attachmentID < 1 {
		return 0, ErrInvalidImageReference
	}
	var id int64
	if err := pool.QueryRow(ctx, `
INSERT INTO channels (name,code,status,config,created_by,updated_by,created_at,updated_at)
VALUES ($1,$2,'active',jsonb_build_object('schema_version',1,'welcome_attachment_library_ids',jsonb_build_array($3::bigint)),1,1,now(),now())
RETURNING id`, name, code, attachmentID).Scan(&id); err != nil {
		return 0, fmt.Errorf("create contact-owned attachment reference: %w", err)
	}
	return id, nil
}

func DeleteImageReference(ctx context.Context, pool *pgxpool.Pool, id int64) error {
	if pool == nil || id < 1 {
		return ErrInvalidImageReference
	}
	result, err := pool.Exec(ctx, `DELETE FROM channels WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete contact-owned image reference: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("delete contact-owned image reference: not found")
	}
	return nil
}

func CountImageReferences(ctx context.Context, pool *pgxpool.Pool, id int64) (int, error) {
	if pool == nil || id < 1 {
		return 0, ErrInvalidImageReference
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM channels WHERE id=$1`, id).Scan(&count); err != nil {
		return 0, fmt.Errorf("count contact-owned image reference: %w", err)
	}
	return count, nil
}
