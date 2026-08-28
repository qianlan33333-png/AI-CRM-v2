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
	cycle "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
	cycledb "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var staticCycleHistoryPostgresDSN = flag.String("static-cycle-history-postgres-dsn", "", "isolated PostgreSQL DSN for schema 122 rollback verification")

func TestStaticCycleHistoryValuesPreserveNullableAndSignedFacts(t *testing.T) {
	at := time.Date(2026, 8, 28, 1, 2, 3, 456789000, time.UTC)
	key, payload := staticCycleStoreDigest(1), staticCycleStoreDigest(2)
	strategy, err := cycleStrategyValue(cycledb.OperationCycleV1StrategyHistory{ID: 1, SourceID: -2, SourceKeyDigest: key, SourcePayloadDigest: payload, CurrentVersion: -3, CreatedAt: staticCycleStoreTime(at), UpdatedAt: staticCycleStoreTime(at.Add(-time.Second))})
	if err != nil || strategy.SourceID != -2 || strategy.CurrentVersion != -3 || !strategy.UpdatedAt.Before(strategy.CreatedAt) {
		t.Fatalf("strategy historical values changed: %#v, %v", strategy, err)
	}
	version, err := cycleVersionValue(cycledb.OperationCycleV1VersionHistory{ID: 2, SourceID: -4, SourceKeyDigest: key, SourcePayloadDigest: payload, StrategySourceID: -5, StrategyHistoryID: 1, Version: -6, EffectiveFrom: pgtype.Timestamptz{}, ConfirmedAt: staticCycleStoreTime(at), CreatedAt: staticCycleStoreTime(at)})
	if err != nil || version.EffectiveFrom != nil || version.ConfirmedAt == nil || version.Version != -6 {
		t.Fatalf("version nullable/signed changed: %#v, %v", version, err)
	}
	document, err := cycleDocumentValue(cycledb.OperationCycleV1DocumentHistory{ID: 3, SourceID: -7, SourceKeyDigest: key, SourcePayloadDigest: payload, StrategyVersionSourceID: -8, VersionHistoryID: 2, ExecutionGuideGeneratedAt: pgtype.Timestamptz{}, CopyGuideGeneratedAt: staticCycleStoreTime(at), MeasurementGuideGeneratedAt: pgtype.Timestamptz{}, CreatedAt: staticCycleStoreTime(at)})
	if err != nil || document.ExecutionGuideGeneratedAt != nil || document.CopyGuideGeneratedAt == nil || document.MeasurementGuideGeneratedAt != nil {
		t.Fatalf("document nullable/signed changed: %#v, %v", document, err)
	}
	bad := cycledb.OperationCycleV1StrategyHistory{ID: 1, SourceKeyDigest: key, SourcePayloadDigest: payload, CreatedAt: staticCycleStoreTime(at), UpdatedAt: staticCycleStoreTime(at)}
	bad.CreatedAt.InfinityModifier = pgtype.Infinity
	if _, err := cycleStrategyValue(bad); !errors.Is(err, cycle.ErrStaticCycleHistoryUnavailable) {
		t.Fatalf("infinite timestamp = %v", err)
	}
}

