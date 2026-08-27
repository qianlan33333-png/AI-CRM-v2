package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	product "github.com/qianlan33333-png/AI-CRM-v2/internal/product"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

// HistoricalStaticProductStore writes a migration-only Product definition. It
// deliberately does not use CatalogRepository.Create: that path reserves
// operation receipts and increments the live catalogue counter.
type HistoricalStaticProductStore struct {
	tx func(context.Context) (pgx.Tx, error)
}

var _ product.HistoricalStaticProductStore = (*HistoricalStaticProductStore)(nil)

const insertHistoricalStaticProductSQL = `INSERT INTO public.products
	(product_code,name,description,price_minor,currency,stock_quantity,created_by,created_at,updated_at,version,local_lifecycle,legacy_admin_projection)
	VALUES ($1,$2,'',$3,$4,0,$5,$6,$7,1,'disabled',$8::jsonb)
	RETURNING id,product_code,name,description,price_minor,currency,stock_quantity,created_by,created_at,updated_at,version,local_lifecycle,legacy_admin_projection`

func NewHistoricalStaticProductStore() *HistoricalStaticProductStore {
	return &HistoricalStaticProductStore{tx: platformstore.TxFromContext}
}

// InsertHistoricalStaticProduct writes products only. It cannot write a
// runtime receipt, event, queue item, entitlement, image, or Provider fact.
func (store *HistoricalStaticProductStore) InsertHistoricalStaticProduct(ctx context.Context, definition product.HistoricalStaticProductDefinition) (productport.Product, error) {
	if store == nil || store.tx == nil || ctx == nil || !validStoredHistoricalStaticProduct(definition) {
		return productport.Product{}, product.ErrHistoricalStaticProductInvalid
	}
	tx, err := store.tx(ctx)
	if err != nil {
		return productport.Product{}, err
	}

	item := definition.Product
	var (
		id         int64
		createdAt  pgtype.Timestamptz
		updatedAt  pgtype.Timestamptz
		projection []byte
	)
	err = tx.QueryRow(ctx, insertHistoricalStaticProductSQL,
		item.ProductCode, item.Name, item.PriceMinor, item.Currency, item.CreatedBy, item.CreatedAt.UTC(), item.UpdatedAt.UTC(), item.LegacyAdminProjection,
	).Scan(
		&id, &item.ProductCode, &item.Name, &item.Description, &item.PriceMinor, &item.Currency, &item.StockQuantity, &item.CreatedBy,
		&createdAt, &updatedAt, &item.Version, &item.LocalLifecycle, &projection,
	)
	if err != nil {
		return productport.Product{}, historicalStaticProductConflict(err)
	}
	if id < 1 || !createdAt.Valid || !updatedAt.Valid {
		return productport.Product{}, product.ErrHistoricalStaticProductInvalid
	}
	item.ID = productport.ID(id)
	item.Images = []string{}
	item.CreatedAt = createdAt.Time
	item.UpdatedAt = updatedAt.Time
	item.LegacyAdminProjection = append([]byte(nil), projection...)
	return item, nil
}

func historicalStaticProductConflict(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return product.ErrHistoricalStaticProductConflict
	}
	return err
}

func validStoredHistoricalStaticProduct(definition product.HistoricalStaticProductDefinition) bool {
	item := definition.Product
	if definition.SourceIdentifier == "" || strings.TrimSpace(definition.SourceIdentifier) != definition.SourceIdentifier || definition.SourceID < 1 ||
		item.ID != 0 || strings.TrimSpace(item.ProductCode) == "" || len(item.ProductCode) > 200 || strings.TrimSpace(item.ProductCode) != item.ProductCode ||
		strings.TrimSpace(item.Name) == "" || len(item.Name) > 200 || strings.TrimSpace(item.Name) != item.Name ||
		item.Description != "" || item.PriceMinor < 0 || len(item.Currency) != 3 || item.StockQuantity != 0 || len(item.Images) != 0 || item.CreatedBy < 1 ||
		item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() || item.UpdatedAt.Before(item.CreatedAt) || item.Version != 1 || item.LocalLifecycle != productport.LocalProductDisabled {
		return false
	}
	for _, character := range item.Currency {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	var projection struct {
		Status  string `json:"status"`
		Enabled *bool  `json:"enabled"`
	}
	return json.Unmarshal(item.LegacyAdminProjection, &projection) == nil && projection.Status == "disabled" && projection.Enabled != nil && !*projection.Enabled
}
