package store

import (
	"context"
	"encoding/json"
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

type CycleObservationStore struct{}
type CycleObservationReader struct{ db cycledb.DBTX }

var _ cycle.CycleObservationStore = (*CycleObservationStore)(nil)
var _ cycle.CycleObservationReader = (*CycleObservationReader)(nil)

func NewCycleObservationStore() *CycleObservationStore { return &CycleObservationStore{} }
func NewCycleObservationReader(db cycledb.DBTX) *CycleObservationReader {
	return &CycleObservationReader{db: db}
}

func (store *CycleObservationStore) queries(ctx context.Context) (*cycledb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, cycle.ErrCycleObservationUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil || tx == nil {
		return nil, cycle.ErrCycleObservationUnavailable
	}
	return cycledb.New(tx), nil
}

func (reader *CycleObservationReader) queries(ctx context.Context) (*cycledb.Queries, error) {
	if reader == nil || ctx == nil || ctx.Err() != nil {
		return nil, cycle.ErrCycleObservationUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil && tx != nil {
		return cycledb.New(tx), nil
	}
	if nilCycleObservationDB(reader.db) {
		return nil, cycle.ErrCycleObservationUnavailable
	}
	return cycledb.New(reader.db), nil
}

func nilCycleObservationDB(value cycledb.DBTX) bool {
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

func (store *CycleObservationStore) CreateHistoricalCycleMetric(ctx context.Context, value cycle.HistoricalCycleMetric) (cycle.HistoricalCycleMetric, error) {
	if value.ID != 0 || invalidCycleObservationMetric(value) {
		return cycle.HistoricalCycleMetric{}, cycle.ErrCycleObservationInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return cycle.HistoricalCycleMetric{}, err
	}
	row, err := queries.CreateHistoricalCycleMetric(ctx, cycledb.CreateHistoricalCycleMetricParams{
		SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:],
		RunSourceID: value.RunSourceID, MetricKey: value.MetricKey, Label: value.Label, Numerator: cycleObservationFloat(value.Numerator), Denominator: cycleObservationFloat(value.Denominator), Value: cycleObservationFloat(value.Value),
		Unit: value.Unit, ObservationWindow: value.ObservationWindow, DataSource: value.DataSource, DataQuality: value.DataQuality, LimitationsJson: string(value.LimitationsJSON), IsCausal: value.IsCausal,
		ValueStatus: value.ValueStatus, LastSnapshotSourceID: value.LastSnapshotSourceID, CreatedAt: cycleObservationTimestamp(value.CreatedAt), UpdatedAt: cycleObservationTimestamp(value.UpdatedAt),
	})
	if err != nil {
		return cycle.HistoricalCycleMetric{}, cycleObservationStoreError(err)
	}
	return cycleObservationMetricValue(row)
}

func (store *CycleObservationStore) GetHistoricalCycleMetric(ctx context.Context, id int64) (cycle.HistoricalCycleMetric, error) {
	return cycleObservationMetricGet(ctx, store.queries, id)
}
func (reader *CycleObservationReader) GetHistoricalCycleMetric(ctx context.Context, id int64) (cycle.HistoricalCycleMetric, error) {
	return cycleObservationMetricGet(ctx, reader.queries, id)
}
func cycleObservationMetricGet(ctx context.Context, queries func(context.Context) (*cycledb.Queries, error), id int64) (cycle.HistoricalCycleMetric, error) {
	if id < 1 {
		return cycle.HistoricalCycleMetric{}, cycle.ErrCycleObservationInvalid
	}
	q, err := queries(ctx)
	if err != nil {
		return cycle.HistoricalCycleMetric{}, err
	}
	row, err := q.GetHistoricalCycleMetric(ctx, id)
	if err != nil {
		return cycle.HistoricalCycleMetric{}, cycleObservationStoreError(err)
	}
	return cycleObservationMetricValue(row)
}

func (reader *CycleObservationReader) ListHistoricalCycleMetric(ctx context.Context, page cycle.CycleObservationQuery) ([]cycle.HistoricalCycleMetric, int64, error) {
	if invalidCycleObservationPage(page) {
		return nil, 0, cycle.ErrCycleObservationInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalCycleMetric(ctx)
	if err != nil {
		return nil, 0, cycleObservationStoreError(err)
	}
	rows, err := q.ListHistoricalCycleMetric(ctx, cycledb.ListHistoricalCycleMetricParams{Limit: page.Limit, Offset: page.Offset})
	if err != nil {
		return nil, 0, cycleObservationStoreError(err)
	}
	values := make([]cycle.HistoricalCycleMetric, 0, len(rows))
	for _, row := range rows {
		value, err := cycleObservationMetricValue(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func (store *CycleObservationStore) CreateHistoricalCycleReference(ctx context.Context, value cycle.HistoricalCycleReference) (cycle.HistoricalCycleReference, error) {
	if value.ID != 0 || invalidCycleObservationReference(value) {
		return cycle.HistoricalCycleReference{}, cycle.ErrCycleObservationInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return cycle.HistoricalCycleReference{}, err
	}
	row, err := queries.CreateHistoricalCycleReference(ctx, cycledb.CreateHistoricalCycleReferenceParams{
		SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:],
		RunSourceID: value.RunSourceID, ReferenceKey: value.ReferenceKey, ReferenceType: value.ReferenceType, Label: value.Label, SourceSystem: value.SourceSystem, ReferenceSourceID: value.ReferenceSourceID,
		Href: value.Href, EvidenceHash: value.EvidenceHash, DataStatus: value.DataStatus, LastSnapshotSourceID: value.LastSnapshotSourceID,
		CreatedAt: cycleObservationTimestamp(value.CreatedAt), UpdatedAt: cycleObservationTimestamp(value.UpdatedAt),
	})
	if err != nil {
		return cycle.HistoricalCycleReference{}, cycleObservationStoreError(err)
	}
	return cycleObservationReferenceValue(row)
}

func (store *CycleObservationStore) GetHistoricalCycleReference(ctx context.Context, id int64) (cycle.HistoricalCycleReference, error) {
	return cycleObservationReferenceGet(ctx, store.queries, id)
}
func (reader *CycleObservationReader) GetHistoricalCycleReference(ctx context.Context, id int64) (cycle.HistoricalCycleReference, error) {
	return cycleObservationReferenceGet(ctx, reader.queries, id)
}
func cycleObservationReferenceGet(ctx context.Context, queries func(context.Context) (*cycledb.Queries, error), id int64) (cycle.HistoricalCycleReference, error) {
	if id < 1 {
		return cycle.HistoricalCycleReference{}, cycle.ErrCycleObservationInvalid
	}
	q, err := queries(ctx)
	if err != nil {
		return cycle.HistoricalCycleReference{}, err
	}
	row, err := q.GetHistoricalCycleReference(ctx, id)
	if err != nil {
		return cycle.HistoricalCycleReference{}, cycleObservationStoreError(err)
	}
	return cycleObservationReferenceValue(row)
}

func (reader *CycleObservationReader) ListHistoricalCycleReference(ctx context.Context, page cycle.CycleObservationQuery) ([]cycle.HistoricalCycleReference, int64, error) {
	if invalidCycleObservationPage(page) {
		return nil, 0, cycle.ErrCycleObservationInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalCycleReference(ctx)
	if err != nil {
		return nil, 0, cycleObservationStoreError(err)
	}
	rows, err := q.ListHistoricalCycleReference(ctx, cycledb.ListHistoricalCycleReferenceParams{Limit: page.Limit, Offset: page.Offset})
	if err != nil {
		return nil, 0, cycleObservationStoreError(err)
	}
	values := make([]cycle.HistoricalCycleReference, 0, len(rows))
	for _, row := range rows {
		value, err := cycleObservationReferenceValue(row)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}

func invalidCycleObservationMetric(value cycle.HistoricalCycleMetric) bool {
	if value.ID == 0 {
		value.ID = 1
	}
	_, err := cycleapp.HistoricalCycleMetricDigest(value)
	return err != nil
}
func invalidCycleObservationReference(value cycle.HistoricalCycleReference) bool {
	if value.ID == 0 {
		value.ID = 1
	}
	_, err := cycleapp.HistoricalCycleReferenceDigest(value)
	return err != nil
}
func invalidCycleObservationPage(value cycle.CycleObservationQuery) bool {
	return value.Limit < 1 || value.Limit > 100 || value.Offset < 0
}

func cycleObservationMetricValue(row cycledb.OperationCycleV1MetricHistory) (cycle.HistoricalCycleMetric, error) {
	key, keyOK := cycleObservationDigest(row.SourceKeyDigest)
	payload, payloadOK := cycleObservationDigest(row.SourcePayloadDigest)
	field, fieldOK := cycleObservationDigest(row.SourceFieldDigest)
	numerator, numeratorOK := cycleObservationFloatValue(row.Numerator)
	denominator, denominatorOK := cycleObservationFloatValue(row.Denominator)
	value, valueOK := cycleObservationFloatValue(row.Value)
	created, createdOK := cycleObservationTimestampValue(row.CreatedAt)
	updated, updatedOK := cycleObservationTimestampValue(row.UpdatedAt)
	limitations := json.RawMessage([]byte(row.LimitationsJson))
	fact := cycle.HistoricalCycleMetric{ID: row.ID, SourceID: row.SourceID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, RunSourceID: row.RunSourceID, MetricKey: row.MetricKey, Label: row.Label, Numerator: numerator, Denominator: denominator, Value: value, Unit: row.Unit, ObservationWindow: row.ObservationWindow, DataSource: row.DataSource, DataQuality: row.DataQuality, LimitationsJSON: limitations, IsCausal: row.IsCausal, ValueStatus: row.ValueStatus, LastSnapshotSourceID: row.LastSnapshotSourceID, CreatedAt: created, UpdatedAt: updated}
	if !keyOK || !payloadOK || !fieldOK || !numeratorOK || !denominatorOK || !valueOK || !createdOK || !updatedOK || invalidCycleObservationMetric(fact) {
		return cycle.HistoricalCycleMetric{}, cycle.ErrCycleObservationUnavailable
	}
	return fact, nil
}

func cycleObservationReferenceValue(row cycledb.OperationCycleV1ReferenceHistory) (cycle.HistoricalCycleReference, error) {
	key, keyOK := cycleObservationDigest(row.SourceKeyDigest)
	payload, payloadOK := cycleObservationDigest(row.SourcePayloadDigest)
	field, fieldOK := cycleObservationDigest(row.SourceFieldDigest)
	created, createdOK := cycleObservationTimestampValue(row.CreatedAt)
	updated, updatedOK := cycleObservationTimestampValue(row.UpdatedAt)
	fact := cycle.HistoricalCycleReference{ID: row.ID, SourceID: row.SourceID, SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, RunSourceID: row.RunSourceID, ReferenceKey: row.ReferenceKey, ReferenceType: row.ReferenceType, Label: row.Label, SourceSystem: row.SourceSystem, ReferenceSourceID: row.ReferenceSourceID, Href: row.Href, EvidenceHash: row.EvidenceHash, DataStatus: row.DataStatus, LastSnapshotSourceID: row.LastSnapshotSourceID, CreatedAt: created, UpdatedAt: updated}
	if !keyOK || !payloadOK || !fieldOK || !createdOK || !updatedOK || invalidCycleObservationReference(fact) {
		return cycle.HistoricalCycleReference{}, cycle.ErrCycleObservationUnavailable
	}
	return fact, nil
}

func cycleObservationDigest(value []byte) ([32]byte, bool) {
	var digest [32]byte
	if len(value) != len(digest) {
		return digest, false
	}
	copy(digest[:], value)
	return digest, digest != ([32]byte{})
}
func cycleObservationFloat(value *float64) pgtype.Float8 {
	if value == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *value, Valid: true}
}
func cycleObservationFloatValue(value pgtype.Float8) (*float64, bool) {
	if !value.Valid {
		return nil, true
	}
	result := value.Float64
	return &result, true
}
func cycleObservationTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}
func cycleObservationTimestampValue(value pgtype.Timestamptz) (time.Time, bool) {
	if !value.Valid || value.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	return value.Time.UTC().Truncate(time.Microsecond), true
}
func cycleObservationStoreError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return cycle.ErrCycleObservationConflict
	}
	return cycle.ErrCycleObservationUnavailable
}
