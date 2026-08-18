package media_acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestImageUpdate0364PostgreSQLMetadataEnabledAndEventTransaction(t *testing.T) {
	pool, ctx := openPool(t)
	service := realService(pool)
	marker := unique("image-update")
	created, err := service.Upload(ctx, command(t, 7901, unique("image-update-key"), marker+"-before"))
	if err != nil {
		t.Fatal(err)
	}
	before := imageUpdateFacts(t, pool, ctx, created.ID)
	name, description, category, enabled := marker+"-after", " updated description ", marker+"-category", false
	tags := []string{" hero ", "hero", marker + "-tag"}
	updated, err := service.UpdateImageMetadata(ctx, mediaapp.ImageMetadataUpdateCommand{ImageID: created.ID, Actor: 7901,
		Patch: mediaapp.ImageMetadataPatch{Name: &name, Description: &description, Tags: &tags, Category: &category, Enabled: &enabled}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Name != name || updated.Description != "updated description" || updated.Tags != "hero,"+marker+"-tag" || updated.Category != category || updated.Enabled || !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("updated=%#v", updated)
	}
	after := imageUpdateFacts(t, pool, ctx, created.ID)
	if !bytes.Equal(before.imageChecksum, after.imageChecksum) || !bytes.Equal(before.blobChecksum, after.blobChecksum) || !bytes.Equal(before.blobContent, after.blobContent) ||
		before.fileName != after.fileName || before.mimeType != after.mimeType || before.fileSize != after.fileSize || before.width != after.width || before.height != after.height || !before.createdAt.Equal(after.createdAt) ||
		after.name != name || after.description != "updated description" || after.tags != "hero,"+marker+"-tag" || after.category != category || after.enabled || after.eventCount != before.eventCount+1 {
		t.Fatalf("before=%#v after=%#v", before, after)
	}
	var eventType string
	var payload []byte
	if err := pool.QueryRow(ctx, `SELECT event_type,payload FROM event_log WHERE event_type='media.image_metadata_updated' AND payload->>'image_id'=$1 ORDER BY id DESC LIMIT 1`, fmt.Sprintf("%d", created.ID)).Scan(&eventType, &payload); err != nil {
		t.Fatal(err)
	}
	var eventPayload struct {
		ImageID       int64    `json:"image_id"`
		Actor         int64    `json:"actor"`
		ChangedFields []string `json:"changed_fields"`
	}
	if eventType != "media.image_metadata_updated" || json.Unmarshal(payload, &eventPayload) != nil || eventPayload.ImageID != created.ID || eventPayload.Actor != 7901 ||
		!reflect.DeepEqual(eventPayload.ChangedFields, []string{"category", "description", "enabled", "name", "tags"}) {
		t.Fatalf("event=%q payload=%s", eventType, payload)
	}

	defaultPage, err := service.ListImages(ctx, mediaport.ImageListQuery{EnabledOnly: true})
	if err != nil || imageListPageHasID(defaultPage, created.ID) {
		t.Fatalf("default page=%#v err=%v", defaultPage, err)
	}
	allPage, err := service.ListImages(ctx, mediaport.ImageListQuery{EnabledOnly: false})
	if err != nil || !imageListPageHasEnabledID(allPage, created.ID, false) {
		t.Fatalf("all page=%#v err=%v", allPage, err)
	}
	facets, err := service.Facets(ctx)
	if err != nil || containsImageFacetValue(facets.Categories, category) || containsImageFacetValue(facets.Tags, marker+"-tag") {
		t.Fatalf("facets=%#v err=%v", facets, err)
	}
	detail, err := service.GetImageDetail(ctx, created.ID)
	if err != nil || detail.Enabled || detail.Name != name {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}

	noOp, err := service.UpdateImageMetadata(ctx, mediaapp.ImageMetadataUpdateCommand{ImageID: created.ID, Actor: 7901, Patch: mediaapp.ImageMetadataPatch{}})
	if err != nil || !noOp.UpdatedAt.Equal(updated.UpdatedAt) {
		t.Fatalf("noOp=%#v err=%v", noOp, err)
	}
	noOpFacts := imageUpdateFacts(t, pool, ctx, created.ID)
	if noOpFacts.eventCount != after.eventCount || !noOpFacts.updatedAt.Equal(after.updatedAt) {
		t.Fatalf("before no-op=%#v after=%#v", after, noOpFacts)
	}

	rollbackName := marker + "-rolled-back"
	failing := mediaapp.NewService(platformstore.NewUnitOfWork(pool), mediastore.NewUploadRepository(), failingAppender{})
	if _, err := failing.UpdateImageMetadata(ctx, mediaapp.ImageMetadataUpdateCommand{ImageID: created.ID, Actor: 7901, Patch: mediaapp.ImageMetadataPatch{Name: &rollbackName}}); !errors.Is(err, mediaapp.ErrImageMetadataUnavailable) {
		t.Fatalf("event failure=%v", err)
	}
	rolledBack := imageUpdateFacts(t, pool, ctx, created.ID)
	if rolledBack.name != noOpFacts.name || !rolledBack.updatedAt.Equal(noOpFacts.updatedAt) || rolledBack.eventCount != noOpFacts.eventCount {
		t.Fatalf("rollback leaked before=%#v after=%#v", noOpFacts, rolledBack)
	}
}

func TestImageUpdate0364PostgreSQLConcurrentUpdatesSerializeWithoutLoss(t *testing.T) {
	pool, ctx := openPool(t)
	marker := unique("image-update-concurrent")
	created, err := realService(pool).Upload(ctx, command(t, 7902, unique("image-update-concurrent-key"), marker+"-before"))
	if err != nil {
		t.Fatal(err)
	}
	before := imageUpdateFacts(t, pool, ctx, created.ID)

	hold, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = hold.Rollback(ctx) }()
	var lockedID int64
	if err = hold.QueryRow(ctx, `SELECT id FROM media_images WHERE id=$1 FOR UPDATE`, created.ID).Scan(&lockedID); err != nil || lockedID != created.ID {
		t.Fatalf("hold lock id=%d err=%v", lockedID, err)
	}

	entered := make(chan struct{}, 2)
	newService := func() *mediaapp.Service {
		return mediaapp.NewService(platformstore.NewUnitOfWork(pool), &imageUpdateLockObserver{UploadRepository: mediastore.NewUploadRepository(), entered: entered}, eventstore.NewAppender())
	}
	type updateResult struct {
		image mediaapp.ImageMetadata
		err   error
	}
	results := make(chan updateResult, 2)
	name, category := marker+"-name", marker+"-category"
	go func() {
		image, updateErr := newService().UpdateImageMetadata(ctx, mediaapp.ImageMetadataUpdateCommand{ImageID: created.ID, Actor: 7902, Patch: mediaapp.ImageMetadataPatch{Name: &name}})
		results <- updateResult{image: image, err: updateErr}
	}()
	go func() {
		image, updateErr := newService().UpdateImageMetadata(ctx, mediaapp.ImageMetadataUpdateCommand{ImageID: created.ID, Actor: 7903, Patch: mediaapp.ImageMetadataPatch{Category: &category}})
		results <- updateResult{image: image, err: updateErr}
	}()
	for range 2 {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent updates did not attempt the row lock")
		}
	}
	waitForImageMetadataRowLocks(t, pool, ctx, 2)
	if held := imageUpdateFacts(t, pool, ctx, created.ID); held.eventCount != before.eventCount {
		t.Fatalf("event committed while row lock was held: before=%#v held=%#v", before, held)
	}
	select {
	case result := <-results:
		t.Fatalf("update completed while row lock was held: %#v", result)
	default:
	}
	if err = hold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case result := <-results:
			if result.err != nil || result.image.ID != created.ID {
				t.Fatalf("concurrent result=%#v", result)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent update did not finish after row lock release")
		}
	}
	after := imageUpdateFacts(t, pool, ctx, created.ID)
	if after.name != name || after.category != category || after.eventCount != before.eventCount+2 || !after.updatedAt.After(before.updatedAt) {
		t.Fatalf("before=%#v after=%#v", before, after)
	}
	var nameEvents, categoryEvents int
	err = pool.QueryRow(ctx, `SELECT
      count(*) FILTER (WHERE payload->'changed_fields' = '["name"]'::jsonb),
      count(*) FILTER (WHERE payload->'changed_fields' = '["category"]'::jsonb)
    FROM event_log
    WHERE event_type='media.image_metadata_updated' AND payload->>'image_id'=$1`, fmt.Sprintf("%d", created.ID)).Scan(&nameEvents, &categoryEvents)
	if err != nil || nameEvents != 1 || categoryEvents != 1 {
		t.Fatalf("events name=%d category=%d err=%v", nameEvents, categoryEvents, err)
	}
}

