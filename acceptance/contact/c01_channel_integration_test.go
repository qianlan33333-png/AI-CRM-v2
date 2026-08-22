package contact_test

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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestC01CreateUpdateReceiptAndEventShareOneUoW(t *testing.T) {
	pool, ctx := c01OpenPool(t)
	service := contactapp.NewChannelService(platformstore.NewUnitOfWork(pool), contactstore.NewChannelRepository(), eventstore.NewAppender())
	code := fmt.Sprintf("c01-%d", time.Now().UnixNano())
	createKey := "c01-create-replay-" + code
	created, err := service.CreateChannel(ctx, contactapp.CreateChannelCommand{Actor: 81, IdempotencyKey: createKey, ChannelCode: code, ChannelName: "公开课", LegacyProjection: json.RawMessage(`{"welcome_message":"欢迎","assignment_config_json":{"mode":"future-passive"}}`)})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.CreateChannel(ctx, contactapp.CreateChannelCommand{Actor: 81, IdempotencyKey: createKey, ChannelCode: code, ChannelName: "公开课", LegacyProjection: json.RawMessage(`{"assignment_config_json":{"mode":"future-passive"},"welcome_message":"欢迎"}`)})
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("replay=%#v err=%v", replayed, err)
	}
	updateKey := "c01-update-replay-" + code
	updated, err := service.UpdateChannel(ctx, contactapp.UpdateChannelCommand{Actor: 82, ChannelID: created.ID, IdempotencyKey: updateKey, Patch: json.RawMessage(`{"status":"archived"}`)})
	if err != nil || updated.Status != "archived" {
		t.Fatalf("update=%#v err=%v", updated, err)
	}
	rows, err := service.ListChannels(ctx, 100, "archived", true)
	if err != nil || len(rows) == 0 {
		t.Fatalf("list=%#v err=%v", rows, err)
	}
	createKeyDigest := sha256.Sum256([]byte(createKey))
	updateKeyDigest := sha256.Sum256([]byte(updateKey))
	var channels, receipts, events int
	err = pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM channels WHERE id=$1 AND code=$2 AND status='archived'),
      (SELECT count(*) FROM channel_operation_receipts WHERE actor_scope IN ('admin:81','admin:82') AND key_digest IN ($3,$4) AND state='completed'),
      (SELECT count(*) FROM event_log WHERE event_type IN ('channel.created','channel.updated') AND payload->>'channel_id'=$1::text)`, created.ID, code, createKeyDigest[:], updateKeyDigest[:]).Scan(&channels, &receipts, &events)
	if err != nil || channels != 1 || receipts != 2 || events != 2 {
		t.Fatalf("facts=%d/%d/%d err=%v", channels, receipts, events, err)
	}
}

func TestC01EventConflictRollsBackChannelAndReceipt(t *testing.T) {
	pool, ctx := c01OpenPool(t)
	service := contactapp.NewChannelService(platformstore.NewUnitOfWork(pool), contactstore.NewChannelRepository(), eventstore.NewAppender())
	code := fmt.Sprintf("c01-rollback-%d", time.Now().UnixNano())
	key := "c01-rollback-key-" + code
	actor := int64(83)
	digest := sha256.Sum256([]byte(fmt.Sprintf("admin:%d\x00create\x00%s", actor, key)))
	eventKey := "channel.create:" + hex.EncodeToString(digest[:])
	err := platformstore.NewUnitOfWork(pool).Within(ctx, func(tx context.Context) error {
		_, e := eventstore.NewAppender().Append(tx, eventport.Event{Type: eventport.EvChannelCreated, Payload: json.RawMessage(`{"channel_id":999,"actor":83}`), OccurredAt: time.Now().UTC(), IdempotencyKey: eventKey})
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.CreateChannel(ctx, contactapp.CreateChannelCommand{Actor: actor, IdempotencyKey: key, ChannelCode: code, ChannelName: "回滚"}); !errors.Is(err, contactapp.ErrChannelUnavailable) {
		t.Fatalf("create err=%v", err)
	}
	keyDigest := sha256.Sum256([]byte(key))
	var channels, receipts int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM channels WHERE code=$1),(SELECT count(*) FROM channel_operation_receipts WHERE actor_scope='admin:83' AND key_digest=$2)`, code, keyDigest[:]).Scan(&channels, &receipts); err != nil || channels != 0 || receipts != 0 {
		t.Fatalf("rollback=%d/%d err=%v", channels, receipts, err)
	}
}

