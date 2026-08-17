package media_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestMiniProgramR1CRUDReplayConflictListAndPhysicalDelete(t *testing.T) {
	pool, ctx := openPool(t)
	service := realMiniProgramService(pool)
	createKey := unique("miniprogram-create")
	created, err := service.Create(ctx, miniProgramCreateCommand(6101, createKey, "", "新人欢迎卡"))
	if err != nil || !created.Changed || created.Item.Name != "新人欢迎卡" || created.Item.Title != "新人欢迎卡" {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	replayed, err := service.Create(ctx, miniProgramCreateCommand(6101, createKey, "", "新人欢迎卡"))
	if err != nil || replayed.Item.ID != created.Item.ID || !replayed.Changed {
		t.Fatalf("replayed=%#v err=%v", replayed, err)
	}
	conflict := miniProgramCreateCommand(6101, createKey, "", "不同标题")
	if _, err = service.Create(ctx, conflict); !errors.Is(err, mediaapp.ErrMiniProgramConflict) {
		t.Fatalf("create conflict err=%v", err)
	}

	emptyName := ""
	updated, err := service.Update(ctx, mediaport.MiniProgramUpdateCommand{ID: created.Item.ID,
		MiniProgramPatch: mediaport.MiniProgramPatch{Name: &emptyName}, Actor: 6101, IdempotencyKey: unique("miniprogram-update")})
	if err != nil || !updated.Changed || updated.Item.Name != "" || updated.Item.Title != "新人欢迎卡" {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	emptyTitle := ""
	if _, err = service.Update(ctx, mediaport.MiniProgramUpdateCommand{ID: created.Item.ID,
		MiniProgramPatch: mediaport.MiniProgramPatch{Title: &emptyTitle}, Actor: 6101, IdempotencyKey: unique("miniprogram-empty-title")}); !errors.Is(err, mediaapp.ErrInvalidMiniProgramOperation) {
		t.Fatalf("empty title err=%v", err)
	}
	unchanged, err := service.Get(ctx, created.Item.ID)
	if err != nil || unchanged.Title != "新人欢迎卡" || unchanged.Version != updated.Item.Version {
		t.Fatalf("unchanged=%#v err=%v", unchanged, err)
	}

	page, err := service.List(ctx, mediaport.MiniProgramListQuery{Limit: 1, Offset: 0})
	if err != nil || len(page.Items) != 1 || page.Total < 1 || page.Limit != 1 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	emptyPage, err := service.List(ctx, mediaport.MiniProgramListQuery{Limit: 100, Offset: int32(page.Total + 10)})
	if err != nil || len(emptyPage.Items) != 0 || emptyPage.Total != page.Total {
		t.Fatalf("empty page=%#v err=%v", emptyPage, err)
	}

	deleteKey := unique("miniprogram-delete")
	deleted, err := service.Delete(ctx, mediaport.MiniProgramDeleteCommand{ID: created.Item.ID, Actor: 6101, IdempotencyKey: deleteKey})
	if err != nil || !deleted.Deleted || deleted.ID != created.Item.ID {
		t.Fatalf("deleted=%#v err=%v", deleted, err)
	}
	deletedReplay, err := service.Delete(ctx, mediaport.MiniProgramDeleteCommand{ID: created.Item.ID, Actor: 6101, IdempotencyKey: deleteKey})
	if err != nil || deletedReplay != deleted {
		t.Fatalf("delete replay=%#v err=%v", deletedReplay, err)
	}
	if _, err = service.Get(ctx, created.Item.ID); !errors.Is(err, mediaapp.ErrMiniProgramNotFound) {
		t.Fatalf("physical delete get err=%v", err)
	}
	assertMiniProgramReceiptAndEventCount(t, pool, ctx, 6101, createKey, "create", "create", 1, 1)
	assertMiniProgramReceiptAndEventCount(t, pool, ctx, 6101, deleteKey, "delete", fmt.Sprintf("%d", created.Item.ID), 1, 1)
}

func TestMiniProgramR1ResolvedCacheBindsCreateChoiceAndRejectsClientMediaID(t *testing.T) {
	pool, ctx := openPool(t)
	imageID := createCover(t, pool, ctx, 6201)
	expiresAt := time.Now().UTC().Add(30 * time.Minute).Truncate(time.Microsecond)
	cacheReceipt := unique("cache-resolved")
	if _, err := pool.Exec(ctx, `INSERT INTO media_thumbnail_cache_entries
		(image_id,state,cache_receipt,media_id,expires_at,updated_at) VALUES ($1,'resolved',$2,'local-media-cache-id',$3,now())`,
		imageID, cacheReceipt, expiresAt); err != nil {
		t.Fatal(err)
	}
	resolve := true
	key := unique("resolved-create")
	command := miniProgramCreateCommand(6201, key, "素材", "素材")
	command.ThumbnailImageID = &imageID
	command.ResolveThumbMedia = &resolve
	result, err := realMiniProgramService(pool).Create(ctx, command)
	if err != nil || result.ThumbnailResolve == nil || result.ThumbnailResolve.Status != mediaport.ThumbnailResolved ||
		result.ThumbnailResolve.RealExternalCallExecuted || result.ThumbnailResolve.SideEffectExecuted ||
		result.Item.ThumbnailMediaID != "local-media-cache-id" || result.Item.ThumbnailMediaExpiresAt == nil {
		t.Fatalf("resolved=%#v err=%v", result, err)
	}
	doNotResolve := false
	changedChoice := command
	changedChoice.ResolveThumbMedia = &doNotResolve
	if _, err = realMiniProgramService(pool).Create(ctx, changedChoice); !errors.Is(err, mediaapp.ErrMiniProgramConflict) {
		t.Fatalf("true/false receipt conflict err=%v", err)
	}

	for _, supplied := range []mediaport.OptionalString{{Present: true}, {Present: true, Value: pointer("client-media-id")}} {
		before := countMiniProgramFacts(t, pool, ctx)
		rejected := miniProgramCreateCommand(6202, unique("client-media-id"), "拒绝", "拒绝")
		rejected.ThumbMediaID = supplied
		if _, err = realMiniProgramService(pool).Create(ctx, rejected); !errors.Is(err, mediaapp.ErrInvalidMiniProgramOperation) {
			t.Fatalf("supplied=%#v err=%v", supplied, err)
		}
		after := countMiniProgramFacts(t, pool, ctx)
		if after != before {
			t.Fatalf("client media id changed facts before=%#v after=%#v", before, after)
		}
	}
	var persistedReceipt, persistedMedia string
	if err = pool.QueryRow(ctx, `SELECT cache_receipt,media_id FROM media_thumbnail_cache_entries WHERE image_id=$1`, imageID).
		Scan(&persistedReceipt, &persistedMedia); err != nil || persistedReceipt != cacheReceipt || persistedMedia != "local-media-cache-id" {
		t.Fatalf("cache mutated receipt=%q media=%q err=%v", persistedReceipt, persistedMedia, err)
	}
}

func TestMiniProgramR1OutcomeUnknownCompletesOnceAndNeverRetries(t *testing.T) {
	pool, ctx := openPool(t)
	imageID := createCover(t, pool, ctx, 6301)
	if _, err := pool.Exec(ctx, `INSERT INTO media_thumbnail_cache_entries
		(image_id,state,cache_receipt,media_id,expires_at,updated_at) VALUES ($1,'outcome_unknown',$2,'',NULL,now())`,
		imageID, unique("cache-unknown")); err != nil {
		t.Fatal(err)
	}
	create := miniProgramCreateCommand(6301, unique("unknown-base"), "未知缓存", "未知缓存")
	create.ThumbnailImageID = &imageID
	doNotResolve := false
	create.ResolveThumbMedia = &doNotResolve
	created, err := realMiniProgramService(pool).Create(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	repository := mediastore.NewMiniProgramRepository()
	resolver := &countingMiniProgramResolver{delegate: repository}
	service := mediaapp.NewMiniProgramService(platformstore.NewUnitOfWork(pool), repository, repository, eventstore.NewAppender(), resolver)
	key := unique("unknown-resolve")
	command := mediaport.MiniProgramResolveThumbnailCommand{ID: created.Item.ID, Actor: 6301, IdempotencyKey: key}
	first, err := service.ResolveThumbnail(ctx, command)
	if err != nil || first.Resolution.Status != mediaport.ThumbnailOutcomeUnknown || first.Changed || resolver.calls.Load() != 1 {
		t.Fatalf("first=%#v calls=%d err=%v", first, resolver.calls.Load(), err)
	}
	replay, err := service.ResolveThumbnail(ctx, command)
	if err != nil || replay.Resolution.Status != mediaport.ThumbnailOutcomeUnknown || replay.Changed || resolver.calls.Load() != 1 {
		t.Fatalf("replay=%#v calls=%d err=%v", replay, resolver.calls.Load(), err)
	}
	assertMiniProgramReceiptAndEventCount(t, pool, ctx, 6301, key, "test-resolve", fmt.Sprintf("%d", created.Item.ID), 1, 0)
}

func TestMiniProgramR1EventFailureRollsBackFactReceiptAndEvent(t *testing.T) {
	pool, ctx := openPool(t)
	repository := mediastore.NewMiniProgramRepository()
	service := mediaapp.NewMiniProgramService(platformstore.NewUnitOfWork(pool), repository, repository, failingAppender{}, repository)
	key := unique("rollback")
	command := miniProgramCreateCommand(6401, key, "回滚", "回滚")
	before := countMiniProgramFacts(t, pool, ctx)
	if _, err := service.Create(ctx, command); !errors.Is(err, mediaapp.ErrMiniProgramUnavailable) {
		t.Fatalf("event failure err=%v", err)
	}
	after := countMiniProgramFacts(t, pool, ctx)
	if after != before {
		t.Fatalf("event failure leaked facts before=%#v after=%#v", before, after)
	}
	assertMiniProgramReceiptAndEventCount(t, pool, ctx, 6401, key, "create", "create", 0, 0)
}

func TestMiniProgramR1ConcurrentCreateProducesOneFactReceiptAndEvent(t *testing.T) {
	pool, ctx := openPool(t)
	service := realMiniProgramService(pool)
	command := miniProgramCreateCommand(6501, unique("concurrent"), "并发", "并发")
	const workers = 16
	results := make(chan mediaport.MiniProgramMutationResult, workers)
	errorsChannel := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := service.Create(ctx, command)
			results <- result
			errorsChannel <- err
		}()
	}
	group.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("concurrent err=%v", err)
		}
	}
	var id int64
	for result := range results {
		if id == 0 {
			id = result.Item.ID
		}
		if result.Item.ID != id {
			t.Fatalf("concurrent ids=%d/%d", id, result.Item.ID)
		}
	}
	var facts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM media_miniprograms WHERE id=$1`, id).Scan(&facts); err != nil || facts != 1 {
		t.Fatalf("facts=%d err=%v", facts, err)
	}
	assertMiniProgramReceiptAndEventCount(t, pool, ctx, command.Actor, command.IdempotencyKey, "create", "create", 1, 1)
}

func TestMiniProgramR1PreflightAndImmutableSourceLedgerAreFailClosed(t *testing.T) {
	pool, ctx := openPool(t)
	digest := sha256.Sum256([]byte(unique("snapshot")))
	rowDigest := sha256.Sum256([]byte(unique("row")))
	if _, err := pool.Exec(ctx, `INSERT INTO media_miniprogram_import_preflights
		(source_snapshot_digest,source_row_count,url_only_row_count,unresolved_image_row_count,state,recorded_at,ready_at,completed_at)
		VALUES ($1,0,0,0,'completed',now(),now(),now())`, digest[:]); err == nil {
		t.Fatal("direct completed preflight insert was accepted")
	}
	var preflightID int64
	if err := pool.QueryRow(ctx, `INSERT INTO media_miniprogram_import_preflights
		(source_snapshot_digest,source_row_count,url_only_row_count,unresolved_image_row_count,state,recorded_at)
		VALUES ($1,1,0,0,'external_gate_required',now()) RETURNING id`, digest[:]).Scan(&preflightID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights SET state='ready',ready_at=now() WHERE id=$1`, preflightID); err == nil {
		t.Fatal("ready transition without external gate evidence was accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights
		SET state='ready',external_gate_ref='EXTERNAL-fixture-read-only',ready_at=now() WHERE id=$1`, preflightID); err != nil {
		t.Fatalf("external gate transition rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights SET state='completed',completed_at=now() WHERE id=$1`, preflightID); err == nil {
		t.Fatal("preflight completed without ledger coverage")
	}
	legacySourceID := time.Now().UnixMicro()
	var targetID int64
	if err := pool.QueryRow(ctx, `INSERT INTO media_miniprograms
		(legacy_source_id,name,app_id,page_path,title,enabled,created_by,updated_by,created_at,updated_at)
		VALUES ($1,'legacy','wx-legacy','pages/legacy','legacy',true,6601,6601,now(),now()) RETURNING id`, legacySourceID).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO media_miniprogram_import_ledger
		(preflight_id,legacy_source_id,source_row_digest,disposition,target_miniprogram_id,image_disposition,
		 source_url_only,source_image_unresolved,provider_cache_disposition,reason,recorded_at)
		VALUES ($1,$2,$3,'migrated',$4,'none',false,false,'dropped','fixture reconciliation only',now())`,
		preflightID, legacySourceID, rowDigest[:], targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO media_miniprogram_import_ledger
		(preflight_id,legacy_source_id,source_row_digest,disposition,target_miniprogram_id,image_disposition,
		 source_url_only,source_image_unresolved,provider_cache_disposition,reason,recorded_at)
		VALUES ($1,$2,$3,'migrated',$4,'none',false,false,'dropped','mismatch must fail',now())`,
		preflightID, legacySourceID+1, rowDigest[:], targetID); err == nil {
		t.Fatal("source/target mismatch was accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_ledger SET reason='tampered' WHERE preflight_id=$1`, preflightID); err == nil {
		t.Fatal("immutable ledger update was accepted")
	}
	deleteBeforeCompletion, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = deleteBeforeCompletion.Exec(ctx, `DELETE FROM media_miniprograms WHERE id=$1`, targetID); err != nil {
		t.Fatal(err)
	}
	if _, err = deleteBeforeCompletion.Exec(ctx, `UPDATE media_miniprogram_import_preflights SET state='completed',completed_at=now() WHERE id=$1`, preflightID); err == nil {
		t.Fatal("preflight completed after migrated target disappeared")
	}
	if err = deleteBeforeCompletion.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights SET state='completed',completed_at=now() WHERE id=$1`, preflightID); err != nil {
		t.Fatalf("reconciled completion rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights SET source_row_count=2 WHERE id=$1`, preflightID); err == nil {
		t.Fatal("completed preflight snapshot mutation was accepted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO media_miniprogram_import_ledger
		(preflight_id,legacy_source_id,source_row_digest,disposition,target_miniprogram_id,image_disposition,
		 source_url_only,source_image_unresolved,provider_cache_disposition,reason,recorded_at)
		VALUES ($1,$2,$3,'migrated',$4,'none',false,false,'dropped','late row',now())`,
		preflightID, legacySourceID, rowDigest[:], targetID); err == nil {
		t.Fatal("ledger mutation after completion was accepted")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM media_miniprograms WHERE id=$1`, targetID); err != nil {
		t.Fatalf("legacy physical delete blocked by audit ledger: %v", err)
	}
	var ledgerRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM media_miniprogram_import_ledger WHERE preflight_id=$1 AND legacy_source_id=$2`, preflightID, legacySourceID).Scan(&ledgerRows); err != nil || ledgerRows != 1 {
		t.Fatalf("ledger rows=%d err=%v", ledgerRows, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM media_miniprogram_import_preflights WHERE id=$1`, preflightID); err == nil {
		t.Fatal("completed preflight deletion was accepted")
	}
}

