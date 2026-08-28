package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

type LegacyMarketingHistoryStore struct{}

type LegacyMarketingHistoryReader struct{ db segmentdb.DBTX }

var _ segmentport.LegacyMarketingHistoryStore = (*LegacyMarketingHistoryStore)(nil)
var _ segmentport.LegacyMarketingHistoryReader = (*LegacyMarketingHistoryReader)(nil)

func NewLegacyMarketingHistoryStore() *LegacyMarketingHistoryStore {
	return &LegacyMarketingHistoryStore{}
}

func NewLegacyMarketingHistoryReader(db segmentdb.DBTX) *LegacyMarketingHistoryReader {
	return &LegacyMarketingHistoryReader{db: db}
}

func (store *LegacyMarketingHistoryStore) queries(ctx context.Context) (*segmentdb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, segmentport.ErrLegacyMarketingHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, segmentport.ErrLegacyMarketingHistoryUnavailable
	}
	return segmentdb.New(tx), nil
}

func (reader *LegacyMarketingHistoryReader) queries(ctx context.Context) (*segmentdb.Queries, error) {
	if reader == nil || ctx == nil || ctx.Err() != nil {
		return nil, segmentport.ErrLegacyMarketingHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil && !nilLegacyMarketingDB(tx) {
		return segmentdb.New(tx), nil
	}
	if nilLegacyMarketingDB(reader.db) {
		return nil, segmentport.ErrLegacyMarketingHistoryUnavailable
	}
	return segmentdb.New(reader.db), nil
}

func (store *LegacyMarketingHistoryStore) CreateHistoricalLegacyMarketingState(ctx context.Context, value segmentport.HistoricalLegacyMarketingState) (segmentport.HistoricalLegacyMarketingState, error) {
	if value.ID != 0 {
		return segmentport.HistoricalLegacyMarketingState{}, segmentport.ErrLegacyMarketingHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := segmentapp.HistoricalLegacyMarketingStateDigest(check); err != nil {
		return segmentport.HistoricalLegacyMarketingState{}, segmentport.ErrLegacyMarketingHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalLegacyMarketingState{}, err
	}
	row, err := q.CreateHistoricalLegacyMarketingState(ctx, legacyMarketingStateParams(value))
	if err != nil {
		return segmentport.HistoricalLegacyMarketingState{}, legacyMarketingStoreError(err)
	}
	return legacyMarketingStateValue(row)
}

func (store *LegacyMarketingHistoryStore) GetHistoricalLegacyMarketingState(ctx context.Context, id int64) (segmentport.HistoricalLegacyMarketingState, error) {
	if id < 1 {
		return segmentport.HistoricalLegacyMarketingState{}, segmentport.ErrLegacyMarketingHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalLegacyMarketingState{}, err
	}
	row, err := q.GetHistoricalLegacyMarketingState(ctx, id)
	if err != nil {
		return segmentport.HistoricalLegacyMarketingState{}, legacyMarketingStoreError(err)
	}
	return legacyMarketingStateValue(row)
}

func (reader *LegacyMarketingHistoryReader) GetHistoricalLegacyMarketingState(ctx context.Context, id int64) (segmentport.HistoricalLegacyMarketingState, error) {
	if id < 1 {
		return segmentport.HistoricalLegacyMarketingState{}, segmentport.ErrLegacyMarketingHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return segmentport.HistoricalLegacyMarketingState{}, err
	}
	row, err := q.GetHistoricalLegacyMarketingState(ctx, id)
	if err != nil {
		return segmentport.HistoricalLegacyMarketingState{}, legacyMarketingStoreError(err)
	}
	return legacyMarketingStateValue(row)
}

func (reader *LegacyMarketingHistoryReader) ListHistoricalLegacyMarketingState(ctx context.Context, query segmentport.LegacyMarketingHistoryQuery) ([]segmentport.HistoricalLegacyMarketingState, int64, error) {
	if !validLegacyMarketingPage(query) {
		return nil, 0, segmentport.ErrLegacyMarketingHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalLegacyMarketingState(ctx)
	if err != nil {
		return nil, 0, legacyMarketingStoreError(err)
	}
	rows, err := q.ListHistoricalLegacyMarketingState(ctx, segmentdb.ListHistoricalLegacyMarketingStateParams{Limit: query.Limit, Offset: query.Offset})
	if err != nil {
		return nil, 0, legacyMarketingStoreError(err)
	}
	items := make([]segmentport.HistoricalLegacyMarketingState, 0, len(rows))
	for _, row := range rows {
		value, err := legacyMarketingStateValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func (store *LegacyMarketingHistoryStore) CreateHistoricalLegacyMarketingValue(ctx context.Context, value segmentport.HistoricalLegacyMarketingValue) (segmentport.HistoricalLegacyMarketingValue, error) {
	if value.ID != 0 {
		return segmentport.HistoricalLegacyMarketingValue{}, segmentport.ErrLegacyMarketingHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := segmentapp.HistoricalLegacyMarketingValueDigest(check); err != nil {
		return segmentport.HistoricalLegacyMarketingValue{}, segmentport.ErrLegacyMarketingHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalLegacyMarketingValue{}, err
	}
	row, err := q.CreateHistoricalLegacyMarketingValue(ctx, legacyMarketingValueParams(value))
	if err != nil {
		return segmentport.HistoricalLegacyMarketingValue{}, legacyMarketingStoreError(err)
	}
	return legacyMarketingValueValue(row)
}

func (store *LegacyMarketingHistoryStore) GetHistoricalLegacyMarketingValue(ctx context.Context, id int64) (segmentport.HistoricalLegacyMarketingValue, error) {
	if id < 1 {
		return segmentport.HistoricalLegacyMarketingValue{}, segmentport.ErrLegacyMarketingHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return segmentport.HistoricalLegacyMarketingValue{}, err
	}
	row, err := q.GetHistoricalLegacyMarketingValue(ctx, id)
	if err != nil {
		return segmentport.HistoricalLegacyMarketingValue{}, legacyMarketingStoreError(err)
	}
	return legacyMarketingValueValue(row)
}

func (reader *LegacyMarketingHistoryReader) GetHistoricalLegacyMarketingValue(ctx context.Context, id int64) (segmentport.HistoricalLegacyMarketingValue, error) {
	if id < 1 {
		return segmentport.HistoricalLegacyMarketingValue{}, segmentport.ErrLegacyMarketingHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return segmentport.HistoricalLegacyMarketingValue{}, err
	}
	row, err := q.GetHistoricalLegacyMarketingValue(ctx, id)
	if err != nil {
		return segmentport.HistoricalLegacyMarketingValue{}, legacyMarketingStoreError(err)
	}
	return legacyMarketingValueValue(row)
}

func (reader *LegacyMarketingHistoryReader) ListHistoricalLegacyMarketingValue(ctx context.Context, query segmentport.LegacyMarketingHistoryQuery) ([]segmentport.HistoricalLegacyMarketingValue, int64, error) {
	if !validLegacyMarketingPage(query) {
		return nil, 0, segmentport.ErrLegacyMarketingHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalLegacyMarketingValue(ctx)
	if err != nil {
		return nil, 0, legacyMarketingStoreError(err)
	}
	rows, err := q.ListHistoricalLegacyMarketingValue(ctx, segmentdb.ListHistoricalLegacyMarketingValueParams{Limit: query.Limit, Offset: query.Offset})
	if err != nil {
		return nil, 0, legacyMarketingStoreError(err)
	}
	items := make([]segmentport.HistoricalLegacyMarketingValue, 0, len(rows))
	for _, row := range rows {
		value, err := legacyMarketingValueValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func legacyMarketingStateParams(value segmentport.HistoricalLegacyMarketingState) segmentdb.CreateHistoricalLegacyMarketingStateParams {
	return segmentdb.CreateHistoricalLegacyMarketingStateParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], SourceID: value.SourceID,
		ExternalUseridDigest: value.ExternalUserIDDigest[:], ScenarioKey: value.ScenarioKey, MarketingPhase: value.MarketingPhase, PhaseLabel: value.PhaseLabel,
		PhaseReason: value.PhaseReason, LifecycleStatus: value.LifecycleStatus, LastBatchSourceID: legacyMarketingInt(value.LastBatchSourceID), LastBatchStatus: value.LastBatchStatus,
		LastBatchWindowStart: value.LastBatchWindowStart, LastBatchWindowEnd: value.LastBatchWindowEnd, LastTriggerMessageAt: value.LastTriggerMessageAt,
		EnteredAt: legacyMarketingTime(value.EnteredAt), ExitedAt: legacyMarketingTime(value.ExitedAt), ExitReason: value.ExitReason,
		StatePayloadDigest: value.StatePayloadDigest[:], CreatedAt: legacyMarketingTime(&value.CreatedAt), UpdatedAt: legacyMarketingTime(&value.UpdatedAt),
	}
}

func legacyMarketingValueParams(value segmentport.HistoricalLegacyMarketingValue) segmentdb.CreateHistoricalLegacyMarketingValueParams {
	return segmentdb.CreateHistoricalLegacyMarketingValueParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], SourceID: value.SourceID,
		ExternalUseridDigest: value.ExternalUserIDDigest[:], ScenarioKey: value.ScenarioKey, ValueSegment: value.ValueSegment, SegmentLabel: value.SegmentLabel,
		Score: value.Score, ScoreBreakdownDigest: value.ScoreBreakdownDigest[:], StatePayloadDigest: value.StatePayloadDigest[:],
		CreatedAt: legacyMarketingTime(&value.CreatedAt), UpdatedAt: legacyMarketingTime(&value.UpdatedAt),
	}
}

func legacyMarketingStateValue(row segmentdb.SegmentV1LegacyMarketingState) (segmentport.HistoricalLegacyMarketingState, error) {
	if row.ID < 1 || !legacyMarketingDigest32(row.SourceKeyDigest) || !legacyMarketingDigest32(row.SourcePayloadDigest) || !legacyMarketingDigest32(row.SourceFieldDigest) || !legacyMarketingDigest32(row.ExternalUseridDigest) || !legacyMarketingDigest32(row.StatePayloadDigest) || !legacyMarketingFinite(row.CreatedAt) || !legacyMarketingFinite(row.UpdatedAt) || !legacyMarketingOptionalFinite(row.EnteredAt) || !legacyMarketingOptionalFinite(row.ExitedAt) {
		return segmentport.HistoricalLegacyMarketingState{}, segmentport.ErrLegacyMarketingHistoryUnavailable
	}
	value := segmentport.HistoricalLegacyMarketingState{
		ID: row.ID, SourceID: row.SourceID, ScenarioKey: row.ScenarioKey, MarketingPhase: row.MarketingPhase, PhaseLabel: row.PhaseLabel,
		PhaseReason: row.PhaseReason, LifecycleStatus: row.LifecycleStatus, LastBatchStatus: row.LastBatchStatus, LastBatchWindowStart: row.LastBatchWindowStart,
		LastBatchWindowEnd: row.LastBatchWindowEnd, LastTriggerMessageAt: row.LastTriggerMessageAt, ExitReason: row.ExitReason,
		CreatedAt: row.CreatedAt.Time.UTC().Truncate(time.Microsecond), UpdatedAt: row.UpdatedAt.Time.UTC().Truncate(time.Microsecond),
	}
	copy(value.SourceKeyDigest[:], row.SourceKeyDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	copy(value.SourceFieldDigest[:], row.SourceFieldDigest)
	copy(value.ExternalUserIDDigest[:], row.ExternalUseridDigest)
	copy(value.StatePayloadDigest[:], row.StatePayloadDigest)
	if row.LastBatchSourceID.Valid {
		id := row.LastBatchSourceID.Int64
		value.LastBatchSourceID = &id
	}
	if row.EnteredAt.Valid {
		entered := row.EnteredAt.Time.UTC().Truncate(time.Microsecond)
		value.EnteredAt = &entered
	}
	if row.ExitedAt.Valid {
		exited := row.ExitedAt.Time.UTC().Truncate(time.Microsecond)
		value.ExitedAt = &exited
	}
	if _, err := segmentapp.HistoricalLegacyMarketingStateDigest(value); err != nil {
		return segmentport.HistoricalLegacyMarketingState{}, segmentport.ErrLegacyMarketingHistoryUnavailable
	}
	return value, nil
}

func legacyMarketingValueValue(row segmentdb.SegmentV1LegacyMarketingValue) (segmentport.HistoricalLegacyMarketingValue, error) {
	if row.ID < 1 || !legacyMarketingDigest32(row.SourceKeyDigest) || !legacyMarketingDigest32(row.SourcePayloadDigest) || !legacyMarketingDigest32(row.SourceFieldDigest) || !legacyMarketingDigest32(row.ExternalUseridDigest) || !legacyMarketingDigest32(row.ScoreBreakdownDigest) || !legacyMarketingDigest32(row.StatePayloadDigest) || !legacyMarketingFinite(row.CreatedAt) || !legacyMarketingFinite(row.UpdatedAt) {
		return segmentport.HistoricalLegacyMarketingValue{}, segmentport.ErrLegacyMarketingHistoryUnavailable
	}
	value := segmentport.HistoricalLegacyMarketingValue{ID: row.ID, SourceID: row.SourceID, ScenarioKey: row.ScenarioKey, ValueSegment: row.ValueSegment, SegmentLabel: row.SegmentLabel, Score: row.Score, CreatedAt: row.CreatedAt.Time.UTC().Truncate(time.Microsecond), UpdatedAt: row.UpdatedAt.Time.UTC().Truncate(time.Microsecond)}
	copy(value.SourceKeyDigest[:], row.SourceKeyDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	copy(value.SourceFieldDigest[:], row.SourceFieldDigest)
	copy(value.ExternalUserIDDigest[:], row.ExternalUseridDigest)
	copy(value.ScoreBreakdownDigest[:], row.ScoreBreakdownDigest)
	copy(value.StatePayloadDigest[:], row.StatePayloadDigest)
	if _, err := segmentapp.HistoricalLegacyMarketingValueDigest(value); err != nil {
		return segmentport.HistoricalLegacyMarketingValue{}, segmentport.ErrLegacyMarketingHistoryUnavailable
	}
	return value, nil
}

func validLegacyMarketingPage(query segmentport.LegacyMarketingHistoryQuery) bool {
	return query.Limit >= 1 && query.Limit <= 100 && query.Offset >= 0
}

func legacyMarketingInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func legacyMarketingTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func legacyMarketingDigest32(value []byte) bool { return len(value) == 32 }
func legacyMarketingFinite(value pgtype.Timestamptz) bool {
	return value.Valid && value.InfinityModifier == pgtype.Finite
}
func legacyMarketingOptionalFinite(value pgtype.Timestamptz) bool {
	return !value.Valid || value.InfinityModifier == pgtype.Finite
}

func legacyMarketingStoreError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return segmentport.ErrLegacyMarketingHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return segmentport.ErrLegacyMarketingHistoryUnavailable
	}
	return segmentport.ErrLegacyMarketingHistoryUnavailable
}

func nilLegacyMarketingDB(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}
