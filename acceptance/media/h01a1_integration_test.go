package media_acceptance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var databaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 H01A1 database")

func TestH01A1MigrationHistoryFixture(t *testing.T) {
	pool, ctx := openPool(t)
	marker := unique("migration-history")
	err := platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		_, appendErr := eventstore.NewAppender().Append(tx, eventport.Event{Type: "h01a1.media.migration_fixture",
			Payload: json.RawMessage(fmt.Sprintf(`{"marker":%q}`, marker)), OccurredAt: time.Now().UTC(), IdempotencyKey: marker})
		return appendErr
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestH01A1UploadReplayActorIsolationAndEventShareOneUoW(t *testing.T) {
	pool, ctx := openPool(t)
	service := realService(pool)
	key := unique("upload-key")
	command := command(t, 4101, key, unique("image"))
	created, err := service.Upload(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Upload(ctx, command)
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("replay=%+v error=%v", replayed, err)
	}
	changed := command
	changed.Description = "changed"
	if _, err = service.Upload(ctx, changed); !errors.Is(err, mediaapp.ErrConflict) {
		t.Fatalf("changed replay error=%v", err)
	}
	other := command
	other.Actor, other.Name = 4102, unique("other-actor")
	otherCreated, err := service.Upload(ctx, other)
	if err != nil || otherCreated.ID == created.ID {
		t.Fatalf("actor isolation=%+v error=%v", otherCreated, err)
	}
	assertFacts(t, pool, ctx, command.Actor, key, command.Name, 1, 1, 1)
	assertFacts(t, pool, ctx, other.Actor, key, other.Name, 1, 1, 1)
	var blobMatches bool
	checksum := sha256.Sum256(command.Content)
	if err = pool.QueryRow(ctx, `SELECT b.content=$2 AND b.checksum=$3 AND i.checksum=b.checksum
	      FROM media_images i JOIN media_image_blobs b ON b.image_id=i.id WHERE i.id=$1`, created.ID, command.Content, checksum[:]).Scan(&blobMatches); err != nil || !blobMatches {
		t.Fatalf("blob metadata mismatch=%v/%v", blobMatches, err)
	}
}

func TestH01A1EventConflictRollsBackMetadataBlobAndReceipt(t *testing.T) {
	pool, ctx := openPool(t)
	actor, key, name := int64(4201), unique("rollback-key"), unique("rollback-image")
	digest := sha256.Sum256([]byte(fmt.Sprintf("admin:%d\x00%s", actor, key)))
	eventKey := "media.image_created:" + hex.EncodeToString(digest[:])
	err := platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		_, appendErr := eventstore.NewAppender().Append(tx, eventport.Event{Type: "h01a1.conflict.fixture", Payload: json.RawMessage(`{"fixture":true}`), OccurredAt: time.Now().UTC(), IdempotencyKey: eventKey})
		return appendErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = realService(pool).Upload(ctx, command(t, actor, key, name)); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatalf("event conflict error=%v", err)
	}
	assertFacts(t, pool, ctx, actor, key, name, 0, 0, 1)
}

func TestH01A1S200KReceiptLookupUsesActorBusinessKeyIndex(t *testing.T) {
	pool, ctx := openPool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO media_image_upload_receipts
      (operation,actor_scope,key_digest,payload_digest,state,result_snapshot,created_at,completed_at)
      SELECT 'upload','admin:'||g,decode(md5('key-'||g)||md5('key2-'||g),'hex'),
             decode(md5('payload-'||g)||md5('payload2-'||g),'hex'),'completed',jsonb_build_object('id',g),now(),now()
      FROM generate_series(1000000,1199999) AS g`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ANALYZE media_image_upload_receipts`); err != nil {
		t.Fatal(err)
	}
	plan := explain(t, ctx, tx, `EXPLAIN (FORMAT JSON, COSTS OFF)
      SELECT id,state,result_snapshot FROM media_image_upload_receipts
      WHERE operation='upload' AND actor_scope='admin:1100000'
        AND key_digest=decode(md5('key-1100000')||md5('key2-1100000'),'hex')`)
	if strings.Contains(plan, `"Node Type": "Seq Scan"`) || !strings.Contains(plan, `"Node Type": "Index Scan"`) || !strings.Contains(plan, `media_image_upload_receipts_operation_actor_scope_key_diges_key`) {
		t.Fatalf("illegal S200K receipt plan:\n%s", plan)
	}
}

