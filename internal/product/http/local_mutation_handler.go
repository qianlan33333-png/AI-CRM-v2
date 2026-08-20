package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

const localProductBodyLimit = 64 << 10

// LocalMutationApplication is deliberately independent of generated OpenAPI
// types. Lane E owns the generated transport adapter and route registration.
type LocalMutationApplication interface {
	Update(context.Context, productport.UpdateCommand) (productport.Product, error)
}

type LocalEntitlementApplication interface {
	List(context.Context, productport.ID, int32) ([]productport.LocalEntitlement, error)
	Get(context.Context, productport.EntitlementID) (productport.LocalEntitlement, error)
	Grant(context.Context, productport.GrantLocalEntitlementCommand) (productport.LocalEntitlement, error)
	Revoke(context.Context, productport.RevokeLocalEntitlementCommand) (productport.LocalEntitlement, error)
}

type LocalMutationHandler struct {
	products     LocalMutationApplication
	entitlements LocalEntitlementApplication
}

func NewLocalMutationHandler(products LocalMutationApplication, entitlements LocalEntitlementApplication) (*LocalMutationHandler, error) {
	if nilInterface(products) || nilInterface(entitlements) {
		return nil, productapp.ErrUnavailable
	}
	return &LocalMutationHandler{products: products, entitlements: entitlements}, nil
}

type UpdateProductRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	PriceMinor      int64  `json:"price_minor"`
	Currency        string `json:"currency"`
	StockQuantity   int32  `json:"stock_quantity"`
}

// ProductResponse is the closed browser DTO for the native product mutation.
// It intentionally excludes the legacy compatibility projection.
type ProductResponse struct {
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
	Version       int64    `json:"version"`
}

type GrantProductLocalEntitlementRequest struct {
	OrderID int64 `json:"order_id"`
}

type RevokeProductLocalEntitlementRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

// LocalEntitlementResponse is intentionally exact: actor receipt fields never
// cross the browser boundary.
type LocalEntitlementResponse struct {
	ID        int64   `json:"id"`
	ProductID int64   `json:"product_id"`
	OrderID   int64   `json:"order_id"`
	State     string  `json:"state"`
	Version   int64   `json:"version"`
	GrantedAt string  `json:"granted_at"`
	RevokedAt *string `json:"revoked_at"`
}

type ListProductLocalEntitlementsResponse struct {
	Items []LocalEntitlementResponse `json:"items"`
}

func (h *LocalMutationHandler) UpdateProduct(w http.ResponseWriter, r *http.Request, productID int64) {
	principal, key, err := h.writeOperation(r, authport.CapabilityProductsWrite)
	if err != nil {
		writeLocalError(w, r, err)
		return
	}
	var body UpdateProductRequest
	if err = decodeLocalBody(w, r, &body); err != nil {
		writeLocalError(w, r, err)
		return
	}
	product, err := h.products.Update(r.Context(), productport.UpdateCommand{
		ID: productport.ID(productID), ExpectedVersion: body.ExpectedVersion,
		Name: body.Name, Description: body.Description, PriceMinor: body.PriceMinor,
		Currency: body.Currency, StockQuantity: body.StockQuantity,
		Actor: principal.AdminUserID, IdempotencyKey: key,
	})
	if err != nil {
		writeLocalError(w, r, err)
		return
	}
	response, err := localProductResponse(product)
	if err != nil {
		writeLocalError(w, r, err)
		return
	}
	write(w, http.StatusOK, response)
}

func (h *LocalMutationHandler) ListProductLocalEntitlements(w http.ResponseWriter, r *http.Request, productID int64, limit int32) {
	if err := h.readOperation(r, authport.CapabilityEntitlementsRead); err != nil {
		writeLocalError(w, r, err)
		return
	}
	if limit == 0 {
		limit = productapp.DefaultLimit
	}
	items, err := h.entitlements.List(r.Context(), productport.ID(productID), limit)
	if err != nil {
		writeLocalError(w, r, err)
		return
	}
	response := ListProductLocalEntitlementsResponse{Items: make([]LocalEntitlementResponse, len(items))}
	for i, item := range items {
		response.Items[i], err = localEntitlementResponse(item)
		if err != nil {
			writeLocalError(w, r, err)
			return
		}
	}
	write(w, http.StatusOK, response)
}

