package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

const legacyProductUpdateBodyLimit = 64 << 10

var (
	errInvalidLegacyProductUpdate  = errors.New("invalid legacy product update request")
	errLegacyProductReadbackUnsafe = errors.New("legacy product update readback is unsafe")
)

type legacyProductUpdateRequest struct {
	ExpectedVersion int64
	Name            string
	Description     string
	PriceMinor      int64
	Currency        string
	StockQuantity   int32
}

// UpdateProduct is a compatibility transport over productapp.Service.Update.
// It owns no product lifecycle, payment, provider, or retry behavior.
func (handler *Handler) UpdateProduct(writer http.ResponseWriter, request *http.Request) {
	if writer == nil {
		return
	}
	secured := legacyProductUpdateSecurityWriter{ResponseWriter: writer}
	applyLegacyProductUpdateSecurityHeaders(secured.Header())
	writer = secured
	if request == nil {
		request = &http.Request{}
		writeLegacyProductUpdateError(writer, request, productapp.ErrUnavailable)
		return
	}
	if handler == nil || nilLegacyDependency(handler.products) {
		writeLegacyProductUpdateError(writer, request, productapp.ErrUnavailable)
		return
	}
	principal, err := legacyProductUpdatePrincipal(request)
	if err != nil {
		writeLegacyProductUpdateError(writer, request, err)
		return
	}
	key, err := legacyProductUpdateIdempotencyKey(request)
	if err != nil {
		writeLegacyProductUpdateError(writer, request, err)
		return
	}
	id, err := legacyProductUpdateID(request)
	if err != nil {
		writeLegacyProductUpdateError(writer, request, err)
		return
	}
	body, err := decodeLegacyProductUpdateBody(writer, request)
	if err != nil {
		writeLegacyProductUpdateError(writer, request, err)
		return
	}
	command := productport.UpdateCommand{
		ID: productport.ID(id), ExpectedVersion: body.ExpectedVersion,
		Name: body.Name, Description: body.Description, PriceMinor: body.PriceMinor,
		Currency: body.Currency, StockQuantity: body.StockQuantity,
		Actor: principal.AdminUserID, IdempotencyKey: key,
	}
	updated, err := handler.products.Update(request.Context(), command)
	if err != nil {
		writeLegacyProductUpdateError(writer, request, err)
		return
	}
	// The write is never retried here. A failed or inconsistent readback means
	// the externally visible result is unknown and must remain fail-closed.
	readback, err := handler.products.Get(request.Context(), command.ID)
	if err != nil || !legacyProductUpdateReadbackMatches(command, updated, readback) {
		writeLegacyProductUpdateError(writer, request, errLegacyProductReadbackUnsafe)
		return
	}
	mapped, err := legacyProduct(readback)
	if err != nil {
		writeLegacyProductUpdateError(writer, request, errLegacyProductReadbackUnsafe)
		return
	}
	mapped["version"] = readback.Version
	writeJSON(writer, http.StatusOK, legacyProductEnvelope(map[string]any{
		"ok": true, "product": mapped,
	}))
}

func legacyProductUpdatePrincipal(request *http.Request) (authport.Principal, error) {
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	if principal.AdminUserID < 1 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != authport.CapabilityProductsWrite || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	return principal, nil
}

func legacyProductUpdateIdempotencyKey(request *http.Request) (string, error) {
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || len(values[0]) < 16 || len(values[0]) > 128 || strings.TrimSpace(values[0]) != values[0] {
		return "", errInvalidLegacyProductUpdate
	}
	return values[0], nil
}

func legacyProductUpdateID(request *http.Request) (int64, error) {
	raw := strings.TrimSpace(chi.URLParam(request, "product_id"))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		return 0, errInvalidLegacyProductUpdate
	}
	return id, nil
}

