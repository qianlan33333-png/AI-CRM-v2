package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	product "github.com/qianlan33333-png/AI-CRM-v2/internal/product"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

func TestHistoricalStaticProductStoreUsesGeneratedStaticQuery(t *testing.T) {
	definition := historicalStaticProductStoreDefinition(t)
	query := &historicalStaticProductQueriesFake{}
	store := historicalStaticProductStoreFake(query)
	stored, err := store.InsertHistoricalStaticProduct(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	if query.calls != 1 || query.params.ProductCode != definition.Product.ProductCode || query.params.Name != definition.Product.Name ||
		query.params.PriceMinor != definition.Product.PriceMinor || query.params.Currency != definition.Product.Currency || query.params.CreatedBy != definition.Product.CreatedBy ||
		!query.params.CreatedAt.Time.Equal(definition.Product.CreatedAt) || !query.params.UpdatedAt.Time.Equal(definition.Product.UpdatedAt) ||
		string(query.params.LegacyAdminProjection) != string(definition.Product.LegacyAdminProjection) {
		t.Fatalf("generated query params = %#v", query.params)
	}
	if stored.ID != 41 || stored.LocalLifecycle != productport.LocalProductDisabled || stored.StockQuantity != 0 || stored.Description != "" || stored.Version != 1 || len(stored.Images) != 0 {
		t.Fatalf("stored product = %#v", stored)
	}
}

func TestHistoricalStaticProductStoreRequiresCallerTransaction(t *testing.T) {
	store := NewHistoricalStaticProductStore()
	_, err := store.InsertHistoricalStaticProduct(context.Background(), historicalStaticProductStoreDefinition(t))
	if !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("InsertHistoricalStaticProduct() error = %v, want transaction required", err)
	}
}

func TestHistoricalStaticProductStoreRejectsAnythingButDisabledStaticDefinition(t *testing.T) {
	definition := historicalStaticProductStoreDefinition(t)
	definition.Product.LocalLifecycle = productport.LocalProductEnabled
	query := &historicalStaticProductQueriesFake{}
	_, err := historicalStaticProductStoreFake(query).InsertHistoricalStaticProduct(context.Background(), definition)
	if !errors.Is(err, product.ErrHistoricalStaticProductInvalid) {
		t.Fatalf("InsertHistoricalStaticProduct() error = %v, want invalid definition", err)
	}
	if query.calls != 0 {
		t.Fatalf("invalid definition called generated query %d times", query.calls)
	}
}

func TestHistoricalStaticProductConflictClassifiesUniqueViolation(t *testing.T) {
	err := historicalStaticProductConflict(&pgconn.PgError{Code: "23505"})
	if !errors.Is(err, product.ErrHistoricalStaticProductConflict) {
		t.Fatalf("historicalStaticProductConflict() error = %v", err)
	}
}

func historicalStaticProductStoreDefinition(t *testing.T) product.HistoricalStaticProductDefinition {
	t.Helper()
	projection, err := json.Marshal(map[string]any{"schema_version": 1, "status": "disabled", "enabled": false})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	return product.HistoricalStaticProductDefinition{
		SourceIdentifier: "wechat_pay_products/opaque-key",
		SourceID:         29,
		Product: productport.Product{
			ProductCode:           "hxc-annual",
			Name:                  "HXC 年度服务",
			PriceMinor:            19900,
			Currency:              "CNY",
			CreatedBy:             9,
			CreatedAt:             now,
			UpdatedAt:             now,
			Version:               1,
			LocalLifecycle:        productport.LocalProductDisabled,
			LegacyAdminProjection: projection,
		},
	}
}

func historicalStaticProductStoreFake(query *historicalStaticProductQueriesFake) *HistoricalStaticProductStore {
	return &HistoricalStaticProductStore{
		tx: func(context.Context) (pgx.Tx, error) { return nil, nil },
		newQueries: func(pgx.Tx) historicalStaticProductQueries {
			return query
		},
	}
}

type historicalStaticProductQueriesFake struct {
	calls  int
	params productdb.InsertHistoricalStaticProductParams
	err    error
}

func (fake *historicalStaticProductQueriesFake) InsertHistoricalStaticProduct(_ context.Context, params productdb.InsertHistoricalStaticProductParams) (productdb.InsertHistoricalStaticProductRow, error) {
	fake.calls++
	fake.params = params
	if fake.err != nil {
		return productdb.InsertHistoricalStaticProductRow{}, fake.err
	}
	return productdb.InsertHistoricalStaticProductRow{
		ID:                    41,
		ProductCode:           params.ProductCode,
		Name:                  params.Name,
		Description:           "",
		PriceMinor:            params.PriceMinor,
		Currency:              params.Currency,
		StockQuantity:         0,
		CreatedBy:             params.CreatedBy,
		CreatedAt:             params.CreatedAt,
		UpdatedAt:             params.UpdatedAt,
		Version:               1,
		LocalLifecycle:        string(productport.LocalProductDisabled),
		LegacyAdminProjection: params.LegacyAdminProjection,
	}, nil
}
