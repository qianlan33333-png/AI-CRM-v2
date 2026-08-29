package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type CustomerTimelineHistoryStore struct{}
type CustomerTimelineHistoryReader struct{ db contactdb.DBTX }

var _ contact.CustomerTimelineHistoryStore = (*CustomerTimelineHistoryStore)(nil)
var _ contact.CustomerTimelineHistoryReader = (*CustomerTimelineHistoryReader)(nil)

func NewCustomerTimelineHistoryStore() *CustomerTimelineHistoryStore {
	return &CustomerTimelineHistoryStore{}
}

func NewCustomerTimelineHistoryReader(db contactdb.DBTX) *CustomerTimelineHistoryReader {
	return &CustomerTimelineHistoryReader{db: db}
}

func (s *CustomerTimelineHistoryStore) queries(ctx context.Context) (*contactdb.Queries, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return nil, contact.ErrCustomerTimelineHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, contact.ErrCustomerTimelineHistoryUnavailable
	}
	return contactdb.New(tx), nil
}

func (r *CustomerTimelineHistoryReader) queries(ctx context.Context) (*contactdb.Queries, error) {
	if r == nil || ctx == nil || ctx.Err() != nil {
		return nil, contact.ErrCustomerTimelineHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return contactdb.New(tx), nil
	}
	if nilCustomerTimelineDB(r.db) {
		return nil, contact.ErrCustomerTimelineHistoryUnavailable
	}
	return contactdb.New(r.db), nil
}

func (s *CustomerTimelineHistoryStore) CreateHistoricalCustomerTimelineEvent(ctx context.Context, value contact.HistoricalCustomerTimelineEvent) (contact.HistoricalCustomerTimelineEvent, error) {
	if value.ID != 0 {
		return contact.HistoricalCustomerTimelineEvent{}, contact.ErrCustomerTimelineHistoryInvalid
	}
	probe := value
	probe.ID = 1
	if _, err := contactapp.HistoricalCustomerTimelineEventDigest(probe); err != nil {
		return contact.HistoricalCustomerTimelineEvent{}, contact.ErrCustomerTimelineHistoryInvalid
	}
	queries, err := s.queries(ctx)
	if err != nil {
		return contact.HistoricalCustomerTimelineEvent{}, err
	}
	row, err := queries.CreateHistoricalCustomerTimelineEvent(ctx, contactdb.CreateHistoricalCustomerTimelineEventParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:],
		SourceID: value.SourceID, EventID: value.EventID, EventType: value.EventType,
		EventTime: timelineTimestamp(value.EventTime), Title: value.Title, Summary: value.Summary,
		SourceTable: value.SourceTable, SourceValue: value.SourceValue, MetadataJson: string(value.MetadataJSON),
		CreatedAt: timelineTimestamp(value.CreatedAt), Unionid: value.UnionID, CustomerID: timelineOptionalID(value.CustomerID),
	})
	if err != nil {
		return contact.HistoricalCustomerTimelineEvent{}, timelineStoreError(err)
	}
	return timelineHistoryValue(row)
}

func (s *CustomerTimelineHistoryStore) GetHistoricalCustomerTimelineEvent(ctx context.Context, id int64) (contact.HistoricalCustomerTimelineEvent, error) {
	if id < 1 {
		return contact.HistoricalCustomerTimelineEvent{}, contact.ErrCustomerTimelineHistoryInvalid
	}
	queries, err := s.queries(ctx)
	if err != nil {
		return contact.HistoricalCustomerTimelineEvent{}, err
	}
	row, err := queries.GetHistoricalCustomerTimelineEvent(ctx, id)
	if err != nil {
		return contact.HistoricalCustomerTimelineEvent{}, timelineStoreError(err)
	}
	return timelineHistoryValue(row)
}

func (r *CustomerTimelineHistoryReader) GetHistoricalCustomerTimelineEvent(ctx context.Context, id int64) (contact.CustomerTimelineHistoryRead, error) {
	if id < 1 {
		return contact.CustomerTimelineHistoryRead{}, contact.ErrCustomerTimelineHistoryInvalid
	}
	queries, err := r.queries(ctx)
	if err != nil {
		return contact.CustomerTimelineHistoryRead{}, err
	}
	row, err := queries.GetHistoricalCustomerTimelineEvent(ctx, id)
	if err != nil {
		return contact.CustomerTimelineHistoryRead{}, timelineStoreError(err)
	}
	value, err := timelineHistoryValue(row)
	if err != nil {
		return contact.CustomerTimelineHistoryRead{}, err
	}
	return timelineSafeRead(value), nil
}

