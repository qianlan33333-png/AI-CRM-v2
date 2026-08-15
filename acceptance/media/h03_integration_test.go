package media_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediastore "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestH03MigrationHistoryFixture(t *testing.T) {
	pool, ctx := openPool(t)
	marker := unique("h03-migration-history")
	if err := platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		_, err := eventstore.NewAppender().Append(tx, eventport.Event{Type: "h03.media.migration_fixture", Payload: json.RawMessage(`{"fixture":true}`),
			OccurredAt: time.Now().UTC(), IdempotencyKey: marker})
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestH03CRUDReplayConflictActorBusinessIsolationAndArchive(t *testing.T) {
	pool, ctx := openPool(t)
	cover := createCover(t, pool, ctx, 5101)
	service := realGroupInviteService(pool)
	key := unique("group-invite-key")
	missingCoverKey := unique("missing-cover-key")
	missingCover := groupInviteCommand(5101, missingCoverKey, 999999999, unique("missing-cover"))
	if _, err := service.Create(ctx, missingCover); !errors.Is(err, mediaapp.ErrGroupInviteInvalidReference) {
		t.Fatalf("missing cover err=%v", err)
	}
	assertGroupInviteFacts(t, pool, ctx, missingCover.Actor, missingCoverKey, 0, 0, 0)
	command := groupInviteCommand(5101, key, cover, unique("group-invite"))
	created, err := service.Create(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.Create(ctx, command)
	if err != nil || replay.ID != created.ID {
		t.Fatalf("replay=%#v err=%v", replay, err)
	}
	conflict := command
	conflict.Title += "不同"
	if _, err = service.Create(ctx, conflict); !errors.Is(err, mediaapp.ErrGroupInviteConflict) {
		t.Fatalf("conflict err=%v", err)
	}
	other := command
	other.Actor, other.Name = 5102, unique("other-actor")
	otherCreated, err := service.Create(ctx, other)
	if err != nil || otherCreated.ID == created.ID {
		t.Fatalf("actor isolation=%#v err=%v", otherCreated, err)
	}
	title := "更新标题"
	updated, err := service.Update(ctx, mediaport.GroupInviteUpdateCommand{ID: created.ID,
		GroupInvitePatch: mediaport.GroupInvitePatch{Title: &title}, Actor: 5101, IdempotencyKey: key})
	if err != nil || updated.Title != title || updated.Version != 2 {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	archived, err := service.Archive(ctx, mediaport.GroupInviteArchiveCommand{ID: created.ID, Actor: 5101, IdempotencyKey: key})
	if err != nil || archived.ArchivedAt == nil || archived.Enabled || archived.Version != 3 {
		t.Fatalf("archived=%#v err=%v", archived, err)
	}
	archivedReplay, err := service.Archive(ctx, mediaport.GroupInviteArchiveCommand{ID: created.ID, Actor: 5101, IdempotencyKey: key})
	if err != nil || archivedReplay.ArchivedAt == nil || !archivedReplay.ArchivedAt.Equal(*archived.ArchivedAt) {
		t.Fatalf("archive replay=%#v err=%v", archivedReplay, err)
	}
	assertGroupInviteFacts(t, pool, ctx, 5101, key, 1, 3, 3)
}

func TestH03FourFailurePointsRollbackBusinessReceiptAndEvent(t *testing.T) {
	for _, operation := range []string{"create", "update", "archive"} {
		for _, fault := range []string{"image", "fact", "event", "complete"} {
			t.Run(operation+"/"+fault, func(t *testing.T) {
				pool, ctx := openPool(t)
				actor := int64(5201)
				cover := createCover(t, pool, ctx, actor)
				repository := mediastore.NewGroupInviteRepository()
				var existing mediaport.GroupInvite
				if operation != "create" {
					var err error
					existing, err = realGroupInviteService(pool).Create(ctx,
						groupInviteCommand(actor, unique("rollback-base-key"), cover, unique("rollback-base")))
					if err != nil {
						t.Fatal(err)
					}
				}
				store := mediaapp.GroupInviteStore(repository)
				images := mediaport.ImageMetadataReader(repository)
				events := eventport.Appender(eventstore.NewAppender())
				switch fault {
				case "image":
					images = failingImageReader{}
				case "fact", "complete":
					store = failingGroupInviteStore{GroupInviteStore: repository, fault: fault}
				case "event":
					events = failingAppender{}
				}
				service := mediaapp.NewGroupInviteService(platformstore.NewUnitOfWork(pool), store, images, events)
				key := unique("rollback-" + operation + "-" + fault)
				var err error
				switch operation {
				case "create":
					_, err = service.Create(ctx, groupInviteCommand(actor, key, cover, unique("rollback")))
				case "update":
					title := "rollback update"
					_, err = service.Update(ctx, mediaport.GroupInviteUpdateCommand{ID: existing.ID,
						GroupInvitePatch: mediaport.GroupInvitePatch{Title: &title}, Actor: actor, IdempotencyKey: key})
				case "archive":
					_, err = service.Archive(ctx, mediaport.GroupInviteArchiveCommand{ID: existing.ID, Actor: actor, IdempotencyKey: key})
				}
				if !errors.Is(err, mediaapp.ErrGroupInviteUnavailable) {
					t.Fatalf("operation=%s fault=%s err=%v", operation, fault, err)
				}
				assertGroupInviteFacts(t, pool, ctx, actor, key, 0, 0, 0)
				if operation != "create" {
					unchanged, getErr := realGroupInviteService(pool).Get(ctx, existing.ID)
					if getErr != nil || unchanged.Version != 1 || unchanged.Title != existing.Title || unchanged.ArchivedAt != nil {
						t.Fatalf("operation=%s fault=%s changed=%#v getErr=%v", operation, fault, unchanged, getErr)
					}
				}
			})
		}
	}
}

func TestH03ConcurrentSameActorOperationBusinessKeyMakesOneFact(t *testing.T) {
	pool, ctx := openPool(t)
	service := realGroupInviteService(pool)
	command := groupInviteCommand(5301, unique("concurrent-key"), 0, unique("concurrent-item"))
	const workers = 16
	results := make(chan mediaport.GroupInvite, workers)
	errorsChannel := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			item, err := service.Create(ctx, command)
			results <- item
			errorsChannel <- err
		}()
	}
	group.Wait()
	close(results)
	close(errorsChannel)
	var id int64
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("concurrent err=%v", err)
		}
	}
	for item := range results {
		if id == 0 {
			id = item.ID
		}
		if item.ID != id {
			t.Fatalf("ids=%d/%d", id, item.ID)
		}
	}
	assertGroupInviteFacts(t, pool, ctx, command.Actor, command.IdempotencyKey, 1, 1, 1)
}

