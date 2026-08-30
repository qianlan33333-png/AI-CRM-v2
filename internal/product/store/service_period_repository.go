package store

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"time"

	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

var _ productapp.ServicePeriodStore = (*CatalogRepository)(nil)

// servicePeriodProjectionUpdater is intentionally structural. The checked-in
// SQL source defines this sqlc method, while the generated file remains owned
// by the central generation step and is not hand-edited in this lane.
type servicePeriodProjectionUpdater interface {
	UpdateServicePeriodProductProjection(context.Context, []byte) (int64, error)
}

func (repository *CatalogRepository) ListServicePeriodProducts(ctx context.Context, limit, offset int32) ([]productport.Product, int64, error) {
	if repository == nil || limit < 1 || limit > productapp.MaximumLimit || offset < 0 || offset > productapp.MaximumLegacyOffset {
		return nil, 0, productapp.ErrInvalidCursor
	}

	querySet, err := queries(ctx)
	if err != nil {
		return nil, 0, unavailable(err)
	}
	rows, err := querySet.ListServicePeriodProductRows(ctx, productdb.ListServicePeriodProductRowsParams{RowLimit: limit, RowOffset: offset})
	if err != nil {
		return nil, 0, unavailable(err)
	}
	total, err := querySet.CountServicePeriodProductRows(ctx)
	if err != nil || total < 0 {
		return nil, 0, unavailable(err)
	}
	items := make([]productport.Product, len(rows))
	for index, row := range rows {
		items[index], err = mapRow(row.ID, row.ProductCode, row.Name, row.Description, row.PriceMinor, row.Currency, row.StockQuantity, row.CreatedBy, row.CreatedAt, row.UpdatedAt, row.Version, row.LocalLifecycle, row.LegacyAdminProjection, row.Images)
		if err != nil || !servicePeriodProjection(items[index].LegacyAdminProjection) {
			return nil, 0, productapp.ErrUnavailable
		}
	}
	return items, total, nil
}

func (repository *CatalogRepository) GetServicePeriodProduct(ctx context.Context, id productport.ID) (productport.Product, error) {
	if repository == nil || id < 1 {
		return productport.Product{}, productapp.ErrNotFound
	}
	querySet, err := queries(ctx)
	if err != nil {
		return productport.Product{}, unavailable(err)
	}
	row, err := querySet.GetServicePeriodProductRow(ctx, int64(id))
	if err != nil {
		return productport.Product{}, unavailable(err)
	}
	product, err := mapRow(row.ID, row.ProductCode, row.Name, row.Description, row.PriceMinor, row.Currency, row.StockQuantity, row.CreatedBy, row.CreatedAt, row.UpdatedAt, row.Version, row.LocalLifecycle, row.LegacyAdminProjection, row.Images)
	if err != nil || !servicePeriodProjection(product.LegacyAdminProjection) {
		return productport.Product{}, productapp.ErrUnavailable
	}
	return product, nil
}

func (repository *CatalogRepository) GetServicePeriodProductForUpdate(ctx context.Context, id productport.ID) (productport.Product, error) {
	if repository == nil || id < 1 {
		return productport.Product{}, productapp.ErrNotFound
	}
	querySet, err := queries(ctx)
	if err != nil {
		return productport.Product{}, unavailable(err)
	}
	row, err := querySet.GetServicePeriodProductRowForUpdate(ctx, int64(id))
	if err != nil {
		return productport.Product{}, unavailable(err)
	}
	product, err := mapRow(row.ID, row.ProductCode, row.Name, row.Description, row.PriceMinor, row.Currency, row.StockQuantity, row.CreatedBy, row.CreatedAt, row.UpdatedAt, row.Version, row.LocalLifecycle, row.LegacyAdminProjection, row.Images)
	if err != nil || !servicePeriodProjection(product.LegacyAdminProjection) {
		return productport.Product{}, productapp.ErrUnavailable
	}
	return product, nil
}

func (repository *CatalogRepository) UpdateServicePeriodProduct(ctx context.Context, command productapp.ServicePeriodStoreUpdate, now time.Time) (productport.Product, error) {
	if repository == nil || command.ID < 1 || command.ExpectedVersion < 1 || now.IsZero() ||
		command.Name == "" || command.Currency == "" || !json.Valid(command.LegacyAdminProjection) || !servicePeriodProjection(command.LegacyAdminProjection) {
		return productport.Product{}, productapp.ErrUnavailable
	}
	querySet, err := queries(ctx)
	if err != nil {
		return productport.Product{}, unavailable(err)
	}
	updater, ok := any(querySet).(servicePeriodProjectionUpdater)
	if !ok {
		// The query source is present, but this exact-ref lane intentionally does
		// not modify sqlc output. Central generation must materialize the method.
		return productport.Product{}, productapp.ErrUnavailable
	}
	payload, err := json.Marshal(struct {
		ProductID             int64           `json:"product_id"`
		ExpectedVersion       int64           `json:"expected_version"`
		Name                  string          `json:"name"`
		Description           string          `json:"description"`
		PriceMinor            int64           `json:"price_minor"`
		Currency              string          `json:"currency"`
		StockQuantity         int32           `json:"stock_quantity"`
		LegacyAdminProjection json.RawMessage `json:"legacy_admin_projection"`
		UpdatedAt             time.Time       `json:"updated_at"`
	}{
		ProductID:             int64(command.ID),
		ExpectedVersion:       command.ExpectedVersion,
		Name:                  command.Name,
		Description:           command.Description,
		PriceMinor:            command.PriceMinor,
		Currency:              command.Currency,
		StockQuantity:         command.StockQuantity,
		LegacyAdminProjection: command.LegacyAdminProjection,
		UpdatedAt:             now,
	})
	if err != nil {
		return productport.Product{}, productapp.ErrUnavailable
	}
	affected, err := updater.UpdateServicePeriodProductProjection(ctx, payload)
	if err != nil {
		return productport.Product{}, unavailable(err)
	}
	if affected != 1 {
		return productport.Product{}, productapp.ErrConflict
	}
	if err = querySet.DeleteProductImages(ctx, int64(command.ID)); err != nil {
		return productport.Product{}, unavailable(err)
	}
	for position, imageURL := range command.Images {
		if err = querySet.InsertProductImage(ctx, productdb.InsertProductImageParams{ProductID: int64(command.ID), Position: int32(position), ImageUrl: imageURL}); err != nil {
			return productport.Product{}, unavailable(err)
		}
	}
	updated, err := repository.GetServicePeriodProduct(ctx, command.ID)
	if err != nil {
		return productport.Product{}, err
	}
	if updated.Version != command.ExpectedVersion+1 || !jsonEqual(updated.LegacyAdminProjection, command.LegacyAdminProjection) {
		return productport.Product{}, productapp.ErrUnavailable
	}
	return updated, nil
}

func servicePeriodProjection(raw json.RawMessage) bool {
	return productapp.IsServicePeriodProjection(raw)
}

func jsonEqual(left, right []byte) bool {
	leftValue, leftOK := decodeJSON(left)
	rightValue, rightOK := decodeJSON(right)
	return leftOK && rightOK && reflect.DeepEqual(leftValue, rightValue)
}

func decodeJSON(raw []byte) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, false
	}
	return value, decoder.Decode(&struct{}{}) == io.EOF
}