func (h *LocalMutationHandler) GetProductLocalEntitlement(w http.ResponseWriter, r *http.Request, entitlementID int64) {
	if err := h.readOperation(r, authport.CapabilityEntitlementsRead); err != nil {
		writeLocalError(w, r, err)
		return
	}
	item, err := h.entitlements.Get(r.Context(), productport.EntitlementID(entitlementID))
	if err != nil {
		writeLocalError(w, r, err)
		return
	}
	response, err := localEntitlementResponse(item)
	if err != nil {
		writeLocalError(w, r, err)
		return
	}
	write(w, http.StatusOK, response)
}

func (h *LocalMutationHandler) GrantProductLocalEntitlement(w http.ResponseWriter, r *http.Request, productID int64) {
	principal, key, err := h.writeOperation(r, authport.CapabilityEntitlementsWrite)
	if err != nil {
		writeLocalError(w, r, err)
		return
	}
	var body GrantProductLocalEntitlementRequest
	if err = decodeLocalBody(w, r, &body); err != nil {
		writeLocalError(w, r, err)
		return
	}
	item, err := h.entitlements.Grant(r.Context(), productport.GrantLocalEntitlementCommand{ProductID: productport.ID(productID), OrderID: body.OrderID, Actor: principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		writeLocalError(w, r, err)
		return
	}
	response, err := localEntitlementResponse(item)
	if err != nil {
		writeLocalError(w, r, err)
		return
	}
	write(w, http.StatusCreated, response)
}

func (h *LocalMutationHandler) RevokeProductLocalEntitlement(w http.ResponseWriter, r *http.Request, entitlementID int64) {
	principal, key, err := h.writeOperation(r, authport.CapabilityEntitlementsWrite)
	if err != nil {
		writeLocalError(w, r, err)
		return
	}
	var body RevokeProductLocalEntitlementRequest
	if err = decodeLocalBody(w, r, &body); err != nil {
		writeLocalError(w, r, err)
		return
	}
	item, err := h.entitlements.Revoke(r.Context(), productport.RevokeLocalEntitlementCommand{ID: productport.EntitlementID(entitlementID), ExpectedVersion: body.ExpectedVersion, Actor: principal.AdminUserID, IdempotencyKey: key})
	if err != nil {
		writeLocalError(w, r, err)
		return
	}
	response, err := localEntitlementResponse(item)
	if err != nil {
		writeLocalError(w, r, err)
		return
	}
	write(w, http.StatusOK, response)
}

func (h *LocalMutationHandler) readOperation(r *http.Request, capability authport.Capability) error {
	if h == nil || r == nil || nilInterface(h.entitlements) {
		return platformhttp.NewError(platformhttp.CodeDependencyUnavailable, productapp.ErrUnavailable)
	}
	if _, err := localPrincipal(r, capability); err != nil {
		return err
	}
	return nil
}

func (h *LocalMutationHandler) writeOperation(r *http.Request, capability authport.Capability) (authport.Principal, string, error) {
	if h == nil || r == nil {
		return authport.Principal{}, "", platformhttp.NewError(platformhttp.CodeDependencyUnavailable, productapp.ErrUnavailable)
	}
	principal, err := localPrincipal(r, capability)
	if err != nil {
		return authport.Principal{}, "", err
	}
	// Lane E registers every mutation under the canonical CSRF middleware before
	// this handler. Keeping that enforcement in one place prevents divergent
	// session semantics; this adapter only requires its post-CSRF authorization.
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !validLocalIdempotencyKey(keys[0]) {
		return authport.Principal{}, "", productapp.ErrInvalidProduct
	}
	return principal, keys[0], nil
}

func localPrincipal(r *http.Request, capability authport.Capability) (authport.Principal, error) {
	authorization, ok := authport.AuthorizationFromContext(r.Context())
	if !ok || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(r.Context())
	if !ok || principal.AdminUserID < 1 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	return principal, nil
}

func decodeLocalBody(w http.ResponseWriter, r *http.Request, target any) error {
	if r == nil || r.Body == nil || target == nil {
		return productapp.ErrInvalidProduct
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return productapp.ErrInvalidProduct
	}
	r.Body = http.MaxBytesReader(w, r.Body, localProductBodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return productapp.ErrInvalidProduct
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return productapp.ErrInvalidProduct
	}
	return nil
}

func localProductResponse(product productport.Product) (ProductResponse, error) {
	if !validLocalProduct(product) {
		return ProductResponse{}, productapp.ErrUnavailable
	}
	return ProductResponse{ID: int64(product.ID), ProductCode: product.ProductCode, Name: product.Name, Description: product.Description, PriceMinor: product.PriceMinor, Currency: product.Currency, StockQuantity: product.StockQuantity, Images: append([]string(nil), product.Images...), CreatedBy: product.CreatedBy, CreatedAt: product.CreatedAt.UTC().Format(timeRFC3339Nano), UpdatedAt: product.UpdatedAt.UTC().Format(timeRFC3339Nano), Version: product.Version}, nil
}

func localEntitlementResponse(item productport.LocalEntitlement) (LocalEntitlementResponse, error) {
	if item.ID < 1 || item.ProductID < 1 || item.OrderID < 1 || item.CustomerID < 1 || item.Version < 1 || item.GrantedAt.IsZero() || (item.State != "active" && item.State != "revoked") {
		return LocalEntitlementResponse{}, productapp.ErrUnavailable
	}
	if item.State == "active" && item.RevokedAt != nil {
		return LocalEntitlementResponse{}, productapp.ErrUnavailable
	}
	if item.State == "revoked" && (item.RevokedAt == nil || item.RevokedAt.IsZero() || item.RevokedAt.Before(item.GrantedAt)) {
		return LocalEntitlementResponse{}, productapp.ErrUnavailable
	}
	response := LocalEntitlementResponse{ID: int64(item.ID), ProductID: int64(item.ProductID), OrderID: item.OrderID, State: item.State, Version: item.Version, GrantedAt: item.GrantedAt.UTC().Format(timeRFC3339Nano)}
	if item.RevokedAt != nil {
		value := item.RevokedAt.UTC().Format(timeRFC3339Nano)
		response.RevokedAt = &value
	}
	return response, nil
}

func validLocalIdempotencyKey(key string) bool {
	return len(key) >= 16 && len(key) <= 128 && strings.TrimSpace(key) == key
}

func validLocalProduct(product productport.Product) bool {
	if product.ID < 1 || product.Version < 1 || product.CreatedBy < 1 || product.CreatedAt.IsZero() || product.UpdatedAt.IsZero() || product.UpdatedAt.Before(product.CreatedAt) ||
		product.ProductCode == "" || strings.TrimSpace(product.ProductCode) != product.ProductCode || len(product.ProductCode) > 200 ||
		product.Name == "" || strings.TrimSpace(product.Name) != product.Name || len(product.Name) > 200 ||
		strings.TrimSpace(product.Description) != product.Description || len(product.Description) > 10_000 || product.PriceMinor < 0 || product.StockQuantity < 0 ||
		len(product.Currency) != 3 || !validCurrency(product.Currency) {
		return false
	}
	for _, image := range product.Images {
		if image == "" || strings.TrimSpace(image) != image || len(image) > 2048 {
			return false
		}
	}
	return len(product.Images) <= 20
}

func validCurrency(value string) bool {
	for _, letter := range value {
		if letter < 'A' || letter > 'Z' {
			return false
		}
	}
	return true
}

const timeRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"

func writeLocalError(w http.ResponseWriter, r *http.Request, err error) {
	var httpError *platformhttp.HTTPError
	if errors.As(err, &httpError) {
		platformhttp.WriteError(w, r, httpError)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, productapp.ErrInvalidProduct), errors.Is(err, productapp.ErrInvalidCursor):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, productapp.ErrNotFound), errors.Is(err, productapp.ErrEntitlementNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, productapp.ErrConflict), errors.Is(err, productapp.ErrEntitlementOrderIneligible):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(w, r, platformhttp.NewError(code, err))
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface || v.Kind() == reflect.Func || v.Kind() == reflect.Map || v.Kind() == reflect.Slice) && v.IsNil()
}
