package media_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	automationfixture "github.com/qianlan33333-png/AI-CRM-v2/acceptance/automationfixture"
	contactfixture "github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	automationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	radarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
	radarstore "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store"
	radarfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store/acceptancefixture"
)

func TestImageDelete0362PostgreSQLHardDeleteReplayAndCascades(t *testing.T) {
	pool, ctx := openPool(t)
	actor, uploadKey, deleteKey := int64(9362), unique("delete-upload-key"), unique("delete-key")
	created, err := realService(pool).Upload(ctx, command(t, actor, uploadKey, unique("delete-image")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO media_thumbnail_cache_entries (image_id,state,cache_receipt,updated_at) VALUES ($1,'outcome_unknown',$2,now())`, created.ID, unique("cache-receipt")); err != nil {
		t.Fatal(err)
	}
	service := realImageDeleteService(pool)
	commandValue := mediaapp.ImageDeleteCommand{ImageID: created.ID, Actor: actor, IdempotencyKey: deleteKey}
	result, err := service.DeleteImage(ctx, commandValue)
	if err != nil || !result.Deleted || !result.HardDeleted || result.ID != created.ID || result.References.Any() {
		t.Fatalf("delete=%#v err=%v", result, err)
	}
	replayed, err := service.DeleteImage(ctx, commandValue)
	if err != nil || replayed.ID != result.ID || !replayed.Deleted || !replayed.HardDeleted || replayed.References.Any() {
		t.Fatalf("replay=%#v err=%v", replayed, err)
	}
	changed := commandValue
	changed.Force = true
	if _, err = service.DeleteImage(ctx, changed); !errors.Is(err, mediaapp.ErrImageDeleteConflict) {
		t.Fatalf("changed payload error=%v", err)
	}

	deleteDigest := sha256.Sum256([]byte(deleteKey))
	uploadDigest := sha256.Sum256([]byte(uploadKey))
	uploadEventDigest := sha256.Sum256([]byte(fmt.Sprintf("admin:%d\x00%s", actor, uploadKey)))
	eventDigest := sha256.Sum256([]byte(fmt.Sprintf("admin:%d\x00%s", actor, deleteKey)))
	var images, blobs, caches, uploadReceipts, deleteReceipts, uploads, deletes int
	err = pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM media_images WHERE id=$1),
      (SELECT count(*) FROM media_image_blobs WHERE image_id=$1),
      (SELECT count(*) FROM media_thumbnail_cache_entries WHERE image_id=$1),
      (SELECT count(*) FROM media_image_upload_receipts WHERE actor_scope=$2 AND key_digest=$5),
      (SELECT count(*) FROM media_image_delete_receipts WHERE operation='delete' AND actor_scope=$2 AND key_digest=$3 AND state='completed'),
      (SELECT count(*) FROM event_log WHERE event_type='media.image_created' AND idempotency_key=$6),
      (SELECT count(*) FROM event_log WHERE event_type='media.image_deleted' AND idempotency_key=$4 AND payload = jsonb_build_object('image_id',$1::bigint,'actor',$7::bigint))`,
		created.ID, fmt.Sprintf("admin:%d", actor), deleteDigest[:], "media.image_deleted:"+hex.EncodeToString(eventDigest[:]), uploadDigest[:], "media.image_created:"+hex.EncodeToString(uploadEventDigest[:]), actor).Scan(&images, &blobs, &caches, &uploadReceipts, &deleteReceipts, &uploads, &deletes)
	if err != nil || images != 0 || blobs != 0 || caches != 0 || uploadReceipts != 1 || deleteReceipts != 1 || uploads != 1 || deletes != 1 {
		t.Fatalf("facts=%d/%d/%d/%d/%d/%d/%d err=%v", images, blobs, caches, uploadReceipts, deleteReceipts, uploads, deletes, err)
	}
}

func TestImageDelete0362PostgreSQLReferencesFailClosedForBothForceValues(t *testing.T) {
	pool, ctx := openPool(t)
	for _, fixture := range []struct {
		name  string
		seed  func(*testing.T, context.Context, *pgxpool.Pool, int64) int64
		match func(mediaapp.ImageDeleteReferences) bool
	}{
		{"miniprogram", seedDeleteMiniprogramReference, func(refs mediaapp.ImageDeleteReferences) bool { return len(refs.Miniprograms) == 1 }},
		{"archived group invite", seedDeleteArchivedGroupInviteReference, func(refs mediaapp.ImageDeleteReferences) bool { return len(refs.GroupInvites) == 1 }},
		{"automation agent", seedDeleteAutomationReference, func(refs mediaapp.ImageDeleteReferences) bool { return len(refs.AutomationAgents) == 1 }},
		{"channel", seedDeleteChannelReference, func(refs mediaapp.ImageDeleteReferences) bool { return len(refs.Channels) == 1 }},
		{"radar", seedDeleteRadarReference, func(refs mediaapp.ImageDeleteReferences) bool { return len(refs.RadarLinks) == 1 }},
		{"incomplete preflight", seedDeletePreflightReference, func(refs mediaapp.ImageDeleteReferences) bool { return len(refs.ImportPreflights) == 1 }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			actor := int64(9370)
			created, err := realService(pool).Upload(ctx, command(t, actor, unique("reference-upload-key"), unique("reference-image")))
			if err != nil {
				t.Fatal(err)
			}
			ownerID := fixture.seed(t, ctx, pool, created.ID)
			for _, force := range []bool{false, true} {
				before := deleteReferenceOwnerCount(t, ctx, pool, fixture.name, ownerID)
				result, deleteErr := realImageDeleteService(pool).DeleteImage(ctx, mediaapp.ImageDeleteCommand{ImageID: created.ID, Actor: actor, Force: force, IdempotencyKey: unique("reference-delete-key")})
				if !errors.Is(deleteErr, mediaapp.ErrImageHasReferences) || !fixture.match(result.References) || !imageReferenceListsSorted(result.References) || deleteReferenceOwnerCount(t, ctx, pool, fixture.name, ownerID) != before {
					t.Fatalf("force=%v result=%#v err=%v before=%d", force, result, deleteErr, before)
				}
			}
			var imageRows, receipts, events int
			if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM media_images WHERE id=$1), (SELECT count(*) FROM media_image_delete_receipts WHERE business_key=$1::bigint::text), (SELECT count(*) FROM event_log WHERE event_type='media.image_deleted' AND payload->>'image_id'=$1::bigint::text)`, created.ID).Scan(&imageRows, &receipts, &events); err != nil || imageRows != 1 || receipts != 0 || events != 0 {
				t.Fatalf("conflict facts=%d/%d/%d err=%v", imageRows, receipts, events, err)
			}
		})
	}
}

func TestImageDelete0362PostgreSQLMalformedJSONReferencesFailClosed(t *testing.T) {
	pool, ctx := openPool(t)
	for _, owner := range []struct {
		name   string
		seed   func(*testing.T, context.Context, *pgxpool.Pool, string) int64
		remove func(*testing.T, context.Context, *pgxpool.Pool, int64)
	}{
		{name: "automation", seed: seedMalformedAutomationReference, remove: removeMalformedAutomationReference},
		{name: "channel", seed: seedMalformedChannelReference, remove: removeMalformedChannelReference},
	} {
		for _, malformed := range []struct {
			name string
			raw  func(int64) string
		}{
			{name: "leading zero string", raw: func(int64) string { return `["042"]` }},
			{name: "plus string", raw: func(int64) string { return `["+42"]` }},
			{name: "whitespace string", raw: func(int64) string { return `[" 42"]` }},
			{name: "object", raw: func(int64) string { return `[{}]` }},
			{name: "illegal string", raw: func(int64) string { return `["image-42"]` }},
			{name: "mixed valid and invalid", raw: func(imageID int64) string { return fmt.Sprintf(`[%d,"042"]`, imageID) }},
		} {
			t.Run(owner.name+"/"+malformed.name, func(t *testing.T) {
				actor := int64(9378)
				created, err := realService(pool).Upload(ctx, command(t, actor, unique("malformed-reference-upload"), unique("malformed-reference-image")))
				if err != nil {
					t.Fatal(err)
				}
				ownerID := owner.seed(t, ctx, pool, malformed.raw(created.ID))
				defer owner.remove(t, ctx, pool, ownerID)

				result, deleteErr := realImageDeleteService(pool).DeleteImage(ctx, mediaapp.ImageDeleteCommand{ImageID: created.ID, Actor: actor, IdempotencyKey: unique("malformed-reference-delete")})
				if !errors.Is(deleteErr, mediaapp.ErrImageDeleteUnavailable) || result.ID != 0 || result.Deleted || result.HardDeleted || result.References.Any() {
					t.Fatalf("delete result=%#v err=%v", result, deleteErr)
				}
				var images, receipts, events int
				if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM media_images WHERE id=$1), (SELECT count(*) FROM media_image_delete_receipts WHERE business_key=$1::bigint::text), (SELECT count(*) FROM event_log WHERE event_type='media.image_deleted' AND payload->>'image_id'=$1::bigint::text)`, created.ID).Scan(&images, &receipts, &events); err != nil || images != 1 || receipts != 0 || events != 0 {
					t.Fatalf("failed-close facts=%d/%d/%d err=%v", images, receipts, events, err)
				}
			})
		}
	}
}

