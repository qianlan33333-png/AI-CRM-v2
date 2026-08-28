package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	outbounddb "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var broadcastJobHistoryPostgresDSN = flag.String("broadcast-job-history-postgres-dsn", "", "isolated PostgreSQL DSN for schema 129 rollback verification")

func TestBroadcastJobHistoryStoreRequiresCallerTransactionAndReaderDependency(t *testing.T) {
	if _, err := NewBroadcastJobHistoryStore().GetHistoricalBroadcastJob(context.Background(), 1); !errors.Is(err, outboundport.ErrBroadcastJobHistoryUnavailable) {
		t.Fatal("store_read_escaped_caller_transaction")
	}
	var pool *pgxpool.Pool
	for _, reader := range []*BroadcastJobHistoryReader{nil, NewBroadcastJobHistoryReader(nil), NewBroadcastJobHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalBroadcastJobs(context.Background(), outboundport.BroadcastJobHistoryQuery{}); !errors.Is(err, outboundport.ErrBroadcastJobHistoryUnavailable) {
			t.Fatal("nil_reader_not_closed")
		}
	}
	for _, query := range []outboundport.BroadcastJobHistoryQuery{{Limit: -1}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewBroadcastJobHistoryReader(nil).ListHistoricalBroadcastJobs(context.Background(), query); !errors.Is(err, outboundport.ErrBroadcastJobHistoryInvalid) {
			t.Fatal("invalid_page_accepted")
		}
	}
}

func TestBroadcastJobHistoryValuePreservesSignedNullableAndDigestFields(t *testing.T) {
	at := time.Date(2026, 8, 28, 13, 14, 15, 123456000, time.UTC)
	row := outbounddb.OutboundV1BroadcastJobHistory{ID: 9, SourceID: 7, OriginalSourceType: "unknown", SourceTable: "legacy", ScheduledFor: pgtype.Timestamptz{Time: at, Valid: true}, Priority: -1, OriginalStatus: "unknown", RequiresApproval: true, TargetCount: -2, ContentType: "opaque", AttemptCount: -3, SentCount: -4, FailedCount: -5, CreatedAt: pgtype.Timestamptz{Time: at, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: at, Valid: true}, MaxAttempts: -6, OriginalSideEffectExecuted: true, OriginalProviderResultReceived: true, OriginalReconciliationRequired: true, LegacyOutboundTaskID: pgtype.Int8{Int64: -7, Valid: true}, RedactedRoots: []string{"claim_token"}}
	for _, target := range []*[]byte{&row.SourceReferenceDigest, &row.BatchKeyDigest, &row.ApprovedByDigest, &row.CancelledByDigest, &row.CancelReasonDigest, &row.TargetSummaryDigest, &row.ContentPayloadDigest, &row.ContentSummaryDigest, &row.LastErrorDigest, &row.TraceIDDigest, &row.CreatedByDigest, &row.ClaimTokenDigest, &row.RetryPolicyDigest, &row.MetadataDigest, &row.TargetUnionIDsDigest, &row.ResultSummaryDigest, &row.HoldReasonDigest, &row.ExecutionIDDigest, &row.ExecutionOwnerDigest, &row.SourceKeyDigest, &row.SourcePayloadDigest, &row.SourceFieldDigest} {
		*target = make([]byte, 32)
		(*target)[0] = 1
	}
	value, err := broadcastJobHistoryValue(row)
	if err != nil || value.LegacyOutboundTaskID == nil || *value.LegacyOutboundTaskID != -7 || value.Priority != -1 || value.TargetCount != -2 || value.MaxAttempts != -6 || value.RedactedRoots[0] != "claim_token" {
		t.Fatalf("history_value_changed value=%#v err=%v", value, err)
	}
	row.SourceFieldDigest = make([]byte, 31)
	if _, err := broadcastJobHistoryValue(row); !errors.Is(err, outboundport.ErrBroadcastJobHistoryUnavailable) {
		t.Fatal("short_digest_accepted")
	}
}

func TestBroadcastJobHistoryPostgresRoundTripRollback(t *testing.T) {
	if *broadcastJobHistoryPostgresDSN == "" {
		t.Skip("set -broadcast-job-history-postgres-dsn for isolated schema 129 rollback verification")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *broadcastJobHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := outbounddb.New(pool)
	before, err := queries.CountHistoricalBroadcastJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("broadcast job history forced rollback")
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		store, reader := NewBroadcastJobHistoryStore(), NewBroadcastJobHistoryReader(tx)
		value := storeBroadcastJobHistoryValue()
		created, err := store.CreateHistoricalBroadcastJob(txCtx, value)
		if err != nil {
			return err
		}
		loaded, err := reader.GetHistoricalBroadcastJob(txCtx, created.ID)
		if err != nil || loaded.SourceFieldDigest != created.SourceFieldDigest || loaded.LegacyOutboundTaskID == nil || *loaded.LegacyOutboundTaskID != -7 {
			t.Fatalf("round_trip err=%v loaded=%#v", err, loaded)
		}
		items, total, err := reader.ListHistoricalBroadcastJobs(txCtx, outboundport.BroadcastJobHistoryQuery{})
		if err != nil || total < 1 || len(items) < 1 {
			t.Fatalf("default_page err=%v total=%d items=%d", err, total, len(items))
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback err=%v", err)
	}
	after, err := queries.CountHistoricalBroadcastJobs(ctx)
	if err != nil || before != after {
		t.Fatal("rollback_left_history_row")
	}
}

func storeBroadcastJobHistoryValue() outboundport.HistoricalBroadcastJob {
	at := time.Date(2026, 8, 28, 13, 14, 15, 123456000, time.UTC)
	legacy := int64(-7)
	digest := func(value string) [32]byte { return sha256.Sum256([]byte(value)) }
	return outboundport.HistoricalBroadcastJob{SourceID: 900000000000001, OriginalSourceType: "legacy", SourceReferenceDigest: digest("source"), SourceTable: "legacy", ScheduledFor: at, Priority: -1, BatchKeyDigest: digest("batch"), OriginalStatus: "unknown", ApprovedByDigest: digest("approved"), CancelledByDigest: digest("cancelled"), CancelReasonDigest: digest("reason"), TargetCount: -2, TargetSummaryDigest: digest("targets"), ContentType: "opaque", ContentPayloadDigest: digest("payload"), ContentSummaryDigest: digest("summary"), AttemptCount: -3, LastErrorDigest: digest("error"), LegacyOutboundTaskID: &legacy, SentCount: -4, FailedCount: -5, TraceIDDigest: digest("trace"), CreatedByDigest: digest("creator"), CreatedAt: at, UpdatedAt: at, ClaimTokenDigest: digest("claim"), RetryPolicyDigest: digest("retry"), MetadataDigest: digest("metadata"), TargetUnionIDsDigest: digest("unionids"), MaxAttempts: -6, ResultSummaryDigest: digest("result"), HoldReasonDigest: digest("hold"), ExecutionIDDigest: digest("execution"), ExecutionOwnerDigest: digest("owner"), SourceKeyDigest: digest("key"), SourcePayloadDigest: digest("payload-root"), SourceFieldDigest: digest("fields"), RedactedRoots: []string{}}
}
