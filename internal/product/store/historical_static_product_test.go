package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	product "github.com/qianlan33333-png/AI-CRM-v2/internal/product"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func TestHistoricalStaticProductSQLWritesProductsOnly(t *testing.T) {
	sql := strings.ToLower(insertHistoricalStaticProductSQL)
	for _, required := range []string{
		"insert into public.products",
		"stock_quantity",
		",0,",
		"local_lifecycle",
		"'disabled'",
		"version",
		",1,",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("historical product SQL missing %q: %s", required, sql)
		}
	}
	for _, forbidden := range []string{
		"product_operation_receipts",
		"product_catalog_counters",
		"product_images",
		"product_local_entitlements",
		"event_",
		"provider",
		"queue",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("historical product SQL must not touch runtime state %q: %s", forbidden, sql)
		}
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
	_, err := NewHistoricalStaticProductStore().InsertHistoricalStaticProduct(context.Background(), definition)
	if !errors.Is(err, product.ErrHistoricalStaticProductInvalid) {
		t.Fatalf("InsertHistoricalStaticProduct() error = %v, want invalid definition", err)
	}
}

func TestHistoricalStaticProductConflictClassifiesUniqueViolation(t *testing.T) {
	err := historicalStaticProductConflict(&pgconn.PgError{Code: "23505"})
	if !errors.Is(err, product.ErrHistoricalStaticProductConflict) {
		t.Fatalf("historicalStaticProductConflict() error = %v", err)
	}
}

func TestHistoricalStaticProductStorePostgreSQL16WritesDefinitionOnly(t *testing.T) {
	databaseURL := os.Getenv("AICRM_HISTORICAL_PRODUCT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AICRM_HISTORICAL_PRODUCT_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v", version, err)
	}

	definition := historicalStaticProductStoreDefinition(t)
	definition.Product.ProductCode = fmt.Sprintf("historical-product-%d", time.Now().UnixNano())
	uow := platformstore.NewUnitOfWork(pool)
	store := NewHistoricalStaticProductStore()
	err = uow.Within(ctx, func(tx context.Context) error {
		db, txErr := platformstore.TxFromContext(tx)
		if txErr != nil {
			return txErr
		}
		var countersBefore, receiptsBefore int64
		if txErr = db.QueryRow(tx, `SELECT total_products FROM public.product_catalog_counters WHERE singleton`).Scan(&countersBefore); txErr != nil {
			return txErr
		}
		if txErr = db.QueryRow(tx, `SELECT count(*) FROM public.product_operation_receipts`).Scan(&receiptsBefore); txErr != nil {
			return txErr
		}
		stored, insertErr := store.InsertHistoricalStaticProduct(tx, definition)
		if insertErr != nil {
			return insertErr
		}
		if stored.ID < 1 || stored.LocalLifecycle != productport.LocalProductDisabled || stored.StockQuantity != 0 || stored.PriceMinor != definition.Product.PriceMinor || stored.Currency != definition.Product.Currency {
			return fmt.Errorf("stored static product=%#v", stored)
		}
		var countersAfter, receiptsAfter, images int64
		if txErr = db.QueryRow(tx, `SELECT total_products FROM public.product_catalog_counters WHERE singleton`).Scan(&countersAfter); txErr != nil {
			return txErr
		}
		if txErr = db.QueryRow(tx, `SELECT count(*) FROM public.product_operation_receipts`).Scan(&receiptsAfter); txErr != nil {
			return txErr
		}
		if txErr = db.QueryRow(tx, `SELECT count(*) FROM public.product_images WHERE product_id=$1`, stored.ID).Scan(&images); txErr != nil {
			return txErr
		}
		if countersAfter != countersBefore || receiptsAfter != receiptsBefore || images != 0 {
			return fmt.Errorf("runtime state changed counters=%d/%d receipts=%d/%d images=%d", countersBefore, countersAfter, receiptsBefore, receiptsAfter, images)
		}
		return errHistoricalStaticProductStoreRollback
	})
	if !errors.Is(err, errHistoricalStaticProductStoreRollback) {
		t.Fatalf("historical static product transaction = %v", err)
	}
	var rows int64
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.products WHERE product_code=$1`, definition.Product.ProductCode).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("rollback rows=%d err=%v", rows, err)
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

var errHistoricalStaticProductStoreRollback = errors.New("rollback historical static product store")
