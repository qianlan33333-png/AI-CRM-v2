package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const historicalChannelCivilLayout = "2006-01-02T15:04:05.000000"

type HistoricalChannelRelationsStore struct {
	tx func(context.Context) (pgx.Tx, error)
}

type HistoricalChannelHistoryReader struct {
	db contactdb.DBTX
}

var _ contactport.HistoricalChannelRelationsStore = (*HistoricalChannelRelationsStore)(nil)
var _ contactport.HistoricalChannelHistoryReader = (*HistoricalChannelHistoryReader)(nil)

func NewHistoricalChannelRelationsStore() *HistoricalChannelRelationsStore {
	return &HistoricalChannelRelationsStore{tx: platformstore.TxFromContext}
}

func NewHistoricalChannelHistoryReader(pool *pgxpool.Pool) *HistoricalChannelHistoryReader {
	reader := &HistoricalChannelHistoryReader{}
	if pool != nil {
		reader.db = pool
	}
	return reader
}

func (store *HistoricalChannelRelationsStore) queries(ctx context.Context) (*contactdb.Queries, error) {
	if store == nil {
		return nil, contactport.ErrHistoricalChannelUnavailable
	}
	return (&HistoricalChannelStore{tx: store.tx}).queries(ctx)
}

func (store *HistoricalChannelRelationsStore) CreateHistoricalChannelContact(ctx context.Context, value contactport.HistoricalChannelContact) (contactport.HistoricalChannelContact, error) {
	if value.ID != 0 || value.ChannelID < 1 || value.SourceContactID < 1 || value.EnterCount < 1 ||
		(value.CustomerID != nil && *value.CustomerID < 1) || value.FirstEnteredAt.IsZero() || value.LastEnteredAt.IsZero() ||
		value.CreatedAt.IsZero() || value.UpdatedAt.IsZero() || value.LastEnteredAt.Before(value.FirstEnteredAt) || value.UpdatedAt.Before(value.CreatedAt) {
		return contactport.HistoricalChannelContact{}, contactport.ErrHistoricalChannelInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return contactport.HistoricalChannelContact{}, err
	}
	row, err := queries.CreateHistoricalChannelContact(ctx, contactdb.CreateHistoricalChannelContactParams{
		ChannelID: value.ChannelID, SourceContactID: value.SourceContactID, CustomerID: nullableInt64(value.CustomerID),
		OwnerReference: value.OwnerReference, EnterCount: value.EnterCount,
		FirstEnteredAt: historicalChannelTimestamp(value.FirstEnteredAt), LastEnteredAt: historicalChannelTimestamp(value.LastEnteredAt),
		CreatedAt: historicalChannelTimestamp(value.CreatedAt), UpdatedAt: historicalChannelTimestamp(value.UpdatedAt),
	})
	if err != nil {
		return contactport.HistoricalChannelContact{}, historicalChannelRelationError(err)
	}
	return historicalChannelContactRecord(row)
}

func (store *HistoricalChannelRelationsStore) GetHistoricalChannelContact(ctx context.Context, id int64) (contactport.HistoricalChannelContact, error) {
	if id < 1 {
		return contactport.HistoricalChannelContact{}, contactport.ErrHistoricalChannelInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return contactport.HistoricalChannelContact{}, err
	}
	row, err := queries.GetHistoricalChannelContact(ctx, id)
	if err != nil {
		return contactport.HistoricalChannelContact{}, historicalChannelRelationError(err)
	}
	return historicalChannelContactRecord(row)
}

func (store *HistoricalChannelRelationsStore) CreateHistoricalChannelAssignee(ctx context.Context, value contactport.HistoricalChannelAssignee) (contactport.HistoricalChannelAssignee, error) {
	created, createdErr := time.Parse(historicalChannelCivilLayout, value.SourceCreatedAt)
	updated, updatedErr := time.Parse(historicalChannelCivilLayout, value.SourceUpdatedAt)
	if value.ID != 0 || value.ChannelID < 1 || value.SourceAssigneeID < 1 || value.Priority < 0 || value.Status == "" ||
		createdErr != nil || updatedErr != nil || created.IsZero() || updated.IsZero() || updated.Before(created) ||
		created.Format(historicalChannelCivilLayout) != value.SourceCreatedAt || updated.Format(historicalChannelCivilLayout) != value.SourceUpdatedAt {
		return contactport.HistoricalChannelAssignee{}, contactport.ErrHistoricalChannelInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return contactport.HistoricalChannelAssignee{}, err
	}
	row, err := queries.CreateHistoricalChannelAssignee(ctx, contactdb.CreateHistoricalChannelAssigneeParams{
		ChannelID: value.ChannelID, SourceAssigneeID: value.SourceAssigneeID, StaffReference: value.StaffReference,
		DisplayNameSnapshot: value.DisplayNameSnapshot, Priority: value.Priority, Status: value.Status,
		RatioPercent: historicalChannelNullableInt32(value.RatioPercent), MaxScans24h: historicalChannelNullableInt32(value.MaxScans24h),
		SourceCreatedAt: pgtype.Timestamp{Time: created, Valid: true}, SourceUpdatedAt: pgtype.Timestamp{Time: updated, Valid: true},
	})
	if err != nil {
		return contactport.HistoricalChannelAssignee{}, historicalChannelRelationError(err)
	}
	return historicalChannelAssigneeRecord(row)
}

