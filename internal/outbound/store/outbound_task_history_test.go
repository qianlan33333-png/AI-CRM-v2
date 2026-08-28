package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	outbounddb "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var outboundTaskHistoryTestDatabaseURL = flag.String("outbound-task-history-test-database-url", "", "isolated PostgreSQL DSN for schema 130 rollback verification")

func TestOutboundTaskHistoryStoreRequiresCallerTransactionAndReaderDependency(t *testing.T) {
	store := NewOutboundTaskHistoryStore()
	value := storeOutboundTaskHistoryValue(-7, nil, nil)
	if _, err := store.CreateHistoricalOutboundTask(context.Background(), value); !errors.Is(err, outboundport.ErrOutboundTaskHistoryUnavailable) {
		t.Fatalf("create_escaped_caller_transaction=%v", err)
	}
	if _, err := store.GetHistoricalOutboundTask(context.Background(), 1); !errors.Is(err, outboundport.ErrOutboundTaskHistoryUnavailable) {
		t.Fatalf("get_escaped_caller_transaction=%v", err)
	}
	if _, err := store.LookupOutboundTaskHistoryParents(context.Background(), -1); !errors.Is(err, outboundport.ErrOutboundTaskHistoryUnavailable) {
		t.Fatalf("lookup_escaped_caller_transaction=%v", err)
	}
	if _, err := store.GetHistoricalOutboundTask(context.Background(), 0); !errors.Is(err, outboundport.ErrOutboundTaskHistoryInvalid) {
		t.Fatalf("invalid_target_accepted=%v", err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*OutboundTaskHistoryReader{nil, NewOutboundTaskHistoryReader(nil), NewOutboundTaskHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalOutboundTasks(context.Background(), outboundport.OutboundTaskHistoryQuery{}); !errors.Is(err, outboundport.ErrOutboundTaskHistoryUnavailable) {
			t.Fatalf("nil_reader_not_closed=%v", err)
		}
	}
	for _, query := range []outboundport.OutboundTaskHistoryQuery{{Limit: -1}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewOutboundTaskHistoryReader(nil).ListHistoricalOutboundTasks(context.Background(), query); !errors.Is(err, outboundport.ErrOutboundTaskHistoryInvalid) {
			t.Fatalf("invalid_page_accepted query=%#v err=%v", query, err)
		}
	}
}

func TestOutboundTaskHistoryValuePreservesPrivateSignedNullableFields(t *testing.T) {
	at := time.Date(2026, 8, 28, 13, 14, 15, 123456000, time.UTC)
	row := outbounddb.OutboundV1TaskHistory{
		ID: 9, SourceID: -7, TaskType: "", Status: "", CreatedAt: pgtype.Timestamptz{Time: at, Valid: true},
		BroadcastJobHistoryID: pgtype.Int8{Int64: 11, Valid: true}, LegacyBroadcastJobID: pgtype.Int8{Int64: -8, Valid: true}, RedactedRoots: []string{"request_payload"},
		WecomTaskIDDigest: digestBytes(1), RequestPayloadDigest: digestBytes(2), ResponsePayloadDigest: digestBytes(3), TraceIDDigest: digestBytes(4),
		SourceKeyDigest: digestBytes(5), SourcePayloadDigest: digestBytes(6), SourceFieldDigest: digestBytes(7),
	}
	value, err := outboundTaskHistoryValue(row)
	if err != nil || value.SourceID != -7 || value.BroadcastJobHistoryID == nil || *value.BroadcastJobHistoryID != 11 || value.LegacyBroadcastJobID == nil || *value.LegacyBroadcastJobID != -8 || value.WeComTaskIDDigest == nil || value.CreatedAt.Location() != time.UTC || value.CreatedAt.Nanosecond()%1000 != 0 || value.RedactedRoots[0] != "request_payload" {
		t.Fatalf("history_value_changed value=%#v err=%v", value, err)
	}
	row.WecomTaskIDDigest = nil
	value, err = outboundTaskHistoryValue(row)
	if err != nil || value.WeComTaskIDDigest != nil {
		t.Fatalf("nullable_wecom_changed value=%#v err=%v", value, err)
	}
	row.WecomTaskIDDigest = make([]byte, 31)
	if _, err := outboundTaskHistoryValue(row); !errors.Is(err, outboundport.ErrOutboundTaskHistoryUnavailable) {
		t.Fatalf("short_nullable_digest_accepted=%v", err)
	}
	row.WecomTaskIDDigest = digestBytes(1)
	row.SourceFieldDigest = make([]byte, 31)
	if _, err := outboundTaskHistoryValue(row); !errors.Is(err, outboundport.ErrOutboundTaskHistoryUnavailable) {
		t.Fatalf("short_required_digest_accepted=%v", err)
	}
}

func TestOutboundTaskHistoryStoreErrorMapping(t *testing.T) {
	for _, cause := range []error{pgx.ErrNoRows, &pgconn.PgError{Code: "23505"}, &pgconn.PgError{Code: "23503"}} {
		if !errors.Is(outboundTaskHistoryStoreError(cause), outboundport.ErrOutboundTaskHistoryConflict) {
			t.Fatalf("conflict_not_mapped cause=%v", cause)
		}
	}
	if !errors.Is(outboundTaskHistoryStoreError(errors.New("private database failure")), outboundport.ErrOutboundTaskHistoryUnavailable) {
		t.Fatal("unavailable_not_mapped")
	}
}

func TestOutboundTaskHistoryPostgresRoundTripRollback(t *testing.T) {
	if *outboundTaskHistoryTestDatabaseURL == "" {
		t.Skip("set -outbound-task-history-test-database-url for isolated schema 130 rollback verification")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *outboundTaskHistoryTestDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := outbounddb.New(pool)
	beforeHistory, err := queries.CountHistoricalOutboundTasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	beforeRuntime, err := outboundTaskHistoryRuntimeCounts(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("outbound task history forced rollback")
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store := NewOutboundTaskHistoryStore()
		parent, err := NewBroadcastJobHistoryStore().CreateHistoricalBroadcastJob(txCtx, storeBroadcastJobHistoryValue())
		if err != nil {
			return fmt.Errorf("stage=create_parent: %w", err)
		}
		parents, err := store.LookupOutboundTaskHistoryParents(txCtx, parent.SourceID)
		if err != nil || len(parents) != 1 || parents[0].ID != parent.ID || parents[0].LegacyOutboundTaskID == nil || *parents[0].LegacyOutboundTaskID != -7 {
			return fmt.Errorf("stage=lookup_parent parents=%#v err=%w", parents, err)
		}
		value := storeOutboundTaskHistoryValue(-7, &parent.SourceID, &parent.ID)
		created, err := store.CreateHistoricalOutboundTask(txCtx, value)
		if err != nil {
			return fmt.Errorf("stage=create: %w", err)
		}
		second := storeOutboundTaskHistoryValue(-8, nil, nil)
		second.WeComTaskIDDigest = nil
		if _, err = store.CreateHistoricalOutboundTask(txCtx, second); err != nil {
			return fmt.Errorf("stage=create_nullable: %w", err)
		}
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return fmt.Errorf("stage=tx: %w", err)
		}
		reader := NewOutboundTaskHistoryReader(nil)
		loaded, err := reader.GetHistoricalOutboundTask(txCtx, created.ID)
		if err != nil || loaded.SourceID != -7 || loaded.BroadcastJobHistoryID == nil || *loaded.BroadcastJobHistoryID != parent.ID || loaded.LegacyBroadcastJobID == nil || *loaded.LegacyBroadcastJobID != parent.SourceID || loaded.WeComTaskIDDigest == nil || loaded.SourceFieldDigest != created.SourceFieldDigest {
			return fmt.Errorf("stage=get loaded=%#v err=%w", loaded, err)
		}
		direct, err := NewOutboundTaskHistoryReader(tx).GetHistoricalOutboundTask(context.Background(), created.ID)
		if err != nil || direct.ID != created.ID || direct.SourcePayloadDigest != created.SourcePayloadDigest {
			return fmt.Errorf("stage=bare_tx direct=%#v err=%w", direct, err)
		}
		items, total, err := reader.ListHistoricalOutboundTasks(txCtx, outboundport.OutboundTaskHistoryQuery{})
		if err != nil || total != beforeHistory+2 || len(items) == 0 {
			return fmt.Errorf("stage=list total=%d items=%#v err=%w", total, items, err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback=%v", err)
	}
	afterHistory, err := queries.CountHistoricalOutboundTasks(ctx)
	if err != nil || beforeHistory != afterHistory {
		t.Fatalf("rollback_left_history before=%d after=%d err=%v", beforeHistory, afterHistory, err)
	}
	afterRuntime, err := outboundTaskHistoryRuntimeCounts(ctx, pool)
	if err != nil || beforeRuntime != afterRuntime {
		t.Fatalf("runtime_side_effect_changed before=%v after=%v err=%v", beforeRuntime, afterRuntime, err)
	}

	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store := NewOutboundTaskHistoryStore()
		value := storeOutboundTaskHistoryValue(-7, nil, nil)
		if _, err := store.CreateHistoricalOutboundTask(txCtx, value); err != nil {
			return fmt.Errorf("stage=unique_first: %w", err)
		}
		_, err := store.CreateHistoricalOutboundTask(txCtx, value)
		if !errors.Is(err, outboundport.ErrOutboundTaskHistoryConflict) {
			return fmt.Errorf("stage=unique_second: %w", err)
		}
		return outboundport.ErrOutboundTaskHistoryConflict
	})
	if !errors.Is(err, outboundport.ErrOutboundTaskHistoryConflict) {
		t.Fatalf("unique_conflict=%v", err)
	}
	afterHistory, err = queries.CountHistoricalOutboundTasks(ctx)
	if err != nil || beforeHistory != afterHistory {
		t.Fatalf("unique_rollback_left_history before=%d after=%d err=%v", beforeHistory, afterHistory, err)
	}
	afterRuntime, err = outboundTaskHistoryRuntimeCounts(ctx, pool)
	if err != nil || beforeRuntime != afterRuntime {
		t.Fatalf("unique_rollback_changed_runtime before=%v after=%v err=%v", beforeRuntime, afterRuntime, err)
	}
}

type outboundTaskHistoryRuntimeCount struct{ Tasks, Batches, Events, River, Effects int64 }

func outboundTaskHistoryRuntimeCounts(ctx context.Context, db *pgxpool.Pool) (outboundTaskHistoryRuntimeCount, error) {
	var value outboundTaskHistoryRuntimeCount
	err := db.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM outbound_tasks),
  (SELECT count(*) FROM outbound_batches),
  (SELECT count(*) FROM event_log),
  (SELECT count(*) FROM river_job),
  (SELECT count(*) FROM external_effects)`).Scan(&value.Tasks, &value.Batches, &value.Events, &value.River, &value.Effects)
	return value, err
}

func storeOutboundTaskHistoryValue(sourceID int64, legacy, parent *int64) outboundport.HistoricalOutboundTask {
	at := time.Date(2026, 8, 28, 13, 14, 15, 123456000, time.UTC)
	digest := func(label string) [32]byte { return sha256.Sum256([]byte(fmt.Sprintf("%s/%d", label, sourceID))) }
	wecom := digest("wecom")
	return outboundport.HistoricalOutboundTask{
		SourceID: sourceID, TaskType: "", Status: "", CreatedAt: at, BroadcastJobHistoryID: parent,
		RequestPayloadDigest: digest("request"), ResponsePayloadDigest: digest("response"), WeComTaskIDDigest: &wecom, TraceIDDigest: digest("trace"), LegacyBroadcastJobID: legacy,
		SourceKeyDigest: digest("key"), SourcePayloadDigest: digest("payload"), SourceFieldDigest: digest("field"), RedactedRoots: []string{"trace_id"},
	}
}

func digestBytes(first byte) []byte {
	value := make([]byte, 32)
	value[0] = first
	return value
}
