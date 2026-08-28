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
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

type MemberGridHistoryStore struct{}
type MemberGridHistoryReader struct{ db productdb.DBTX }

var _ productport.MemberGridHistoryStore = (*MemberGridHistoryStore)(nil)
var _ productport.MemberGridHistoryReader = (*MemberGridHistoryReader)(nil)

func NewMemberGridHistoryStore() *MemberGridHistoryStore { return &MemberGridHistoryStore{} }
func NewMemberGridHistoryReader(db productdb.DBTX) *MemberGridHistoryReader {
	return &MemberGridHistoryReader{db: db}
}

func (store *MemberGridHistoryStore) queries(ctx context.Context) (*productdb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, productport.ErrMemberGridHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, productport.ErrMemberGridHistoryUnavailable
	}
	return productdb.New(tx), nil
}

func (reader *MemberGridHistoryReader) queries(ctx context.Context) (*productdb.Queries, error) {
	if reader == nil || reader.db == nil || ctx == nil || ctx.Err() != nil {
		return nil, productport.ErrMemberGridHistoryUnavailable
	}
	v := reflect.ValueOf(reader.db)
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return nil, productport.ErrMemberGridHistoryUnavailable
	}
	return productdb.New(reader.db), nil
}

func (store *MemberGridHistoryStore) CreateHistoricalMemberView(ctx context.Context, value productport.HistoricalMemberView) (productport.HistoricalMemberView, error) {
	if value.ID != 0 {
		return productport.HistoricalMemberView{}, productport.ErrMemberGridHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := productapp.HistoricalMemberViewDigest(check); err != nil {
		return productport.HistoricalMemberView{}, productport.ErrMemberGridHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return productport.HistoricalMemberView{}, err
	}
	row, err := q.CreateHistoricalMemberView(ctx, productdb.CreateHistoricalMemberViewParams{
		SourceKeyDigest: value.SourceKeyDigest[:], SourceViewID: value.SourceViewID, SourceServiceProductID: value.SourceServiceProductID,
		ProductID: memberGridHistoryInt(value.ProductID), Name: value.Name, Position: value.Position, IsDefault: value.IsDefault,
		SchemaVersion: value.SchemaVersion, ConfigDigest: value.ConfigDigest[:], Version: value.Version,
		CreatedAt: memberGridHistoryTimestamp(value.CreatedAt), UpdatedAt: memberGridHistoryTimestamp(value.UpdatedAt), SourcePayloadDigest: value.SourcePayloadDigest[:],
	})
	if err != nil {
		return productport.HistoricalMemberView{}, memberGridHistoryError(err)
	}
	return memberGridHistoryView(row)
}

func (store *MemberGridHistoryStore) GetHistoricalMemberView(ctx context.Context, id int64) (productport.HistoricalMemberView, error) {
	if id < 1 {
		return productport.HistoricalMemberView{}, productport.ErrMemberGridHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return productport.HistoricalMemberView{}, err
	}
	row, err := q.GetHistoricalMemberView(ctx, id)
	if err != nil {
		return productport.HistoricalMemberView{}, memberGridHistoryError(err)
	}
	return memberGridHistoryView(row)
}

func (reader *MemberGridHistoryReader) GetHistoricalMemberView(ctx context.Context, id int64) (productport.HistoricalMemberView, error) {
	if id < 1 {
		return productport.HistoricalMemberView{}, productport.ErrMemberGridHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return productport.HistoricalMemberView{}, err
	}
	row, err := q.GetHistoricalMemberView(ctx, id)
	if err != nil {
		return productport.HistoricalMemberView{}, memberGridHistoryError(err)
	}
	return memberGridHistoryView(row)
}

func (reader *MemberGridHistoryReader) ListHistoricalMemberViews(ctx context.Context, query productport.MemberGridHistoryQuery) ([]productport.HistoricalMemberView, int64, error) {
	if !memberGridHistoryPage(query) || query.CustomerID != nil || (query.ProductID != nil && *query.ProductID < 1) {
		return nil, 0, productport.ErrMemberGridHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalMemberViews(ctx, memberGridHistoryInt(query.ProductID))
	if err != nil {
		return nil, 0, memberGridHistoryError(err)
	}
	rows, err := q.ListHistoricalMemberViews(ctx, productdb.ListHistoricalMemberViewsParams{ProductID: memberGridHistoryInt(query.ProductID), RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, memberGridHistoryError(err)
	}
	items := make([]productport.HistoricalMemberView, 0, len(rows))
	for _, row := range rows {
		value, err := memberGridHistoryView(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func (store *MemberGridHistoryStore) CreateHistoricalMemberUsage(ctx context.Context, value productport.HistoricalMemberUsage) (productport.HistoricalMemberUsage, error) {
	if value.ID != 0 {
		return productport.HistoricalMemberUsage{}, productport.ErrMemberGridHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := productapp.HistoricalMemberUsageDigest(check); err != nil {
		return productport.HistoricalMemberUsage{}, productport.ErrMemberGridHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return productport.HistoricalMemberUsage{}, err
	}
	row, err := q.CreateHistoricalMemberUsage(ctx, productdb.CreateHistoricalMemberUsageParams{
		SourceKeyDigest: value.SourceKeyDigest[:], CustomerID: memberGridHistoryInt(value.CustomerID), FormallyLoggedIn: value.FormallyLoggedIn,
		HasTokenUsage: value.HasTokenUsage, LearningPlanID: value.LearningPlanID, LearningPlanCurrent: memberGridHistoryInt(value.LearningPlanCurrent),
		LearningPlanTotal: memberGridHistoryInt(value.LearningPlanTotal), OpenCount7d: value.OpenCount7D, LastOpenAt: memberGridHistoryTime(value.LastOpenAt),
		RefreshedAt: memberGridHistoryTimestamp(value.RefreshedAt), SourcePayloadDigest: value.SourcePayloadDigest[:], RecoveryEntryDigest: value.RecoveryEntryDigest[:],
	})
	if err != nil {
		return productport.HistoricalMemberUsage{}, memberGridHistoryError(err)
	}
	return memberGridHistoryUsage(row)
}

func (store *MemberGridHistoryStore) GetHistoricalMemberUsage(ctx context.Context, id int64) (productport.HistoricalMemberUsage, error) {
	if id < 1 {
		return productport.HistoricalMemberUsage{}, productport.ErrMemberGridHistoryInvalid
	}
	q, err := store.queries(ctx)
	if err != nil {
		return productport.HistoricalMemberUsage{}, err
	}
	row, err := q.GetHistoricalMemberUsage(ctx, id)
	if err != nil {
		return productport.HistoricalMemberUsage{}, memberGridHistoryError(err)
	}
	return memberGridHistoryUsage(row)
}

func (reader *MemberGridHistoryReader) GetHistoricalMemberUsage(ctx context.Context, id int64) (productport.HistoricalMemberUsage, error) {
	if id < 1 {
		return productport.HistoricalMemberUsage{}, productport.ErrMemberGridHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return productport.HistoricalMemberUsage{}, err
	}
	row, err := q.GetHistoricalMemberUsage(ctx, id)
	if err != nil {
		return productport.HistoricalMemberUsage{}, memberGridHistoryError(err)
	}
	return memberGridHistoryUsage(row)
}

func (reader *MemberGridHistoryReader) ListHistoricalMemberUsage(ctx context.Context, query productport.MemberGridHistoryQuery) ([]productport.HistoricalMemberUsage, int64, error) {
	if !memberGridHistoryPage(query) || query.ProductID != nil || (query.CustomerID != nil && *query.CustomerID < 1) {
		return nil, 0, productport.ErrMemberGridHistoryInvalid
	}
	q, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountHistoricalMemberUsage(ctx, memberGridHistoryInt(query.CustomerID))
	if err != nil {
		return nil, 0, memberGridHistoryError(err)
	}
	rows, err := q.ListHistoricalMemberUsage(ctx, productdb.ListHistoricalMemberUsageParams{CustomerID: memberGridHistoryInt(query.CustomerID), RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, memberGridHistoryError(err)
	}
	items := make([]productport.HistoricalMemberUsage, 0, len(rows))
	for _, row := range rows {
		value, err := memberGridHistoryUsage(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, nil
}

func memberGridHistoryPage(query productport.MemberGridHistoryQuery) bool {
	return query.Limit >= 1 && query.Limit <= 100 && query.Offset >= 0
}

func memberGridHistoryInt(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func memberGridHistoryTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func memberGridHistoryTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return memberGridHistoryTimestamp(*value)
}

func memberGridHistoryView(row productdb.ProductV1MemberViewHistory) (productport.HistoricalMemberView, error) {
	if !memberGridHistoryFinite(row.CreatedAt) || !memberGridHistoryFinite(row.UpdatedAt) || len(row.SourceKeyDigest) != 32 || len(row.ConfigDigest) != 32 || len(row.SourcePayloadDigest) != 32 {
		return productport.HistoricalMemberView{}, productport.ErrMemberGridHistoryUnavailable
	}
	value := productport.HistoricalMemberView{ID: row.ID, SourceViewID: row.SourceViewID, SourceServiceProductID: row.SourceServiceProductID,
		Name: row.Name, Position: row.Position, IsDefault: row.IsDefault, SchemaVersion: row.SchemaVersion, Version: row.Version,
		CreatedAt: row.CreatedAt.Time.UTC().Truncate(time.Microsecond), UpdatedAt: row.UpdatedAt.Time.UTC().Truncate(time.Microsecond)}
	copy(value.SourceKeyDigest[:], row.SourceKeyDigest)
	copy(value.ConfigDigest[:], row.ConfigDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	if row.ProductID.Valid {
		productID := row.ProductID.Int64
		value.ProductID = &productID
	}
	if _, err := productapp.HistoricalMemberViewDigest(value); err != nil {
		return productport.HistoricalMemberView{}, productport.ErrMemberGridHistoryUnavailable
	}
	return value, nil
}

func memberGridHistoryUsage(row productdb.ProductV1MemberUsageHistory) (productport.HistoricalMemberUsage, error) {
	if !memberGridHistoryFinite(row.RefreshedAt) || len(row.SourceKeyDigest) != 32 || len(row.SourcePayloadDigest) != 32 || len(row.RecoveryEntryDigest) != 32 {
		return productport.HistoricalMemberUsage{}, productport.ErrMemberGridHistoryUnavailable
	}
	value := productport.HistoricalMemberUsage{ID: row.ID, FormallyLoggedIn: row.FormallyLoggedIn, HasTokenUsage: row.HasTokenUsage,
		LearningPlanID: row.LearningPlanID, OpenCount7D: row.OpenCount7d, RefreshedAt: row.RefreshedAt.Time.UTC().Truncate(time.Microsecond)}
	copy(value.SourceKeyDigest[:], row.SourceKeyDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	copy(value.RecoveryEntryDigest[:], row.RecoveryEntryDigest)
	var err error
	if value.CustomerID, err = memberGridHistoryOptionalInt(row.CustomerID); err != nil {
		return productport.HistoricalMemberUsage{}, err
	}
	if value.LearningPlanCurrent, err = memberGridHistoryOptionalInt(row.LearningPlanCurrent); err != nil {
		return productport.HistoricalMemberUsage{}, err
	}
	if value.LearningPlanTotal, err = memberGridHistoryOptionalInt(row.LearningPlanTotal); err != nil {
		return productport.HistoricalMemberUsage{}, err
	}
	if value.LastOpenAt, err = memberGridHistoryOptionalTime(row.LastOpenAt); err != nil {
		return productport.HistoricalMemberUsage{}, err
	}
	if _, err := productapp.HistoricalMemberUsageDigest(value); err != nil {
		return productport.HistoricalMemberUsage{}, productport.ErrMemberGridHistoryUnavailable
	}
	return value, nil
}

func memberGridHistoryFinite(value pgtype.Timestamptz) bool {
	return value.Valid && value.InfinityModifier == pgtype.Finite
}

func memberGridHistoryOptionalInt(value pgtype.Int8) (*int64, error) {
	if !value.Valid {
		return nil, nil
	}
	result := value.Int64
	return &result, nil
}

func memberGridHistoryOptionalTime(value pgtype.Timestamptz) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	if !memberGridHistoryFinite(value) {
		return nil, productport.ErrMemberGridHistoryUnavailable
	}
	result := value.Time.UTC().Truncate(time.Microsecond)
	return &result, nil
}

func memberGridHistoryError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return productport.ErrMemberGridHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return productport.ErrMemberGridHistoryConflict
	}
	return productport.ErrMemberGridHistoryUnavailable
}