func (store *HistoricalChannelRelationsStore) GetHistoricalChannelAssignee(ctx context.Context, id int64) (contactport.HistoricalChannelAssignee, error) {
	if id < 1 {
		return contactport.HistoricalChannelAssignee{}, contactport.ErrHistoricalChannelInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return contactport.HistoricalChannelAssignee{}, err
	}
	row, err := queries.GetHistoricalChannelAssignee(ctx, id)
	if err != nil {
		return contactport.HistoricalChannelAssignee{}, historicalChannelRelationError(err)
	}
	return historicalChannelAssigneeRecord(row)
}

func (reader *HistoricalChannelHistoryReader) ListHistoricalChannelContacts(ctx context.Context, channelID int64, limit, offset int32) ([]contactport.HistoricalChannelContact, int64, error) {
	if channelID < 1 || limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, contactport.ErrHistoricalChannelInvalid
	}
	if reader == nil || reader.db == nil || ctx == nil || ctx.Err() != nil {
		return nil, 0, contactport.ErrHistoricalChannelUnavailable
	}
	queries := contactdb.New(reader.db)
	total, err := queries.CountHistoricalChannelContacts(ctx, channelID)
	if err != nil {
		return nil, 0, historicalChannelRelationError(err)
	}
	rows, err := queries.ListHistoricalChannelContacts(ctx, contactdb.ListHistoricalChannelContactsParams{ChannelID: channelID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, historicalChannelRelationError(err)
	}
	result := make([]contactport.HistoricalChannelContact, 0, len(rows))
	for _, row := range rows {
		value, err := historicalChannelContactRecord(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, total, nil
}

func (reader *HistoricalChannelHistoryReader) ListHistoricalChannelAssignees(ctx context.Context, channelID int64) ([]contactport.HistoricalChannelAssignee, error) {
	if channelID < 1 {
		return nil, contactport.ErrHistoricalChannelInvalid
	}
	if reader == nil || reader.db == nil || ctx == nil || ctx.Err() != nil {
		return nil, contactport.ErrHistoricalChannelUnavailable
	}
	rows, err := contactdb.New(reader.db).ListHistoricalChannelAssignees(ctx, channelID)
	if err != nil {
		return nil, historicalChannelRelationError(err)
	}
	if len(rows) > 200 {
		return nil, fmt.Errorf("%w: assignee history exceeds 200 rows", contactport.ErrHistoricalChannelUnavailable)
	}
	result := make([]contactport.HistoricalChannelAssignee, 0, len(rows))
	for _, row := range rows {
		value, err := historicalChannelAssigneeRecord(row)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func historicalChannelContactRecord(row contactdb.ChannelHistoricalContact) (contactport.HistoricalChannelContact, error) {
	for _, stamp := range []pgtype.Timestamptz{row.FirstEnteredAt, row.LastEnteredAt, row.CreatedAt, row.UpdatedAt} {
		if !stamp.Valid || stamp.InfinityModifier != pgtype.Finite || stamp.Time.IsZero() {
			return contactport.HistoricalChannelContact{}, contactport.ErrHistoricalChannelUnavailable
		}
	}
	return contactport.HistoricalChannelContact{
		ID: row.ID, ChannelID: row.ChannelID, SourceContactID: row.SourceContactID, CustomerID: int64Pointer(row.CustomerID),
		OwnerReference: row.OwnerReference, EnterCount: row.EnterCount,
		FirstEnteredAt: row.FirstEnteredAt.Time.UTC().Truncate(time.Microsecond), LastEnteredAt: row.LastEnteredAt.Time.UTC().Truncate(time.Microsecond),
		CreatedAt: row.CreatedAt.Time.UTC().Truncate(time.Microsecond), UpdatedAt: row.UpdatedAt.Time.UTC().Truncate(time.Microsecond),
	}, nil
}

func historicalChannelAssigneeRecord(row contactdb.ChannelHistoricalAssignee) (contactport.HistoricalChannelAssignee, error) {
	for _, stamp := range []pgtype.Timestamp{row.SourceCreatedAt, row.SourceUpdatedAt} {
		if !stamp.Valid || stamp.InfinityModifier != pgtype.Finite || stamp.Time.IsZero() {
			return contactport.HistoricalChannelAssignee{}, contactport.ErrHistoricalChannelUnavailable
		}
	}
	value := contactport.HistoricalChannelAssignee{
		ID: row.ID, ChannelID: row.ChannelID, SourceAssigneeID: row.SourceAssigneeID, StaffReference: row.StaffReference,
		DisplayNameSnapshot: row.DisplayNameSnapshot, Priority: row.Priority, Status: row.Status,
		SourceCreatedAt: row.SourceCreatedAt.Time.Format(historicalChannelCivilLayout), SourceUpdatedAt: row.SourceUpdatedAt.Time.Format(historicalChannelCivilLayout),
	}
	if row.RatioPercent.Valid {
		value.RatioPercent = &row.RatioPercent.Int32
	}
	if row.MaxScans24h.Valid {
		value.MaxScans24h = &row.MaxScans24h.Int32
	}
	return value, nil
}

func historicalChannelTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}

func historicalChannelNullableInt32(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func historicalChannelRelationError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && strings.HasPrefix(postgresError.Code, "23") {
		return contactport.ErrHistoricalChannelConflict
	}
	return historicalChannelError(err)
}
