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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	cycle "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
	cycledb "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var cycleObservationHistoryPostgresDSN = flag.String("cycle-observation-history-postgres-dsn", "", "isolated PostgreSQL DSN for schema 136 rollback verification")

func TestCycleObservationHistoryRowValuesPreserveNullableJSONAndSignedFacts(t *testing.T) {
	at := time.Date(2026, 8, 29, 1, 2, 3, 456789000, time.FixedZone("source", 8*3600))
	metric, err := cycleObservationMetricValue(cycledb.OperationCycleV1MetricHistory{
		ID: 1, SourceID: -2, SourceKeyDigest: cycleObservationTestDigest(1), SourcePayloadDigest: cycleObservationTestDigest(2), SourceFieldDigest: cycleObservationTestDigest(3),
		RunSourceID: 0, MetricKey: "", Label: " ", Numerator: pgtype.Float8{}, Denominator: pgtype.Float8{Float64: -2.5, Valid: true}, Value: pgtype.Float8{},
		Unit: "", ObservationWindow: "", DataSource: "", DataQuality: "", LimitationsJson: "null", IsCausal: false, ValueStatus: "", LastSnapshotSourceID: -4,
		CreatedAt: cycleObservationTestTime(at), UpdatedAt: cycleObservationTestTime(at.Add(-time.Second)),
	})
	if err != nil || metric.SourceID != -2 || metric.RunSourceID != 0 || metric.Numerator != nil || metric.Value != nil || metric.Denominator == nil || *metric.Denominator != -2.5 || string(metric.LimitationsJSON) != "null" || metric.LastSnapshotSourceID != -4 || metric.CreatedAt.Location() != time.UTC || metric.CreatedAt.Nanosecond() != 456789000 {
		t.Fatalf("metric fidelity = %#v, %v", metric, err)
	}
	reference, err := cycleObservationReferenceValue(cycledb.OperationCycleV1ReferenceHistory{
		ID: 2, SourceID: 0, SourceKeyDigest: cycleObservationTestDigest(4), SourcePayloadDigest: cycleObservationTestDigest(5), SourceFieldDigest: cycleObservationTestDigest(6),
		RunSourceID: -7, ReferenceKey: "", ReferenceType: "", Label: "", SourceSystem: "", ReferenceSourceID: "", Href: "private://legacy", EvidenceHash: "", DataStatus: "", LastSnapshotSourceID: -8,
		CreatedAt: cycleObservationTestTime(at), UpdatedAt: cycleObservationTestTime(at.Add(-time.Second)),
	})
	if err != nil || reference.SourceID != 0 || reference.RunSourceID != -7 || reference.LastSnapshotSourceID != -8 || reference.Href != "private://legacy" || reference.CreatedAt.Location() != time.UTC {
		t.Fatalf("reference fidelity = %#v, %v", reference, err)
	}
	bad := cycledb.OperationCycleV1MetricHistory{ID: 1, SourceKeyDigest: cycleObservationTestDigest(1), SourcePayloadDigest: cycleObservationTestDigest(2), SourceFieldDigest: cycleObservationTestDigest(3), LimitationsJson: "null", CreatedAt: cycleObservationTestTime(at), UpdatedAt: cycleObservationTestTime(at)}
	bad.CreatedAt.InfinityModifier = pgtype.Infinity
	if _, err := cycleObservationMetricValue(bad); !errors.Is(err, cycle.ErrCycleObservationUnavailable) {
		t.Fatalf("infinite timestamp = %v", err)
	}
}

