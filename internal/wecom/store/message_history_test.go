package store

import (
	"context"
	"errors"
	"flag"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
	wecomdb "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store/generated"
)

var messageHistoryPostgresDSN = flag.String("message-history-store-postgres-dsn", "", "isolated PostgreSQL DSN for schema 115 rollback verification")

func TestMessageHistoryStorePreservesNullableHistoricalFacts(t *testing.T) {
	at := time.Date(2025, 2, 3, 4, 5, 6, 123456000, time.UTC)
	row := wecomdb.WecomV1MessageHistory{ID: 10, SourceID: 99, ChatType: "private", MessageType: "text", OriginalSendTime: "2025-02-03 12:05:06", SendTimeBasis: "civil_unzoned", CreatedAt: pgtype.Timestamptz{Time: at, Valid: true}, SourcePayloadDigest: make([]byte, 32)}
	row.SourcePayloadDigest[0] = 1
	value, err := messageHistoryValue(row)
	if err != nil || value.Sequence != nil || value.CustomerID != nil || value.ContentMasked != nil || value.SentAt != nil || value.OriginalSendTime != row.OriginalSendTime {
		t.Fatalf("lost nullable historical facts: %v", err)
	}
	row.Sequence = pgtype.Int8{Int64: -1, Valid: true}
	row.ContentMasked = pgtype.Text{String: " \nmasked [masked-phone]\t ", Valid: true}
	value, err = messageHistoryValue(row)
	if err != nil || *value.Sequence != -1 || *value.ContentMasked != row.ContentMasked.String {
		t.Fatal("history text/sequence changed")
	}
	row.SentAt = pgtype.Timestamptz{Time: at, Valid: true}
	if _, err = messageHistoryValue(row); !errors.Is(err, wecomport.ErrMessageHistoryUnavailable) {
		t.Fatal("civil clock acquired instant")
	}
	row.SendTimeBasis, row.OriginalSendTime = "explicit_offset", "2025-02-03T12:05:06.123456+08:00"
	if _, err = messageHistoryValue(row); err != nil {
		t.Fatal(err)
	}
	row.CreatedAt.InfinityModifier = pgtype.Infinity
	if _, err = messageHistoryValue(row); !errors.Is(err, wecomport.ErrMessageHistoryUnavailable) {
		t.Fatal("infinite source time accepted")
	}
}

func TestMessageHistoryStoreRequiresCallerTransactionAndReaderDependency(t *testing.T) {
	ctx := context.Background()
	if _, err := NewMessageHistoryStore().GetHistoricalMessage(ctx, 1); !errors.Is(err, wecomport.ErrMessageHistoryUnavailable) {
		t.Fatal("read escaped caller transaction")
	}
	var pool *pgxpool.Pool
	for _, reader := range []*MessageHistoryReader{nil, NewMessageHistoryReader(nil), NewMessageHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalMessages(ctx, wecomport.MessageHistoryQuery{Limit: 20}); !errors.Is(err, wecomport.ErrMessageHistoryUnavailable) {
			t.Fatal("nil reader did not fail closed")
		}
	}
	for _, q := range []wecomport.MessageHistoryQuery{{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}, {Limit: 1, ChatType: "invented"}} {
		if _, _, err := NewMessageHistoryReader(nil).ListHistoricalMessages(ctx, q); !errors.Is(err, wecomport.ErrMessageHistoryInvalid) {
			t.Fatal("invalid pagination accepted")
		}
	}
}

func TestMessageHistoryPostgresRoundTripRollback(t *testing.T) {
	if *messageHistoryPostgresDSN == "" {
		t.Skip("set -message-history-store-postgres-dsn for isolated schema 115 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *messageHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := wecomdb.New(pool)
	before, err := queries.CountHistoricalMessages(ctx, wecomdb.CountHistoricalMessagesParams{})
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("message history forced rollback")
	ids := make([]int64, 0, 3)
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		store, reader := NewMessageHistoryStore(), NewMessageHistoryReader(tx)
		at := time.Date(2025, 2, 3, 4, 5, 6, 123456000, time.UTC)
		customer, sequence, content := int64(999999999), int64(-2), " \nmessage [masked-phone]\t "
		for i := 0; i < 3; i++ {
			value := wecomport.HistoricalMessage{SourceID: int64(900000000000001 + i), ChatType: "private", MessageType: "text", OriginalSendTime: "2025-02-03 12:05:06", SendTimeBasis: "civil_unzoned", CreatedAt: at, SourcePayloadDigest: [32]byte{1}}
			if i > 0 {
				value.CustomerID, value.Sequence, value.ContentMasked = &customer, &sequence, &content
			}
			if i == 2 {
				value.ChatType, value.OriginalSendTime, value.SendTimeBasis, value.SentAt = "group", "2025-02-03T12:05:06.123456+08:00", "explicit_offset", &at
			}
			created, err := store.CreateHistoricalMessage(txCtx, value)
			if err != nil {
				return err
			}
			ids = append(ids, created.ID)
			value.ID = created.ID
			if !reflect.DeepEqual(value, created) {
				t.Fatal("SQL create changed historical fields")
			}
			loaded, err := reader.GetHistoricalMessage(txCtx, created.ID)
			if err != nil || !reflect.DeepEqual(loaded, value) {
				t.Fatalf("SQL round trip failed: %v", err)
			}
		}
		items, total, err := reader.ListHistoricalMessages(txCtx, wecomport.MessageHistoryQuery{CustomerID: &customer, Limit: 1, Offset: 1})
		if err != nil || total != 2 || len(items) != 1 || items[0].ChatType != "group" {
			t.Fatalf("SQL paging failed: total=%d count=%d err=%v", total, len(items), err)
		}
		items, total, err = reader.ListHistoricalMessages(txCtx, wecomport.MessageHistoryQuery{CustomerID: &customer, ChatType: "private", Limit: 20})
		if err != nil || total != 1 || len(items) != 1 || items[0].ChatType != "private" {
			t.Fatal("SQL customer/chat filter failed")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback transaction: %v", err)
	}
	after, err := queries.CountHistoricalMessages(ctx, wecomdb.CountHistoricalMessagesParams{})
	if err != nil || after != before {
		t.Fatal("forced rollback did not preserve history")
	}
	for _, id := range ids {
		if _, err := queries.GetHistoricalMessage(ctx, id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("rolled back history row remained")
		}
	}
}
