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
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var staticMediaHistoryPostgresDSN = flag.String("static-media-history-postgres-dsn", "", "isolated PostgreSQL DSN for schema 122 rollback verification")

func TestStaticMediaHistoryValuePreservesSignedAndNullableFacts(t *testing.T) {
	at := time.Date(2026, 8, 28, 10, 11, 12, 123456000, time.UTC)
	row := mediadb.MediaV1GroupInviteHistory{ID: 10, SourceID: -7, SourceKeyDigest: staticMediaHistoryDigest(1), SourcePayloadDigest: staticMediaHistoryDigest(2), Name: "", Title: "title", Description: "\n", OriginalState: "old", OriginalAutoCreate: true, RoomBaseName: "", RoomBaseSourceID: pgtype.Int8{Int64: -3, Valid: true}, OriginalEnabled: false, OriginalBindingState: "unbound", CreatedAt: pgtype.Timestamptz{Time: at, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: at, Valid: true}}
	value, err := staticMediaHistoryValue(row)
	if err != nil || value.SourceID != -7 || value.RoomBaseSourceID == nil || *value.RoomBaseSourceID != -3 || value.Name != "" || value.Description != "\n" {
		t.Fatalf("historical fact changed: %#v, %v", value, err)
	}
	row.RoomBaseSourceID = pgtype.Int8{}
	value, err = staticMediaHistoryValue(row)
	if err != nil || value.RoomBaseSourceID != nil {
		t.Fatalf("nullable source reference changed: %#v, %v", value, err)
	}
	row.CreatedAt.InfinityModifier = pgtype.Infinity
	if _, err = staticMediaHistoryValue(row); !errors.Is(err, mediaport.ErrStaticMediaHistoryUnavailable) {
		t.Fatalf("infinite time accepted: %v", err)
	}
}

func TestStaticMediaHistoryStoreRequiresCallerTransactionAndStrictPage(t *testing.T) {
	ctx := context.Background()
	if _, err := NewStaticMediaHistoryStore().GetHistoricalGroupInvite(ctx, 1); !errors.Is(err, mediaport.ErrStaticMediaHistoryUnavailable) {
		t.Fatal("store read escaped caller transaction")
	}
	var pool *pgxpool.Pool
	for _, reader := range []*StaticMediaHistoryReader{nil, NewStaticMediaHistoryReader(nil), NewStaticMediaHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalGroupInvite(ctx, mediaport.StaticMediaHistoryQuery{Limit: 20}); !errors.Is(err, mediaport.ErrStaticMediaHistoryUnavailable) {
			t.Fatalf("nil reader did not fail closed: %v", err)
		}
	}
	for _, query := range []mediaport.StaticMediaHistoryQuery{{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewStaticMediaHistoryReader(nil).ListHistoricalGroupInvite(ctx, query); !errors.Is(err, mediaport.ErrStaticMediaHistoryInvalid) {
			t.Fatalf("invalid page accepted: %#v, %v", query, err)
		}
	}
}

func TestStaticMediaHistoryReaderPrefersCallerTransaction(t *testing.T) {
	tx := &staticMediaHistoryCallerTx{}
	err := platformstore.NewUnitOfWork(staticMediaHistoryCallerBeginner{tx: tx}).Within(context.Background(), func(ctx context.Context) error {
		_, err := NewStaticMediaHistoryReader(nil).queries(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("caller transaction was not used: %v", err)
	}
}

func TestStaticMediaHistoryPostgresRoundTripRollback(t *testing.T) {
	if *staticMediaHistoryPostgresDSN == "" {
		t.Skip("set -static-media-history-postgres-dsn for isolated schema 122 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *staticMediaHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := mediadb.New(pool)
	before, err := queries.CountHistoricalGroupInvite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("static media history forced rollback")
	var ids []int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		store, reader := NewStaticMediaHistoryStore(), NewStaticMediaHistoryReader(tx)
		at := time.Date(2026, 8, 28, 10, 11, 12, 123456000, time.UTC)
		for i := byte(1); i <= 2; i++ {
			value := staticMediaHistoryStoreFixture(i, at)
			if i == 2 {
				value.Title = " \nsecond\t "
			}
			created, err := store.CreateHistoricalGroupInvite(txCtx, value)
			if err != nil {
				return err
			}
			ids = append(ids, created.ID)
			value.ID = created.ID
			if !reflect.DeepEqual(created, value) {
				return errors.New("SQL create changed historical media fact")
			}
			loaded, err := reader.GetHistoricalGroupInvite(txCtx, created.ID)
			if err != nil || !reflect.DeepEqual(loaded, value) {
				return errors.New("SQL media read changed historical fact")
			}
		}
		items, total, err := reader.ListHistoricalGroupInvite(txCtx, mediaport.StaticMediaHistoryQuery{Limit: 1, Offset: int32(before + 1)})
		if err != nil || total != before+2 || len(items) != 1 || items[0].ID != ids[1] || items[0].Title != " \nsecond\t " {
			return errors.New("SQL media page did not use caller transaction")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback transaction: %v", err)
	}
	after, err := queries.CountHistoricalGroupInvite(ctx)
	if err != nil || after != before {
		t.Fatalf("rollback changed count: before=%d after=%d err=%v", before, after, err)
	}
	for _, id := range ids {
		if _, err := queries.GetHistoricalGroupInvite(ctx, id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("rolled back historical media fact remained")
		}
	}
}

func staticMediaHistoryDigest(first byte) []byte {
	value := make([]byte, 32)
	value[0] = first
	return value
}
func staticMediaHistoryStoreFixture(first byte, at time.Time) mediaport.HistoricalGroupInvite {
	var source, payload [32]byte
	copy(source[:], staticMediaHistoryDigest(first))
	copy(payload[:], staticMediaHistoryDigest(first+20))
	base := int64(-3)
	return mediaport.HistoricalGroupInvite{SourceID: -7, SourceKeyDigest: source, SourcePayloadDigest: payload, Name: "", Title: "title", Description: "\n", OriginalState: "old", OriginalAutoCreate: true, RoomBaseName: "", RoomBaseSourceID: &base, OriginalEnabled: false, OriginalBindingState: "unbound", CreatedAt: at, UpdatedAt: at}
}

type staticMediaHistoryCallerBeginner struct{ tx pgx.Tx }

func (beginner staticMediaHistoryCallerBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return beginner.tx, nil
}

type staticMediaHistoryCallerTx struct{ pgx.Tx }

func (*staticMediaHistoryCallerTx) Commit(context.Context) error   { return nil }
func (*staticMediaHistoryCallerTx) Rollback(context.Context) error { return nil }
