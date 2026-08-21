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

	items := make([]productport.Product, 0, limit)
	var total int64
	var after *productport.ID
	for {
		rows, err := repository.List(ctx, after, productapp.MaximumLimit)
		if err != nil {
			return nil, 0, err
		}
		if len(rows) == 0 {
			break
		}
		for index := range rows {
			row := rows[index]
			if !servicePeriodProjection(row.LegacyAdminProjection) {
				continue
			}
			if total >= int64(offset) && len(items) < int(limit) {
				items = append(items, row)
			}
			total++
		}
		last := rows[len(rows)-1].ID
		if last < 1 || after != nil && last <= *after {
			return nil, 0, productapp.ErrUnavailable
		}
		after = &last
		if len(rows) < int(productapp.MaximumLimit) {
			break
		}
	}
	return items, total, nil
}

func (repository *CatalogRepository) GetServicePeriodProduct(ctx context.Context, id productport.ID) (productport.Product, error) {
	if repository == nil || id < 1 {
		return productport.Product{}, productapp.ErrNotFound
	}
	product, err := repository.Get(ctx, id)
	if err != nil {
		return productport.Product{}, err
	}
	if !servicePeriodProjection(product.LegacyAdminProjection) {
		return productport.Product{}, productapp.ErrNotFound
	}
	return product, nil
}

func (repository *CatalogRepository) GetServicePeriodProductForUpdate(ctx context.Context, id productport.ID) (productport.Product, error) {
	if repository == nil || id < 1 {
		return productport.Product{}, productapp.ErrNotFound
	}
	product, err := repository.GetForUpdate(ctx, id)
	if err != nil {
		return productport.Product{}, err
	}
	if !servicePeriodProjection(product.LegacyAdminProjection) {
		return productport.Product{}, productapp.ErrNotFound
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
	updated, err := repository.Get(ctx, command.ID)
	if err != nil {
		return productport.Product{}, err
	}
	if updated.Version != command.ExpectedVersion+1 || !jsonEqual(updated.LegacyAdminProjection, command.LegacyAdminProjection) {
		return productport.Product{}, productapp.ErrUnavailable
	}
	return updated, nil
}

func servicePeriodProjection(raw json.RawMessage) bool {
	canonical, err := productapp.CanonicalLegacyAdminProjection(raw)
	if err != nil || !jsonEqual(canonical, raw) {
		return false
	}
	var projection struct {
		Status  string `json:"status"`
		Enabled bool   `json:"enabled"`
	}
	if json.Unmarshal(canonical, &projection) != nil {
		return false
	}
	switch projection.Status {
	case productapp.ServicePeriodProjectionDraftStatus,
		productapp.ServicePeriodProjectionDisabledStatus,
		productapp.ServicePeriodProjectionArchivedStatus:
		return !projection.Enabled
	case productapp.ServicePeriodProjectionEnabledStatus:
		return projection.Enabled
	default:
		return false
	}
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
