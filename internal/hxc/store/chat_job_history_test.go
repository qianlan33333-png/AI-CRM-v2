package store

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	hxcdb "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var hxcChatJobHistoryPostgresDSN = flag.String("hxc-chat-job-history-postgres-dsn", "", "isolated PostgreSQL DSN containing the HXC chat-job history schema for rollback verification")

func TestHXCChatJobMappingPreservesAllFields(t *testing.T) {
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	want := hxcChatJobStoreFixture(9, at)
	want.ID = 10
	got, err := chatJob(hxcChatJobStoreRow(want))
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("chat-job mapping got=%#v want=%#v err=%v", got, want, err)
	}
}

func TestHXCChatJobWriteNormalizesTimesToUTCMicroseconds(t *testing.T) {
	value := hxcChatJobStoreFixture(9, time.Date(2026, 8, 29, 10, 11, 12, 123456789, time.FixedZone("source", 8*60*60)))
	value.UpdatedAt = value.UpdatedAt.Add(987 * time.Nanosecond)
	normalized := normalizeHXCChatJobForStore(value)
	if normalized.CreatedAt.Location() != time.UTC || normalized.CreatedAt.Nanosecond() != 123456000 || normalized.UpdatedAt.Location() != time.UTC || normalized.UpdatedAt.Nanosecond()%1000 != 0 {
		t.Fatalf("time normalization changed: %#v", normalized)
	}
}

func TestHXCChatJobStoreAndReaderBoundaries(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	if _, err := NewHXCHistoryStore().GetHistoricalHXCChatJob(ctx, 1); !errors.Is(err, hxc.ErrHXCHistoryUnavailable) {
		t.Fatal("chat-job store escaped caller transaction")
	}
	invalid := hxcChatJobStoreFixture(1, at)
	invalid.SourceFieldDigest = [32]byte{}
	if _, err := NewHXCHistoryStore().CreateHistoricalHXCChatJob(ctx, invalid); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
		t.Fatal("invalid chat-job reached database")
	}
	var nilPool *pgxpool.Pool
	for _, reader := range []*HXCHistoryReader{nil, NewHXCHistoryReader(nil), NewHXCHistoryReader(nilPool)} {
		if _, _, err := reader.ListHistoricalHXCChatJob(ctx, hxc.HXCChatJobHistoryQuery{Limit: 1}); !errors.Is(err, hxc.ErrHXCHistoryUnavailable) {
			t.Fatalf("nil reader did not fail closed: %v", err)
		}
	}
	for _, query := range []hxc.HXCChatJobHistoryQuery{{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewHXCHistoryReader(nil).ListHistoricalHXCChatJob(ctx, query); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
			t.Fatalf("invalid page accepted: %#v err=%v", query, err)
		}
	}
}

func TestHXCChatJobHistoryPostgresRoundTripRollback(t *testing.T) {
	if *hxcChatJobHistoryPostgresDSN == "" {
		t.Skip("set -hxc-chat-job-history-postgres-dsn for isolated chat-job history rollback verification")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *hxcChatJobHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := hxcdb.New(pool)
	before, err := queries.CountHistoricalHXCChatJob(ctx)
	if err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	forced := errors.New("HXC chat-job history forced rollback")
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store, reader := NewHXCHistoryStore(), NewHXCHistoryReader(nil)
		job, err := store.CreateHistoricalHXCChatJob(txCtx, hxcChatJobStoreFixture(10, at))
		if err != nil {
			return fmt.Errorf("stage chat-job create: %w", err)
		}
		if loaded, err := reader.GetHistoricalHXCChatJob(txCtx, job.ID); err != nil || !reflect.DeepEqual(loaded, job) {
			return fmt.Errorf("stage chat-job context get: %v", err)
		}
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		if loaded, err := NewHXCHistoryReader(tx).GetHistoricalHXCChatJob(context.Background(), job.ID); err != nil || !reflect.DeepEqual(loaded, job) {
			return fmt.Errorf("stage chat-job bare transaction get: %v", err)
		}
		items, total, err := reader.ListHistoricalHXCChatJob(txCtx, hxc.HXCChatJobHistoryQuery{Limit: 1, Offset: int32(before)})
		if err != nil || total != before+1 || len(items) != 1 || !reflect.DeepEqual(items[0], job) {
			return fmt.Errorf("stage chat-job list: total=%d items=%d err=%v", total, len(items), err)
		}
		if _, err := store.CreateHistoricalHXCChatJob(txCtx, hxcChatJobStoreFixture(10, at)); !errors.Is(err, hxc.ErrHXCHistoryConflict) {
			return fmt.Errorf("stage chat-job conflict: %v", err)
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("rollback transaction: %v", err)
	}
	after, err := queries.CountHistoricalHXCChatJob(ctx)
	if err != nil || after != before {
		t.Fatalf("rollback count: before=%d after=%d err=%v", before, after, err)
	}
}

func hxcChatJobStoreFixture(first byte, at time.Time) hxc.HistoricalHXCChatJob {
	queue, sendRecord := int64(-3), int64(0)
	var key, payload, field [32]byte
	key[0], payload[0], field[0] = first, first+20, first+40
	return hxc.HistoricalHXCChatJob{
		SourceID: int64(first) - 20, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field,
		QueueSourceID: &queue, MemberSourceID: nil, ExternalContactID: "external", Phone: "phone", ExternalMessageID: "message", ExternalSessionID: "session", LaohuangTaskID: "task",
		RequestPayloadJSON: json.RawMessage(`{ "request" : 1 }`), AcceptedPayloadJSON: json.RawMessage("null"), CallbackPayloadJSON: json.RawMessage(`[]`),
		OriginalStatus: "legacy_status", ReplyText: "reply", ErrorCode: "code", ErrorMessage: "error", SendChannel: "legacy_channel",
		SendRecordSourceID: &sendRecord, SendResultJSON: json.RawMessage(`"result"`), CreatedAt: at, UpdatedAt: at.Add(-time.Second), FinishedAtSource: "legacy civil timestamp",
	}
}

func hxcChatJobStoreRow(value hxc.HistoricalHXCChatJob) hxcdb.HxcV1ChatJobHistory {
	return hxcdb.HxcV1ChatJobHistory{
		ID: value.ID, SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:],
		QueueSourceID: i8(value.QueueSourceID), MemberSourceID: i8(value.MemberSourceID), ExternalContactID: value.ExternalContactID, Phone: value.Phone,
		ExternalMessageID: value.ExternalMessageID, ExternalSessionID: value.ExternalSessionID, LaohuangTaskID: value.LaohuangTaskID,
		RequestPayloadJson: string(value.RequestPayloadJSON), AcceptedPayloadJson: string(value.AcceptedPayloadJSON), CallbackPayloadJson: string(value.CallbackPayloadJSON),
		OriginalStatus: value.OriginalStatus, ReplyText: value.ReplyText, ErrorCode: value.ErrorCode, ErrorMessage: value.ErrorMessage, SendChannel: value.SendChannel,
		SendRecordSourceID: i8(value.SendRecordSourceID), SendResultJson: string(value.SendResultJSON),
		CreatedAt: pgtype.Timestamptz{Time: value.CreatedAt, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: value.UpdatedAt, Valid: true}, FinishedAtSource: value.FinishedAtSource,
	}
}
