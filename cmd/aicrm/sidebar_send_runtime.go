package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	outboundprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/provider"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
	sidebarstore "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/store"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

var errInvalidSidebarTemporaryMediaConfig = errors.New("invalid sidebar temporary-media configuration")

type sidebarShareableProductCatalog struct {
	ordinary *productapp.Service
	period   *productapp.ServicePeriodService
}

func (catalog sidebarShareableProductCatalog) ListShareableProducts(ctx context.Context, limit int32) ([]sidebarapp.ShareableProduct, error) {
	if catalog.ordinary == nil || catalog.period == nil || ctx == nil || limit < 1 || limit > productapp.MaximumLimit {
		return nil, sidebarapp.ErrUnavailable
	}
	ordinary, err := catalog.listOrdinary(ctx, limit)
	if err != nil {
		return nil, err
	}
	period, err := catalog.listPeriod(ctx, limit)
	if err != nil {
		return nil, err
	}
	items := make([]sidebarapp.ShareableProduct, 0, limit)
	for index := 0; len(items) < int(limit) && (index < len(ordinary) || index < len(period)); index++ {
		if index < len(ordinary) {
			items = append(items, ordinary[index])
		}
		if index < len(period) && len(items) < int(limit) {
			items = append(items, period[index])
		}
	}
	return items, nil
}

func (catalog sidebarShareableProductCatalog) GetShareableProduct(ctx context.Context, kind sidebarapp.ShareableProductKind, productID int64) (sidebarapp.ShareableProduct, error) {
	if catalog.ordinary == nil || catalog.period == nil || ctx == nil || productID < 1 {
		return sidebarapp.ShareableProduct{}, sidebarapp.ErrNotFound
	}
	switch kind {
	case sidebarapp.ShareableProductOrdinary:
		product, err := catalog.ordinary.Get(ctx, productport.ID(productID))
		if err != nil {
			return sidebarapp.ShareableProduct{}, mapSidebarProductError(err)
		}
		local, err := productapp.ProjectLocalProduct(product)
		if err != nil || !local.Enabled {
			return sidebarapp.ShareableProduct{}, sidebarapp.ErrNotFound
		}
		return sidebarOrdinaryProduct(local), nil
	case sidebarapp.ShareableProductServicePeriod:
		product, err := catalog.period.GetServicePeriodProduct(ctx, productport.ID(productID))
		if err != nil {
			return sidebarapp.ShareableProduct{}, mapSidebarProductError(err)
		}
		if !product.Enabled || product.Archived {
			return sidebarapp.ShareableProduct{}, sidebarapp.ErrNotFound
		}
		return sidebarServicePeriodProduct(product), nil
	default:
		return sidebarapp.ShareableProduct{}, sidebarapp.ErrNotFound
	}
}

func (catalog sidebarShareableProductCatalog) listOrdinary(ctx context.Context, limit int32) ([]sidebarapp.ShareableProduct, error) {
	items := make([]sidebarapp.ShareableProduct, 0, limit)
	for cursor := ""; len(items) < int(limit); {
		page, err := catalog.ordinary.List(ctx, cursor, productapp.MaximumLimit)
		if err != nil {
			return nil, mapSidebarProductError(err)
		}
		for _, product := range page.Items {
			local, projectErr := productapp.ProjectLocalProduct(product)
			if projectErr == nil && local.Enabled {
				items = append(items, sidebarOrdinaryProduct(local))
				if len(items) == int(limit) {
					return items, nil
				}
			}
		}
		if page.NextCursor == "" {
			return items, nil
		}
		cursor = page.NextCursor
	}
	return items, nil
}

func (catalog sidebarShareableProductCatalog) listPeriod(ctx context.Context, limit int32) ([]sidebarapp.ShareableProduct, error) {
	items := make([]sidebarapp.ShareableProduct, 0, limit)
	for offset := int32(0); len(items) < int(limit); {
		page, err := catalog.period.ListServicePeriodProducts(ctx, productapp.MaximumLimit, offset)
		if err != nil {
			return nil, mapSidebarProductError(err)
		}
		for _, product := range page.Items {
			if product.Enabled && !product.Archived {
				items = append(items, sidebarServicePeriodProduct(product))
				if len(items) == int(limit) {
					return items, nil
				}
			}
		}
		if int64(offset)+int64(len(page.Items)) >= page.Total {
			return items, nil
		}
		offset += page.Limit
	}
	return items, nil
}

func sidebarOrdinaryProduct(product productport.LocalProduct) sidebarapp.ShareableProduct {
	return sidebarapp.ShareableProduct{
		Kind: sidebarapp.ShareableProductOrdinary, ProductID: int64(product.ID), ProductCode: product.ProductCode,
		Name: product.Name, Description: product.Description, PriceMinor: product.PriceMinor, Currency: product.Currency, StockQuantity: product.StockQuantity,
	}
}

