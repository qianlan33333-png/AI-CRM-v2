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
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

var legacyMarketingHistoryPostgresDSN = flag.String("legacy-marketing-history-postgres-dsn", "", "isolated PostgreSQL DSN for schema 128 rollback verification")

func TestLegacyMarketingHistoryValuesPreserveNullableAndEmptyFacts(t *testing.T) {
	at := legacyMarketingStoreTime()
	state := segmentdb.SegmentV1LegacyMarketingState{
		ID: 1, SourceKeyDigest: legacyMarketingStoreDigest(1), SourcePayloadDigest: legacyMarketingStoreDigest(2), SourceFieldDigest: legacyMarketingStoreDigest(3),
		ExternalUseridDigest: legacyMarketingStoreDigest(4), StatePayloadDigest: legacyMarketingStoreDigest(5), SourceID: -3, ScenarioKey: "", MarketingPhase: "", PhaseLabel: "", PhaseReason: "", LifecycleStatus: "",
		CreatedAt: legacyMarketingStoreTimestamp(at), UpdatedAt: legacyMarketingStoreTimestamp(at), EnteredAt: pgtype.Timestamptz{}, ExitedAt: pgtype.Timestamptz{},
	}
	value, err := legacyMarketingStateValue(state)
	if err != nil || value.SourceID != -3 || value.EnteredAt != nil || value.LastBatchSourceID != nil || value.ScenarioKey != "" {
		t.Fatalf("state mapping changed nullable or empty facts: %#v, %v", value, err)
	}
	batch := int64(0)
	state.LastBatchSourceID = pgtype.Int8{Int64: batch, Valid: true}
	state.EnteredAt = legacyMarketingStoreTimestamp(at)
	value, err = legacyMarketingStateValue(state)
	if err != nil || value.LastBatchSourceID == nil || *value.LastBatchSourceID != batch || value.EnteredAt == nil || *value.EnteredAt != at {
		t.Fatalf("state mapping changed optional facts: %#v, %v", value, err)
	}
	state.CreatedAt.InfinityModifier = pgtype.Infinity
	if _, err = legacyMarketingStateValue(state); !errors.Is(err, segmentport.ErrLegacyMarketingHistoryUnavailable) {
		t.Fatal("infinite state timestamp accepted")
	}
}

