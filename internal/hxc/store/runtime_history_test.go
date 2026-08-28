package store

import (
	"context"
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

var hxcRuntimeHistoryPostgresDSN = flag.String("hxc-runtime-history-postgres-dsn", "", "isolated PostgreSQL DSN with migration 00134 for HXC runtime history rollback verification")

func TestHXCRuntimeHistoryMappingPreservesPrivateAndNullableFacts(t *testing.T) {
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	key, payload, field, private := runtimeHistoryBytes(1), runtimeHistoryBytes(2), runtimeHistoryBytes(3), runtimeHistoryBytes(4)
	sender, err := senderConfig(hxcdb.HxcV1SenderConfigHistory{ID: 10, SourceID: -4, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, PrivateDigest: private, Priority: -3, CreatedAt: pgtype.Timestamptz{Time: at, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: at.Add(-time.Second), Valid: true}})
	if err != nil || sender.SourceID != -4 || sender.SourceFieldDigest[0] != 3 || sender.PrivateDigest[0] != 4 {
		t.Fatalf("sender mapping: %#v %v", sender, err)
	}
	record, err := sendRecord(hxcdb.HxcV1SendRecordHistory{ID: 11, SourceID: 0, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, PrivateDigest: private, TargetSourceID: pgtype.Int8{Int64: -9, Valid: true}, CreatedAt: pgtype.Timestamptz{Time: at, Valid: true}, LastStatusSyncAt: pgtype.Timestamptz{}, LastRefreshedAt: pgtype.Timestamptz{Time: at.Add(time.Second), Valid: true}, SelectedCount: -1})
	if err != nil || record.SourceID != 0 || record.TargetSourceID == nil || *record.TargetSourceID != -9 || record.LastStatusSyncAt != nil || record.LastRefreshedAt == nil || record.PrivateDigest[0] != 4 {
		t.Fatalf("record mapping: %#v %v", record, err)
	}
}

func TestHXCRuntimeHistoryStoreAndReaderBoundaries(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	if _, err := NewHXCHistoryStore().GetHistoricalHXCSenderConfig(ctx, 1); !errors.Is(err, hxc.ErrHXCHistoryUnavailable) {
		t.Fatal("runtime store escaped caller transaction")
	}
	invalid := runtimeStoreSenderFixture(1, at)
	invalid.PrivateDigest = [32]byte{}
	if _, err := NewHXCHistoryStore().CreateHistoricalHXCSenderConfig(ctx, invalid); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
		t.Fatal("invalid runtime sender reached database")
	}
	var nilPool *pgxpool.Pool
	for _, reader := range []*HXCHistoryReader{nil, NewHXCHistoryReader(nil), NewHXCHistoryReader(nilPool)} {
		if _, _, err := reader.ListHistoricalHXCSendRecord(ctx, hxc.HXCHistoryQuery{Limit: 1}); !errors.Is(err, hxc.ErrHXCHistoryUnavailable) {
			t.Fatal("runtime nil reader did not fail closed")
		}
	}
	badCustomer := int64(1)
	for _, query := range []hxc.HXCHistoryQuery{{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}, {Limit: 1, CustomerID: &badCustomer}, {Limit: 1, SourceTable: "public/ignored"}} {
		if _, _, err := NewHXCHistoryReader(nil).ListHistoricalHXCSenderConfig(ctx, query); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
			t.Fatalf("invalid runtime page accepted: %#v: %v", query, err)
		}
	}
}