func TestImageDelete0362PostgreSQLLegalJSONReferencesRemainAscending(t *testing.T) {
	pool, ctx := openPool(t)
	created, err := realService(pool).Upload(ctx, command(t, 9379, unique("ascending-reference-upload"), unique("ascending-reference-image")))
	if err != nil {
		t.Fatal(err)
	}
	firstAgent := seedDeleteAutomationReference(t, ctx, pool, created.ID)
	secondAgent := seedDeleteAutomationReference(t, ctx, pool, created.ID)
	firstChannel := seedDeleteChannelReference(t, ctx, pool, created.ID)
	secondChannel := seedDeleteChannelReference(t, ctx, pool, created.ID)
	defer removeMalformedAutomationReference(t, ctx, pool, secondAgent)
	defer removeMalformedAutomationReference(t, ctx, pool, firstAgent)
	defer removeMalformedChannelReference(t, ctx, pool, secondChannel)
	defer removeMalformedChannelReference(t, ctx, pool, firstChannel)

	var agentIDs, channelIDs []int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		var readErr error
		agentIDs, readErr = automationstore.NewAgentRepository().ListImageReferenceAgentIDs(tx, created.ID)
		if readErr != nil {
			return readErr
		}
		channelIDs, readErr = contactstore.NewChannelRepository().ListImageReferenceChannelIDs(tx, created.ID)
		return readErr
	})
	if err != nil || len(agentIDs) != 2 || agentIDs[0] != firstAgent || agentIDs[1] != secondAgent || len(channelIDs) != 2 || channelIDs[0] != firstChannel || channelIDs[1] != secondChannel {
		t.Fatalf("reference ids=%v/%v err=%v", agentIDs, channelIDs, err)
	}
}

