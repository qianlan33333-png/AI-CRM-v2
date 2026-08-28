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

var hxcHistoryPostgresDSN = flag.String("hxc-history-store-postgres-dsn", "", "isolated PostgreSQL DSN with migration 00121 for HXC history rollback verification")

type hxcHistoryTestRow struct {
	values []any
	err    error
}

func (r hxcHistoryTestRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(r.values[i]))
	}
	return nil
}

func TestHXCHistoryMappingPreservesHistoricalFacts(t *testing.T) {
	at := time.Date(2026, 8, 28, 10, 11, 12, 123456000, time.UTC)
	key, payload := hxcHistoryDigest(1), hxcHistoryDigest(2)
	metaValue, err := meta(hxcdb.HxcV1DashboardRefreshHistory{ID: 10, SourceID: -2, SourceKeyDigest: key, SourcePayloadDigest: payload, StartedAt: pgtype.Timestamptz{Time: at, Valid: true}})
	if err != nil || metaValue.SourceID != -2 {
		t.Fatalf("meta mapping: %#v %v", metaValue, err)
	}
	date := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	snapshotValue, err := snapshot(hxcdb.HxcV1DashboardObservation{ID: 11, SourceID: -3, SourceKeyDigest: key, SourcePayloadDigest: payload, Observation: "observed_snapshot", ObservedAt: pgtype.Timestamptz{Time: at, Valid: true}, CrmCreatedAt: pgtype.Date{Time: date, Valid: true}})
	if err != nil || snapshotValue.CRMCreatedAt == nil || *snapshotValue.CRMCreatedAt != "2026-02-03" {
		t.Fatalf("snapshot mapping: %#v %v", snapshotValue, err)
	}
	activationValue, err := activation(hxcdb.HxcV1ActivationObservation{ID: 12, SourceID: -4, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceTable: "public/user_ops_activation_status_source", CreatedAt: pgtype.Timestamptz{Time: at, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: at.Add(-time.Second), Valid: true}})
	if err != nil || activationValue.SourceID != -4 {
		t.Fatalf("activation mapping: %#v %v", activationValue, err)
	}
	leadValue, err := lead(hxcdb.HxcV1ExperienceLeadHistory{ID: 13, SourceID: -5, SourceKeyDigest: key, SourcePayloadDigest: payload, CreatedAt: pgtype.Timestamptz{Time: at, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: at.Add(-time.Second), Valid: true}})
	if err != nil || leadValue.SourceID != -5 {
		t.Fatalf("lead mapping: %#v %v", leadValue, err)
	}
	batchValue, err := batch(hxcdb.HxcV1ImportBatchHistory{ID: 14, SourceID: -6, SourceKeyDigest: key, SourcePayloadDigest: payload, CreatedAt: pgtype.Timestamptz{Time: at, Valid: true}, TotalRows: -1})
	if err != nil || batchValue.TotalRows != -1 {
		t.Fatalf("batch mapping: %#v %v", batchValue, err)
	}
}

