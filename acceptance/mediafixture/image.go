// Package mediafixture creates Media-owned rows for acceptance tests.
package mediafixture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidImage = errors.New("invalid media image fixture")

// CreateImage creates a valid, blob-backed Media image and returns its generated ID.
func CreateImage(ctx context.Context, pool *pgxpool.Pool, name string) (int64, error) {
	if pool == nil || name == "" || len(name) > 200 {
		return 0, ErrInvalidImage
	}
	var content bytes.Buffer
	pngImage := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	pngImage.SetNRGBA(0, 0, color.NRGBA{A: 0xff})
	if err := png.Encode(&content, pngImage); err != nil {
		return 0, fmt.Errorf("encode media-owned image fixture: %w", err)
	}
	checksum := sha256.Sum256(content.Bytes())
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin media-owned image fixture: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id int64
	if err = tx.QueryRow(ctx, `
INSERT INTO media_images (name,file_name,mime_type,file_size,width,height,checksum,description,tags,category,created_by,created_at,updated_at)
VALUES ($1,'mediafixture.png','image/png',$2,1,1,$3,'','','',1,now(),now())
RETURNING id`, name, content.Len(), checksum[:]).Scan(&id); err != nil {
		return 0, fmt.Errorf("create media-owned image fixture: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO media_image_blobs (image_id,content,checksum,created_at) VALUES ($1,$2,$3,now())`, id, content.Bytes(), checksum[:]); err != nil {
		return 0, fmt.Errorf("create media-owned image blob fixture: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit media-owned image fixture: %w", err)
	}
	return id, nil
}
