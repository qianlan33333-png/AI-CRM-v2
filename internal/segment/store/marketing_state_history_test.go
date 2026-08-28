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
	segment "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

var marketingStateHistoryPostgresDSN = flag.String("marketing-state-history-postgres-dsn", "", "isolated PostgreSQL DSN for schema 124 rollback verification")

func TestMarketingStateHistoryReaderFailsClosedAndMapsPrivateFacts(t *testing.T) {
	ctx := context.Background()
	if _, err := NewMarketingStateHistoryStore().GetHistoricalMarketingStateSnapshot(ctx, 1); !errors.Is(err, segment.ErrMarketingStateHistoryUnavailable) {
		t.Fatal(err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*MarketingStateHistoryReader{nil, NewMarketingStateHistoryReader(nil), NewMarketingStateHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalMarketingStateSnapshot(ctx, segment.MarketingStateHistoryQuery{Limit: 1}); !errors.Is(err, segment.ErrMarketingStateHistoryUnavailable) {
			t.Fatal(err)
		}
	}
	for _, query := range []segment.MarketingStateHistoryQuery{{}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewMarketingStateHistoryReader(nil).ListHistoricalValueSegmentChange(ctx, query); !errors.Is(err, segment.ErrMarketingStateHistoryInvalid) {
			t.Fatal(err)
		}
	}
	if _, err := NewMarketingStateHistoryReader(nil).GetHistoricalValueSegmentChange(ctx, 0); !errors.Is(err, segment.ErrMarketingStateHistoryInvalid) {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	person, batch := int64(-2), int64(-3)
	row := segmentdb.SegmentV1MarketingStateChange{ID: 1, SourceKeyDigest: testBytes(1), SourcePayloadDigest: testBytes(2), SourceFieldDigest: testBytes(3), SourceID: -4, PersonSourceID: pgtype.Int8{Int64: person, Valid: true}, BatchSourceID: pgtype.Int8{Int64: batch, Valid: true}, ExternalUseridDigest: testBytes(4), StatePayloadDigest: testBytes(5), RecordedAt: testTime(at), CreatedAt: testTime(at.Add(-time.Second))}
	v, err := marketingStateChangeValue(row)
	if err != nil || v.SourceID != -4 || v.PersonSourceID == nil || *v.PersonSourceID != person || v.BatchSourceID == nil || *v.BatchSourceID != batch {
		t.Fatalf("%+v %v", v, err)
	}
	value := segmentdb.SegmentV1ValueSegmentSnapshot{ID: 2, SourceKeyDigest: testBytes(6), SourcePayloadDigest: testBytes(7), SourceFieldDigest: testBytes(8), SourceID: -9, ExternalUseridDigest: testBytes(9), MatchedQuestionIdsDigest: testBytes(10), StatePayloadDigest: testBytes(11), SubmissionSourceID: pgtype.Int8{Int64: -12, Valid: true}, EvaluatedAt: testTime(at), ComputedAt: testTime(at), CreatedAt: testTime(at), UpdatedAt: testTime(at)}
	got, err := valueSegmentSnapshotValue(value)
	if err != nil || got.SourceID != -9 || got.SubmissionSourceID == nil || *got.SubmissionSourceID != -12 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestMarketingStateHistoryPostgresRoundTripRollback(t *testing.T) {
	if *marketingStateHistoryPostgresDSN == "" {
		t.Skip("set -marketing-state-history-postgres-dsn for isolated schema 124 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *marketingStateHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := segmentdb.New(pool)
	before, err := q.CountHistoricalMarketingStateSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("marketing-state rollback")
	var snapshotID int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store := NewMarketingStateHistoryStore()
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return fmt.Errorf("marketing-state stage=tx: %w", err)
		}
		reader := NewMarketingStateHistoryReader(tx)
		at := time.Date(2026, 8, 28, 1, 2, 3, 123456789, time.FixedZone("+8", 8*3600))
		snapshot, err := store.CreateHistoricalMarketingStateSnapshot(txCtx, storeMarketingSnapshot(at))
		if err != nil {
			return fmt.Errorf("marketing-state stage=snapshot: %w", err)
		}
		snapshotID = snapshot.ID
		if loaded, err := reader.GetHistoricalMarketingStateSnapshot(context.Background(), snapshot.ID); err != nil || !reflect.DeepEqual(loaded, snapshot) {
			return fmt.Errorf("marketing-state stage=bare-tx-snapshot: %w", err)
		}
		change, err := store.CreateHistoricalMarketingStateChange(txCtx, storeMarketingChange(at))
		if err != nil {
			return fmt.Errorf("marketing-state stage=change: %w", err)
		}
		if loaded, err := reader.GetHistoricalMarketingStateChange(txCtx, change.ID); err != nil || !reflect.DeepEqual(loaded, change) {
			return fmt.Errorf("marketing-state stage=change-get: %w", err)
		}
		valueSnapshot, err := store.CreateHistoricalValueSegmentSnapshot(txCtx, storeValueSnapshot(at))
		if err != nil {
			return fmt.Errorf("marketing-state stage=value-snapshot: %w", err)
		}
		if loaded, err := reader.GetHistoricalValueSegmentSnapshot(txCtx, valueSnapshot.ID); err != nil || !reflect.DeepEqual(loaded, valueSnapshot) {
			return fmt.Errorf("marketing-state stage=value-snapshot-get: %w", err)
		}
		valueChange, err := store.CreateHistoricalValueSegmentChange(txCtx, storeValueChange(at))
		if err != nil {
			return fmt.Errorf("marketing-state stage=value-change: %w", err)
		}
		if loaded, err := reader.GetHistoricalValueSegmentChange(txCtx, valueChange.ID); err != nil || !reflect.DeepEqual(loaded, valueChange) {
			return fmt.Errorf("marketing-state stage=value-change-get: %w", err)
		}
		items, total, err := reader.ListHistoricalMarketingStateSnapshot(txCtx, segment.MarketingStateHistoryQuery{Limit: 1, Offset: int32(before + 1)})
		if err != nil || total != before+1 || len(items) != 0 {
			return fmt.Errorf("marketing-state stage=page: %w", err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	after, err := q.CountHistoricalMarketingStateSnapshot(ctx)
	if err != nil || after != before {
		t.Fatal("rollback retained snapshot")
	}
	if _, err := q.GetHistoricalMarketingStateSnapshot(ctx, snapshotID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("rolled row remains")
	}
}

func testBytes(v byte) []byte                 { x := make([]byte, 32); x[0] = v; return x }
func testDigest(v byte) [32]byte              { var x [32]byte; x[0] = v; return x }
func testTime(v time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: v, Valid: true} }
func storeMarketingSnapshot(at time.Time) segment.HistoricalMarketingStateSnapshot {
	person, batch := int64(-2), int64(-3)
	entered := at.UTC().Truncate(time.Microsecond)
	return segment.HistoricalMarketingStateSnapshot{SourceKeyDigest: testDigest(1), SourcePayloadDigest: testDigest(2), SourceFieldDigest: testDigest(3), SourceID: -1, PersonSourceID: &person, ExternalUserIDDigest: testDigest(4), LastBatchSourceID: &batch, StatePayloadDigest: testDigest(5), EnteredAt: &entered, CreatedAt: at.UTC().Truncate(time.Microsecond), UpdatedAt: at.Add(-time.Second).UTC().Truncate(time.Microsecond)}
}
func storeMarketingChange(at time.Time) segment.HistoricalMarketingStateChange {
	person, batch := int64(-4), int64(-5)
	return segment.HistoricalMarketingStateChange{SourceKeyDigest: testDigest(6), SourcePayloadDigest: testDigest(7), SourceFieldDigest: testDigest(8), SourceID: -2, PersonSourceID: &person, BatchSourceID: &batch, ExternalUserIDDigest: testDigest(9), StatePayloadDigest: testDigest(10), RecordedAt: at.UTC().Truncate(time.Microsecond), CreatedAt: at.Add(-time.Second).UTC().Truncate(time.Microsecond)}
}
func storeValueSnapshot(at time.Time) segment.HistoricalValueSegmentSnapshot {
	submission := int64(-6)
	return segment.HistoricalValueSegmentSnapshot{SourceKeyDigest: testDigest(11), SourcePayloadDigest: testDigest(12), SourceFieldDigest: testDigest(13), SourceID: -3, ExternalUserIDDigest: testDigest(14), SubmissionSourceID: &submission, MatchedQuestionIDsDigest: testDigest(15), StatePayloadDigest: testDigest(16), EvaluatedAt: at.UTC().Truncate(time.Microsecond), ComputedAt: at.Add(-time.Second).UTC().Truncate(time.Microsecond), CreatedAt: at.Add(-2 * time.Second).UTC().Truncate(time.Microsecond), UpdatedAt: at.Add(-3 * time.Second).UTC().Truncate(time.Microsecond)}
}
func storeValueChange(at time.Time) segment.HistoricalValueSegmentChange {
	submission := int64(-7)
	return segment.HistoricalValueSegmentChange{SourceKeyDigest: testDigest(17), SourcePayloadDigest: testDigest(18), SourceFieldDigest: testDigest(19), SourceID: -4, ExternalUserIDDigest: testDigest(20), SubmissionSourceID: &submission, MatchedQuestionIDsDigest: testDigest(21), StatePayloadDigest: testDigest(22), EvaluatedAt: at.UTC().Truncate(time.Microsecond), RecordedAt: at.Add(-time.Second).UTC().Truncate(time.Microsecond), CreatedAt: at.Add(-2 * time.Second).UTC().Truncate(time.Microsecond)}
}