func TestHXCHistoryStoreInputAndReaderBoundaries(t *testing.T) {
	ctx := context.Background()
	if _, err := NewHXCHistoryStore().GetHistoricalHXCBatch(ctx, 1); !errors.Is(err, hxc.ErrHXCHistoryUnavailable) {
		t.Fatal("store escaped caller transaction")
	}
	invalid := hxcMetaFixture(1, time.Date(2026, 8, 28, 10, 11, 12, 0, time.UTC))
	invalid.SourceKeyDigest = [32]byte{}
	if _, err := NewHXCHistoryStore().CreateHistoricalHXCMeta(ctx, invalid); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
		t.Fatal("invalid input reached the database")
	}
	var nilPool *pgxpool.Pool
	for _, reader := range []*HXCHistoryReader{nil, NewHXCHistoryReader(nil), NewHXCHistoryReader(nilPool)} {
		if _, _, err := reader.ListHistoricalHXCBatch(ctx, hxc.HXCHistoryQuery{Limit: 1}); !errors.Is(err, hxc.ErrHXCHistoryUnavailable) {
			t.Fatal("nil reader did not fail closed")
		}
	}
	reader := NewHXCHistoryReader(nil)
	badCustomer := int64(0)
	for _, q := range []hxc.HXCHistoryQuery{{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}, {CustomerID: &badCustomer, Limit: 1}} {
		if _, _, err := reader.ListHistoricalHXCSnapshot(ctx, q); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
			t.Fatalf("invalid snapshot page accepted: %#v", q)
		}
	}
	validCustomer := int64(1)
	if _, _, err := reader.ListHistoricalHXCMeta(ctx, hxc.HXCHistoryQuery{CustomerID: &validCustomer, Limit: 1}); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
		t.Fatal("meta customer filter was ignored")
	}
	if _, _, err := reader.ListHistoricalHXCActivation(ctx, hxc.HXCHistoryQuery{CustomerID: &validCustomer, Limit: 1}); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
		t.Fatal("activation customer filter was ignored")
	}
	if _, _, err := reader.ListHistoricalHXCActivation(ctx, hxc.HXCHistoryQuery{SourceTable: "invalid", Limit: 1}); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
		t.Fatal("unknown activation source table accepted")
	}
	if _, _, err := reader.ListHistoricalHXCLead(ctx, hxc.HXCHistoryQuery{SourceTable: "public/user_ops_activation_status_source", Limit: 1}); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
		t.Fatal("lead source table filter was ignored")
	}
}