func TestH01A1StorageCatalogSingleInstanceAndValidatedOwnership(t *testing.T) {
	pool, ctx := openPool(t)
	var waterline, invalidConstraints, invalidIndexes, tenantColumns, eventFKs int
	err := pool.QueryRow(ctx, `SELECT
      (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('media_images'::regclass,'media_image_blobs'::regclass,'media_image_upload_receipts'::regclass) AND NOT convalidated),
      (SELECT count(*) FROM pg_index WHERE indrelid IN ('media_images'::regclass,'media_image_blobs'::regclass,'media_image_upload_receipts'::regclass) AND (NOT indisvalid OR NOT indisready OR NOT indislive)),
      (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name IN ('media_images','media_image_blobs','media_image_upload_receipts') AND column_name ~* 'tenant|workspace|organization'),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('media_images'::regclass,'media_image_blobs'::regclass,'media_image_upload_receipts'::regclass) AND confrelid='event_log'::regclass)`).Scan(&waterline, &invalidConstraints, &invalidIndexes, &tenantColumns, &eventFKs)
	if err != nil || waterline != 30 || invalidConstraints != 0 || invalidIndexes != 0 || tenantColumns != 0 || eventFKs != 0 {
		t.Fatalf("catalog=%d/%d/%d/%d/%d err=%v", waterline, invalidConstraints, invalidIndexes, tenantColumns, eventFKs, err)
	}
}

func command(t *testing.T, actor int64, key, name string) mediaport.UploadCommand {
	t.Helper()
	imageValue := image.NewRGBA(image.Rect(0, 0, 2, 3))
	imageValue.Set(0, 0, color.RGBA{R: 9, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, imageValue); err != nil {
		t.Fatal(err)
	}
	return mediaport.UploadCommand{Actor: actor, IdempotencyKey: key, FileName: "safe.png", DeclaredType: "image/png",
		Content: data.Bytes(), Name: name, Description: "H01A1", Tags: "cover", Category: "image"}
}
func realService(pool *pgxpool.Pool) *mediaapp.Service {
	return mediaapp.NewService(platformstore.NewUnitOfWork(pool), mediastore.NewUploadRepository(), eventstore.NewAppender())
}
func assertFacts(t *testing.T, pool *pgxpool.Pool, ctx context.Context, actor int64, key, name string, images, receipts, events int) {
	t.Helper()
	keyDigest := sha256.Sum256([]byte(key))
	eventDigest := sha256.Sum256([]byte(fmt.Sprintf("admin:%d\x00%s", actor, key)))
	eventKey := "media.image_created:" + hex.EncodeToString(eventDigest[:])
	var gotImages, gotReceipts, gotEvents int
	err := pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM media_images WHERE created_by=$1 AND name=$2),
      (SELECT count(*) FROM media_image_upload_receipts WHERE actor_scope=$3 AND key_digest=$4),
      (SELECT count(*) FROM event_log WHERE idempotency_key=$5)`, actor, name, fmt.Sprintf("admin:%d", actor), keyDigest[:], eventKey).Scan(&gotImages, &gotReceipts, &gotEvents)
	if err != nil || gotImages != images || gotReceipts != receipts || gotEvents != events {
		t.Fatalf("facts=%d/%d/%d err=%v want=%d/%d/%d", gotImages, gotReceipts, gotEvents, err, images, receipts, events)
	}
}
func openPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*databaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v", version, err)
	}
	return pool, ctx
}
func explain(t *testing.T, ctx context.Context, tx pgx.Tx, query string) string {
	t.Helper()
	rows, err := tx.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
func unique(prefix string) string { return fmt.Sprintf("h01a1-%s-%d", prefix, time.Now().UnixNano()) }