func TestMiniProgramR1URLOnlyDecisionAndLineageRemainDurableAfterPhysicalDelete(t *testing.T) {
	pool, ctx := openPool(t)
	digest := sha256.Sum256([]byte(unique("url-only-snapshot")))
	rowDigest := sha256.Sum256([]byte(unique("url-only-row")))
	var preflightID int64
	if err := pool.QueryRow(ctx, `INSERT INTO media_miniprogram_import_preflights
		(source_snapshot_digest,source_row_count,url_only_row_count,unresolved_image_row_count,state,recorded_at)
		VALUES ($1,1,1,1,'external_gate_required',now()) RETURNING id`, digest[:]).Scan(&preflightID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights
		SET state='human_decision_required',external_gate_ref='EXTERNAL-fixture-url-only' WHERE id=$1`, preflightID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights SET state='ready',ready_at=now() WHERE id=$1`, preflightID); err == nil {
		t.Fatal("URL-only ready transition without HUMAN decision was accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights
		SET state='ready',url_only_decision='retain_metadata_without_fetch',human_decision_ref='HUMAN-fixture-retain',ready_at=now()
		WHERE id=$1`, preflightID); err != nil {
		t.Fatalf("URL-only HUMAN transition rejected: %v", err)
	}
	legacySourceID := time.Now().UnixMicro()
	var targetID int64
	if err := pool.QueryRow(ctx, `INSERT INTO media_miniprograms
		(legacy_source_id,name,app_id,page_path,title,thumbnail_image_url,enabled,created_by,updated_by,created_at,updated_at)
		VALUES ($1,'url-only','wx-url-only','pages/url-only','url-only','https://metadata.invalid/not-fetched',true,6602,6602,now(),now()) RETURNING id`,
		legacySourceID).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO media_miniprogram_import_ledger
		(preflight_id,legacy_source_id,source_row_digest,disposition,target_miniprogram_id,image_disposition,
		 source_url_only,source_image_unresolved,provider_cache_disposition,reason,recorded_at)
		VALUES ($1,$2,$3,'quarantined',NULL,'metadata_url_only',true,true,'dropped','wrong HUMAN choice',now())`,
		preflightID, legacySourceID, rowDigest[:]); err == nil {
		t.Fatal("URL-only ledger contradicting HUMAN decision was accepted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO media_miniprogram_import_ledger
		(preflight_id,legacy_source_id,source_row_digest,disposition,target_miniprogram_id,image_disposition,
		 source_url_only,source_image_unresolved,provider_cache_disposition,reason,recorded_at)
		VALUES ($1,$2,$3,'migrated',$4,'metadata_url_only',true,true,'dropped','metadata retained without fetch',now())`,
		preflightID, legacySourceID, rowDigest[:], targetID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights SET state='completed',completed_at=now() WHERE id=$1`, preflightID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM media_miniprograms WHERE id=$1`, targetID); err != nil {
		t.Fatal(err)
	}
	var decision, humanRef, imageDisposition, cacheDisposition string
	if err := pool.QueryRow(ctx, `SELECT p.url_only_decision,p.human_decision_ref,l.image_disposition,l.provider_cache_disposition
		FROM media_miniprogram_import_preflights p JOIN media_miniprogram_import_ledger l ON l.preflight_id=p.id
		WHERE p.id=$1`, preflightID).Scan(&decision, &humanRef, &imageDisposition, &cacheDisposition); err != nil ||
		decision != "retain_metadata_without_fetch" || humanRef != "HUMAN-fixture-retain" ||
		imageDisposition != "metadata_url_only" || cacheDisposition != "dropped" {
		t.Fatalf("durable URL-only lineage=%q/%q/%q/%q err=%v", decision, humanRef, imageDisposition, cacheDisposition, err)
	}

	quarantineDigest := sha256.Sum256([]byte(unique("url-only-quarantine-snapshot")))
	quarantineRow := sha256.Sum256([]byte(unique("url-only-quarantine-row")))
	var quarantinePreflightID int64
	if err := pool.QueryRow(ctx, `INSERT INTO media_miniprogram_import_preflights
		(source_snapshot_digest,source_row_count,url_only_row_count,unresolved_image_row_count,state,recorded_at)
		VALUES ($1,1,1,1,'external_gate_required',now()) RETURNING id`, quarantineDigest[:]).Scan(&quarantinePreflightID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights
		SET state='human_decision_required',external_gate_ref='EXTERNAL-fixture-url-quarantine' WHERE id=$1`, quarantinePreflightID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights
		SET state='ready',url_only_decision='quarantine_row',human_decision_ref='HUMAN-fixture-quarantine',ready_at=now()
		WHERE id=$1`, quarantinePreflightID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO media_miniprogram_import_ledger
		(preflight_id,legacy_source_id,source_row_digest,disposition,target_miniprogram_id,image_disposition,
		 source_url_only,source_image_unresolved,provider_cache_disposition,reason,recorded_at)
		VALUES ($1,$2,$3,'quarantined',NULL,'metadata_url_only',true,true,'dropped','URL-only row quarantined without fetch',now())`,
		quarantinePreflightID, legacySourceID+1, quarantineRow[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights SET state='completed',completed_at=now() WHERE id=$1`, quarantinePreflightID); err != nil {
		t.Fatalf("URL-only quarantine reconciliation rejected: %v", err)
	}
}

func TestMiniProgramR1RemapAndRebuildLineageReconcilesBeforeCompletion(t *testing.T) {
	pool, ctx := openPool(t)
	digest := sha256.Sum256([]byte(unique("image-lineage-snapshot")))
	rowOne := sha256.Sum256([]byte(unique("remap-row")))
	rowTwo := sha256.Sum256([]byte(unique("rebuild-row")))
	var preflightID int64
	if err := pool.QueryRow(ctx, `INSERT INTO media_miniprogram_import_preflights
		(source_snapshot_digest,source_row_count,url_only_row_count,unresolved_image_row_count,state,recorded_at)
		VALUES ($1,2,0,1,'external_gate_required',now()) RETURNING id`, digest[:]).Scan(&preflightID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights
		SET state='ready',external_gate_ref='EXTERNAL-fixture-image-lineage',ready_at=now() WHERE id=$1`, preflightID); err != nil {
		t.Fatal(err)
	}
	remappedImageID := createCover(t, pool, ctx, 6603)
	rebuiltImageID := createCover(t, pool, ctx, 6603)
	var rebuildDigest []byte
	if err := pool.QueryRow(ctx, `SELECT image.checksum FROM media_images image
		JOIN media_image_blobs blob ON blob.image_id=image.id AND blob.checksum=image.checksum
		WHERE image.id=$1`, rebuiltImageID).Scan(&rebuildDigest); err != nil || len(rebuildDigest) != sha256.Size {
		t.Fatalf("read rebuilt Media checksum len=%d err=%v", len(rebuildDigest), err)
	}
	legacyOne, legacyTwo := time.Now().UnixMicro(), time.Now().UnixMicro()+1
	var targetOne, targetTwo int64
	for _, item := range []struct {
		legacy, image *int64
		target        *int64
		title         string
	}{{&legacyOne, &remappedImageID, &targetOne, "remapped"}, {&legacyTwo, &rebuiltImageID, &targetTwo, "rebuilt"}} {
		if err := pool.QueryRow(ctx, `INSERT INTO media_miniprograms
			(legacy_source_id,name,app_id,page_path,title,thumbnail_image_id,enabled,created_by,updated_by,created_at,updated_at)
			VALUES ($1,$2,'wx-lineage','pages/lineage',$2,$3,true,6603,6603,now(),now()) RETURNING id`,
			*item.legacy, item.title, *item.image).Scan(item.target); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprograms SET thumbnail_media_id='legacy-provider-cache',
		thumbnail_media_expires_at=now()+interval '30 minutes' WHERE id=$1`, targetOne); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO media_miniprogram_import_ledger
		(preflight_id,legacy_source_id,source_row_digest,disposition,target_miniprogram_id,image_disposition,
		 legacy_thumbnail_image_id,target_media_image_id,source_url_only,source_image_unresolved,provider_cache_disposition,reason,recorded_at)
		VALUES ($1,$2,$3,'migrated',$4,'remapped',70001,$5,false,false,'dropped','cache DROP must bind target',now())`,
		preflightID, legacyOne, rowOne[:], targetOne, remappedImageID); err == nil {
		t.Fatal("ledger claimed provider cache DROP while target retained cache state")
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprograms SET thumbnail_media_id='',thumbnail_media_expires_at=NULL WHERE id=$1`, targetOne); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO media_miniprogram_import_ledger
		(preflight_id,legacy_source_id,source_row_digest,disposition,target_miniprogram_id,image_disposition,
		 legacy_thumbnail_image_id,target_media_image_id,source_url_only,source_image_unresolved,provider_cache_disposition,reason,recorded_at)
		VALUES ($1,$2,$3,'migrated',$4,'remapped',70001,$5,false,false,'dropped','Media-owned remap',now())`,
		preflightID, legacyOne, rowOne[:], targetOne, remappedImageID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO media_miniprogram_import_ledger
		(preflight_id,legacy_source_id,source_row_digest,disposition,target_miniprogram_id,image_disposition,
		 legacy_thumbnail_image_id,target_media_image_id,source_url_only,source_image_unresolved,provider_cache_disposition,reason,recorded_at)
		VALUES ($1,$2,$3,'migrated',$4,'rebuilt',70002,$5,false,true,'dropped','missing rebuild digest',now())`,
		preflightID, legacyTwo, rowTwo[:], targetTwo, rebuiltImageID); err == nil {
		t.Fatal("rebuild lineage without content digest was accepted")
	}
	wrongRebuildDigest := sha256.Sum256([]byte("wrong-rebuilt-media-content"))
	if _, err := pool.Exec(ctx, `INSERT INTO media_miniprogram_import_ledger
		(preflight_id,legacy_source_id,source_row_digest,disposition,target_miniprogram_id,image_disposition,
		 legacy_thumbnail_image_id,target_media_image_id,rebuild_content_digest,source_url_only,source_image_unresolved,
		 provider_cache_disposition,reason,recorded_at)
		VALUES ($1,$2,$3,'migrated',$4,'rebuilt',70002,$5,$6,false,true,'dropped','wrong Media rebuild digest',now())`,
		preflightID, legacyTwo, rowTwo[:], targetTwo, rebuiltImageID, wrongRebuildDigest[:]); err == nil {
		t.Fatal("rebuild digest unrelated to target Media checksum was accepted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO media_miniprogram_import_ledger
		(preflight_id,legacy_source_id,source_row_digest,disposition,target_miniprogram_id,image_disposition,
		 legacy_thumbnail_image_id,target_media_image_id,rebuild_content_digest,source_url_only,source_image_unresolved,
		 provider_cache_disposition,reason,recorded_at)
		VALUES ($1,$2,$3,'migrated',$4,'rebuilt',70002,$5,$6,false,true,'dropped','Media-owned rebuild',now())`,
		preflightID, legacyTwo, rowTwo[:], targetTwo, rebuiltImageID, rebuildDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_images SET checksum=$2 WHERE id=$1`, rebuiltImageID, wrongRebuildDigest[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights SET state='completed',completed_at=now() WHERE id=$1`, preflightID); err == nil {
		t.Fatal("reconciliation completed after target Media checksum drifted from rebuild digest")
	}
	if _, err := pool.Exec(ctx, `UPDATE media_images SET checksum=$2 WHERE id=$1`, rebuiltImageID, rebuildDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprograms SET thumbnail_media_id='local-cache-after-ledger',
		thumbnail_media_expires_at=now()+interval '30 minutes' WHERE id=$1`, targetTwo); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights SET state='completed',completed_at=now() WHERE id=$1`, preflightID); err == nil {
		t.Fatal("reconciliation completed after target acquired unbound provider cache state")
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprograms SET thumbnail_media_id='',thumbnail_media_expires_at=NULL WHERE id=$1`, targetTwo); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_miniprogram_import_preflights SET state='completed',completed_at=now() WHERE id=$1`, preflightID); err != nil {
		t.Fatalf("lineage reconciliation rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM media_miniprograms WHERE id IN ($1,$2)`, targetOne, targetTwo); err != nil {
		t.Fatal(err)
	}
	var remapped, rebuilt, dropped int
	var durableLegacyImageID, durableTargetImageID int64
	var durableRebuildDigest string
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE image_disposition='remapped'),
		count(*) FILTER (WHERE image_disposition='rebuilt' AND octet_length(rebuild_content_digest)=32),
		count(*) FILTER (WHERE provider_cache_disposition='dropped'),
		max(legacy_thumbnail_image_id) FILTER (WHERE image_disposition='rebuilt'),
		max(target_media_image_id) FILTER (WHERE image_disposition='rebuilt'),
		max(encode(rebuild_content_digest,'hex')) FILTER (WHERE image_disposition='rebuilt')
		FROM media_miniprogram_import_ledger WHERE preflight_id=$1`, preflightID).
		Scan(&remapped, &rebuilt, &dropped, &durableLegacyImageID, &durableTargetImageID, &durableRebuildDigest); err != nil ||
		remapped != 1 || rebuilt != 1 || dropped != 2 {
		t.Fatalf("durable remap/rebuild lineage=%d/%d/%d err=%v", remapped, rebuilt, dropped, err)
	}
	if durableLegacyImageID != 70002 || durableTargetImageID != rebuiltImageID ||
		durableRebuildDigest != hex.EncodeToString(rebuildDigest) {
		t.Fatalf("durable rebuild lineage=%d/%d/%q", durableLegacyImageID, durableTargetImageID, durableRebuildDigest)
	}
}

func TestMiniProgramR1S200KReceiptLookupUsesCompositeIndex(t *testing.T) {
	pool, ctx := openPool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO media_miniprogram_operation_receipts
		(operation,actor_scope,business_key,key_digest,payload_digest,state,result_snapshot,created_at,completed_at)
		SELECT 'create','admin:'||g,'create',decode(md5('key-'||g)||md5('key2-'||g),'hex'),
		decode(md5('payload-'||g)||md5('payload2-'||g),'hex'),'completed',jsonb_build_object('command',jsonb_build_object(),'result',jsonb_build_object()),now(),now()
		FROM generate_series(3000000,3199999) AS g`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ANALYZE media_miniprogram_operation_receipts`); err != nil {
		t.Fatal(err)
	}
	plan := explain(t, ctx, tx, `EXPLAIN (FORMAT JSON, COSTS OFF)
		SELECT id,state,result_snapshot FROM media_miniprogram_operation_receipts
		WHERE operation='create' AND actor_scope='admin:3100000' AND business_key='create'
		AND key_digest=decode(md5('key-3100000')||md5('key2-3100000'),'hex')`)
	if strings.Contains(plan, `"Node Type": "Seq Scan"`) || !strings.Contains(plan, `"Node Type": "Index Scan"`) {
		t.Fatalf("illegal S200K receipt plan:\n%s", plan)
	}
}

func TestMiniProgramR1StorageCatalogIsSingleInstanceLocalAndValidated(t *testing.T) {
	pool, ctx := openPool(t)
	var waterline, invalidConstraints, invalidIndexes, tenantColumns, thumbnailFKIndex int
	err := pool.QueryRow(ctx, `SELECT
		(SELECT max(version_id) FROM goose_db_version WHERE is_applied),
		(SELECT count(*) FROM pg_constraint WHERE conrelid IN
			('media_miniprograms'::regclass,'media_thumbnail_cache_entries'::regclass,'media_miniprogram_operation_receipts'::regclass,
			 'media_miniprogram_import_preflights'::regclass,'media_miniprogram_import_ledger'::regclass) AND NOT convalidated),
		(SELECT count(*) FROM pg_index WHERE indrelid IN
			('media_miniprograms'::regclass,'media_thumbnail_cache_entries'::regclass,'media_miniprogram_operation_receipts'::regclass,
			 'media_miniprogram_import_preflights'::regclass,'media_miniprogram_import_ledger'::regclass) AND (NOT indisvalid OR NOT indisready OR NOT indislive)),
		(SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND
			table_name IN ('media_miniprograms','media_thumbnail_cache_entries','media_miniprogram_operation_receipts','media_miniprogram_import_preflights','media_miniprogram_import_ledger') AND
			column_name ~* 'tenant|workspace|organization|corp_id'),
		(to_regclass('public.media_miniprograms_thumbnail_image_id_idx') IS NOT NULL)::int`).
		Scan(&waterline, &invalidConstraints, &invalidIndexes, &tenantColumns, &thumbnailFKIndex)
	if err != nil || waterline != 45 || invalidConstraints != 0 || invalidIndexes != 0 || tenantColumns != 0 || thumbnailFKIndex != 1 {
		t.Fatalf("catalog=%d/%d/%d/%d/%d err=%v", waterline, invalidConstraints, invalidIndexes, tenantColumns, thumbnailFKIndex, err)
	}
}

type miniProgramFactCounts struct{ items, receipts, events int }

type countingMiniProgramResolver struct {
	delegate mediaport.ThumbnailCacheResolver
	calls    atomic.Int32
}

func (resolver *countingMiniProgramResolver) ResolveThumbnailFromCache(ctx context.Context, item mediaport.MiniProgram) (mediaport.ThumbnailCacheResolution, error) {
	resolver.calls.Add(1)
	return resolver.delegate.ResolveThumbnailFromCache(ctx, item)
}

func realMiniProgramService(pool *pgxpool.Pool) *mediaapp.MiniProgramService {
	repository := mediastore.NewMiniProgramRepository()
	return mediaapp.NewMiniProgramService(platformstore.NewUnitOfWork(pool), repository, repository, eventstore.NewAppender(), repository)
}

func miniProgramCreateCommand(actor int64, key, name, title string) mediaport.MiniProgramCreateCommand {
	return mediaport.MiniProgramCreateCommand{Name: name, AppID: "wx-local-only", PagePath: "pages/index", Title: title,
		Actor: actor, IdempotencyKey: key}
}

func pointer(value string) *string { return &value }

func countMiniProgramFacts(t *testing.T, pool *pgxpool.Pool, ctx context.Context) miniProgramFactCounts {
	t.Helper()
	var counts miniProgramFactCounts
	err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM media_miniprograms),
		(SELECT count(*) FROM media_miniprogram_operation_receipts),
		(SELECT count(*) FROM event_log WHERE event_type LIKE 'media.miniprogram.%')`).Scan(&counts.items, &counts.receipts, &counts.events)
	if err != nil {
		t.Fatal(err)
	}
	return counts
}

func assertMiniProgramReceiptAndEventCount(t *testing.T, pool *pgxpool.Pool, ctx context.Context, actor int64, key, operation, business string, receipts, events int) {
	t.Helper()
	keyDigest := sha256.Sum256([]byte(key))
	eventDigest := sha256.Sum256([]byte(fmt.Sprintf("admin:%d\x00%s\x00%s\x00%s", actor, operation, business, hex.EncodeToString(keyDigest[:]))))
	eventKey := "media.miniprogram:" + operation + ":" + hex.EncodeToString(eventDigest[:])
	var gotReceipts, gotEvents int
	err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM media_miniprogram_operation_receipts WHERE operation=$1 AND actor_scope=$2 AND business_key=$3 AND key_digest=$4),
		(SELECT count(*) FROM event_log WHERE idempotency_key=$5)`, operation, fmt.Sprintf("admin:%d", actor), business, keyDigest[:], eventKey).
		Scan(&gotReceipts, &gotEvents)
	if err != nil || gotReceipts != receipts || gotEvents != events {
		t.Fatalf("receipt/event=%d/%d want=%d/%d err=%v", gotReceipts, gotEvents, receipts, events, err)
	}
}
