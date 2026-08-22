package contact_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
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