func TestC01ChannelOwnerLockBlocksConcurrentStaffDeactivation(t *testing.T) {
	pool, ctx := c01OpenPool(t)
	weComUserID := fmt.Sprintf("c01-owner-lock-%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, `INSERT INTO staff (wecom_userid, name, is_active, updated_at) VALUES ($1, '锁定成员', TRUE, now())`, weComUserID); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	appender := &c01BlockingAppender{delegate: eventstore.NewAppender(), entered: make(chan struct{}), release: release}
	service := contactapp.NewChannelServiceWithReferences(
		platformstore.NewUnitOfWork(pool), contactstore.NewChannelRepository(), nil, nil, nil, nil,
		contactstore.NewTagCatalogRepository(), contactstore.NewStaffDirectoryRepository(pool), appender,
	)
	result := make(chan error, 1)
	go func() {
		_, err := service.CreateChannel(ctx, contactapp.CreateChannelCommand{
			Actor: 91, IdempotencyKey: "c01-owner-lock-key-" + weComUserID, ChannelCode: weComUserID,
			ChannelName: "成员锁", LegacyProjection: json.RawMessage(fmt.Sprintf(`{"owner_staff_id":%q}`, weComUserID)),
		})
		result <- err
	}()
	select {
	case <-appender.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("channel mutation did not reach the event barrier")
	}
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = blocker.Exec(ctx, `SET LOCAL statement_timeout = '200ms'`); err != nil {
		t.Fatal(err)
	}
	_, err = blocker.Exec(ctx, `UPDATE staff SET is_active = FALSE WHERE wecom_userid = $1`, weComUserID)
	_ = blocker.Rollback(ctx)
	var databaseErr *pgconn.PgError
	if !errors.As(err, &databaseErr) || databaseErr.Code != "57014" {
		t.Fatalf("deactivation error=%v, want statement timeout while Channel owns FOR SHARE", err)
	}
	close(release)
	if err = <-result; err != nil {
		t.Fatalf("channel mutation=%v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE staff SET is_active = FALSE WHERE wecom_userid = $1`, weComUserID); err != nil {
		t.Fatalf("deactivation after Channel commit=%v", err)
	}
}

func TestC01TagReferenceLockBlocksGroupArchiveThenFailsClosed(t *testing.T) {
	pool, ctx := c01OpenPool(t)
	groupName := fmt.Sprintf("c01-tag-lock-%d", time.Now().UnixNano())
	var groupID, tagID int64
	if err := pool.QueryRow(ctx, `INSERT INTO tag_groups (name, sort_order) VALUES ($1, 0) RETURNING id`, groupName).Scan(&groupID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO tags (group_id, name, sort_order) VALUES ($1, '锁定标签', 0) RETURNING id`, groupID).Scan(&tagID); err != nil {
		t.Fatal(err)
	}
	locked := make(chan struct{})
	release := make(chan struct{})
	result := make(chan error, 1)
	uow := platformstore.NewUnitOfWork(pool)
	go func() {
		result <- uow.Within(ctx, func(tx context.Context) error {
			if _, err := contactstore.NewTagCatalogRepository().LockActiveTag(tx, tagID); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		t.Fatal("tag reference lock was not acquired")
	}
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = blocker.Exec(ctx, `SET LOCAL statement_timeout = '200ms'`); err != nil {
		t.Fatal(err)
	}
	_, err = blocker.Exec(ctx, `UPDATE tag_groups SET name = 'archived:' || id::text WHERE id = $1`, groupID)
	_ = blocker.Rollback(ctx)
	var databaseErr *pgconn.PgError
	if !errors.As(err, &databaseErr) || databaseErr.Code != "57014" {
		t.Fatalf("archive error=%v, want statement timeout while tag/group share locks are held", err)
	}
	close(release)
	if err = <-result; err != nil {
		t.Fatalf("tag lock transaction=%v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE tag_groups SET name = 'archived:' || id::text WHERE id = $1`, groupID); err != nil {
		t.Fatalf("archive after reference commit=%v", err)
	}
	err = uow.Within(ctx, func(tx context.Context) error {
		_, lookupErr := contactstore.NewTagCatalogRepository().LockActiveTag(tx, tagID)
		return lookupErr
	})
	if !errors.Is(err, contactport.ErrTagReferenceNotFound) {
		t.Fatalf("post-archive tag lookup=%v, want not found", err)
	}
}

type c01BlockingAppender struct {
	delegate eventport.Appender
	entered  chan struct{}
	release  <-chan struct{}
	once     sync.Once
}

func (appender *c01BlockingAppender) Append(ctx context.Context, event eventport.Event) (eventport.EventID, error) {
	appender.once.Do(func() { close(appender.entered) })
	<-appender.release
	return appender.delegate.Append(ctx, event)
}

func TestC01S200KChannelQueriesUseIndexes(t *testing.T) {
	pool, ctx := c01OpenPool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO channels (name,code,config,status,created_by,updated_by,created_at,updated_at) SELECT '性能渠道','c01-perf-'||g,jsonb_build_object('schema_version',1,'channel_code','c01-perf-'||g,'channel_name','性能渠道','status',CASE WHEN g%2=0 THEN 'active' ELSE 'inactive' END),CASE WHEN g%2=0 THEN 'active' ELSE 'inactive' END,99,99,now(),now() FROM generate_series(1,200000) g ON CONFLICT (code) DO NOTHING`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ANALYZE channels`); err != nil {
		t.Fatal(err)
	}
	plan := c01Explain(t, ctx, tx, `EXPLAIN (FORMAT JSON,COSTS OFF) SELECT id,code FROM channels WHERE status='active' ORDER BY updated_at DESC,id DESC LIMIT 100`)
	if strings.Contains(plan, `"Node Type": "Seq Scan"`) ||
		(!strings.Contains(plan, `"Index Name": "channels_status_updated_id_idx"`) &&
			!strings.Contains(plan, `"Index Name": "channels_updated_id_idx"`)) {
		t.Fatalf("illegal plan:\n%s", plan)
	}
}

func c01OpenPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *databaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*databaseURL); err != nil {
		if c01Err := acceptancefixtures.ValidateDatabaseURLForDatabase(*databaseURL, acceptancefixtures.C01ChannelDatabaseName); c01Err != nil {
			t.Fatal(err)
		}
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
		t.Fatalf("postgres=%s err=%v", version, err)
	}
	return pool, ctx
}
func c01Explain(t *testing.T, ctx context.Context, tx pgx.Tx, query string) string {
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
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "\n")
}