func TestHXCRuntimeHistoryPostgresRoundTripRollback(t *testing.T) {
	if *hxcRuntimeHistoryPostgresDSN == "" {
		t.Skip("set -hxc-runtime-history-postgres-dsn for isolated migration 00134 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *hxcRuntimeHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	before, err := hxcRuntimeHistoryCounts(ctx, hxcdb.New(pool))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	forced := errors.New("HXC runtime history forced rollback")
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store, reader := NewHXCHistoryStore(), NewHXCHistoryReader(nil)
		sender, err := store.CreateHistoricalHXCSenderConfig(txCtx, runtimeStoreSenderFixture(10, at))
		if err != nil {
			return fmt.Errorf("stage sender create: %w", err)
		}
		if loaded, err := reader.GetHistoricalHXCSenderConfig(txCtx, sender.ID); err != nil || !reflect.DeepEqual(loaded, sender) {
			return fmt.Errorf("stage sender get: %v", err)
		}
		record, err := store.CreateHistoricalHXCSendRecord(txCtx, runtimeStoreSendFixture(11, at))
		if err != nil {
			return fmt.Errorf("stage record create: %w", err)
		}
		if loaded, err := reader.GetHistoricalHXCSendRecord(txCtx, record.ID); err != nil || !reflect.DeepEqual(loaded, record) {
			return fmt.Errorf("stage record get: %v", err)
		}
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		if loaded, err := NewHXCHistoryReader(tx).GetHistoricalHXCSendRecord(context.Background(), record.ID); err != nil || !reflect.DeepEqual(loaded, record) {
			return fmt.Errorf("stage bare transaction get: %v", err)
		}
		items, total, err := reader.ListHistoricalHXCSendRecord(txCtx, hxc.HXCHistoryQuery{Limit: 1})
		if err != nil || total < 1 || len(items) != 1 || items[len(items)-1].ID != record.ID {
			return fmt.Errorf("stage record list: total=%d items=%d err=%v", total, len(items), err)
		}
		if _, err := store.CreateHistoricalHXCSenderConfig(txCtx, runtimeStoreSenderFixture(10, at)); !errors.Is(err, hxc.ErrHXCHistoryConflict) {
			return fmt.Errorf("stage sender conflict: %v", err)
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("rollback transaction: %v", err)
	}
	after, err := hxcRuntimeHistoryCounts(ctx, hxcdb.New(pool))
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("rollback counts: before=%v after=%v err=%v", before, after, err)
	}
}

func hxcRuntimeHistoryCounts(ctx context.Context, queries *hxcdb.Queries) ([]int64, error) {
	sender, err := queries.CountHistoricalHXCSenderConfig(ctx)
	if err != nil {
		return nil, err
	}
	record, err := queries.CountHistoricalHXCSendRecord(ctx)
	if err != nil {
		return nil, err
	}
	return []int64{sender, record}, nil
}

func runtimeHistoryBytes(first byte) []byte {
	value := make([]byte, 32)
	value[0] = first
	return value
}
func runtimeStoreIdentity(first byte) hxc.HistoricalHXCRuntimeIdentity {
	var key, payload, field, private [32]byte
	key[0], payload[0], field[0], private[0] = first, first+20, first+40, first+60
	return hxc.HistoricalHXCRuntimeIdentity{SourceID: int64(first) - 20, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, PrivateDigest: private}
}
func runtimeStoreSenderFixture(first byte, at time.Time) hxc.HistoricalHXCSenderConfig {
	return hxc.HistoricalHXCSenderConfig{HistoricalHXCRuntimeIdentity: runtimeStoreIdentity(first), Priority: -1, OriginalIsActive: false, CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}
func runtimeStoreSendFixture(first byte, at time.Time) hxc.HistoricalHXCSendRecord {
	target := int64(-2)
	last := at.Add(time.Second)
	return hxc.HistoricalHXCSendRecord{HistoricalHXCRuntimeIdentity: runtimeStoreIdentity(first), TaskType: "", OriginalStatus: "", SelectedCount: -1, EligibleCount: 0, SentCount: 1, SkippedCount: -2, PlannedCount: 3, QueuedCount: -4, DispatchingCount: 5, SucceededCount: -6, FailedCount: 7, BlockedCount: -8, CancelledCount: 9, ImageCount: -10, TargetSourceID: &target, CreatedAt: at, LastStatusSyncAt: &last}
}
