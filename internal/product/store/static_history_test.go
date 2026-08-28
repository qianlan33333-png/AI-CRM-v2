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
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

var staticProductHistoryPostgresDSN = flag.String("static-product-history-postgres-dsn", "", "isolated PostgreSQL DSN for schema 122 rollback verification")

func TestStaticProductHistoryValuePreservesSignedFacts(t *testing.T) {
	at := time.Date(2026, 8, 28, 10, 11, 12, 123456000, time.UTC)
	row := productdb.ProductV1PageSliceHistory{ID: 10, SourceID: -7, ProductSourceID: 0, ImageSourceID: -3, SortOrder: -4, OriginalEnabled: false, SourceKeyDigest: staticProductHistoryDigest(1), SourcePayloadDigest: staticProductHistoryDigest(2), CreatedAt: pgtype.Timestamptz{Time: at, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: at, Valid: true}}
	value, err := staticProductHistoryValue(row)
	if err != nil || value.SourceID != -7 || value.ProductSourceID != 0 || value.ImageSourceID != -3 || value.SortOrder != -4 {
		t.Fatalf("historical fact changed: %#v, %v", value, err)
	}
	row.UpdatedAt.InfinityModifier = pgtype.NegativeInfinity
	if _, err = staticProductHistoryValue(row); !errors.Is(err, productport.ErrStaticProductHistoryUnavailable) {
		t.Fatalf("infinite time accepted: %v", err)
	}
}

func TestStaticProductHistoryStoreRequiresCallerTransactionAndStrictPage(t *testing.T) {
	ctx := context.Background()
	if _, err := NewStaticProductHistoryStore().GetHistoricalProductPageSlice(ctx, 1); !errors.Is(err, productport.ErrStaticProductHistoryUnavailable) {
		t.Fatal("store read escaped caller transaction")
	}
	var pool *pgxpool.Pool
	for _, reader := range []*StaticProductHistoryReader{nil, NewStaticProductHistoryReader(nil), NewStaticProductHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalProductPageSlice(ctx, productport.StaticProductHistoryQuery{Limit: 20}); !errors.Is(err, productport.ErrStaticProductHistoryUnavailable) {
			t.Fatalf("nil reader did not fail closed: %v", err)
		}
	}
	for _, query := range []productport.StaticProductHistoryQuery{{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewStaticProductHistoryReader(nil).ListHistoricalProductPageSlice(ctx, query); !errors.Is(err, productport.ErrStaticProductHistoryInvalid) {
			t.Fatalf("invalid page accepted: %#v, %v", query, err)
		}
	}
}

func TestStaticProductHistoryReaderPrefersCallerTransaction(t *testing.T) {
	tx := &staticProductHistoryCallerTx{}
	err := platformstore.NewUnitOfWork(staticProductHistoryCallerBeginner{tx: tx}).Within(context.Background(), func(ctx context.Context) error {
		_, err := NewStaticProductHistoryReader(nil).queries(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("caller transaction was not used: %v", err)
	}
}

func TestStaticProductHistoryPostgresRoundTripRollback(t *testing.T) {
	if *staticProductHistoryPostgresDSN == "" {
		t.Skip("set -static-product-history-postgres-dsn for isolated schema 122 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *staticProductHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := productdb.New(pool)
	before, err := queries.CountHistoricalProductPageSlice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("static product history forced rollback")
	var ids []int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		store, reader := NewStaticProductHistoryStore(), NewStaticProductHistoryReader(tx)
		at := time.Date(2026, 8, 28, 10, 11, 12, 123456000, time.UTC)
		for i := byte(1); i <= 2; i++ {
			value := staticProductHistoryStoreFixture(i, at)
			if i == 2 {
				value.SortOrder = -9
			}
			created, err := store.CreateHistoricalProductPageSlice(txCtx, value)
			if err != nil {
				return err
			}
			ids = append(ids, created.ID)
			value.ID = created.ID
			if !reflect.DeepEqual(created, value) {
				return errors.New("SQL create changed historical product fact")
			}
			loaded, err := reader.GetHistoricalProductPageSlice(txCtx, created.ID)
			if err != nil || !reflect.DeepEqual(loaded, value) {
				return errors.New("SQL product read changed historical fact")
			}
		}
		items, total, err := reader.ListHistoricalProductPageSlice(txCtx, productport.StaticProductHistoryQuery{Limit: 1, Offset: int32(before + 1)})
		if err != nil || total != before+2 || len(items) != 1 || items[0].ID != ids[1] || items[0].SortOrder != -9 {
			return errors.New("SQL product page did not use caller transaction")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback transaction: %v", err)
	}
	after, err := queries.CountHistoricalProductPageSlice(ctx)
	if err != nil || after != before {
		t.Fatalf("rollback changed count: before=%d after=%d err=%v", before, after, err)
	}
	for _, id := range ids {
		if _, err := queries.GetHistoricalProductPageSlice(ctx, id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("rolled back historical product fact remained")
		}
	}
}

func staticProductHistoryDigest(first byte) []byte {
	value := make([]byte, 32)
	value[0] = first
	return value
}
func staticProductHistoryStoreFixture(first byte, at time.Time) productport.HistoricalProductPageSlice {
	var source, payload [32]byte
	copy(source[:], staticProductHistoryDigest(first))
	copy(payload[:], staticProductHistoryDigest(first+20))
	return productport.HistoricalProductPageSlice{SourceID: -7, SourceKeyDigest: source, SourcePayloadDigest: payload, ProductSourceID: 0, ImageSourceID: -3, SortOrder: -4, OriginalEnabled: false, CreatedAt: at, UpdatedAt: at}
}

type staticProductHistoryCallerBeginner struct{ tx pgx.Tx }

func (beginner staticProductHistoryCallerBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return beginner.tx, nil
}

type staticProductHistoryCallerTx struct{ pgx.Tx }

func (*staticProductHistoryCallerTx) Commit(context.Context) error   { return nil }
func (*staticProductHistoryCallerTx) Rollback(context.Context) error { return nil }