func TestImageDelete0362PostgreSQLSameKeyConvergesToOneMutation(t *testing.T) {
	pool, ctx := openPool(t)
	created, err := realService(pool).Upload(ctx, command(t, 9381, unique("concurrent-delete-upload"), unique("concurrent-delete")))
	if err != nil {
		t.Fatal(err)
	}
	commandValue := mediaapp.ImageDeleteCommand{ImageID: created.ID, Actor: 9381, IdempotencyKey: unique("concurrent-delete-key")}
	results := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, deleteErr := realImageDeleteService(pool).DeleteImage(ctx, commandValue)
			results <- deleteErr
		}()
	}
	group.Wait()
	close(results)
	for deleteErr := range results {
		if deleteErr != nil {
			t.Fatalf("concurrent delete err=%v", deleteErr)
		}
	}
	var receipts, events int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM media_image_delete_receipts WHERE business_key=$1::bigint::text), (SELECT count(*) FROM event_log WHERE event_type='media.image_deleted' AND payload->>'image_id'=$1::bigint::text)`, created.ID).Scan(&receipts, &events); err != nil || receipts != 1 || events != 1 {
		t.Fatalf("concurrent facts=%d/%d err=%v", receipts, events, err)
	}
}

func TestImageDelete0362PostgreSQLReferenceWritersCoordinateWithDeleteLock(t *testing.T) {
	pool, ctx := openPool(t)
	actor := int64(9391)
	created, err := realService(pool).Upload(ctx, command(t, actor, unique("writer-lock-upload"), unique("writer-lock-image")))
	if err != nil {
		t.Fatal(err)
	}
	// A reference writer which commits before deletion is an observed 409.
	beforeDelete := miniProgramCreateCommand(actor, unique("writer-before-delete"), unique("writer-before"), unique("writer-before"))
	beforeDelete.ThumbnailImageID = &created.ID
	doNotResolve := false
	beforeDelete.ResolveThumbMedia = &doNotResolve
	if _, err = realMiniProgramService(pool).Create(ctx, beforeDelete); err != nil {
		t.Fatal(err)
	}
	if _, err = realImageDeleteService(pool).DeleteImage(ctx, mediaapp.ImageDeleteCommand{ImageID: created.ID, Actor: actor, IdempotencyKey: unique("writer-before-delete-key")}); !errors.Is(err, mediaapp.ErrImageHasReferences) {
		t.Fatalf("committed reference delete error=%v", err)
	}

	// A writer which starts after the delete lock is held cannot add a JSON or
	// FK reference: its FOR KEY SHARE validation resumes after hard deletion and
	// reports the image as missing.
	raceImage, err := realService(pool).Upload(ctx, command(t, actor, unique("writer-race-upload"), unique("writer-race-image")))
	if err != nil {
		t.Fatal(err)
	}
	locked, release := make(chan struct{}, 1), make(chan struct{})
	store := &imageDeleteLockObserver{UploadRepository: mediastore.NewUploadRepository(), locked: locked, release: release}
	deleteResults := make(chan error, 1)
	go func() {
		_, deleteErr := mediaapp.NewImageDeleteService(platformstore.NewUnitOfWork(pool), store, automationstore.NewAgentRepository(), contactstore.NewChannelRepository(), radarstore.NewPostgresRepository(), eventstore.NewAppender()).DeleteImage(ctx, mediaapp.ImageDeleteCommand{ImageID: raceImage.ID, Actor: actor, IdempotencyKey: unique("writer-race-delete")})
		deleteResults <- deleteErr
	}()
	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not acquire image row lock")
	}
	writerResults := make(chan error, 1)
	go func() {
		mutation := miniProgramCreateCommand(actor, unique("writer-after-delete"), unique("writer-after"), unique("writer-after"))
		mutation.ThumbnailImageID, mutation.ResolveThumbMedia = &raceImage.ID, &doNotResolve
		_, writerErr := realMiniProgramService(pool).Create(ctx, mutation)
		writerResults <- writerErr
	}()
	select {
	case writerErr := <-writerResults:
		t.Fatalf("writer completed before delete lock release: %v", writerErr)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case deleteErr := <-deleteResults:
		if deleteErr != nil {
			t.Fatalf("delete result=%v", deleteErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("delete did not finish")
	}
	select {
	case writerErr := <-writerResults:
		if !errors.Is(writerErr, mediaapp.ErrMiniProgramImageNotFound) {
			t.Fatalf("writer error=%v", writerErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("writer did not finish")
	}
}

func TestImageDelete0362PostgreSQLRadarCoverWriterCoordinatesWithDeleteLock(t *testing.T) {
	pool, ctx := openPool(t)
	actor := int64(9392)
	raceImage, err := realService(pool).Upload(ctx, command(t, actor, unique("radar-race-upload"), unique("radar-race-image")))
	if err != nil {
		t.Fatal(err)
	}
	locked, release := make(chan struct{}, 1), make(chan struct{})
	store := &imageDeleteLockObserver{UploadRepository: mediastore.NewUploadRepository(), locked: locked, release: release}
	deleteResults := make(chan error, 1)
	go func() {
		_, deleteErr := mediaapp.NewImageDeleteService(platformstore.NewUnitOfWork(pool), store, automationstore.NewAgentRepository(), contactstore.NewChannelRepository(), radarstore.NewPostgresRepository(), eventstore.NewAppender()).DeleteImage(ctx, mediaapp.ImageDeleteCommand{ImageID: raceImage.ID, Actor: actor, IdempotencyKey: unique("radar-race-delete")})
		deleteResults <- deleteErr
	}()
	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not acquire image row lock")
	}
	writerResults := make(chan error, 1)
	go func() {
		service, serviceErr := radarapp.NewServiceWithImageReferences(platformstore.NewUnitOfWork(pool), radarstore.NewPostgresRepository(), mediastore.NewUploadRepository(), eventstore.NewAppender())
		if serviceErr != nil {
			writerResults <- serviceErr
			return
		}
		_, writerErr := service.Create(ctx, radarport.CreateCommand{ExpectedVersion: 0, Name: unique("radar-race"), Title: "Radar race", DestinationURL: "https://example.com/radar-race", CoverImageID: &raceImage.ID, ActorID: actor, IdempotencyKey: unique("radar-race-writer")})
		writerResults <- writerErr
	}()
	select {
	case writerErr := <-writerResults:
		t.Fatalf("radar writer completed before delete lock release: %v", writerErr)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case deleteErr := <-deleteResults:
		if deleteErr != nil {
			t.Fatalf("delete result=%v", deleteErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("delete did not finish")
	}
	select {
	case writerErr := <-writerResults:
		if !errors.Is(writerErr, radarport.ErrInvalidArgument) {
			t.Fatalf("radar writer error=%v", writerErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("radar writer did not finish")
	}
	var links int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM radar_links WHERE cover_image_id=$1`, raceImage.ID).Scan(&links); err != nil || links != 0 {
		t.Fatalf("radar links=%d err=%v", links, err)
	}
}

