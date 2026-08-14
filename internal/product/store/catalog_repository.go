package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

type CatalogRepository struct{}

var _ productapp.Store = (*CatalogRepository)(nil)

func NewCatalogRepository() *CatalogRepository { return &CatalogRepository{} }
func queries(ctx context.Context) (*productdb.Queries, error) {
	tx, e := platformstore.TxFromContext(ctx)
	if e != nil {
		return nil, e
	}
	return productdb.New(tx), nil
}
func (r *CatalogRepository) List(ctx context.Context, after *productport.ID, limit int32) ([]productport.Product, error) {
	q, e := queries(ctx)
	if r == nil || e != nil || limit < 1 {
		return nil, unavailable(e)
	}
	rows, e := q.ListProducts(ctx, productdb.ListProductsParams{AfterID: optionalID(after), RowLimit: limit})
	if e != nil {
		return nil, unavailable(e)
	}
	out := make([]productport.Product, len(rows))
	for i, row := range rows {
		out[i], e = mapRow(row.ID, row.ProductCode, row.Name, row.Description, row.PriceMinor, row.Currency, row.StockQuantity, row.CreatedBy, row.CreatedAt, row.UpdatedAt, row.LegacyAdminProjection, row.Images)
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (r *CatalogRepository) ListOffset(ctx context.Context, limit, offset int32) ([]productport.Product, error) {
	q, e := queries(ctx)
	if r == nil || e != nil || limit < 1 || offset < 0 {
		return nil, unavailable(e)
	}
	rows, e := q.ListProductsOffset(ctx, productdb.ListProductsOffsetParams{RowLimit: limit, RowOffset: offset})
	if e != nil {
		return nil, unavailable(e)
	}
	out := make([]productport.Product, len(rows))
	for i, row := range rows {
		out[i], e = mapRow(row.ID, row.ProductCode, row.Name, row.Description, row.PriceMinor, row.Currency, row.StockQuantity, row.CreatedBy, row.CreatedAt, row.UpdatedAt, row.LegacyAdminProjection, row.Images)
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (r *CatalogRepository) Count(ctx context.Context) (int64, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return 0, unavailable(e)
	}
	total, e := q.CountProducts(ctx)
	if e != nil || total < 0 {
		return 0, unavailable(e)
	}
	return total, nil
}
func (r *CatalogRepository) Get(ctx context.Context, id productport.ID) (productport.Product, error) {
	q, e := queries(ctx)
	if r == nil || e != nil || id < 1 {
		return productport.Product{}, unavailable(e)
	}
	row, e := q.GetProduct(ctx, int64(id))
	if e != nil {
		return productport.Product{}, unavailable(e)
	}
	return mapRow(row.ID, row.ProductCode, row.Name, row.Description, row.PriceMinor, row.Currency, row.StockQuantity, row.CreatedBy, row.CreatedAt, row.UpdatedAt, row.LegacyAdminProjection, row.Images)
}
func (r *CatalogRepository) Create(ctx context.Context, c productport.CreateCommand, now time.Time) (productport.Product, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return productport.Product{}, unavailable(e)
	}
	row, e := q.CreateProduct(ctx, productdb.CreateProductParams{ProductCode: c.ProductCode, Name: c.Name, Description: c.Description, PriceMinor: c.PriceMinor, Currency: c.Currency, StockQuantity: c.StockQuantity, CreatedBy: c.Actor, CreatedAt: stamp(now), LegacyAdminProjection: c.LegacyAdminProjection})
	if e != nil {
		return productport.Product{}, unavailable(e)
	}
	for pos, url := range c.Images {
		if e = q.InsertProductImage(ctx, productdb.InsertProductImageParams{ProductID: row.ID, Position: int32(pos), ImageUrl: url}); e != nil {
			return productport.Product{}, unavailable(e)
		}
	}
	if total, countErr := q.IncrementProductCount(ctx); countErr != nil || total < 1 {
		return productport.Product{}, unavailable(countErr)
	}
	return productport.Product{ID: productport.ID(row.ID), ProductCode: row.ProductCode, Name: row.Name, Description: row.Description, PriceMinor: row.PriceMinor, Currency: row.Currency, StockQuantity: row.StockQuantity, Images: append([]string(nil), c.Images...), CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, LegacyAdminProjection: append([]byte(nil), row.LegacyAdminProjection...)}, nil
}
func (r *CatalogRepository) Reserve(ctx context.Context, x productapp.Reservation) (productapp.Receipt, bool, error) {
	q, e := queries(ctx)
	if r == nil || e != nil {
		return productapp.Receipt{}, false, unavailable(e)
	}
	args := productdb.ReserveProductOperationReceiptParams{ActorScope: x.ActorScope, KeyDigest: x.KeyDigest[:], PayloadDigest: x.PayloadDigest[:], CreatedAt: stamp(x.CreatedAt)}
	row, e := q.ReserveProductOperationReceipt(ctx, args)
	if e == nil {
		return receipt(row.ID, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), true, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return productapp.Receipt{}, false, unavailable(e)
	}
	old, e := q.GetProductOperationReceipt(ctx, productdb.GetProductOperationReceiptParams{ActorScope: x.ActorScope, KeyDigest: x.KeyDigest[:]})
	if e != nil {
		return productapp.Receipt{}, false, unavailable(e)
	}
	return receipt(old.ID, old.ActorScope, old.KeyDigest, old.PayloadDigest, old.State, old.ResultSnapshot), false, nil
}
func (r *CatalogRepository) Complete(ctx context.Context, id int64, snapshot json.RawMessage, now time.Time) (productapp.Receipt, error) {
	q, e := queries(ctx)
	if r == nil || e != nil || id < 1 || !json.Valid(snapshot) {
		return productapp.Receipt{}, unavailable(e)
	}
	row, e := q.CompleteProductOperationReceipt(ctx, productdb.CompleteProductOperationReceiptParams{ID: id, ResultSnapshot: snapshot, CompletedAt: stamp(now)})
	if e != nil {
		return productapp.Receipt{}, unavailable(e)
	}
	return receipt(row.ID, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot), nil
}
func receipt(id int64, actor string, key, payload []byte, state string, snapshot []byte) productapp.Receipt {
	var r productapp.Receipt
	r.ID = id
	r.ActorScope = actor
	r.State = state
	r.ResultSnapshot = append([]byte(nil), snapshot...)
	copy(r.KeyDigest[:], key)
	copy(r.PayloadDigest[:], payload)
	return r
}
func mapRow(id int64, productCode, name, description string, price int64, currency string, stock int32, createdBy int64, created, updated pgtype.Timestamptz, projection []byte, raw any) (productport.Product, error) {
	images, ok := raw.([]byte)
	if !ok {
		return productport.Product{}, productapp.ErrUnavailable
	}
	var urls []string
	if json.Unmarshal(images, &urls) != nil {
		return productport.Product{}, productapp.ErrUnavailable
	}
	return productport.Product{ID: productport.ID(id), ProductCode: productCode, Name: name, Description: description, PriceMinor: price, Currency: currency, StockQuantity: stock, Images: urls, CreatedBy: createdBy, CreatedAt: created.Time, UpdatedAt: updated.Time, LegacyAdminProjection: append([]byte(nil), projection...)}, nil
}
func optionalID(id *productport.ID) pgtype.Int8 {
	if id == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: int64(*id), Valid: true}
}
func stamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
func unavailable(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return productapp.ErrNotFound
	}
	if e != nil {
		var pgErr *pgconn.PgError
		if errors.As(e, &pgErr) && pgErr.Code == "23505" {
			return productapp.ErrConflict
		}
		return e
	}
	return productapp.ErrUnavailable
}