func TestCycleObservationHistoryRequiresTransactionAndValidPage(t *testing.T) {
	ctx := context.Background()
	if _, err := NewCycleObservationStore().GetHistoricalCycleMetric(ctx, 1); !errors.Is(err, cycle.ErrCycleObservationUnavailable) {
		t.Fatalf("store escaped transaction = %v", err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*CycleObservationReader{nil, NewCycleObservationReader(nil), NewCycleObservationReader(pool)} {
		if _, _, err := reader.ListHistoricalCycleMetric(ctx, cycle.CycleObservationQuery{Limit: 1}); !errors.Is(err, cycle.ErrCycleObservationUnavailable) {
			t.Fatalf("typed nil reader = %v", err)
		}
	}
	for _, page := range []cycle.CycleObservationQuery{{}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewCycleObservationReader(nil).ListHistoricalCycleReference(ctx, page); !errors.Is(err, cycle.ErrCycleObservationInvalid) {
			t.Fatalf("invalid page %#v = %v", page, err)
		}
	}
}

func TestCycleObservationHistoryPostgresRoundTripRollback(t *testing.T) {
	if *cycleObservationHistoryPostgresDSN == "" {
		t.Skip("set -cycle-observation-history-postgres-dsn for isolated schema 136 rollback verification")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *cycleObservationHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := cycledb.New(pool)
	metricsBefore, err := queries.CountHistoricalCycleMetric(ctx)
	if err != nil {
		t.Fatal(err)
	}
	referencesBefore, err := queries.CountHistoricalCycleReference(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("cycle observation forced rollback")
	var metricID, referenceID int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store := NewCycleObservationStore()
		poolReader := NewCycleObservationReader(pool)
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return fmt.Errorf("stage=tx: %w", err)
		}
		txReader := NewCycleObservationReader(tx)
		at := time.Date(2026, 8, 29, 1, 2, 3, 456789123, time.FixedZone("source", 8*3600))
		metric, err := store.CreateHistoricalCycleMetric(txCtx, cycleObservationMetricFixture(1, at))
		if err != nil {
			return fmt.Errorf("stage=create metric: %w", err)
		}
		metricID = metric.ID
		if loaded, err := poolReader.GetHistoricalCycleMetric(txCtx, metric.ID); err != nil || !reflect.DeepEqual(loaded, metric) {
			return fmt.Errorf("stage=caller-tx metric loaded=%#v err=%w", loaded, err)
		}
		if loaded, err := txReader.GetHistoricalCycleMetric(context.Background(), metric.ID); err != nil || !reflect.DeepEqual(loaded, metric) {
			return fmt.Errorf("stage=bare-tx metric loaded=%#v err=%w", loaded, err)
		}
		reference, err := store.CreateHistoricalCycleReference(txCtx, cycleObservationReferenceFixture(2, at))
		if err != nil {
			return fmt.Errorf("stage=create reference: %w", err)
		}
		referenceID = reference.ID
		metrics, total, err := poolReader.ListHistoricalCycleMetric(txCtx, cycle.CycleObservationQuery{Limit: 1})
		if err != nil || total != metricsBefore+1 || len(metrics) == 0 || metrics[0].ID != metric.ID {
			return fmt.Errorf("stage=metric page total=%d count=%d: %w", total, len(metrics), err)
		}
		if referencesBefore > int64(^uint32(0)>>1) {
			return errors.New("stage=reference offset overflows int32")
		}
		references, total, err := poolReader.ListHistoricalCycleReference(txCtx, cycle.CycleObservationQuery{Limit: 1, Offset: int32(referencesBefore)})
		if err != nil || total != referencesBefore+1 || len(references) != 1 || references[0].ID != reference.ID {
			return fmt.Errorf("stage=reference page total=%d count=%d: %w", total, len(references), err)
		}
		if metric.Numerator != nil || metric.Value == nil || string(metric.LimitationsJSON) != "null" || metric.CreatedAt.Nanosecond() != 456789000 || reference.Href != "private://source" {
			return errors.New("stage=roundtrip fidelity")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("stage=rollback: %v", err)
	}
	metricsAfter, err := queries.CountHistoricalCycleMetric(ctx)
	if err != nil || metricsAfter != metricsBefore {
		t.Fatalf("stage=metric rollback before=%d after=%d err=%v", metricsBefore, metricsAfter, err)
	}
	referencesAfter, err := queries.CountHistoricalCycleReference(ctx)
	if err != nil || referencesAfter != referencesBefore {
		t.Fatalf("stage=reference rollback before=%d after=%d err=%v", referencesBefore, referencesAfter, err)
	}
	for _, id := range []int64{metricID, referenceID} {
		if id < 1 {
			t.Fatal("generated history ID missing")
		}
	}
	if _, err := queries.GetHistoricalCycleMetric(ctx, metricID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("metric remained after rollback: %v", err)
	}
}

func cycleObservationTestDigest(first byte) []byte {
	value := make([]byte, 32)
	value[0] = first
	return value
}
func cycleObservationTestTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func cycleObservationDigestFixture(first byte) [32]byte {
	var result [32]byte
	result[0] = first
	return result
}
func cycleObservationMetricFixture(first byte, at time.Time) cycle.HistoricalCycleMetric {
	value := 0.0
	return cycle.HistoricalCycleMetric{SourceID: int64(first) - 3, SourceKeyDigest: cycleObservationDigestFixture(first), SourcePayloadDigest: cycleObservationDigestFixture(first + 30), SourceFieldDigest: cycleObservationDigestFixture(first + 60), RunSourceID: -2, MetricKey: "", Label: "", Numerator: nil, Denominator: nil, Value: &value, Unit: "", ObservationWindow: "", DataSource: "", DataQuality: "", LimitationsJSON: json.RawMessage("null"), IsCausal: false, ValueStatus: "", LastSnapshotSourceID: -4, CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}
func cycleObservationReferenceFixture(first byte, at time.Time) cycle.HistoricalCycleReference {
	return cycle.HistoricalCycleReference{SourceID: int64(first) - 3, SourceKeyDigest: cycleObservationDigestFixture(first), SourcePayloadDigest: cycleObservationDigestFixture(first + 30), SourceFieldDigest: cycleObservationDigestFixture(first + 60), RunSourceID: -2, ReferenceKey: "", ReferenceType: "", Label: "", SourceSystem: "", ReferenceSourceID: "", Href: "private://source", EvidenceHash: "", DataStatus: "", LastSnapshotSourceID: -4, CreatedAt: at, UpdatedAt: at.Add(-time.Second)}
}
