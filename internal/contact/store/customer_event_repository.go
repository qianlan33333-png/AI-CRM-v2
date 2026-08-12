package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var errInvalidCustomerEventRow = errors.New("customer event query returned an invalid row")

// CustomerEventRepository serves contact-owned timeline reads through the
// transaction-bound unit-of-work context.
type CustomerEventRepository struct{}

var _ contactapp.CustomerEventStore = (*CustomerEventRepository)(nil)

func NewCustomerEventRepository() *CustomerEventRepository {
	return &CustomerEventRepository{}
}

func (*CustomerEventRepository) ListCustomerEvents(
	ctx context.Context,
	query contactapp.CustomerEventQuery,
) (contactapp.CustomerEventStoreResult, error) {
	if err := validateCustomerEventQuery(query); err != nil {
		return contactapp.CustomerEventStoreResult{}, err
	}
	queries, err := customerEventQueriesFromContext(ctx)
	if err != nil {
		return contactapp.CustomerEventStoreResult{}, err
	}

	rows, err := queries.ListCustomerEvents(ctx, customerEventListParams(query))
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.CustomerEventStoreResult{}, contactapp.ErrCustomerNotFound
	}
	if err != nil {
		return contactapp.CustomerEventStoreResult{}, err
	}
	if len(rows) == 0 {
		return contactapp.CustomerEventStoreResult{}, contactapp.ErrCustomerNotFound
	}
	if len(rows) == 1 && rows[0].EventID == 0 {
		if rows[0].CustomerID != int64(query.CustomerID) {
			return contactapp.CustomerEventStoreResult{}, errInvalidCustomerEventRow
		}
		return contactapp.CustomerEventStoreResult{Items: make([]contactapp.CustomerEventRecord, 0)}, nil
	}

	hasMore := len(rows) > int(query.Limit)
	if hasMore {
		rows = rows[:query.Limit]
	}
	items := make([]contactapp.CustomerEventRecord, 0, len(rows))
	for _, row := range rows {
		if row.CustomerID <= 0 || row.EventID <= 0 {
			return contactapp.CustomerEventStoreResult{}, errInvalidCustomerEventRow
		}
		items = append(items, contactapp.CustomerEventRecord{
			ID:         row.EventID,
			CustomerID: contactport.CustomerID(row.CustomerID),
			EventType:  row.EventType,
			Payload:    append(json.RawMessage(nil), row.Payload...),
			Actor:      row.Actor,
			OccurredAt: row.OccurredAt.Time.UTC(),
		})
	}
	return contactapp.CustomerEventStoreResult{Items: items, HasMore: hasMore}, nil
}

func validateCustomerEventQuery(query contactapp.CustomerEventQuery) error {
	if query.CustomerID <= 0 || query.Limit < 1 || query.Limit > contactapp.CustomerListMaximumLimit ||
		(query.OwnerStaffID != nil && *query.OwnerStaffID <= 0) ||
		(query.AfterOccurredAt == nil) != (query.AfterID == nil) {
		return contactapp.ErrInvalidCustomerEventQuery
	}
	if query.AfterOccurredAt != nil && (query.AfterOccurredAt.IsZero() || *query.AfterID <= 0) {
		return contactapp.ErrInvalidCustomerEventQuery
	}
	return nil
}

func customerEventQueriesFromContext(ctx context.Context) (*contactdb.Queries, error) {
	if isNilCustomerEventStoreValue(ctx) {
		return nil, platformport.ErrTransactionRequired
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if isNilCustomerEventStoreValue(tx) {
		return nil, platformport.ErrTransactionRequired
	}
	return contactdb.New(tx), nil
}

func isNilCustomerEventStoreValue(value any) bool {
	if value == nil {
		return true
	}
	typed := reflect.ValueOf(value)
	switch typed.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return typed.IsNil()
	default:
		return false
	}
}

func customerEventListParams(query contactapp.CustomerEventQuery) contactdb.ListCustomerEventsParams {
	return contactdb.ListCustomerEventsParams{
		AfterOccurredAt: nullableCustomerEventTimestamp(query.AfterOccurredAt),
		AfterID:         nullableInt64(query.AfterID),
		RowLimit:        query.Limit + 1,
		CustomerID:      int64(query.CustomerID),
		OwnerStaffID:    nullableInt64(query.OwnerStaffID),
	}
}

func nullableCustomerEventTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}