func TestLegacyMarketingHistoryStoreRequiresCallerTransactionAndStrictPage(t *testing.T) {
	ctx := context.Background()
	state := legacyMarketingStoreStateFixture(1)
	if _, err := NewLegacyMarketingHistoryStore().CreateHistoricalLegacyMarketingState(ctx, state); !errors.Is(err, segmentport.ErrLegacyMarketingHistoryUnavailable) {
		t.Fatalf("write escaped caller transaction: %v", err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*LegacyMarketingHistoryReader{nil, NewLegacyMarketingHistoryReader(nil), NewLegacyMarketingHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalLegacyMarketingState(ctx, segmentport.LegacyMarketingHistoryQuery{Limit: 20}); !errors.Is(err, segmentport.ErrLegacyMarketingHistoryUnavailable) {
			t.Fatalf("nil reader error = %v", err)
		}
	}
	for _, query := range []segmentport.LegacyMarketingHistoryQuery{{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewLegacyMarketingHistoryReader(nil).ListHistoricalLegacyMarketingValue(ctx, query); !errors.Is(err, segmentport.ErrLegacyMarketingHistoryInvalid) {
			t.Fatalf("invalid page %#v error = %v", query, err)
		}
	}
}

func TestLegacyMarketingHistoryPostgresRoundTripRollback(t *testing.T) {
	if *legacyMarketingHistoryPostgresDSN == "" {
		t.Skip("set -legacy-marketing-history-postgres-dsn for isolated schema 128 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *legacyMarketingHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := segmentdb.New(pool)
	stateBefore, err := queries.CountHistoricalLegacyMarketingState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	valueBefore, err := queries.CountHistoricalLegacyMarketingValue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("legacy marketing history forced rollback")
	var stateIDs, valueIDs []int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return fmt.Errorf("stage=transaction: %w", err)
		}
		store := NewLegacyMarketingHistoryStore()
		poolReader, txReader := NewLegacyMarketingHistoryReader(pool), NewLegacyMarketingHistoryReader(tx)
		for i := byte(1); i <= 2; i++ {
			state := legacyMarketingStoreStateFixture(i)
			if i == 2 {
				state.LastBatchSourceID = nil
				state.EnteredAt = nil
				state.ExitedAt = nil
			}
			created, err := store.CreateHistoricalLegacyMarketingState(txCtx, state)
			if err != nil {
				return fmt.Errorf("stage=state-create: %w", err)
			}
			state.ID = created.ID
			if !reflect.DeepEqual(created, state) {
				return errors.New("stage=state-create-mapping")
			}
			loaded, err := poolReader.GetHistoricalLegacyMarketingState(txCtx, created.ID)
			if err != nil || !reflect.DeepEqual(loaded, state) {
				return fmt.Errorf("stage=state-context-read: %w", err)
			}
			loaded, err = txReader.GetHistoricalLegacyMarketingState(context.Background(), created.ID)
			if err != nil || !reflect.DeepEqual(loaded, state) {
				return fmt.Errorf("stage=state-raw-tx-read: %w", err)
			}
			stateIDs = append(stateIDs, created.ID)
		}
		for i := byte(11); i <= 12; i++ {
			value := legacyMarketingStoreValueFixture(i)
			created, err := store.CreateHistoricalLegacyMarketingValue(txCtx, value)
			if err != nil {
				return fmt.Errorf("stage=value-create: %w", err)
			}
			value.ID = created.ID
			if !reflect.DeepEqual(created, value) {
				return errors.New("stage=value-create-mapping")
			}
			loaded, err := poolReader.GetHistoricalLegacyMarketingValue(txCtx, created.ID)
			if err != nil || !reflect.DeepEqual(loaded, value) {
				return fmt.Errorf("stage=value-context-read: %w", err)
			}
			loaded, err = txReader.GetHistoricalLegacyMarketingValue(context.Background(), created.ID)
			if err != nil || !reflect.DeepEqual(loaded, value) {
				return fmt.Errorf("stage=value-raw-tx-read: %w", err)
			}
			valueIDs = append(valueIDs, created.ID)
		}
		states, total, err := poolReader.ListHistoricalLegacyMarketingState(txCtx, segmentport.LegacyMarketingHistoryQuery{Limit: 1, Offset: int32(stateBefore + 1)})
		if err != nil || total != stateBefore+2 || len(states) != 1 || states[0].ID != stateIDs[1] {
			return fmt.Errorf("stage=state-page total=%d len=%d err=%w", total, len(states), err)
		}
		values, total, err := poolReader.ListHistoricalLegacyMarketingValue(txCtx, segmentport.LegacyMarketingHistoryQuery{Limit: 1, Offset: int32(valueBefore + 1)})
		if err != nil || total != valueBefore+2 || len(values) != 1 || values[0].ID != valueIDs[1] {
			return fmt.Errorf("stage=value-page total=%d len=%d err=%w", total, len(values), err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback transaction: %v", err)
	}
	stateAfter, err := queries.CountHistoricalLegacyMarketingState(ctx)
	if err != nil || stateAfter != stateBefore {
		t.Fatalf("rollback state count = %d, %v", stateAfter, err)
	}
	valueAfter, err := queries.CountHistoricalLegacyMarketingValue(ctx)
	if err != nil || valueAfter != valueBefore {
		t.Fatalf("rollback value count = %d, %v", valueAfter, err)
	}
	for _, id := range append(stateIDs, valueIDs...) {
		if id < 1 {
			t.Fatal("missing generated id")
		}
	}
	for _, id := range stateIDs {
		if _, err := queries.GetHistoricalLegacyMarketingState(ctx, id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("rolled back state %d remained: %v", id, err)
		}
	}
	for _, id := range valueIDs {
		if _, err := queries.GetHistoricalLegacyMarketingValue(ctx, id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("rolled back value %d remained: %v", id, err)
		}
	}
}

func legacyMarketingStoreStateFixture(first byte) segmentport.HistoricalLegacyMarketingState {
	at := legacyMarketingStoreTime()
	batch := int64(-4)
	return segmentport.HistoricalLegacyMarketingState{
		SourceKeyDigest: legacyMarketingStoreArray(first), SourcePayloadDigest: legacyMarketingStoreArray(first + 20), SourceFieldDigest: legacyMarketingStoreArray(first + 40),
		SourceID: -1, ExternalUserIDDigest: legacyMarketingStoreArray(first + 60), ScenarioKey: "", MarketingPhase: "", PhaseLabel: "", PhaseReason: "", LifecycleStatus: "",
		LastBatchSourceID: &batch, LastBatchStatus: "", LastBatchWindowStart: "", LastBatchWindowEnd: "", LastTriggerMessageAt: "", EnteredAt: &at, ExitedAt: &at, ExitReason: "", StatePayloadDigest: legacyMarketingStoreArray(first + 80), CreatedAt: at, UpdatedAt: at,
	}
}
func legacyMarketingStoreValueFixture(first byte) segmentport.HistoricalLegacyMarketingValue {
	at := legacyMarketingStoreTime()
	return segmentport.HistoricalLegacyMarketingValue{SourceKeyDigest: legacyMarketingStoreArray(first), SourcePayloadDigest: legacyMarketingStoreArray(first + 20), SourceFieldDigest: legacyMarketingStoreArray(first + 40), SourceID: 0, ExternalUserIDDigest: legacyMarketingStoreArray(first + 60), ScenarioKey: "", ValueSegment: "", SegmentLabel: "", Score: -7, ScoreBreakdownDigest: legacyMarketingStoreArray(first + 80), StatePayloadDigest: legacyMarketingStoreArray(first + 100), CreatedAt: at, UpdatedAt: at}
}
func legacyMarketingStoreArray(first byte) [32]byte {
	var value [32]byte
	value[0] = first
	return value
}
func legacyMarketingStoreDigest(first byte) []byte {
	value := make([]byte, 32)
	value[0] = first
	return value
}
func legacyMarketingStoreTime() time.Time {
	return time.Date(2026, 8, 28, 12, 13, 14, 123456000, time.UTC)
}
func legacyMarketingStoreTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