func TestStaticCycleHistoryRequiresCallerTransactionAndStrictFilters(t *testing.T) {
	ctx := context.Background()
	if _, err := NewStaticCycleHistoryStore().GetHistoricalCycleStrategy(ctx, 1); !errors.Is(err, cycle.ErrStaticCycleHistoryUnavailable) {
		t.Fatalf("store escaped caller transaction: %v", err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*StaticCycleHistoryReader{nil, NewStaticCycleHistoryReader(nil), NewStaticCycleHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalCycleStrategy(ctx, cycle.StaticCycleHistoryQuery{Limit: 1}); !errors.Is(err, cycle.ErrStaticCycleHistoryUnavailable) {
			t.Fatalf("typed nil reader: %v", err)
		}
	}
	for _, query := range []cycle.StaticCycleHistoryQuery{{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}, {Limit: 1, StrategyHistoryID: staticCycleStoreInt64(1)}, {Limit: 1, VersionHistoryID: staticCycleStoreInt64(1)}} {
		if _, _, err := NewStaticCycleHistoryReader(nil).ListHistoricalCycleStrategy(ctx, query); !errors.Is(err, cycle.ErrStaticCycleHistoryInvalid) {
			t.Fatalf("strategy query %#v: %v", query, err)
		}
	}
	for _, query := range []cycle.StaticCycleHistoryQuery{{Limit: 1, VersionHistoryID: staticCycleStoreInt64(1)}, {Limit: 1, StrategyHistoryID: staticCycleStoreInt64(0)}} {
		if _, _, err := NewStaticCycleHistoryReader(nil).ListHistoricalCycleVersion(ctx, query); !errors.Is(err, cycle.ErrStaticCycleHistoryInvalid) {
			t.Fatalf("version query %#v: %v", query, err)
		}
	}
	for _, query := range []cycle.StaticCycleHistoryQuery{{Limit: 1, StrategyHistoryID: staticCycleStoreInt64(1)}, {Limit: 1, VersionHistoryID: staticCycleStoreInt64(0)}} {
		if _, _, err := NewStaticCycleHistoryReader(nil).ListHistoricalCycleDocument(ctx, query); !errors.Is(err, cycle.ErrStaticCycleHistoryInvalid) {
			t.Fatalf("document query %#v: %v", query, err)
		}
	}
}

func TestStaticCycleHistoryPostgresRoundTripRollback(t *testing.T) {
	if *staticCycleHistoryPostgresDSN == "" {
		t.Skip("set -static-cycle-history-postgres-dsn for isolated schema 122 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *staticCycleHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := cycledb.New(pool)
	strategyBefore, err := queries.CountHistoricalCycleStrategy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	versionBefore, err := queries.CountHistoricalCycleVersion(ctx, pgtype.Int8{})
	if err != nil {
		t.Fatal(err)
	}
	documentBefore, err := queries.CountHistoricalCycleDocument(ctx, pgtype.Int8{})
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("static cycle forced rollback")
	var strategyID, versionID, documentID int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store := NewStaticCycleHistoryStore()
		readerPool := NewStaticCycleHistoryReader(pool)
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return fmt.Errorf("static cycle stage=tx: %w", err)
		}
		readerTx := NewStaticCycleHistoryReader(tx)
		at := time.Date(2026, 8, 28, 1, 2, 3, 456789123, time.FixedZone("+8", 8*3600))
		strategy, err := store.CreateHistoricalCycleStrategy(txCtx, staticCycleStoreStrategy(1, at))
		if err != nil {
			return fmt.Errorf("static cycle stage=create strategy: %w", err)
		}
		strategyID = strategy.ID
		if loaded, err := readerPool.GetHistoricalCycleStrategy(txCtx, strategy.ID); err != nil || !reflect.DeepEqual(loaded, strategy) {
			return fmt.Errorf("static cycle stage=pool caller-tx strategy: %w", err)
		}
		if loaded, err := readerTx.GetHistoricalCycleStrategy(context.Background(), strategy.ID); err != nil || !reflect.DeepEqual(loaded, strategy) {
			return fmt.Errorf("static cycle stage=bare-tx strategy: %w", err)
		}
		versionFact := staticCycleStoreVersion(2, at)
		versionFact.StrategyHistoryID = strategy.ID
		version, err := store.CreateHistoricalCycleVersion(txCtx, versionFact)
		if err != nil {
			return fmt.Errorf("static cycle stage=create version: %w", err)
		}
		versionID = version.ID
		documentFact := staticCycleStoreDocument(3, at)
		documentFact.VersionHistoryID = version.ID
		document, err := store.CreateHistoricalCycleDocument(txCtx, documentFact)
		if err != nil {
			return fmt.Errorf("static cycle stage=create document: %w", err)
		}
		documentID = document.ID
		versions, total, err := readerPool.ListHistoricalCycleVersion(txCtx, cycle.StaticCycleHistoryQuery{StrategyHistoryID: &strategy.ID, Limit: 1})
		if err != nil || total != 1 || len(versions) != 1 || versions[0].ID != version.ID {
			return fmt.Errorf("static cycle stage=page version total=%d n=%d: %w", total, len(versions), err)
		}
		documents, total, err := readerPool.ListHistoricalCycleDocument(txCtx, cycle.StaticCycleHistoryQuery{VersionHistoryID: &version.ID, Limit: 1, Offset: 1})
		if err != nil || total != 1 || len(documents) != 0 {
			return fmt.Errorf("static cycle stage=empty document total=%d n=%d: %w", total, len(documents), err)
		}
		if document.ExecutionGuideGeneratedAt == nil || document.CopyGuideGeneratedAt != nil || document.MeasurementGuideGeneratedAt == nil || document.CreatedAt.Nanosecond() != 456789000 {
			return errors.New("static cycle stage=time roundtrip")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("static cycle stage=rollback: %v", err)
	}
	strategyAfter, err := queries.CountHistoricalCycleStrategy(ctx)
	if err != nil || strategyAfter != strategyBefore {
		t.Fatalf("static cycle stage=strategy rollback: %v", err)
	}
	versionAfter, err := queries.CountHistoricalCycleVersion(ctx, pgtype.Int8{})
	if err != nil || versionAfter != versionBefore {
		t.Fatalf("static cycle stage=version rollback: %v", err)
	}
	documentAfter, err := queries.CountHistoricalCycleDocument(ctx, pgtype.Int8{})
	if err != nil || documentAfter != documentBefore {
		t.Fatalf("static cycle stage=document rollback: %v", err)
	}
	for _, id := range []int64{strategyID, versionID, documentID} {
		if id < 1 {
			t.Fatal("static cycle stage=generated id")
		}
	}
	if _, err := queries.GetHistoricalCycleStrategy(ctx, strategyID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("static cycle stage=strategy remains: %v", err)
	}
}

func staticCycleStoreDigest(first byte) []byte {
	value := make([]byte, 32)
	value[0] = first
	return value
}
func staticCycleStoreTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func staticCycleStoreInt64(value int64) *int64 { return &value }
func staticCycleStoreStrategy(seed byte, at time.Time) cycle.HistoricalCycleStrategy {
	var key, payload [32]byte
	copy(key[:], staticCycleStoreDigest(seed))
	copy(payload[:], staticCycleStoreDigest(seed+20))
	return cycle.HistoricalCycleStrategy{SourceID: -1, SourceKeyDigest: key, SourcePayloadDigest: payload, StrategyKey: "", Title: "", Description: " ", Cadence: "", Timezone: "", OriginalStatus: "", CurrentVersion: -2, CreatedAt: at.UTC().Truncate(time.Microsecond), UpdatedAt: at.Add(-time.Second).UTC().Truncate(time.Microsecond)}
}
func staticCycleStoreVersion(seed byte, at time.Time) cycle.HistoricalCycleVersion {
	var key, payload [32]byte
	copy(key[:], staticCycleStoreDigest(seed))
	copy(payload[:], staticCycleStoreDigest(seed+20))
	effective := at.UTC().Truncate(time.Microsecond)
	return cycle.HistoricalCycleVersion{SourceID: -3, SourceKeyDigest: key, SourcePayloadDigest: payload, StrategySourceID: -4, Version: -5, Label: "", Objective: "", VersionHash: "", EffectiveFrom: &effective, OriginalGovernance: "", OperationSkillHash: "", CreatedAt: effective}
}
func staticCycleStoreDocument(seed byte, at time.Time) cycle.HistoricalCycleDocument {
	var key, payload [32]byte
	copy(key[:], staticCycleStoreDigest(seed))
	copy(payload[:], staticCycleStoreDigest(seed+20))
	generated := at.UTC().Truncate(time.Microsecond)
	return cycle.HistoricalCycleDocument{SourceID: -6, SourceKeyDigest: key, SourcePayloadDigest: payload, StrategyVersionSourceID: -7, SchemaVersion: "", ExecutionGuideSHA256: "", ExecutionGuideGeneratedAt: &generated, CopyGuideSHA256: "", MeasurementGuideSHA256: "", MeasurementGuideGeneratedAt: &generated, DocumentPackHash: "", CreatedAt: generated}
}
