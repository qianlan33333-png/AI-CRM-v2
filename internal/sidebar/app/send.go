package app

import (
	"context"
	"time"
)

type ShareableProductKind string

const (
	ShareableProductOrdinary      ShareableProductKind = "ordinary"
	ShareableProductServicePeriod ShareableProductKind = "service_period"
)

// ShareableProduct is the deliberately small, read-only projection used by
// the sidebar and its public detail page. It contains no purchase, payment,
// entitlement, or provider fields.
type ShareableProduct struct {
	Kind          ShareableProductKind
	ProductID     int64
	ProductCode   string
	Name          string
	Description   string
	PriceMinor    int64
	Currency      string
	StockQuantity int32
}

type ShareableProductCatalog interface {
	ListShareableProducts(context.Context, int32) ([]ShareableProduct, error)
	GetShareableProduct(context.Context, ShareableProductKind, int64) (ShareableProduct, error)
}

type TemporaryMediaResult struct {
	ImageID                int64
	MediaID                string
	MediaExpiresAt         time.Time
	UploadState            string
	ProviderCallDispatched bool
}

type TemporaryMediaPreparer interface {
	PrepareTemporaryImageMedia(context.Context, int64, int64, int64, string) (TemporaryMediaResult, error)
}

type ServiceOptions struct {
	Products ShareableProductCatalog
	Media    TemporaryMediaPreparer
}

type ShareableProductResult struct {
	Items  []ShareableProduct
	Safety Safety
}

func (service *Service) ShareableProducts(ctx context.Context, scope Scope, limit int32) (ShareableProductResult, error) {
	if service == nil || service.products == nil || scope.CustomerID < 1 || scope.Principal.AdminUserID < 1 || limit < 1 || limit > 100 {
		return ShareableProductResult{}, ErrUnavailable
	}
	items, err := service.products.ListShareableProducts(ctx, limit)
	if err != nil {
		return ShareableProductResult{}, mapDependencyError(err)
	}
	for _, item := range items {
		if !validShareableProduct(item) {
			return ShareableProductResult{}, ErrUnavailable
		}
	}
	return ShareableProductResult{Items: append([]ShareableProduct(nil), items...), Safety: localSafety()}, nil
}

func (service *Service) PublicProduct(ctx context.Context, kind ShareableProductKind, productID int64) (ShareableProduct, error) {
	if service == nil || service.products == nil || productID < 1 || (kind != ShareableProductOrdinary && kind != ShareableProductServicePeriod) {
		return ShareableProduct{}, ErrNotFound
	}
	product, err := service.products.GetShareableProduct(ctx, kind, productID)
	if err != nil {
		return ShareableProduct{}, mapDependencyError(err)
	}
	if product.Kind != kind || product.ProductID != productID || !validShareableProduct(product) {
		return ShareableProduct{}, ErrUnavailable
	}
	return product, nil
}

func (service *Service) PrepareTemporaryImageMedia(ctx context.Context, scope Scope, imageID int64, idempotencyKey string) (TemporaryMediaResult, error) {
	if service == nil || service.temporaryMedia == nil || scope.CustomerID < 1 || scope.Principal.AdminUserID < 1 || imageID < 1 || idempotencyKey == "" {
		return TemporaryMediaResult{}, ErrUnavailable
	}
	result, err := service.temporaryMedia.PrepareTemporaryImageMedia(ctx, imageID, scope.Principal.AdminUserID, scope.CustomerID, idempotencyKey)
	if err != nil {
		return TemporaryMediaResult{}, mapDependencyError(err)
	}
	if result.ImageID != imageID || (result.UploadState != "ready" && result.UploadState != "outcome_unknown" && result.UploadState != "final_failed") ||
		(result.UploadState == "ready" && (result.MediaID == "" || result.MediaExpiresAt.IsZero() || !result.ProviderCallDispatched)) ||
		(result.UploadState != "ready" && (result.MediaID != "" || !result.MediaExpiresAt.IsZero())) {
		return TemporaryMediaResult{}, ErrUnavailable
	}
	return result, nil
}

func validShareableProduct(item ShareableProduct) bool {
	return (item.Kind == ShareableProductOrdinary || item.Kind == ShareableProductServicePeriod) && item.ProductID > 0 && item.ProductCode != "" && item.Name != "" &&
		item.PriceMinor >= 0 && item.StockQuantity >= 0 && len(item.ProductCode) <= 200 && len(item.Name) <= 200 && len(item.Description) <= 10_000 && len(item.Currency) == 3
}
