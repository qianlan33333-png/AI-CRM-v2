package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	product "github.com/qianlan33333-png/AI-CRM-v2/internal/product"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

// HistoricalStaticProductStore writes a migration-only Product definition. It
// deliberately does not use CatalogRepository.Create: that path reserves
// operation receipts and increments the live catalogue counter.
type HistoricalStaticProductStore struct {
	tx         func(context.Context) (pgx.Tx, error)
	newQueries func(pgx.Tx) historicalStaticProductQueries
}

var _ product.HistoricalStaticProductStore = (*HistoricalStaticProductStore)(nil)

type historicalStaticProductQueries interface {
	InsertHistoricalStaticProduct(context.Context, productdb.InsertHistoricalStaticProductParams) (productdb.InsertHistoricalStaticProductRow, error)
}

func NewHistoricalStaticProductStore() *HistoricalStaticProductStore {
	return &HistoricalStaticProductStore{
		tx: platformstore.TxFromContext,
		newQueries: func(tx pgx.Tx) historicalStaticProductQueries {
			return productdb.New(tx)
		},
	}
}

// InsertHistoricalStaticProduct writes products only. It cannot write a
// runtime receipt, event, queue item, entitlement, image, or Provider fact.
func (store *HistoricalStaticProductStore) InsertHistoricalStaticProduct(ctx context.Context, definition product.HistoricalStaticProductDefinition) (productport.Product, error) {
	if store == nil || store.tx == nil || store.newQueries == nil || ctx == nil || !validStoredHistoricalStaticProduct(definition) {
		return productport.Product{}, product.ErrHistoricalStaticProductInvalid
	}
	tx, err := store.tx(ctx)
	if err != nil {
		return productport.Product{}, err
	}
	queries := store.newQueries(tx)
	if queries == nil {
		return productport.Product{}, product.ErrHistoricalStaticProductInvalid
	}
	item := definition.Product
	row, err := queries.InsertHistoricalStaticProduct(ctx, productdb.InsertHistoricalStaticProductParams{
		ProductCode:           item.ProductCode,
		Name:                  item.Name,
		PriceMinor:            item.PriceMinor,
		Currency:              item.Currency,
		CreatedBy:             item.CreatedBy,
		CreatedAt:             stamp(item.CreatedAt.UTC()),
		UpdatedAt:             stamp(item.UpdatedAt.UTC()),
		LegacyAdminProjection: item.LegacyAdminProjection,
	})
	if err != nil {
		return productport.Product{}, historicalStaticProductConflict(err)
	}
	if row.ID < 1 || row.Description != "" || row.StockQuantity != 0 || !row.CreatedAt.Valid || !row.UpdatedAt.Valid || row.Version != 1 || row.LocalLifecycle != string(productport.LocalProductDisabled) {
		return productport.Product{}, product.ErrHistoricalStaticProductInvalid
	}
	return productport.Product{
		ID:                    productport.ID(row.ID),
		ProductCode:           row.ProductCode,
		Name:                  row.Name,
		Description:           row.Description,
		PriceMinor:            row.PriceMinor,
		Currency:              row.Currency,
		StockQuantity:         row.StockQuantity,
		Images:                []string{},
		CreatedBy:             row.CreatedBy,
		CreatedAt:             row.CreatedAt.Time,
		UpdatedAt:             row.UpdatedAt.Time,
		Version:               row.Version,
		LocalLifecycle:        productport.LocalProductLifecycle(row.LocalLifecycle),
		LegacyAdminProjection: append([]byte(nil), row.LegacyAdminProjection...),
	}, nil
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
