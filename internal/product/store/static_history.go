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

type StaticProductHistoryStore struct{}
type StaticProductHistoryReader struct{ db productdb.DBTX }

var _ productport.StaticProductHistoryStore = (*StaticProductHistoryStore)(nil)
var _ productport.StaticProductHistoryReader = (*StaticProductHistoryReader)(nil)

func NewStaticProductHistoryStore() *StaticProductHistoryStore { return &StaticProductHistoryStore{} }
func NewStaticProductHistoryReader(db productdb.DBTX) *StaticProductHistoryReader {
	return &StaticProductHistoryReader{db: db}
}

func (store *StaticProductHistoryStore) queries(ctx context.Context) (*productdb.Queries, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return nil, productport.ErrStaticProductHistoryUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, productport.ErrStaticProductHistoryUnavailable
	}
	return productdb.New(tx), nil
}

func (reader *StaticProductHistoryReader) queries(ctx context.Context) (*productdb.Queries, error) {
	if reader == nil || ctx == nil || ctx.Err() != nil {
		return nil, productport.ErrStaticProductHistoryUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil && tx != nil {
		return productdb.New(tx), nil
	}
	if reader.db == nil {
		return nil, productport.ErrStaticProductHistoryUnavailable
	}
	v := reflect.ValueOf(reader.db)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return nil, productport.ErrStaticProductHistoryUnavailable
	}
	return productdb.New(reader.db), nil
}

func (store *StaticProductHistoryStore) CreateHistoricalProductPageSlice(ctx context.Context, value productport.HistoricalProductPageSlice) (productport.HistoricalProductPageSlice, error) {
	if value.ID != 0 {
		return productport.HistoricalProductPageSlice{}, productport.ErrStaticProductHistoryInvalid
	}
	check := value
	check.ID = 1
	if _, err := productapp.HistoricalProductPageSliceDigest(check); err != nil {
		return productport.HistoricalProductPageSlice{}, productport.ErrStaticProductHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return productport.HistoricalProductPageSlice{}, err
	}
	row, err := queries.CreateHistoricalProductPageSlice(ctx, productdb.CreateHistoricalProductPageSliceParams{
		SourceID: value.SourceID, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:],
		ProductSourceID: value.ProductSourceID, ImageSourceID: value.ImageSourceID, SortOrder: value.SortOrder, OriginalEnabled: value.OriginalEnabled,
		CreatedAt: staticProductHistoryTimestamp(value.CreatedAt), UpdatedAt: staticProductHistoryTimestamp(value.UpdatedAt),
	})
	if err != nil {
		return productport.HistoricalProductPageSlice{}, staticProductHistoryStoreError(err)
	}
	return staticProductHistoryValue(row)
}

func (store *StaticProductHistoryStore) GetHistoricalProductPageSlice(ctx context.Context, id int64) (productport.HistoricalProductPageSlice, error) {
	if id < 1 {
		return productport.HistoricalProductPageSlice{}, productport.ErrStaticProductHistoryInvalid
	}
	queries, err := store.queries(ctx)
	if err != nil {
		return productport.HistoricalProductPageSlice{}, err
	}
	row, err := queries.GetHistoricalProductPageSlice(ctx, id)
	if err != nil {
		return productport.HistoricalProductPageSlice{}, staticProductHistoryStoreError(err)
	}
	return staticProductHistoryValue(row)
}

func (reader *StaticProductHistoryReader) GetHistoricalProductPageSlice(ctx context.Context, id int64) (productport.HistoricalProductPageSlice, error) {
	if id < 1 {
		return productport.HistoricalProductPageSlice{}, productport.ErrStaticProductHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return productport.HistoricalProductPageSlice{}, err
	}
	row, err := queries.GetHistoricalProductPageSlice(ctx, id)
	if err != nil {
		return productport.HistoricalProductPageSlice{}, staticProductHistoryStoreError(err)
	}
	return staticProductHistoryValue(row)
}

func (reader *StaticProductHistoryReader) ListHistoricalProductPageSlice(ctx context.Context, query productport.StaticProductHistoryQuery) ([]productport.HistoricalProductPageSlice, int64, error) {
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 {
		return nil, 0, productport.ErrStaticProductHistoryInvalid
	}
	queries, err := reader.queries(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, err := queries.CountHistoricalProductPageSlice(ctx)
	if err != nil || total < 0 {
		return nil, 0, staticProductHistoryStoreError(err)
	}
	rows, err := queries.ListHistoricalProductPageSlice(ctx, productdb.ListHistoricalProductPageSliceParams{RowLimit: query.Limit, RowOffset: query.Offset})
	if err != nil {
		return nil, 0, staticProductHistoryStoreError(err)
	}
	items := make([]productport.HistoricalProductPageSlice, 0, len(rows))
	for _, row := range rows {
		item, err := staticProductHistoryValue(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, nil
}

func staticProductHistoryValue(row productdb.ProductV1PageSliceHistory) (productport.HistoricalProductPageSlice, error) {
	if row.ID < 1 || len(row.SourceKeyDigest) != 32 || len(row.SourcePayloadDigest) != 32 || !staticProductHistoryFinite(row.CreatedAt) || !staticProductHistoryFinite(row.UpdatedAt) {
		return productport.HistoricalProductPageSlice{}, productport.ErrStaticProductHistoryUnavailable
	}
	value := productport.HistoricalProductPageSlice{ID: row.ID, SourceID: row.SourceID, ProductSourceID: row.ProductSourceID, ImageSourceID: row.ImageSourceID,
		SortOrder: row.SortOrder, OriginalEnabled: row.OriginalEnabled, CreatedAt: row.CreatedAt.Time.UTC().Truncate(time.Microsecond), UpdatedAt: row.UpdatedAt.Time.UTC().Truncate(time.Microsecond)}
	copy(value.SourceKeyDigest[:], row.SourceKeyDigest)
	copy(value.SourcePayloadDigest[:], row.SourcePayloadDigest)
	if _, err := productapp.HistoricalProductPageSliceDigest(value); err != nil {
		return productport.HistoricalProductPageSlice{}, productport.ErrStaticProductHistoryUnavailable
	}
	return value, nil
}

func staticProductHistoryTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func staticProductHistoryFinite(value pgtype.Timestamptz) bool {
	return value.Valid && value.InfinityModifier == pgtype.Finite
}
func staticProductHistoryStoreError(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return productport.ErrStaticProductHistoryConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return productport.ErrStaticProductHistoryConflict
	}
	return productport.ErrStaticProductHistoryUnavailable
}
