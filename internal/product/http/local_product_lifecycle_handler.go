package http

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

// LocalProductLifecycleApplication is kept separate from the native Product
// update handler so the central route lane can wire only the four restored
// legacy operations without widening the existing generated contract.
type LocalProductLifecycleApplication interface {
	SetLocalProductEnabled(context.Context, productport.SetLocalProductEnabledCommand) (productport.LocalProduct, error)
	CopyLocalProduct(context.Context, productport.CopyLocalProductCommand) (productport.LocalProduct, error)
	DeleteLocalProduct(context.Context, productport.DeleteLocalProductCommand) (productport.DeleteLocalProductResult, error)
	ShareLocalProduct(context.Context, productport.ID) (productport.LocalProductShare, error)
}

type LocalProductLifecycleHandler struct {
	application LocalProductLifecycleApplication
}

func NewLocalProductLifecycleHandler(application LocalProductLifecycleApplication) (*LocalProductLifecycleHandler, error) {
	if nilInterface(application) {
		return nil, productapp.ErrUnavailable
	}
	return &LocalProductLifecycleHandler{application: application}, nil
}

type localProductLifecycleVersionRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

type LocalProductLifecycleProductResponse struct {
	ID            int64    `json:"id"`
	ProductCode   string   `json:"product_code"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	PriceMinor    int64    `json:"price_minor"`
	Currency      string   `json:"currency"`
	StockQuantity int32    `json:"stock_quantity"`
	Images        []string `json:"images"`
	CreatedBy     int64    `json:"created_by"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	Lifecycle     string   `json:"lifecycle"`
	Enabled       bool     `json:"enabled"`
	Version       int64    `json:"version"`
}

type LocalProductLifecycleDeleteResponse struct {
	OK        bool  `json:"ok"`
	Deleted   bool  `json:"deleted"`
	ProductID int64 `json:"product_id"`
}

type LocalProductLifecycleShareResponse struct {
	OK                       bool   `json:"ok"`
	ProductID                int64  `json:"product_id"`
	ProductCode              string `json:"product_code"`
	Lifecycle                string `json:"lifecycle"`
	PublicPath               string `json:"public_path"`
	LocalOnly                bool   `json:"local_only"`
	RealExternalCallExecuted bool   `json:"real_external_call_executed"`
}

func (handler *LocalProductLifecycleHandler) SetLocalProductEnabled(writer http.ResponseWriter, request *http.Request, productID int64, enabled bool) {
	principal, key, err := localLifecycleWriteOperation(request)
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	version, err := decodeLocalLifecycleVersion(writer, request)
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	if handler == nil || nilInterface(handler.application) {
		writeLocalError(writer, request, productapp.ErrUnavailable)
		return
	}
	product, err := handler.application.SetLocalProductEnabled(request.Context(), productport.SetLocalProductEnabledCommand{
		ID: productport.ID(productID), ExpectedVersion: version, Enabled: enabled, Actor: principal.AdminUserID, IdempotencyKey: key,
	})
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	response, err := localProductLifecycleProductResponse(product)
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	write(writer, http.StatusOK, response)
}

func (handler *LocalProductLifecycleHandler) CopyLocalProduct(writer http.ResponseWriter, request *http.Request, productID int64) {
	principal, key, err := localLifecycleWriteOperation(request)
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	version, err := decodeLocalLifecycleVersion(writer, request)
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	if handler == nil || nilInterface(handler.application) {
		writeLocalError(writer, request, productapp.ErrUnavailable)
		return
	}
	product, err := handler.application.CopyLocalProduct(request.Context(), productport.CopyLocalProductCommand{
		ID: productport.ID(productID), ExpectedVersion: version, Actor: principal.AdminUserID, IdempotencyKey: key,
	})
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	response, err := localProductLifecycleProductResponse(product)
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	write(writer, http.StatusCreated, response)
}

func (handler *LocalProductLifecycleHandler) DeleteLocalProduct(writer http.ResponseWriter, request *http.Request, productID int64) {
	principal, key, err := localLifecycleWriteOperation(request)
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	version, err := decodeLocalLifecycleVersion(writer, request)
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	if handler == nil || nilInterface(handler.application) {
		writeLocalError(writer, request, productapp.ErrUnavailable)
		return
	}
	result, err := handler.application.DeleteLocalProduct(request.Context(), productport.DeleteLocalProductCommand{
		ID: productport.ID(productID), ExpectedVersion: version, Actor: principal.AdminUserID, IdempotencyKey: key,
	})
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	if result.ProductID != productport.ID(productID) || !result.Deleted {
		writeLocalError(writer, request, productapp.ErrUnavailable)
		return
	}
	write(writer, http.StatusOK, LocalProductLifecycleDeleteResponse{OK: true, Deleted: true, ProductID: int64(result.ProductID)})
}

