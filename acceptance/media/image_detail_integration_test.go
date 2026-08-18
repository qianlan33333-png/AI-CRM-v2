package media_acceptance

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
)

// The HTTP route's strict query, authentication, and method behavior is
// covered in cmd/aicrm. This isolated PostgreSQL 16.14 acceptance test proves
// the underlying one-UoW detail projection returns the original validated
// bytes needed by both metadata-only and include_data=true response modes,
// without a durable read-side effect.
func TestImageDetail0363PostgreSQLProjectionIsReadOnly(t *testing.T) {
	pool, ctx := openPool(t)
	service := realService(pool)
	uploadCommand := command(t, 7301, unique("detail-key"), unique("detail-image"))
	uploadCommand.Description, uploadCommand.Tags, uploadCommand.Category = "detail description", "hero, hero, 首页", "detail"
	created, err := service.Upload(ctx, uploadCommand)
	if err != nil {
		t.Fatal(err)
	}
	before := imageFacetsFactSnapshot(t, pool, ctx)
	detail, err := service.GetImageDetail(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	after := imageFacetsFactSnapshot(t, pool, ctx)
	if before != after {
		t.Fatalf("read changed durable facts before=%#v after=%#v", before, after)
	}
	if detail.ID != created.ID || detail.Name != uploadCommand.Name || detail.FileName != uploadCommand.FileName || detail.MimeType != "image/png" ||
		detail.FileSize != int32(len(uploadCommand.Content)) || detail.Description != uploadCommand.Description || detail.Category != uploadCommand.Category ||
		detail.Width != 2 || detail.Height != 3 || detail.CreatedAt.Location() != time.UTC || detail.UpdatedAt.Location() != time.UTC ||
		len(detail.Tags) != 2 || detail.Tags[0] != "hero" || detail.Tags[1] != "首页" || !bytes.Equal(detail.Content, uploadCommand.Content) {
		t.Fatalf("detail=%#v", detail)
	}
	// include_data=true is a transport-only compatibility projection over this
	// already validated original blob; the standard base64 round-trip remains
	// byte exact and does not require another Media read.
	encoded := base64.StdEncoding.EncodeToString(detail.Content)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !bytes.Equal(decoded, uploadCommand.Content) {
		t.Fatalf("decoded=%x err=%v", decoded, err)
	}

	missingBlobCommand := command(t, 7302, unique("detail-missing-blob-key"), unique("detail-missing-blob-image"))
	missingBlob, err := service.Upload(ctx, missingBlobCommand)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM media_image_blobs WHERE image_id = $1", missingBlob.ID); err != nil {
		t.Fatal(err)
	}
	var imageRows int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM media_images WHERE id = $1", missingBlob.ID).Scan(&imageRows); err != nil {
		t.Fatal(err)
	}
	if imageRows != 1 {
		t.Fatalf("image row count after blob deletion = %d, want 1", imageRows)
	}
	if _, err := service.GetImageDetail(ctx, missingBlob.ID); !errors.Is(err, mediaapp.ErrImageDetailNotFound) {
		t.Fatalf("GetImageDetail missing blob error = %v, want ErrImageDetailNotFound", err)
	}
}
