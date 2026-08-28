package store

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var customerStateHistoryPostgresDSN = flag.String("customer-state-history-postgres-dsn", "", "isolated PostgreSQL DSN for schema 123 rollback verification")

func TestCustomerStateHistoryStrictReader(t *testing.T) {
	ctx := context.Background()
	if _, err := NewCustomerStateHistoryStore().GetHistoricalCustomerStatusSnapshot(ctx, 1); !errors.Is(err, contact.ErrCustomerStateHistoryUnavailable) {
		t.Fatal(err)
	}
	var pool *pgxpool.Pool
	for _, r := range []*CustomerStateHistoryReader{nil, NewCustomerStateHistoryReader(nil), NewCustomerStateHistoryReader(pool)} {
		if _, _, err := r.ListHistoricalCustomerStatusSnapshot(ctx, contact.CustomerStateHistoryQuery{Limit: 1}); !errors.Is(err, contact.ErrCustomerStateHistoryUnavailable) {
			t.Fatal(err)
		}
	}
	for _, q := range []contact.CustomerStateHistoryQuery{{}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewCustomerStateHistoryReader(nil).ListHistoricalCustomerStatusChange(ctx, q); !errors.Is(err, contact.ErrCustomerStateHistoryInvalid) {
			t.Fatal(err)
		}
	}
}
func TestCustomerStateHistoryValuesKeepPrivateAndSigned(t *testing.T) {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	row := contactdb.ContactV1CustomerStatusChange{ID: 1, SourceKeyDigest: storeD(1), SourcePayloadDigest: storeD(2), SourceFieldDigest: storeD(3), SourceID: -4, CustomerNameSnapshot: "customer", OwnerUseridSnapshot: "owner", SetByUseridDigest: storeD(4), SetAt: storeT(at), WecomTagSyncErrorHash: storeD(5), StatusFlagsDigest: storeD(6), CreatedAt: storeT(at), Unionid: "union"}
	v, err := changeValue(row)
	if err != nil || v.SourceID != -4 || v.CustomerNameSnapshot != "customer" || v.UnionID != "union" {
		t.Fatalf("%+v %v", v, err)
	}
	term, err := termValue(contactdb.ContactV1ClassTermTagHistory{ID: 2, SourceKeyDigest: storeD(7), SourcePayloadDigest: storeD(8), SourceFieldDigest: storeD(9), SourceID: -10, ClassTermNo: -11, CreatedAt: storeT(at), UpdatedAt: storeT(at), StrategySourceID: "s", GroupSourceID: "g", TagSourceID: "t"})
	if err != nil || term.SourceID != -10 || term.ClassTermNo != -11 || term.StrategySourceID != "s" {
		t.Fatalf("%+v %v", term, err)
	}
}
func TestCustomerStateHistoryPostgresRoundTripRollback(t *testing.T) {
	if *customerStateHistoryPostgresDSN == "" {
		t.Skip("set -customer-state-history-postgres-dsn for isolated schema 123 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *customerStateHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := contactdb.New(pool)
	before, err := q.CountHistoricalCustomerStatusSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("customer state rollback")
	var id int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store := NewCustomerStateHistoryStore()
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return fmt.Errorf("customer-state stage=tx: %w", err)
		}
		reader := NewCustomerStateHistoryReader(tx)
		at := time.Date(2026, 8, 28, 1, 2, 3, 123456789, time.FixedZone("+8", 8*3600))
		s := storeSnapshot(at)
		created, err := store.CreateHistoricalCustomerStatusSnapshot(txCtx, s)
		if err != nil {
			return fmt.Errorf("customer-state stage=snapshot: %w", err)
		}
		id = created.ID
		if loaded, err := reader.GetHistoricalCustomerStatusSnapshot(context.Background(), id); err != nil || !reflect.DeepEqual(loaded, created) {
			return fmt.Errorf("customer-state stage=bare-tx: %w", err)
		}
		change := storeChange(at)
		changeCreated, err := store.CreateHistoricalCustomerStatusChange(txCtx, change)
		if err != nil {
			return fmt.Errorf("customer-state stage=change: %w", err)
		}
		if loaded, err := reader.GetHistoricalCustomerStatusChange(context.Background(), changeCreated.ID); err != nil || !reflect.DeepEqual(loaded, changeCreated) {
			return fmt.Errorf("customer-state stage=change-get: %w", err)
		}
		term := storeTerm(at)
		termCreated, err := store.CreateHistoricalClassTermTagMapping(txCtx, term)
		if err != nil {
			return fmt.Errorf("customer-state stage=term: %w", err)
		}
		if loaded, err := reader.GetHistoricalClassTermTagMapping(context.Background(), termCreated.ID); err != nil || !reflect.DeepEqual(loaded, termCreated) {
			return fmt.Errorf("customer-state stage=term-get: %w", err)
		}
		items, total, err := reader.ListHistoricalCustomerStatusSnapshot(txCtx, contact.CustomerStateHistoryQuery{Limit: 1, Offset: int32(before + 1)})
		if err != nil || total != before+1 || len(items) != 0 {
			return fmt.Errorf("customer-state stage=page: %w", err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	after, err := q.CountHistoricalCustomerStatusSnapshot(ctx)
	if err != nil || after != before {
		t.Fatal("rollback retained snapshot")
	}
	if _, err := q.GetHistoricalCustomerStatusSnapshot(ctx, id); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("rolled row remains")
	}
}
func storeD(b byte) []byte                  { x := make([]byte, 32); x[0] = b; return x }
func storeT(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func storeArray(b byte) [32]byte            { var x [32]byte; x[0] = b; return x }
func storeSnapshot(at time.Time) contact.HistoricalCustomerStatusSnapshot {
	return contact.HistoricalCustomerStatusSnapshot{SourceKeyDigest: storeArray(1), SourcePayloadDigest: storeArray(2), SourceFieldDigest: storeArray(3), SetByUserIDDigest: storeArray(4), SetAt: at.UTC().Truncate(time.Microsecond), WeComTagSyncErrorHash: storeArray(5), StatusFlagsDigest: storeArray(6), CreatedAt: at.UTC().Truncate(time.Microsecond), UpdatedAt: at.Add(-time.Second).UTC().Truncate(time.Microsecond), CustomerNameSnapshot: "private", OwnerUserIDSnapshot: "owner", UnionID: "union"}
}
func storeChange(at time.Time) contact.HistoricalCustomerStatusChange {
	return contact.HistoricalCustomerStatusChange{SourceKeyDigest: storeArray(7), SourcePayloadDigest: storeArray(8), SourceFieldDigest: storeArray(9), SourceID: -1, SetByUserIDDigest: storeArray(10), SetAt: at.UTC().Truncate(time.Microsecond), WeComTagSyncErrorHash: storeArray(11), StatusFlagsDigest: storeArray(12), CreatedAt: at.UTC().Truncate(time.Microsecond), CustomerNameSnapshot: "private", OwnerUserIDSnapshot: "owner", UnionID: "union"}
}
func storeTerm(at time.Time) contact.HistoricalClassTermTagMapping {
	return contact.HistoricalClassTermTagMapping{SourceKeyDigest: storeArray(13), SourcePayloadDigest: storeArray(14), SourceFieldDigest: storeArray(15), SourceID: -2, ClassTermNo: -3, CreatedAt: at.UTC().Truncate(time.Microsecond), UpdatedAt: at.Add(-time.Second).UTC().Truncate(time.Microsecond), StrategySourceID: "s", GroupSourceID: "g", TagSourceID: "t"}
}