func (handler *LocalProductLifecycleHandler) ShareLocalProduct(writer http.ResponseWriter, request *http.Request, productID int64) {
	if handler == nil || nilInterface(handler.application) || request == nil {
		writeLocalError(writer, request, productapp.ErrUnavailable)
		return
	}
	if _, err := localLifecycleReadOperation(request); err != nil {
		writeLocalError(writer, request, err)
		return
	}
	if request.URL == nil || request.URL.RawQuery != "" || !localLifecycleEmptyBody(request) {
		writeLocalError(writer, request, productapp.ErrInvalidProduct)
		return
	}
	share, err := handler.application.ShareLocalProduct(request.Context(), productport.ID(productID))
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	response, err := localProductLifecycleShareResponse(share)
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	write(writer, http.StatusOK, response)
}

func localLifecycleEmptyBody(request *http.Request) bool {
	if request == nil || request.Body == nil || request.Body == http.NoBody {
		return true
	}
	var probe [1]byte
	read, err := request.Body.Read(probe[:])
	return read == 0 && err == io.EOF
}

func localLifecycleWriteOperation(request *http.Request) (authport.Principal, string, error) {
	if request == nil {
		return authport.Principal{}, "", platformhttp.NewError(platformhttp.CodeDependencyUnavailable, productapp.ErrUnavailable)
	}
	principal, err := localPrincipal(request, authport.CapabilityProductsWrite)
	if err != nil {
		return authport.Principal{}, "", err
	}
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || !validLocalIdempotencyKey(values[0]) {
		return authport.Principal{}, "", productapp.ErrInvalidProduct
	}
	return principal, values[0], nil
}

func localLifecycleReadOperation(request *http.Request) (authport.Principal, error) {
	if request == nil {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, productapp.ErrUnavailable)
	}
	return localPrincipal(request, authport.CapabilityProductsRead)
}

func decodeLocalLifecycleVersion(writer http.ResponseWriter, request *http.Request) (int64, error) {
	var body localProductLifecycleVersionRequest
	if err := decodeLocalBody(writer, request, &body); err != nil || body.ExpectedVersion < 1 {
		return 0, productapp.ErrInvalidProduct
	}
	return body.ExpectedVersion, nil
}

func localProductLifecycleProductResponse(product productport.LocalProduct) (LocalProductLifecycleProductResponse, error) {
	if product.ID < 1 || product.Version < 1 || product.CreatedBy < 1 || product.ProductCode == "" || strings.TrimSpace(product.ProductCode) != product.ProductCode || len(product.ProductCode) > 200 ||
		product.Name == "" || strings.TrimSpace(product.Name) != product.Name || len(product.Name) > 200 || strings.TrimSpace(product.Description) != product.Description || len(product.Description) > 10000 ||
		product.CreatedAt.IsZero() || product.UpdatedAt.IsZero() || product.UpdatedAt.Before(product.CreatedAt) ||
		product.PriceMinor < 0 || product.StockQuantity < 0 || len(product.Currency) != 3 || !validCurrency(product.Currency) {
		return LocalProductLifecycleProductResponse{}, productapp.ErrUnavailable
	}
	if product.Lifecycle != productport.LocalProductDraft && product.Lifecycle != productport.LocalProductEnabled && product.Lifecycle != productport.LocalProductDisabled {
		return LocalProductLifecycleProductResponse{}, productapp.ErrUnavailable
	}
	if (product.Lifecycle == productport.LocalProductEnabled) != product.Enabled {
		return LocalProductLifecycleProductResponse{}, productapp.ErrUnavailable
	}
	if len(product.Images) > 20 {
		return LocalProductLifecycleProductResponse{}, productapp.ErrUnavailable
	}
	for _, image := range product.Images {
		if image == "" || strings.TrimSpace(image) != image || len(image) > 2048 {
			return LocalProductLifecycleProductResponse{}, productapp.ErrUnavailable
		}
	}
	return LocalProductLifecycleProductResponse{
		ID: int64(product.ID), ProductCode: product.ProductCode, Name: product.Name, Description: product.Description,
		PriceMinor: product.PriceMinor, Currency: product.Currency, StockQuantity: product.StockQuantity,
		Images: append([]string(nil), product.Images...), CreatedBy: product.CreatedBy,
		CreatedAt: product.CreatedAt.UTC().Format(timeRFC3339Nano), UpdatedAt: product.UpdatedAt.UTC().Format(timeRFC3339Nano),
		Lifecycle: string(product.Lifecycle), Enabled: product.Enabled, Version: product.Version,
	}, nil
}

func localProductLifecycleShareResponse(share productport.LocalProductShare) (LocalProductLifecycleShareResponse, error) {
	if share.ProductID < 1 || share.ProductCode == "" || strings.TrimSpace(share.ProductCode) != share.ProductCode {
		return LocalProductLifecycleShareResponse{}, productapp.ErrUnavailable
	}
	if share.Lifecycle != productport.LocalProductEnabled {
		return LocalProductLifecycleShareResponse{}, productapp.ErrUnavailable
	}
	return LocalProductLifecycleShareResponse{
		OK: true, ProductID: int64(share.ProductID), ProductCode: share.ProductCode, Lifecycle: string(share.Lifecycle),
		PublicPath: "/p/ordinary/" + strconv.FormatInt(int64(share.ProductID), 10), LocalOnly: true, RealExternalCallExecuted: false,
	}, nil
}