func TestHXCHistoryPostgresRoundTripRollback(t *testing.T) {
	if *hxcHistoryPostgresDSN == "" {
		t.Skip("set -hxc-history-store-postgres-dsn for isolated migration 00121 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *hxcHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	before, err := hxcHistoryCounts(ctx, hxcdb.New(pool))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 28, 10, 11, 12, 123456000, time.UTC)
	forced := errors.New("HXC history forced rollback")
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store, reader := NewHXCHistoryStore(), NewHXCHistoryReader(nil)
		meta, err := store.CreateHistoricalHXCMeta(txCtx, hxcMetaFixture(10, at))
		if err != nil {
			return fmt.Errorf("stage meta create: %w", err)
		}
		if loaded, err := reader.GetHistoricalHXCMeta(txCtx, meta.ID); err != nil || !reflect.DeepEqual(loaded, meta) {
			return fmt.Errorf("stage meta get: %v", err)
		}
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		if loaded, err := NewHXCHistoryReader(tx).GetHistoricalHXCMeta(context.Background(), meta.ID); err != nil || !reflect.DeepEqual(loaded, meta) {
			return fmt.Errorf("stage meta bare transaction get: %v", err)
		}
		snapshot, err := store.CreateHistoricalHXCSnapshot(txCtx, hxcSnapshotFixture(11, at))
		if err != nil {
			return fmt.Errorf("stage snapshot create: %w", err)
		}
		if loaded, err := reader.GetHistoricalHXCSnapshot(txCtx, snapshot.ID); err != nil || !reflect.DeepEqual(loaded, snapshot) {
			return fmt.Errorf("stage snapshot get: %v", err)
		}
		activation, err := store.CreateHistoricalHXCActivation(txCtx, hxcActivationFixture(12, at))
		if err != nil {
			return fmt.Errorf("stage activation create: %w", err)
		}
		if loaded, err := reader.GetHistoricalHXCActivation(txCtx, activation.ID); err != nil || !reflect.DeepEqual(loaded, activation) {
			return fmt.Errorf("stage activation get: %v", err)
		}
		lead, err := store.CreateHistoricalHXCLead(txCtx, hxcLeadFixture(13, at))
		if err != nil {
			return fmt.Errorf("stage lead create: %w", err)
		}
		if loaded, err := reader.GetHistoricalHXCLead(txCtx, lead.ID); err != nil || !reflect.DeepEqual(loaded, lead) {
			return fmt.Errorf("stage lead get: %v", err)
		}
		batch, err := store.CreateHistoricalHXCBatch(txCtx, hxcBatchFixture(14, at))
		if err != nil {
			return fmt.Errorf("stage batch create: %w", err)
		}
		if loaded, err := reader.GetHistoricalHXCBatch(txCtx, batch.ID); err != nil || !reflect.DeepEqual(loaded, batch) {
			return fmt.Errorf("stage batch get: %v", err)
		}
		items, total, err := reader.ListHistoricalHXCActivation(txCtx, hxc.HXCHistoryQuery{SourceTable: activation.SourceTable, Limit: 1})
		if err != nil || total < 1 || len(items) != 1 || items[len(items)-1].ID != activation.ID {
			return fmt.Errorf("activation list: total=%d items=%d err=%v", total, len(items), err)
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("rollback transaction: %v", err)
	}
	after, err := hxcHistoryCounts(ctx, hxcdb.New(pool))
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("rollback counts: before=%v after=%v err=%v", before, after, err)
	}
}

func hxcHistoryCounts(ctx context.Context, q *hxcdb.Queries) ([]int64, error) {
	meta, err := q.CountHistoricalHXCMeta(ctx)
	if err != nil {
		return nil, err
	}
	snapshot, err := q.CountHistoricalHXCSnapshot(ctx, pgtype.Int8{})
	if err != nil {
		return nil, err
	}
	activation, err := q.CountHistoricalHXCActivation(ctx, "")
	if err != nil {
		return nil, err
	}
	lead, err := q.CountHistoricalHXCLead(ctx)
	if err != nil {
		return nil, err
	}
	batch, err := q.CountHistoricalHXCBatch(ctx)
	if err != nil {
		return nil, err
	}
	return []int64{meta, snapshot, activation, lead, batch}, nil
}

func hxcIdentityFixture(first byte) hxc.HistoricalHXCIdentity {
	var key, payload [32]byte
	key[0], payload[0] = first, first+40
	return hxc.HistoricalHXCIdentity{SourceID: -1, SourceKeyDigest: key, SourcePayloadDigest: payload}
}
func hxcHistoryDigest(first byte) []byte { value := make([]byte, 32); value[0] = first; return value }
func hxcMetaFixture(first byte, at time.Time) hxc.HistoricalHXCMeta {
	return hxc.HistoricalHXCMeta{HistoricalHXCIdentity: hxcIdentityFixture(first), StartedAt: at, Status: "", TriggerSource: ""}
}
func hxcSnapshotFixture(first byte, at time.Time) hxc.HistoricalHXCSnapshot {
	date := "2026-02-03"
	return hxc.HistoricalHXCSnapshot{HistoricalHXCIdentity: hxcIdentityFixture(first), Observation: "observed_snapshot", ObservedAt: at, CRMCreatedAt: &date, LastQuestionnaireAt: nil, SubscriptionPeriodStart: &date}
}
func hxcActivationFixture(first byte, at time.Time) hxc.HistoricalHXCActivation {
	return hxc.HistoricalHXCActivation{HistoricalHXCIdentity: hxcIdentityFixture(first), SourceTable: "public/user_ops_activation_status_source", OriginalState: "", CreatedAt: at, UpdatedAt: at}
}
func hxcLeadFixture(first byte, at time.Time) hxc.HistoricalHXCLead {
	return hxc.HistoricalHXCLead{HistoricalHXCIdentity: hxcIdentityFixture(first), OriginalType: "", CreatedAt: at, UpdatedAt: at}
}
func hxcBatchFixture(first byte, at time.Time) hxc.HistoricalHXCBatch {
	return hxc.HistoricalHXCBatch{HistoricalHXCIdentity: hxcIdentityFixture(first), ImportType: "", CreatedAt: at}
}
