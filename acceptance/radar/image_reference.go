package radarfixture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidImageReference = errors.New("invalid radar image reference fixture")

func CreateImageReference(ctx context.Context, pool *pgxpool.Pool, code, name, title string, imageID int64) (int64, error) {
	if pool == nil || code == "" || name == "" || title == "" || imageID < 1 {
		return 0, ErrInvalidImageReference
	}
	var id int64
	if err := pool.QueryRow(ctx, `
INSERT INTO radar_links (public_code,name,title,destination_url,cover_image_id,status,version,created_by,updated_by,created_at,updated_at)
VALUES ($1,$2,$3,'https://example.com/delete-radar',$4,'draft',1,1,1,now(),now())
RETURNING id`, code, name, title, imageID).Scan(&id); err != nil {
		return 0, fmt.Errorf("create radar-owned image reference: %w", err)
	}
	return id, nil
}