func TestH03S200KReceiptLookupHasNoIllegalSequentialScan(t *testing.T) {
	pool, ctx := openPool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO media_group_invite_operation_receipts
      (operation,actor_scope,business_key,key_digest,payload_digest,state,result_snapshot,created_at,completed_at)
      SELECT 'create','admin:'||g,'create',decode(md5('key-'||g)||md5('key2-'||g),'hex'),
             decode(md5('payload-'||g)||md5('payload2-'||g),'hex'),'completed',jsonb_build_object('id',g),now(),now()
      FROM generate_series(2000000,2199999) AS g`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ANALYZE media_group_invite_operation_receipts`); err != nil {
		t.Fatal(err)
	}
	plan := explain(t, ctx, tx, `EXPLAIN (FORMAT JSON, COSTS OFF)
      SELECT id,state,result_snapshot FROM media_group_invite_operation_receipts
      WHERE operation='create' AND actor_scope='admin:2100000' AND business_key='create'
        AND key_digest=decode(md5('key-2100000')||md5('key2-2100000'),'hex')`)
	if strings.Contains(plan, `"Node Type": "Seq Scan"`) || !strings.Contains(plan, `"Node Type": "Index Scan"`) {
		t.Fatalf("illegal S200K receipt plan:\n%s", plan)
	}
}

func TestH03StorageCatalogIsSingleInstanceAndLocalOnly(t *testing.T) {
	pool, ctx := openPool(t)
	var waterline, invalidConstraints, invalidIndexes, forbiddenColumns, eventFKs int
	err := pool.QueryRow(ctx, `SELECT
      (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('media_group_invites'::regclass,'media_group_invite_operation_receipts'::regclass) AND NOT convalidated),
      (SELECT count(*) FROM pg_index WHERE indrelid IN ('media_group_invites'::regclass,'media_group_invite_operation_receipts'::regclass) AND (NOT indisvalid OR NOT indisready OR NOT indislive)),
      (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name IN ('media_group_invites','media_group_invite_operation_receipts') AND column_name ~* 'tenant|workspace|organization|config_id|chat_id|provider|pic_url'),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('media_group_invites'::regclass,'media_group_invite_operation_receipts'::regclass) AND confrelid='event_log'::regclass)`).Scan(&waterline, &invalidConstraints, &invalidIndexes, &forbiddenColumns, &eventFKs)
	if err != nil || waterline != 34 || invalidConstraints != 0 || invalidIndexes != 0 || forbiddenColumns != 0 || eventFKs != 0 {
		t.Fatalf("catalog=%d/%d/%d/%d/%d err=%v", waterline, invalidConstraints, invalidIndexes, forbiddenColumns, eventFKs, err)
	}
}

type failingGroupInviteStore struct {
	mediaapp.GroupInviteStore
	fault string
}

