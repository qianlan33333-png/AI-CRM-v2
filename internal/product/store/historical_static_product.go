package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	product "github.com/qianlan33333-png/AI-CRM-v2/internal/product"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
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

func (store *HistoricalStaticProductStore) ProjectHistoricalEditableProduct(ctx context.Context, projection product.HistoricalEditableProductProjection) (bool, error) {
	if store == nil || store.tx == nil || ctx == nil {
		return false, product.ErrHistoricalStaticProductInvalid
	}
	canonical, err := productapp.CanonicalLegacyAdminProjection(projection.AdminProjection)
	if err != nil {
		return false, product.ErrHistoricalStaticProductInvalid
	}
	var state struct {
		Status  string `json:"status"`
		Enabled bool   `json:"enabled"`
	}
	if json.Unmarshal(canonical, &state) != nil || state.Enabled != (projection.LocalLifecycle == productport.LocalProductEnabled) {
		return false, product.ErrHistoricalStaticProductInvalid
	}
	tx, err := store.tx(ctx)
	if err != nil {
		return false, err
	}
	var sourceID int64
	var digest []byte
	err = tx.QueryRow(ctx, `SELECT source_id, source_payload_digest FROM public.product_v1_editable_projections WHERE product_id=$1`, int64(projection.TargetProductID)).Scan(&sourceID, &digest)
	if err == nil {
		if sourceID != projection.SourceID || !bytes.Equal(digest, projection.PayloadDigest[:]) {
			return false, product.ErrHistoricalStaticProductConflict
		}
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	result, err := tx.Exec(ctx, `
UPDATE public.products
SET legacy_admin_projection=$1::jsonb, local_lifecycle=$2::text
WHERE id=$3 AND version=1 AND local_lifecycle='disabled'
  AND legacy_admin_projection->>'status'='disabled'
  AND legacy_admin_projection->>'enabled'='false'`, canonical, string(projection.LocalLifecycle), int64(projection.TargetProductID))
	if err != nil {
		return false, err
	}
	if result.RowsAffected() != 1 {
		return false, product.ErrHistoricalStaticProductConflict
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO public.product_v1_editable_projections
  (product_id,source_id,source_payload_digest,configuration_projected_at)
VALUES ($1,$2,$3,$4)`, int64(projection.TargetProductID), projection.SourceID, projection.PayloadDigest[:], projection.ProjectedAt); err != nil {
		return false, historicalStaticProductConflict(err)
	}
	return false, nil
}

type HistoricalProductImagesProjectionResult struct {
	ProductsProjected int `json:"products_projected"`
	ImagesProjected   int `json:"images_projected"`
	ProductsReplayed  int `json:"products_replayed"`
}

type HistoricalServicePeriodProjectionResult struct {
	ProductsProjected int `json:"products_projected"`
	ProductsReplayed  int `json:"products_replayed"`
}

func (store *HistoricalStaticProductStore) ProjectHistoricalServicePeriodProducts(ctx context.Context, at time.Time) (HistoricalServicePeriodProjectionResult, error) {
	if store == nil || store.tx == nil || ctx == nil || at.IsZero() || at.Location() != time.UTC {
		return HistoricalServicePeriodProjectionResult{}, product.ErrHistoricalStaticProductInvalid
	}
	tx, err := store.tx(ctx)
	if err != nil {
		return HistoricalServicePeriodProjectionResult{}, err
	}
	if _, err = tx.Exec(ctx, `LOCK TABLE public.product_v1_editable_projections IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return HistoricalServicePeriodProjectionResult{}, err
	}
	var total, pending, unsafe int
	err = tx.QueryRow(ctx, `
SELECT count(*),
       count(*) FILTER (WHERE projection.service_period_projected_at IS NULL),
       count(*) FILTER (WHERE projection.service_period_projected_at IS NULL AND item.version<>1)
FROM public.product_service_period_history AS history
JOIN public.product_v1_editable_projections AS projection ON projection.product_id=history.product_id
JOIN public.products AS item ON item.id=history.product_id`).Scan(&total, &pending, &unsafe)
	if err != nil {
		return HistoricalServicePeriodProjectionResult{}, err
	}
	if total < 1 || unsafe != 0 {
		return HistoricalServicePeriodProjectionResult{}, product.ErrHistoricalStaticProductConflict
	}
	result := HistoricalServicePeriodProjectionResult{ProductsReplayed: total - pending}
	if pending == 0 {
		return result, nil
	}
	command, err := tx.Exec(ctx, `
UPDATE public.products AS item
SET legacy_admin_projection=jsonb_set(
      jsonb_set(item.legacy_admin_projection,'{status}',to_jsonb(CASE
        WHEN history.deleted THEN 'service_period_archived'
        WHEN item.legacy_admin_projection->>'enabled'='true' THEN 'service_period_enabled'
        ELSE 'service_period_disabled'
      END::text),true),
      '{enabled}',to_jsonb(CASE WHEN history.deleted THEN false ELSE (item.legacy_admin_projection->>'enabled')::boolean END),true),
    local_lifecycle=CASE WHEN history.deleted THEN 'disabled' ELSE item.local_lifecycle END
FROM public.product_service_period_history AS history
JOIN public.product_v1_editable_projections AS projection ON projection.product_id=history.product_id
WHERE item.id=history.product_id AND projection.service_period_projected_at IS NULL`)
	if err != nil || command.RowsAffected() != int64(pending) {
		if err != nil {
			return HistoricalServicePeriodProjectionResult{}, err
		}
		return HistoricalServicePeriodProjectionResult{}, product.ErrHistoricalStaticProductConflict
	}
	command, err = tx.Exec(ctx, `
UPDATE public.product_v1_editable_projections AS projection
SET service_period_definition_id=history.id, service_period_projected_at=$1
FROM public.product_service_period_history AS history
WHERE history.product_id=projection.product_id AND projection.service_period_projected_at IS NULL`, at)
	if err != nil || command.RowsAffected() != int64(pending) {
		if err != nil {
			return HistoricalServicePeriodProjectionResult{}, err
		}
		return HistoricalServicePeriodProjectionResult{}, product.ErrHistoricalStaticProductConflict
	}
	result.ProductsProjected = pending
	return result, nil
}

func (store *HistoricalStaticProductStore) ProjectHistoricalEditableProductImages(ctx context.Context, archiveRunID, staticImportVersion, staticTailImportVersion string, at time.Time) (HistoricalProductImagesProjectionResult, error) {
	if store == nil || store.tx == nil || ctx == nil || strings.TrimSpace(archiveRunID) == "" || strings.TrimSpace(staticImportVersion) == "" || strings.TrimSpace(staticTailImportVersion) == "" || at.IsZero() || at.Location() != time.UTC {
		return HistoricalProductImagesProjectionResult{}, product.ErrHistoricalStaticProductInvalid
	}
	tx, err := store.tx(ctx)
	if err != nil {
		return HistoricalProductImagesProjectionResult{}, err
	}
	if _, err = tx.Exec(ctx, `LOCK TABLE public.product_v1_editable_projections IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return HistoricalProductImagesProjectionResult{}, err
	}
	var missingMappings int
	err = tx.QueryRow(ctx, `
WITH product_map AS (
  SELECT (metadata->>'source_id')::bigint AS source_id, target_id::bigint AS product_id
  FROM public.v1_domain_import_receipts
  WHERE archive_run_id=$1 AND import_version=$2 AND table_id='public/wechat_pay_products'
    AND target_table='products' AND disposition='import' AND verified
), image_map AS (
  SELECT (metadata->>'source_id')::bigint AS source_id, target_id::bigint AS image_id
  FROM public.v1_domain_import_receipts
  WHERE archive_run_id=$1 AND import_version=$2 AND table_id='public/image_library'
    AND target_table='media_images' AND disposition='import' AND verified
), eligible AS (
  SELECT slice.id, product_map.product_id, image_map.image_id
  FROM public.product_v1_page_slice_history AS slice
  LEFT JOIN product_map ON product_map.source_id=slice.product_source_id
  LEFT JOIN image_map ON image_map.source_id=slice.image_source_id
  LEFT JOIN public.v1_domain_import_receipts AS receipt
    ON receipt.archive_run_id=$1 AND receipt.import_version=$3
   AND receipt.table_id='public/wechat_pay_product_page_slices'
   AND receipt.target_table='product_v1_page_slice_history'
   AND receipt.target_id=slice.id::text AND receipt.disposition='import' AND receipt.verified
  WHERE slice.original_enabled
    AND (product_map.product_id IS NULL OR image_map.image_id IS NULL OR receipt.target_id IS NULL)
)
SELECT count(*) FROM eligible`, archiveRunID, staticImportVersion, staticTailImportVersion).Scan(&missingMappings)
	if err != nil {
		return HistoricalProductImagesProjectionResult{}, err
	}
	if missingMappings != 0 {
		return HistoricalProductImagesProjectionResult{}, product.ErrHistoricalStaticProductConflict
	}
	var total, pending, unsafe int
	err = tx.QueryRow(ctx, `
SELECT count(*),
       count(*) FILTER (WHERE projection.images_projected_at IS NULL),
       count(*) FILTER (WHERE projection.images_projected_at IS NULL AND (item.version<>1 OR EXISTS(SELECT 1 FROM public.product_images AS image WHERE image.product_id=item.id)))
FROM public.product_v1_editable_projections AS projection
JOIN public.products AS item ON item.id=projection.product_id`).Scan(&total, &pending, &unsafe)
	if err != nil {
		return HistoricalProductImagesProjectionResult{}, err
	}
	if total < 1 || unsafe != 0 {
		return HistoricalProductImagesProjectionResult{}, product.ErrHistoricalStaticProductConflict
	}
	result := HistoricalProductImagesProjectionResult{ProductsReplayed: total - pending}
	if pending == 0 {
		return result, nil
	}
	command, err := tx.Exec(ctx, `
WITH product_map AS (
  SELECT (metadata->>'source_id')::bigint AS source_id, target_id::bigint AS product_id
  FROM public.v1_domain_import_receipts
  WHERE archive_run_id=$1 AND import_version=$2 AND table_id='public/wechat_pay_products'
    AND target_table='products' AND disposition='import' AND verified
), image_map AS (
  SELECT (metadata->>'source_id')::bigint AS source_id, target_id::bigint AS image_id
  FROM public.v1_domain_import_receipts
  WHERE archive_run_id=$1 AND import_version=$2 AND table_id='public/image_library'
    AND target_table='media_images' AND disposition='import' AND verified
)
INSERT INTO public.product_images(product_id,position,image_url)
SELECT product_map.product_id,
       row_number() OVER (PARTITION BY product_map.product_id ORDER BY slice.sort_order,slice.source_id)::integer-1,
       '/api/admin/image-library/'||image_map.image_id::text||'/variants/original'
FROM public.product_v1_page_slice_history AS slice
JOIN product_map ON product_map.source_id=slice.product_source_id
JOIN image_map ON image_map.source_id=slice.image_source_id
JOIN public.product_v1_editable_projections AS projection ON projection.product_id=product_map.product_id AND projection.images_projected_at IS NULL
WHERE slice.original_enabled
ORDER BY product_map.product_id,slice.sort_order,slice.source_id`, archiveRunID, staticImportVersion)
	if err != nil {
		return HistoricalProductImagesProjectionResult{}, err
	}
	result.ImagesProjected = int(command.RowsAffected())
	command, err = tx.Exec(ctx, `
UPDATE public.products AS item
SET legacy_admin_projection=jsonb_set(
  item.legacy_admin_projection,
  '{slices}',
  COALESCE((SELECT jsonb_agg(jsonb_build_object('image_url',image.image_url) ORDER BY image.position) FROM public.product_images AS image WHERE image.product_id=item.id),'[]'::jsonb),
  true)
FROM public.product_v1_editable_projections AS projection
WHERE projection.product_id=item.id AND projection.images_projected_at IS NULL`)
	if err != nil || command.RowsAffected() != int64(pending) {
		if err != nil {
			return HistoricalProductImagesProjectionResult{}, err
		}
		return HistoricalProductImagesProjectionResult{}, product.ErrHistoricalStaticProductConflict
	}
	command, err = tx.Exec(ctx, `
UPDATE public.product_v1_editable_projections AS projection
SET images_projected_at=$1,
    image_count=(SELECT count(*)::integer FROM public.product_images AS image WHERE image.product_id=projection.product_id)
WHERE projection.images_projected_at IS NULL`, at)
	if err != nil || command.RowsAffected() != int64(pending) {
		if err != nil {
			return HistoricalProductImagesProjectionResult{}, err
		}
		return HistoricalProductImagesProjectionResult{}, product.ErrHistoricalStaticProductConflict
	}
	result.ProductsProjected = pending
	return result, nil
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