func sidebarServicePeriodProduct(product productport.ServicePeriodProduct) sidebarapp.ShareableProduct {
	return sidebarapp.ShareableProduct{
		Kind: sidebarapp.ShareableProductServicePeriod, ProductID: int64(product.ServiceProductID), ProductCode: product.ProductCode,
		Name: product.Name, Description: product.Description, PriceMinor: product.PriceMinor, Currency: product.Currency, StockQuantity: product.StockQuantity,
	}
}

func mapSidebarProductError(err error) error {
	if errors.Is(err, productapp.ErrNotFound) {
		return sidebarapp.ErrNotFound
	}
	return sidebarapp.ErrUnavailable
}

type sidebarImageSource struct{ service *mediaapp.Service }

func (source sidebarImageSource) ReadSidebarImage(ctx context.Context, imageID int64) (sidebarapp.SidebarImageSource, error) {
	if source.service == nil {
		return sidebarapp.SidebarImageSource{}, sidebarapp.ErrUnavailable
	}
	image, err := source.service.GetImageDetail(ctx, imageID)
	if errors.Is(err, mediaapp.ErrImageDetailNotFound) {
		return sidebarapp.SidebarImageSource{}, sidebarapp.ErrNotFound
	}
	if err != nil {
		return sidebarapp.SidebarImageSource{}, sidebarapp.ErrUnavailable
	}
	return sidebarapp.SidebarImageSource{ImageID: image.ID, Enabled: image.Enabled, Filename: image.FileName, MIME: image.MimeType, Content: image.Content}, nil
}

type sidebarTemporaryMediaUploader struct {
	uploader *outboundprovider.TemporaryMediaUploader
}

func (uploader sidebarTemporaryMediaUploader) UploadSidebarImageTemporaryMedia(ctx context.Context, input sidebarapp.SidebarTemporaryMediaUpload) (sidebarapp.SidebarTemporaryMediaUploadResult, error) {
	if uploader.uploader == nil {
		return sidebarapp.SidebarTemporaryMediaUploadResult{}, sidebarapp.ErrUnavailable
	}
	result, err := uploader.uploader.Upload(ctx, outboundprovider.TemporaryMediaUpload{
		Kind: "image", Filename: input.Filename, MIME: input.MIME, Bytes: input.Bytes, Checksum: input.Checksum,
	})
	return sidebarapp.SidebarTemporaryMediaUploadResult{
		MediaID: result.MediaID, ExpiresAt: result.ExpiresAt, ProviderCallDispatched: result.BusinessCallDispatched,
		OutcomeUnknown: result.OutcomeUnknown, FinalFailed: result.FinalFailed,
	}, err
}

func newSidebarTemporaryMediaPreparer(pool *pgxpool.Pool, mediaService *mediaapp.Service, config appconfig.WeComOutbound) (sidebarapp.TemporaryMediaPreparer, error) {
	if !config.Enabled || !config.PermissionConfirmed || pool == nil || mediaService == nil {
		return nil, errInvalidSidebarTemporaryMediaConfig
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	tokens, err := newSidebarTemporaryMediaTokens(config, httpClient, time.Now)
	if err != nil {
		return nil, err
	}
	uploader, err := outboundprovider.NewTemporaryMediaUploader(wecomclient.ProductionBaseURL, httpClient, sidebarTemporaryMediaTokenAdapter{provider: tokens}, time.Now)
	if err != nil {
		return nil, errors.Join(errInvalidSidebarTemporaryMediaConfig, err)
	}
	receipts, err := sidebarstore.NewMaterialSendReceiptStore(pool)
	if err != nil {
		return nil, err
	}
	return sidebarapp.NewTemporaryMediaService(receipts, sidebarImageSource{service: mediaService}, sidebarTemporaryMediaUploader{uploader: uploader})
}

func newSidebarTemporaryMediaTokens(config appconfig.WeComOutbound, httpClient *http.Client, now func() time.Time) (*wecomclient.CachingTokenProvider, error) {
	if !config.Enabled || !config.PermissionConfirmed || httpClient == nil || now == nil {
		return nil, errInvalidSidebarTemporaryMediaConfig
	}
	credentials, err := wecomclient.NewCredentials(config.CorpID, config.Secret.Value())
	if err != nil {
		return nil, errors.Join(errInvalidSidebarTemporaryMediaConfig, err)
	}
	tokens, err := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{
		BaseURL: wecomclient.ProductionBaseURL, Credentials: credentials, HTTPClient: httpClient, Now: now,
	})
	if err != nil {
		return nil, errors.Join(errInvalidSidebarTemporaryMediaConfig, err)
	}
	return tokens, nil
}

type sidebarTemporaryMediaTokenAdapter struct {
	provider *wecomclient.CachingTokenProvider
}

func (adapter sidebarTemporaryMediaTokenAdapter) Token(ctx context.Context) (string, error) {
	token, err := adapter.provider.Token(ctx)
	return token.Value(), err
}
