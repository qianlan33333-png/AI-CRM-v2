package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	cycleapp "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/app"
	cycle "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/port"
	cycledb "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type StaticCycleHistoryStore struct{}
type StaticCycleHistoryReader struct{ db cycledb.DBTX }

var _ cycle.StaticCycleHistoryStore = (*StaticCycleHistoryStore)(nil)
var _ cycle.StaticCycleHistoryReader = (*StaticCycleHistoryReader)(nil)

func NewStaticCycleHistoryStore() *StaticCycleHistoryStore { return &StaticCycleHistoryStore{} }
func NewStaticCycleHistoryReader(db cycledb.DBTX) *StaticCycleHistoryReader {
	return &StaticCycleHistoryReader{db: db}
}

func (store *StaticCycleHistoryStore) queries(ctx context.Context) (*cycledb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, cycle.ErrStaticCycleHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, cycle.ErrStaticCycleHistoryUnavailable
	}
	return cycledb.New(tx), nil
}
func (reader *StaticCycleHistoryReader) queries(ctx context.Context) (*cycledb.Queries, error) {
	if reader == nil || ctx == nil || ctx.Err() != nil {
		return nil, cycle.ErrStaticCycleHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return cycledb.New(tx), nil
	}
	if nilStaticCycleDB(reader.db) {
		return nil, cycle.ErrStaticCycleHistoryUnavailable
	}
	return cycledb.New(reader.db), nil
}
func nilStaticCycleDB(value cycledb.DBTX) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func (store *StaticCycleHistoryStore) CreateHistoricalCycleStrategy(ctx context.Context, value cycle.HistoricalCycleStrategy) (cycle.HistoricalCycleStrategy, error) {
	if value.ID != 0 || invalidCycleStrategy(value) {
		return cycle.HistoricalCycleStrategy{}, cycle.ErrStaticCycleHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return cycle.HistoricalCycleStrategy{}, err
	}
	row, err := queries.CreateHistoricalCycleStrategy(ctx, cycledb.CreateHistoricalCycleStrategyParams{SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], StrategyKey: value.StrategyKey, Title: value.Title, Description: value.Description, Cadence: value.Cadence, Timezone: value.Timezone, OriginalStatus: value.OriginalStatus, CurrentVersion: value.CurrentVersion, CreatedAt: cycleTimestamp(value.CreatedAt), UpdatedAt: cycleTimestamp(value.UpdatedAt)})
	if err != nil {
		return cycle.HistoricalCycleStrategy{}, staticCycleStoreError(err)
	}
	return cycleStrategyValue(row)
}
func (store *StaticCycleHistoryStore) GetHistoricalCycleStrategy(ctx context.Context, id int64) (cycle.HistoricalCycleStrategy, error) {
	if id < 1 {
		return cycle.HistoricalCycleStrategy{}, cycle.ErrStaticCycleHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return cycle.HistoricalCycleStrategy{}, err
	}
	row, err := queries.GetHistoricalCycleStrategy(ctx, id)
	if err != nil {
		return cycle.HistoricalCycleStrategy{}, staticCycleStoreError(err)
	}
	return cycleStrategyValue(row)
}
func (reader *StaticCycleHistoryReader) GetHistoricalCycleStrategy(ctx context.Context, id int64) (cycle.HistoricalCycleStrategy, error) {
	if id < 1 {
		return cycle.HistoricalCycleStrategy{}, cycle.ErrStaticCycleHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return cycle.HistoricalCycleStrategy{}, err
	}
	row, err := queries.GetHistoricalCycleStrategy(ctx, id)
	if err != nil {
		return cycle.HistoricalCycleStrategy{}, staticCycleStoreError(err)
	}
	return cycleStrategyValue(row)
}
func (reader *StaticCycleHistoryReader) ListHistoricalCycleStrategy(ctx context.Context, query cycle.StaticCycleHistoryQuery) ([]cycle.HistoricalCycleStrategy, int64, error) {
	if invalidCycleQuery(query, false, false) {
		return nil, 0, cycle.ErrStaticCycleHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalCycleStrategy(ctx)
	if err != nil {
		return nil, 0, staticCycleStoreError(err)
	}
	rows, err := queries.ListHistoricalCycleStrategy(ctx, cycledb.ListHistoricalCycleStrategyParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, staticCycleStoreError(err)
	}
	result := make([]cycle.HistoricalCycleStrategy, 0, len(rows))
	for _, row := range rows {
		value, err := cycleStrategyValue(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, total, nil
}

func (store *StaticCycleHistoryStore) CreateHistoricalCycleVersion(ctx context.Context, value cycle.HistoricalCycleVersion) (cycle.HistoricalCycleVersion, error) {
	if value.ID != 0 || invalidCycleVersion(value) {
		return cycle.HistoricalCycleVersion{}, cycle.ErrStaticCycleHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return cycle.HistoricalCycleVersion{}, err
	}
	row, err := queries.CreateHistoricalCycleVersion(ctx, cycledb.CreateHistoricalCycleVersionParams{SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], StrategySourceID: value.StrategySourceID, StrategyHistoryID: value.StrategyHistoryID, Version: value.Version, Label: value.Label, Objective: value.Objective, VersionHash: value.VersionHash, EffectiveFrom: cycleOptionalTimestamp(value.EffectiveFrom), OriginalGovernance: value.OriginalGovernance, ConfirmedAt: cycleOptionalTimestamp(value.ConfirmedAt), OperationSkillHash: value.OperationSkillHash, CreatedAt: cycleTimestamp(value.CreatedAt)})
	if err != nil {
		return cycle.HistoricalCycleVersion{}, staticCycleStoreError(err)
	}
	return cycleVersionValue(row)
}
func (store *StaticCycleHistoryStore) GetHistoricalCycleVersion(ctx context.Context, id int64) (cycle.HistoricalCycleVersion, error) {
	if id < 1 {
		return cycle.HistoricalCycleVersion{}, cycle.ErrStaticCycleHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return cycle.HistoricalCycleVersion{}, err
	}
	row, err := queries.GetHistoricalCycleVersion(ctx, id)
	if err != nil {
		return cycle.HistoricalCycleVersion{}, staticCycleStoreError(err)
	}
	return cycleVersionValue(row)
}
func (reader *StaticCycleHistoryReader) GetHistoricalCycleVersion(ctx context.Context, id int64) (cycle.HistoricalCycleVersion, error) {
	if id < 1 {
		return cycle.HistoricalCycleVersion{}, cycle.ErrStaticCycleHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return cycle.HistoricalCycleVersion{}, err
	}
	row, err := queries.GetHistoricalCycleVersion(ctx, id)
	if err != nil {
		return cycle.HistoricalCycleVersion{}, staticCycleStoreError(err)
	}
	return cycleVersionValue(row)
}
func (reader *StaticCycleHistoryReader) ListHistoricalCycleVersion(ctx context.Context, query cycle.StaticCycleHistoryQuery) ([]cycle.HistoricalCycleVersion, int64, error) {
	if invalidCycleQuery(query, true, false) {
		return nil, 0, cycle.ErrStaticCycleHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	parent := cycleOptionalInt64(query.StrategyHistoryID)
	total, err := queries.CountHistoricalCycleVersion(ctx, parent)
	if err != nil {
		return nil, 0, staticCycleStoreError(err)
	}
	rows, err := queries.ListHistoricalCycleVersion(ctx, cycledb.ListHistoricalCycleVersionParams{StrategyHistoryID: parent, RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, staticCycleStoreError(err)
	}
	result := make([]cycle.HistoricalCycleVersion, 0, len(rows))
	for _, row := range rows {
		value, err := cycleVersionValue(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, total, nil
}

func (store *StaticCycleHistoryStore) CreateHistoricalCycleDocument(ctx context.Context, value cycle.HistoricalCycleDocument) (cycle.HistoricalCycleDocument, error) {
	if value.ID != 0 || invalidCycleDocument(value) {
		return cycle.HistoricalCycleDocument{}, cycle.ErrStaticCycleHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return cycle.HistoricalCycleDocument{}, err
	}
	row, err := queries.CreateHistoricalCycleDocument(ctx, cycledb.CreateHistoricalCycleDocumentParams{SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], StrategyVersionSourceID: value.StrategyVersionSourceID, VersionHistoryID: value.VersionHistoryID, SchemaVersion: value.SchemaVersion, ExecutionGuideSha256: value.ExecutionGuideSHA256, ExecutionGuideGeneratedAt: cycleOptionalTimestamp(value.ExecutionGuideGeneratedAt), CopyGuideSha256: value.CopyGuideSHA256, CopyGuideGeneratedAt: cycleOptionalTimestamp(value.CopyGuideGeneratedAt), MeasurementGuideSha256: value.MeasurementGuideSHA256, MeasurementGuideGeneratedAt: cycleOptionalTimestamp(value.MeasurementGuideGeneratedAt), DocumentPackHash: value.DocumentPackHash, CreatedAt: cycleTimestamp(value.CreatedAt)})
	if err != nil {
		return cycle.HistoricalCycleDocument{}, staticCycleStoreError(err)
	}
	return cycleDocumentValue(row)
}
func (store *StaticCycleHistoryStore) GetHistoricalCycleDocument(ctx context.Context, id int64) (cycle.HistoricalCycleDocument, error) {
	if id < 1 {
		return cycle.HistoricalCycleDocument{}, cycle.ErrStaticCycleHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return cycle.HistoricalCycleDocument{}, err
	}
	row, err := queries.GetHistoricalCycleDocument(ctx, id)
	if err != nil {
		return cycle.HistoricalCycleDocument{}, staticCycleStoreError(err)
	}
	return cycleDocumentValue(row)
}
func (reader *StaticCycleHistoryReader) GetHistoricalCycleDocument(ctx context.Context, id int64) (cycle.HistoricalCycleDocument, error) {
	if id < 1 {
		return cycle.HistoricalCycleDocument{}, cycle.ErrStaticCycleHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return cycle.HistoricalCycleDocument{}, err
	}
	row, err := queries.GetHistoricalCycleDocument(ctx, id)
	if err != nil {
		return cycle.HistoricalCycleDocument{}, staticCycleStoreError(err)
	}
	return cycleDocumentValue(row)
}
func (reader *StaticCycleHistoryReader) ListHistoricalCycleDocument(ctx context.Context, query cycle.StaticCycleHistoryQuery) ([]cycle.HistoricalCycleDocument, int64, error) {
	if invalidCycleQuery(query, false, true) {
		return nil, 0, cycle.ErrStaticCycleHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	parent := cycleOptionalInt64(query.VersionHistoryID)
	total, err := queries.CountHistoricalCycleDocument(ctx, parent)
	if err != nil {
		return nil, 0, staticCycleStoreError(err)
	}
	rows, err := queries.ListHistoricalCycleDocument(ctx, cycledb.ListHistoricalCycleDocumentParams{VersionHistoryID: parent, RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, staticCycleStoreError(err)
	}
	result := make([]cycle.HistoricalCycleDocument, 0, len(rows))
	for _, row := range rows {
		value, err := cycleDocumentValue(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, total, nil
}

func invalidCycleStrategy(value cycle.HistoricalCycleStrategy) bool {
	if value.ID == 0 {
		value.ID = 1
	}
	_, err := cycleapp.HistoricalCycleStrategyDigest(value)
	return err != nil
}
func invalidCycleVersion(value cycle.HistoricalCycleVersion) bool {
	if value.ID == 0 {
		value.ID = 1
	}
	_, err := cycleapp.HistoricalCycleVersionDigest(value)
	return err != nil
}
func invalidCycleDocument(value cycle.HistoricalCycleDocument) bool {
	if value.ID == 0 {
		value.ID = 1
	}
	_, err := cycleapp.HistoricalCycleDocumentDigest(value)
	return err != nil
}
func invalidCycleQuery(query cycle.StaticCycleHistoryQuery, strategy, version bool) bool {
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 {
		return true
	}
	if (!strategy && query.StrategyHistoryID != nil) || (!version && query.VersionHistoryID != nil) {
		return true
	}
	if query.StrategyHistoryID != nil && *query.StrategyHistoryID < 1 {
		return true
	}
	return query.VersionHistoryID != nil && *query.VersionHistoryID < 1
}
func cycleTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func cycleOptionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return cycleTimestamp(*value)
}
func cycleOptionalInt64(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}
func cycleTimestampValue(value pgtype.Timestamptz) (time.Time, bool) {
	if !value.Valid || value.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	return value.Time.UTC().Truncate(time.Microsecond), true
}
func cycleOptionalTimestampValue(value pgtype.Timestamptz) (*time.Time, bool) {
	if !value.Valid {
		return nil, true
	}
	actual, ok := cycleTimestampValue(value)
	return &actual, ok
}
func cycleDigests(key, payload []byte) ([32]byte, [32]byte, bool) {
	var a, b [32]byte
	if len(key) != 32 || len(payload) != 32 {
		return a, b, false
	}
	copy(a[:], key)
	copy(b[:], payload)
	return a, b, a != ([32]byte{}) && b != ([32]byte{})
}
func cycleStrategyValue(row cycledb.OperationCycleV1StrategyHistory) (cycle.HistoricalCycleStrategy, error) {
	key, payload, ok := cycleDigests(row.SourceKeyDigest, row.SourcePayloadDigest)
	created, createdOK := cycleTimestampValue(row.CreatedAt)
	updated, updatedOK := cycleTimestampValue(row.UpdatedAt)
	value := cycle.HistoricalCycleStrategy{ID: row.ID, SourceID: row.SourceID, SourceKeyDigest: key, SourcePayloadDigest: payload, StrategyKey: row.StrategyKey, Title: row.Title, Description: row.Description, Cadence: row.Cadence, Timezone: row.Timezone, OriginalStatus: row.OriginalStatus, CurrentVersion: row.CurrentVersion, CreatedAt: created, UpdatedAt: updated}
	if !ok || !createdOK || !updatedOK || invalidCycleStrategy(value) {
		return cycle.HistoricalCycleStrategy{}, cycle.ErrStaticCycleHistoryUnavailable
	}
	return value, nil
}
func cycleVersionValue(row cycledb.OperationCycleV1VersionHistory) (cycle.HistoricalCycleVersion, error) {
	key, payload, ok := cycleDigests(row.SourceKeyDigest, row.SourcePayloadDigest)
	effective, effectiveOK := cycleOptionalTimestampValue(row.EffectiveFrom)
	confirmed, confirmedOK := cycleOptionalTimestampValue(row.ConfirmedAt)
	created, createdOK := cycleTimestampValue(row.CreatedAt)
	value := cycle.HistoricalCycleVersion{ID: row.ID, SourceID: row.SourceID, SourceKeyDigest: key, SourcePayloadDigest: payload, StrategySourceID: row.StrategySourceID, StrategyHistoryID: row.StrategyHistoryID, Version: row.Version, Label: row.Label, Objective: row.Objective, VersionHash: row.VersionHash, EffectiveFrom: effective, OriginalGovernance: row.OriginalGovernance, ConfirmedAt: confirmed, OperationSkillHash: row.OperationSkillHash, CreatedAt: created}
	if !ok || !effectiveOK || !confirmedOK || !createdOK || invalidCycleVersion(value) {
		return cycle.HistoricalCycleVersion{}, cycle.ErrStaticCycleHistoryUnavailable
	}
	return value, nil
}
func cycleDocumentValue(row cycledb.OperationCycleV1DocumentHistory) (cycle.HistoricalCycleDocument, error) {
	key, payload, ok := cycleDigests(row.SourceKeyDigest, row.SourcePayloadDigest)
	execution, executionOK := cycleOptionalTimestampValue(row.ExecutionGuideGeneratedAt)
	copyAt, copyOK := cycleOptionalTimestampValue(row.CopyGuideGeneratedAt)
	measurement, measurementOK := cycleOptionalTimestampValue(row.MeasurementGuideGeneratedAt)
	created, createdOK := cycleTimestampValue(row.CreatedAt)
	value := cycle.HistoricalCycleDocument{ID: row.ID, SourceID: row.SourceID, SourceKeyDigest: key, SourcePayloadDigest: payload, StrategyVersionSourceID: row.StrategyVersionSourceID, VersionHistoryID: row.VersionHistoryID, SchemaVersion: row.SchemaVersion, ExecutionGuideSHA256: row.ExecutionGuideSha256, ExecutionGuideGeneratedAt: execution, CopyGuideSHA256: row.CopyGuideSha256, CopyGuideGeneratedAt: copyAt, MeasurementGuideSHA256: row.MeasurementGuideSha256, MeasurementGuideGeneratedAt: measurement, DocumentPackHash: row.DocumentPackHash, CreatedAt: created}
	if !ok || !executionOK || !copyOK || !measurementOK || !createdOK || invalidCycleDocument(value) {
		return cycle.HistoricalCycleDocument{}, cycle.ErrStaticCycleHistoryUnavailable
	}
	return value, nil
}
func staticCycleStoreError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return cycle.ErrStaticCycleHistoryConflict
	}
	return cycle.ErrStaticCycleHistoryUnavailable
}
