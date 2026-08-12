package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var (
	errInvalidCustomerListQuery    = errors.New("invalid customer list query")
	errInvalidCustomerBoundedTotal = errors.New("invalid customer bounded total")
)

// CustomerQueryRepository serves contact-owned customer list reads through the
// transaction-bound unit-of-work context.
type CustomerQueryRepository struct{}

var _ contactapp.CustomerQueryStore = (*CustomerQueryRepository)(nil)

func NewCustomerQueryRepository() *CustomerQueryRepository {
	return &CustomerQueryRepository{}
}

func (*CustomerQueryRepository) ListCustomers(ctx context.Context, query contactapp.CustomerListQuery) (contactapp.CustomerListStoreResult, error) {
	if err := validateCustomerListQuery(query); err != nil {
		return contactapp.CustomerListStoreResult{}, err
	}

	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactapp.CustomerListStoreResult{}, err
	}
	queries := contactdb.New(customerQueryDBTX{Tx: tx})

	boundedTotal, err := queries.CountCustomerIDsBounded(ctx, countCustomerIDsBoundedParams(query))
	if err != nil {
		return contactapp.CustomerListStoreResult{}, err
	}
	if boundedTotal < 0 || boundedTotal > contactapp.CustomerListExactTotalCap+1 {
		return contactapp.CustomerListStoreResult{}, errInvalidCustomerBoundedTotal
	}
	rows, err := queries.ListCustomers(ctx, listCustomersParams(query))
	if err != nil {
		return contactapp.CustomerListStoreResult{}, err
	}

	hasMore := len(rows) > int(query.Limit)
	if hasMore {
		rows = rows[:query.Limit]
	}
	items := make([]contactapp.CustomerRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, customerRecordFromRow(row))
	}
	return contactapp.CustomerListStoreResult{
		Items:        items,
		BoundedTotal: boundedTotal,
		HasMore:      hasMore,
	}, nil
}

// customerQueryDBTX keeps these parameter-sensitive optional-filter queries
// on custom plans. A generic plan cannot prove the active-only predicates that
// make the contact partial indexes usable.
type customerQueryDBTX struct{ pgx.Tx }

func (db customerQueryDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return db.Tx.Query(ctx, sql, append([]any{pgx.QueryExecModeCacheDescribe}, args...)...)
}

func (db customerQueryDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return db.Tx.QueryRow(ctx, sql, append([]any{pgx.QueryExecModeCacheDescribe}, args...)...)
}

func validateCustomerListQuery(query contactapp.CustomerListQuery) error {
	if query.Limit < 1 || query.Limit > contactapp.CustomerListMaximumLimit || query.Watermark.IsZero() {
		return errInvalidCustomerListQuery
	}
	if (query.AfterUpdatedAt == nil) != (query.AfterID == nil) {
		return errInvalidCustomerListQuery
	}
	if query.AfterUpdatedAt != nil && (query.AfterUpdatedAt.IsZero() || query.AfterUpdatedAt.After(query.Watermark) || *query.AfterID <= 0) {
		return errInvalidCustomerListQuery
	}
	for _, value := range []*int64{query.OwnerStaffID, query.StageID, query.ChannelID, query.TagID} {
		if value != nil && *value <= 0 {
			return errInvalidCustomerListQuery
		}
	}
	if invalidTimeRange(query.AddedAfter, query.AddedBefore) || invalidTimeRange(query.LastInteractAfter, query.LastInteractBefore) {
		return errInvalidCustomerListQuery
	}
	return nil
}

func invalidTimeRange(after, before *time.Time) bool {
	return after != nil && before != nil && after.After(*before)
}

func listCustomersParams(query contactapp.CustomerListQuery) contactdb.ListCustomersParams {
	return contactdb.ListCustomersParams{
		Watermark:          nullableTimestamp(&query.Watermark),
		Keyword:            nullableKeyword(query.Keyword),
		OwnerStaffID:       nullableInt64(query.OwnerStaffID),
		StageID:            nullableInt64(query.StageID),
		ChannelID:          nullableInt64(query.ChannelID),
		TagID:              nullableInt64(query.TagID),
		IsDeleted:          query.IsDeleted,
		AddedAfter:         nullableTimestamp(query.AddedAfter),
		AddedBefore:        nullableTimestamp(query.AddedBefore),
		LastInteractAfter:  nullableTimestamp(query.LastInteractAfter),
		LastInteractBefore: nullableTimestamp(query.LastInteractBefore),
		AfterUpdatedAt:     nullableTimestamp(query.AfterUpdatedAt),
		AfterID:            nullableCustomerID(query.AfterID),
		RowLimit:           query.Limit + 1,
	}
}

func countCustomerIDsBoundedParams(query contactapp.CustomerListQuery) contactdb.CountCustomerIDsBoundedParams {
	return contactdb.CountCustomerIDsBoundedParams{
		Watermark:          nullableTimestamp(&query.Watermark),
		Keyword:            nullableKeyword(query.Keyword),
		OwnerStaffID:       nullableInt64(query.OwnerStaffID),
		StageID:            nullableInt64(query.StageID),
		ChannelID:          nullableInt64(query.ChannelID),
		TagID:              nullableInt64(query.TagID),
		IsDeleted:          query.IsDeleted,
		AddedAfter:         nullableTimestamp(query.AddedAfter),
		AddedBefore:        nullableTimestamp(query.AddedBefore),
		LastInteractAfter:  nullableTimestamp(query.LastInteractAfter),
		LastInteractBefore: nullableTimestamp(query.LastInteractBefore),
		TotalLimit:         int32(contactapp.CustomerListExactTotalCap + 1),
	}
}

func nullableKeyword(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func nullableInt64(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func nullableCustomerID(value *contactport.CustomerID) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: int64(*value), Valid: true}
}

func nullableTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func customerRecordFromRow(row contactdb.Customer) contactapp.CustomerRecord {
	return contactapp.CustomerRecord{
		ID:             contactport.CustomerID(row.ID),
		Name:           row.Name,
		AvatarURL:      textPointer(row.AvatarUrl),
		Gender:         int16Pointer(row.Gender),
		StageID:        int64Pointer(row.StageID),
		OwnerStaffID:   int64Pointer(row.OwnerStaffID),
		ChannelID:      int64Pointer(row.ChannelID),
		AddedAt:        timestampPointer(row.AddedAt),
		LastInteractAt: timestampPointer(row.LastInteractAt),
		IsDeleted:      row.IsDeleted,
		Extra:          append(json.RawMessage(nil), row.Extra...),
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func int16Pointer(value pgtype.Int2) *int16 {
	if !value.Valid {
		return nil
	}
	result := value.Int16
	return &result
}

func int64Pointer(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func timestampPointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