type imageUpdateLockObserver struct {
	*mediastore.UploadRepository
	entered chan<- struct{}
}

func (store *imageUpdateLockObserver) LockImageMetadata(ctx context.Context, imageID int64) (mediaapp.ImageMetadata, error) {
	select {
	case store.entered <- struct{}{}:
	case <-ctx.Done():
		return mediaapp.ImageMetadata{}, ctx.Err()
	}
	return store.UploadRepository.LockImageMetadata(ctx, imageID)
}

func waitForImageMetadataRowLocks(t *testing.T, pool *pgxpool.Pool, ctx context.Context, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var locks int
		err := pool.QueryRow(ctx, `SELECT count(*)
      FROM pg_stat_activity
      WHERE datname = current_database()
        AND wait_event_type = 'Lock'
        AND query LIKE '%FROM media_images AS image%'
        AND query LIKE '%FOR UPDATE%'`).Scan(&locks)
		if err != nil {
			t.Fatal(err)
		}
		if locks >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("wanted %d blocked media image row locks", want)
}

type imageUpdateFact struct {
	name, fileName, mimeType, description, tags, category string
	fileSize                                              int
	width, height                                         int
	enabled                                               bool
	createdAt, updatedAt                                  time.Time
	imageChecksum, blobChecksum, blobContent              []byte
	eventCount                                            int
}

func imageUpdateFacts(t *testing.T, pool *pgxpool.Pool, ctx context.Context, imageID int64) imageUpdateFact {
	t.Helper()
	var fact imageUpdateFact
	err := pool.QueryRow(ctx, `SELECT i.name,i.file_name,i.mime_type,i.file_size,i.enabled,i.description,i.tags,i.category,i.width,i.height,i.created_at,i.updated_at,i.checksum,b.checksum,b.content,
	      (SELECT count(*) FROM event_log WHERE event_type='media.image_metadata_updated' AND (payload->>'image_id')=i.id::text)
      FROM media_images i JOIN media_image_blobs b ON b.image_id=i.id WHERE i.id=$1`, imageID).Scan(
		&fact.name, &fact.fileName, &fact.mimeType, &fact.fileSize, &fact.enabled, &fact.description, &fact.tags, &fact.category, &fact.width, &fact.height,
		&fact.createdAt, &fact.updatedAt, &fact.imageChecksum, &fact.blobChecksum, &fact.blobContent, &fact.eventCount)
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func imageListPageHasID(page mediaport.ImageListPage, id int64) bool {
	for _, item := range page.Items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func imageListPageHasEnabledID(page mediaport.ImageListPage, id int64, enabled bool) bool {
	for _, item := range page.Items {
		if item.ID == id && item.Enabled == enabled {
			return true
		}
	}
	return false
}
