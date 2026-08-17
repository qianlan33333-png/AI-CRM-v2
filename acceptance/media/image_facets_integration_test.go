package media_acceptance

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestImageFacets0358PostgreSQLProjectionIsReadOnly(t *testing.T) {
	pool, ctx := openPool(t)
	marker := fmt.Sprintf("facets-0358-%d", time.Now().UnixNano())
	firstID := createCover(t, pool, ctx, 6701)
	secondID := createCover(t, pool, ctx, 6702)
	if _, err := pool.Exec(ctx, `UPDATE media_images
		SET category=$2,tags=$3 WHERE id=$1`, firstID, "  "+marker+"-beta  ", " beta,Alpha, beta ,,中文，逗号"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_images
		SET category=$2,tags=$3 WHERE id=$1`, secondID, marker+"-Alpha", "alpha, Alpha"); err != nil {
		t.Fatal(err)
	}

	before := imageFacetsFactSnapshot(t, pool, ctx)
	service := mediaapp.NewService(platformstore.NewUnitOfWork(pool), mediastore.NewUploadRepository(), nil)
	result, err := service.Facets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after := imageFacetsFactSnapshot(t, pool, ctx)
	if before != after {
		t.Fatalf("read path changed durable facts before=%#v after=%#v", before, after)
	}
	for _, category := range []string{marker + "-Alpha", marker + "-beta"} {
		if !containsImageFacetValue(result.Categories, category) {
			t.Fatalf("missing category %q in %q", category, result.Categories)
		}
	}
	for _, tag := range []string{"Alpha", "alpha", "beta", "中文，逗号"} {
		if !containsImageFacetValue(result.Tags, tag) {
			t.Fatalf("missing tag %q in %q", tag, result.Tags)
		}
	}
}

type imageFacetsFacts struct {
	images, blobs, receipts, events int64
	maxImageID, maxEventID          int64
	maxImageUpdatedAt               time.Time
}

func imageFacetsFactSnapshot(t *testing.T, pool *pgxpool.Pool, ctx context.Context) imageFacetsFacts {
	t.Helper()
	var result imageFacetsFacts
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM media_images),
		(SELECT count(*) FROM media_image_blobs),
		(SELECT count(*) FROM media_image_upload_receipts),
		(SELECT count(*) FROM event_log),
		(SELECT COALESCE(max(id),0) FROM media_images),
		(SELECT COALESCE(max(id),0) FROM event_log),
		(SELECT COALESCE(max(updated_at),'epoch'::timestamptz) FROM media_images)`).Scan(
		&result.images, &result.blobs, &result.receipts, &result.events,
		&result.maxImageID, &result.maxEventID, &result.maxImageUpdatedAt,
	); err != nil {
		t.Fatal(err)
	}
	return result
}

func containsImageFacetValue(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
