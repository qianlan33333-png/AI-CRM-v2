package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type Application interface {
	List(context.Context, string, int32) (productport.Page, error)
	Get(context.Context, productport.ID) (productport.Product, error)
	Create(context.Context, productport.CreateCommand) (productport.Product, error)
}
type Handler struct{ app Application }

func NewHandler(app Application) (*Handler, error) {
	if app == nil {
		return nil, errors.New("product application is required")
	}
	return &Handler{app}, nil
}
func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request, p generated.ListProductsParams) {
	if !authorized(r, authport.CapabilityProductsRead) {
		fail(w, r, authport.ErrUnauthorized)
		return
	}
	cursor := ""
	if p.Cursor != nil {
		cursor = string(*p.Cursor)
	}
	limit := productapp.DefaultLimit
	if p.Limit != nil {
		limit = int32(*p.Limit)
	}
	page, e := h.app.List(r.Context(), cursor, limit)
	if e != nil {
		fail(w, r, e)
		return
	}
	items := make([]generated.Product, len(page.Items))
	for i, x := range page.Items {
		items[i] = mapProduct(x)
	}
	response := generated.ProductPage{Items: items}
	if page.NextCursor != "" {
		v := page.NextCursor
		response.NextCursor = &v
	}
	write(w, http.StatusOK, response)
}
func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request, id generated.ProductID) {
	if !authorized(r, authport.CapabilityProductsRead) {
		fail(w, r, authport.ErrUnauthorized)
		return
	}
	p, e := h.app.Get(r.Context(), productport.ID(id))
	if e != nil {
		fail(w, r, e)
		return
	}
	write(w, http.StatusOK, mapProduct(p))
}
func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request, p generated.CreateProductParams) {
	principal, ok := authport.PrincipalFromContext(r.Context())
	if !ok || !authorized(r, authport.CapabilityProductsWrite) {
		fail(w, r, authport.ErrUnauthorized)
		return
	}
	var body generated.CreateProductRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body) != nil {
		fail(w, r, productapp.ErrInvalidProduct)
		return
	}
	projection := productapp.DefaultLegacyAdminProjection()
	result, e := h.app.Create(r.Context(), productport.CreateCommand{ProductCode: body.ProductCode, Name: body.Name, Description: body.Description, Currency: body.Currency, PriceMinor: body.PriceMinor, StockQuantity: body.StockQuantity, Images: body.Images, LegacyAdminProjection: projection, Actor: principal.AdminUserID, IdempotencyKey: string(p.IdempotencyKey)})
	if e != nil {
		fail(w, r, e)
		return
	}
	write(w, http.StatusCreated, mapProduct(result))
}
func authorized(r *http.Request, c authport.Capability) bool {
	a, ok := authport.AuthorizationFromContext(r.Context())
	return ok && a.Capability == c && a.Scope == authport.ScopeGlobal
}
func mapProduct(p productport.Product) generated.Product {
	return generated.Product{Id: int64(p.ID), ProductCode: p.ProductCode, Name: p.Name, Description: p.Description, PriceMinor: p.PriceMinor, Currency: p.Currency, StockQuantity: p.StockQuantity, Images: append([]string(nil), p.Images...), CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, Version: p.Version}
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, r *http.Request, e error) {
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(e, authport.ErrUnauthorized):
		code = platformhttp.CodeUnauthorized
	case errors.Is(e, productapp.ErrInvalidProduct), errors.Is(e, productapp.ErrInvalidCursor):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(e, productapp.ErrNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(e, productapp.ErrConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(w, r, platformhttp.NewError(code, e))
}