func (r *CustomerTimelineHistoryReader) ListHistoricalCustomerTimelineEvents(ctx context.Context, query contact.CustomerTimelineHistoryQuery) ([]contact.CustomerTimelineHistoryRead, int64, error) {
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 {
		return nil, 0, contact.ErrCustomerTimelineHistoryInvalid
	}
	queries, err := r.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalCustomerTimelineEvents(ctx)
	if err != nil {
		return nil, 0, timelineStoreError(err)
	}
	rows, err := queries.ListHistoricalCustomerTimelineEvents(ctx, contactdb.ListHistoricalCustomerTimelineEventsParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, timelineStoreError(err)
	}
	items := make([]contact.CustomerTimelineHistoryRead, 0, len(rows))
	for _, row := range rows {
		eventTime, eventOK := timelineStoredTime(row.EventTime)
		createdAt, createdOK := timelineStoredTime(row.CreatedAt)
		customerID, customerOK := timelineStoredOptionalID(row.CustomerID)
		if row.ID < 1 || !eventOK || !createdOK || !customerOK {
			return nil, 0, contact.ErrCustomerTimelineHistoryUnavailable
		}
		items = append(items, contact.CustomerTimelineHistoryRead{ID: row.ID, SourceID: row.SourceID, EventID: row.EventID, EventType: row.EventType, EventTime: eventTime, SourceTable: row.SourceTable, SourceValue: row.SourceValue, CreatedAt: createdAt, CustomerID: customerID})
	}
	return items, total, nil
}

func timelineHistoryValue(row contactdb.ContactV1CustomerTimelineHistory) (contact.HistoricalCustomerTimelineEvent, error) {
	if row.ID < 1 || len(row.SourceKeyDigest) != 32 || len(row.SourcePayloadDigest) != 32 || len(row.SourceFieldDigest) != 32 || !json.Valid([]byte(row.MetadataJson)) {
		return contact.HistoricalCustomerTimelineEvent{}, contact.ErrCustomerTimelineHistoryUnavailable
	}
	eventTime, eventOK := timelineStoredTime(row.EventTime)
	createdAt, createdOK := timelineStoredTime(row.CreatedAt)
	customerID, customerOK := timelineStoredOptionalID(row.CustomerID)
	if !eventOK || !createdOK || !customerOK {
		return contact.HistoricalCustomerTimelineEvent{}, contact.ErrCustomerTimelineHistoryUnavailable
	}
	value := contact.HistoricalCustomerTimelineEvent{ID: row.ID, SourceID: row.SourceID, EventID: row.EventID, EventType: row.EventType, EventTime: eventTime, Title: row.Title, Summary: row.Summary, SourceTable: row.SourceTable, SourceValue: row.SourceValue, MetadataJSON: []byte(row.MetadataJson), CreatedAt: createdAt, UnionID: row.Unionid, CustomerID: customerID}
	copy(value.SourceKeyDigest[:], row.SourceKeyDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	copy(value.SourceFieldDigest[:], row.SourceFieldDigest)
	if _, err := contactapp.HistoricalCustomerTimelineEventDigest(value); err != nil {
		return contact.HistoricalCustomerTimelineEvent{}, contact.ErrCustomerTimelineHistoryUnavailable
	}
	return value, nil
}

func timelineSafeRead(value contact.HistoricalCustomerTimelineEvent) contact.CustomerTimelineHistoryRead {
	return contact.CustomerTimelineHistoryRead{ID: value.ID, SourceID: value.SourceID, EventID: value.EventID, EventType: value.EventType, EventTime: value.EventTime, SourceTable: value.SourceTable, SourceValue: value.SourceValue, CreatedAt: value.CreatedAt, CustomerID: value.CustomerID}
}

func timelineTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}

func timelineStoredTime(value pgtype.Timestamptz) (time.Time, bool) {
	if !value.Valid || value.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	return value.Time.UTC().Truncate(time.Microsecond), true
}

func timelineOptionalID(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func timelineStoredOptionalID(value pgtype.Int8) (*int64, bool) {
	if !value.Valid {
		return nil, true
	}
	if value.Int64 < 1 {
		return nil, false
	}
	id := value.Int64
	return &id, true
}

func timelineStoreError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return contact.ErrCustomerTimelineHistoryUnavailable
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return contact.ErrCustomerTimelineHistoryConflict
	}
	return contact.ErrCustomerTimelineHistoryUnavailable
}

func nilCustomerTimelineDB(value contactdb.DBTX) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	return (ref.Kind() == reflect.Pointer || ref.Kind() == reflect.Interface) && ref.IsNil()
}