func realImageDeleteService(pool *pgxpool.Pool) *mediaapp.ImageDeleteService {
	return mediaapp.NewImageDeleteService(platformstore.NewUnitOfWork(pool), mediastore.NewUploadRepository(), automationstore.NewAgentRepository(), contactstore.NewChannelRepository(), radarstore.NewPostgresRepository(), eventstore.NewAppender())
}

type imageDeleteLockObserver struct {
	*mediastore.UploadRepository
	locked  chan<- struct{}
	release <-chan struct{}
}

func (store *imageDeleteLockObserver) LockImageForDelete(ctx context.Context, imageID int64) (bool, error) {
	exists, err := store.UploadRepository.LockImageForDelete(ctx, imageID)
	if err != nil || !exists {
		return exists, err
	}
	select {
	case store.locked <- struct{}{}:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	select {
	case <-store.release:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func seedDeleteMiniprogramReference(t *testing.T, ctx context.Context, pool *pgxpool.Pool, imageID int64) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO media_miniprograms (name,app_id,page_path,title,thumbnail_image_id,enabled,created_by,updated_by,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,true,1,1,now(),now()) RETURNING id`, unique("delete-mini"), unique("wx-delete"), "pages/delete", unique("delete mini"), imageID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedDeleteArchivedGroupInviteReference(t *testing.T, ctx context.Context, pool *pgxpool.Pool, imageID int64) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO media_group_invites (name,title,description,join_url,cover_image_id,enabled,created_by,updated_by,created_at,updated_at,archived_at) VALUES ($1,$2,'',$3,$4,false,1,1,now(),now(),now()) RETURNING id`, unique("delete-invite"), unique("delete invite"), "https://work.weixin.qq.com/gm/delete", imageID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedDeleteAutomationReference(t *testing.T, ctx context.Context, pool *pgxpool.Pool, imageID int64) int64 {
	t.Helper()
	id, err := automationfixture.CreateImageReference(ctx, pool, unique("delete-agent"), unique("delete-agent-code"), imageID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedDeleteChannelReference(t *testing.T, ctx context.Context, pool *pgxpool.Pool, imageID int64) int64 {
	t.Helper()
	id, err := contactfixture.CreateImageReference(ctx, pool, unique("delete-channel"), unique("delete-channel-code"), imageID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedDeleteRadarReference(t *testing.T, ctx context.Context, pool *pgxpool.Pool, imageID int64) int64 {
	t.Helper()
	code := fmt.Sprintf("rd_%022d", imageID)
	id, err := radarfixture.CreateDraftLink(ctx, pool, code, unique("delete-radar"), unique("delete radar"), "https://example.com/delete-radar", imageID, 1)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedMalformedAutomationReference(t *testing.T, ctx context.Context, pool *pgxpool.Pool, raw string) int64 {
	t.Helper()
	id, err := automationfixture.CreateImageReferenceWithRawIDs(ctx, pool, unique("malformed-agent"), unique("malformed-agent-code"), raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func removeMalformedAutomationReference(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64) {
	t.Helper()
	if err := automationfixture.DeleteImageReference(ctx, pool, id); err != nil {
		t.Fatal(err)
	}
}

func seedMalformedChannelReference(t *testing.T, ctx context.Context, pool *pgxpool.Pool, raw string) int64 {
	t.Helper()
	id, err := contactfixture.CreateImageReferenceWithRawIDs(ctx, pool, unique("malformed-channel"), unique("malformed-channel-code"), raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func removeMalformedChannelReference(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64) {
	t.Helper()
	if err := contactfixture.DeleteImageReference(ctx, pool, id); err != nil {
		t.Fatal(err)
	}
}

func seedDeletePreflightReference(t *testing.T, ctx context.Context, pool *pgxpool.Pool, imageID int64) int64 {
	t.Helper()
	digest := sha256.Sum256([]byte(unique("delete-preflight")))
	var preflightID, miniID int64
	if err := pool.QueryRow(ctx, `INSERT INTO media_miniprogram_import_preflights (source_snapshot_digest,source_row_count,url_only_row_count,unresolved_image_row_count,state,recorded_at) VALUES ($1,1,0,0,'external_gate_required',now()) RETURNING id`, digest[:]).Scan(&preflightID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights SET state='ready',external_gate_ref='fixture-delete-preflight',ready_at=now() WHERE id=$1`, preflightID); err != nil {
		t.Fatal(err)
	}
	legacyID := time.Now().UnixMicro()
	if err := pool.QueryRow(ctx, `INSERT INTO media_miniprograms (legacy_source_id,name,app_id,page_path,title,thumbnail_image_id,enabled,created_by,updated_by,created_at,updated_at) VALUES ($1,$2,$3,'pages/delete',$2,$4,true,1,1,now(),now()) RETURNING id`, legacyID, unique("delete-preflight-mini"), unique("wx-preflight"), imageID).Scan(&miniID); err != nil {
		t.Fatal(err)
	}
	rowDigest := sha256.Sum256([]byte(unique("delete-preflight-row")))
	if _, err := pool.Exec(ctx, `INSERT INTO media_miniprogram_import_ledger (preflight_id,legacy_source_id,source_row_digest,disposition,target_miniprogram_id,image_disposition,legacy_thumbnail_image_id,target_media_image_id,source_url_only,source_image_unresolved,provider_cache_disposition,reason,recorded_at) VALUES ($1,$2,$3,'migrated',$4,'remapped',1,$5,false,false,'dropped','delete preflight reference',now())`, preflightID, legacyID, rowDigest[:], miniID, imageID); err != nil {
		t.Fatal(err)
	}
	return preflightID
}

func deleteReferenceOwnerCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, id int64) int {
	t.Helper()
	if name == "automation agent" {
		count, err := automationfixture.CountImageReferences(ctx, pool, id)
		if err != nil {
			t.Fatal(err)
		}
		return count
	}
	if name == "channel" {
		count, err := contactfixture.CountImageReferences(ctx, pool, id)
		if err != nil {
			t.Fatal(err)
		}
		return count
	}
	table := map[string]string{"miniprogram": "media_miniprograms", "archived group invite": "media_group_invites", "radar": "radar_links", "incomplete preflight": "media_miniprogram_import_preflights"}[name]
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id=$1`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func imageReferenceListsSorted(refs mediaapp.ImageDeleteReferences) bool {
	for _, ids := range [][]int64{refs.Miniprograms, refs.CampaignSteps, refs.GroupInvites, refs.AutomationAgents, refs.Channels, refs.RadarLinks, refs.ImportPreflights} {
		for index, id := range ids {
			if id < 1 || index > 0 && ids[index-1] >= id {
				return false
			}
		}
	}
	return true
}