func (store failingGroupInviteStore) CreateGroupInvite(ctx context.Context, item mediaport.GroupInvite) (mediaport.GroupInvite, error) {
	if store.fault == "fact" {
		return mediaport.GroupInvite{}, errors.New("fact failed")
	}
	return store.GroupInviteStore.CreateGroupInvite(ctx, item)
}
func (store failingGroupInviteStore) UpdateGroupInvite(ctx context.Context, item mediaport.GroupInvite) (mediaport.GroupInvite, error) {
	if store.fault == "fact" {
		return mediaport.GroupInvite{}, errors.New("fact failed")
	}
	return store.GroupInviteStore.UpdateGroupInvite(ctx, item)
}
func (store failingGroupInviteStore) ArchiveGroupInvite(ctx context.Context, item mediaport.GroupInvite) (mediaport.GroupInvite, error) {
	if store.fault == "fact" {
		return mediaport.GroupInvite{}, errors.New("fact failed")
	}
	return store.GroupInviteStore.ArchiveGroupInvite(ctx, item)
}
func (store failingGroupInviteStore) CompleteGroupInvite(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (mediaapp.GroupInviteReceipt, error) {
	if store.fault == "complete" {
		return mediaapp.GroupInviteReceipt{}, errors.New("complete failed")
	}
	return store.GroupInviteStore.CompleteGroupInvite(ctx, id, snapshot, now)
}

type failingImageReader struct{}

func (failingImageReader) ImageExists(context.Context, int64) (bool, error) {
	return false, errors.New("image failed")
}

type failingAppender struct{}

func (failingAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, errors.New("event failed")
}

func realGroupInviteService(pool *pgxpool.Pool) *mediaapp.GroupInviteService {
	repository := mediastore.NewGroupInviteRepository()
	return mediaapp.NewGroupInviteService(platformstore.NewUnitOfWork(pool), repository, repository, eventstore.NewAppender())
}

func createCover(t *testing.T, pool *pgxpool.Pool, ctx context.Context, actor int64) int64 {
	t.Helper()
	image, err := realService(pool).Upload(ctx, command(t, actor, unique("cover-key"), unique("cover-name")))
	if err != nil {
		t.Fatal(err)
	}
	return image.ID
}

func groupInviteCommand(actor int64, key string, cover int64, name string) mediaport.GroupInviteCreateCommand {
	return mediaport.GroupInviteCreateCommand{Name: name, Title: "加入体验群", Description: "点击卡片入群",
		JoinURL: "https://work.weixin.qq.com/gm/safe-token", CoverImageID: cover, Actor: actor, IdempotencyKey: key}
}

func assertGroupInviteFacts(t *testing.T, pool *pgxpool.Pool, ctx context.Context, actor int64, key string, items, receipts, events int) {
	t.Helper()
	keyDigest := sha256.Sum256([]byte(key))
	var gotItems, gotReceipts int
	err := pool.QueryRow(ctx, `SELECT
	  (SELECT count(*) FROM media_group_invites item WHERE EXISTS (
	    SELECT 1 FROM media_group_invite_operation_receipts receipt
	    WHERE receipt.operation='create' AND receipt.actor_scope=$1 AND receipt.key_digest=$2
	      AND (receipt.result_snapshot->>'id')::bigint=item.id)),
	  (SELECT count(*) FROM media_group_invite_operation_receipts WHERE actor_scope=$1 AND key_digest=$2)`,
		fmt.Sprintf("admin:%d", actor), keyDigest[:]).Scan(&gotItems, &gotReceipts)
	if err != nil {
		t.Fatal(err)
	}
	gotEvents := countGroupInviteEventsForKey(t, pool, ctx, actor, key)
	if gotItems != items || gotReceipts != receipts || gotEvents != events {
		t.Fatalf("facts=%d/%d/%d want=%d/%d/%d", gotItems, gotReceipts, gotEvents, items, receipts, events)
	}
}

func countGroupInviteEventsForKey(t *testing.T, pool *pgxpool.Pool, ctx context.Context, actor int64, key string) int {
	t.Helper()
	keyDigest := sha256.Sum256([]byte(key))
	rows, err := pool.Query(ctx, `SELECT operation,business_key FROM media_group_invite_operation_receipts
		WHERE actor_scope=$1 AND key_digest=$2 ORDER BY operation`, fmt.Sprintf("admin:%d", actor), keyDigest[:])
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var operation, business string
		if err = rows.Scan(&operation, &business); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte(fmt.Sprintf("admin:%d\x00%s\x00%s\x00%s", actor, operation, business, key)))
		var exists int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_log WHERE idempotency_key=$1`,
			"media.group_invite_"+operation+":"+hex.EncodeToString(digest[:])).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		count += exists
	}
	return count
}