func decodeLegacyProductUpdateBody(writer http.ResponseWriter, request *http.Request) (legacyProductUpdateRequest, error) {
	if request.Body == nil {
		return legacyProductUpdateRequest{}, errInvalidLegacyProductUpdate
	}
	contentTypes := request.Header.Values("Content-Type")
	if len(contentTypes) != 1 {
		return legacyProductUpdateRequest{}, errInvalidLegacyProductUpdate
	}
	contentType, _, err := mime.ParseMediaType(contentTypes[0])
	if err != nil || contentType != "application/json" {
		return legacyProductUpdateRequest{}, errInvalidLegacyProductUpdate
	}
	payload, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, legacyProductUpdateBodyLimit))
	if err != nil || len(payload) == 0 || !utf8.Valid(payload) {
		return legacyProductUpdateRequest{}, errInvalidLegacyProductUpdate
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return legacyProductUpdateRequest{}, errInvalidLegacyProductUpdate
	}
	seen := make(map[string]struct{}, 6)
	var body legacyProductUpdateRequest
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return legacyProductUpdateRequest{}, errInvalidLegacyProductUpdate
		}
		if _, duplicate := seen[key]; duplicate {
			return legacyProductUpdateRequest{}, errInvalidLegacyProductUpdate
		}
		seen[key] = struct{}{}
		var raw json.RawMessage
		if err = decoder.Decode(&raw); err != nil {
			return legacyProductUpdateRequest{}, errInvalidLegacyProductUpdate
		}
		switch key {
		case "expected_version":
			err = decodeLegacyProductUpdateScalar(raw, &body.ExpectedVersion)
		case "name":
			err = decodeLegacyProductUpdateScalar(raw, &body.Name)
		case "description":
			err = decodeLegacyProductUpdateScalar(raw, &body.Description)
		case "price_minor":
			err = decodeLegacyProductUpdateScalar(raw, &body.PriceMinor)
		case "currency":
			err = decodeLegacyProductUpdateScalar(raw, &body.Currency)
		case "stock_quantity":
			err = decodeLegacyProductUpdateScalar(raw, &body.StockQuantity)
		default:
			return legacyProductUpdateRequest{}, errInvalidLegacyProductUpdate
		}
		if err != nil {
			return legacyProductUpdateRequest{}, errInvalidLegacyProductUpdate
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || len(seen) != 6 {
		return legacyProductUpdateRequest{}, errInvalidLegacyProductUpdate
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return legacyProductUpdateRequest{}, errInvalidLegacyProductUpdate
	}
	return body, nil
}

func decodeLegacyProductUpdateScalar(raw json.RawMessage, target any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return errInvalidLegacyProductUpdate
	}
	if err := json.Unmarshal(trimmed, target); err != nil {
		return errInvalidLegacyProductUpdate
	}
	return nil
}

func legacyProductUpdateReadbackMatches(command productport.UpdateCommand, updated, readback productport.Product) bool {
	if command.ID < 1 || command.ExpectedVersion < 1 || updated.ID != command.ID || readback.ID != command.ID || updated.Version < 2 || updated.Version-1 != command.ExpectedVersion || readback.Version != updated.Version ||
		updated.ProductCode != readback.ProductCode || updated.Name != readback.Name ||
		updated.Description != readback.Description || updated.PriceMinor != readback.PriceMinor ||
		updated.Currency != readback.Currency || updated.StockQuantity != readback.StockQuantity ||
		updated.CreatedBy != readback.CreatedBy || updated.CreatedBy < 1 ||
		updated.CreatedAt.IsZero() || updated.UpdatedAt.IsZero() || updated.UpdatedAt.Before(updated.CreatedAt) ||
		!updated.CreatedAt.Equal(readback.CreatedAt) || !updated.UpdatedAt.Equal(readback.UpdatedAt) ||
		!reflect.DeepEqual(updated.Images, readback.Images) ||
		!bytes.Equal(updated.LegacyAdminProjection, readback.LegacyAdminProjection) {
		return false
	}
	return true
}

func writeLegacyProductUpdateError(writer http.ResponseWriter, request *http.Request, err error) {
	var httpError *platformhttp.HTTPError
	if errors.As(err, &httpError) {
		platformhttp.WriteError(writer, request, httpError)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, errInvalidLegacyProductUpdate), errors.Is(err, productapp.ErrInvalidProduct), errors.Is(err, productapp.ErrInvalidCursor):
		code = platformhttp.CodeValidationFailed
	case errors.Is(err, productapp.ErrNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, productapp.ErrConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

type legacyProductUpdateSecurityWriter struct {
	http.ResponseWriter
}

func (writer legacyProductUpdateSecurityWriter) WriteHeader(status int) {
	applyLegacyProductUpdateSecurityHeaders(writer.Header())
	writer.ResponseWriter.WriteHeader(status)
}

func (writer legacyProductUpdateSecurityWriter) Write(payload []byte) (int, error) {
	applyLegacyProductUpdateSecurityHeaders(writer.Header())
	return writer.ResponseWriter.Write(payload)
}

func applyLegacyProductUpdateSecurityHeaders(header http.Header) {
	header.Set("Cache-Control", "private, no-store")
	header.Set("X-Content-Type-Options", "nosniff")
}
