// Package serviceperiod adapts the frozen legacy service-period Product routes.
// It owns no provider, payment, entitlement-effect, or external-call
// dependency. Its share descriptor points only at the existing same-origin,
// read-only Product detail route. State-changing routes must be wrapped by the canonical CSRF
// middleware at the composition root described in ROUTE_FRAGMENT.md.
package serviceperiod

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

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

const (
	BasePath            = "/api/admin/service-period-products"
	maximumRequestBytes = 64 << 10
)

var errMalformedServicePeriodRequest = errors.New("malformed service-period product request")

type Handler struct {
	application productport.ServicePeriodApplication
}

func NewHandler(application productport.ServicePeriodApplication) (*Handler, error) {
	if nilApplication(application) {
		return nil, productapp.ErrUnavailable
	}
	return &Handler{application: application}, nil
}

// ServeHTTP is a strict leaf router so malformed IDs, encoded path material,
// extra segments, methods, query keys, and request bodies fail closed even when
// a composition root accidentally mounts a broader pattern.
func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilApplication(handler.application) || request == nil {
		writeApplicationError(writer, request, productapp.ErrUnavailable)
		return
	}
	if request.URL == nil || request.URL.EscapedPath() != request.URL.Path || strings.Contains(request.URL.Path, `\`) {
		writeMalformed(writer, request)
		return
	}

	path := request.URL.Path
	if path == BasePath {
		handler.serveCollection(writer, request)
		return
	}
	if !strings.HasPrefix(path, BasePath+"/") {
		writeNotFound(writer, request)
		return
	}
	remainder := strings.TrimPrefix(path, BasePath+"/")
	segments := strings.Split(remainder, "/")
	if len(segments) < 1 || len(segments) > 2 || segments[0] == "" {
		writeNotFound(writer, request)
		return
	}
	id, ok := canonicalPositiveID(segments[0])
	if !ok {
		writeMalformed(writer, request)
		return
	}
	if len(segments) == 1 {
		handler.serveItem(writer, request, productport.ID(id))
		return
	}
	if segments[1] == "" {
		writeNotFound(writer, request)
		return
	}
	handler.serveAction(writer, request, productport.ID(id), segments[1])
}

func (handler *Handler) serveCollection(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		handler.list(writer, request)
	case http.MethodPost:
		handler.create(writer, request)
	default:
		writeMethodNotAllowed(writer, request, "GET, POST")
	}
}

func (handler *Handler) serveItem(writer http.ResponseWriter, request *http.Request, id productport.ID) {
	switch request.Method {
	case http.MethodGet:
		handler.get(writer, request, id)
	case http.MethodPut:
		handler.update(writer, request, id)
	case http.MethodDelete:
		handler.archive(writer, request, id)
	default:
		writeMethodNotAllowed(writer, request, "GET, PUT, DELETE")
	}
}

func (handler *Handler) serveAction(writer http.ResponseWriter, request *http.Request, id productport.ID, action string) {
	if action == "share" {
		if request.Method != http.MethodGet {
			writeMethodNotAllowed(writer, request, "GET")
			return
		}
		handler.share(writer, request, id)
		return
	}
	if action != "enable" && action != "disable" && action != "copy" {
		writeNotFound(writer, request)
		return
	}
	if request.Method != http.MethodPost {
		writeMethodNotAllowed(writer, request, "POST")
		return
	}
	switch action {
	case "enable":
		handler.setEnabled(writer, request, id, true)
	case "disable":
		handler.setEnabled(writer, request, id, false)
	case "copy":
		handler.copyProduct(writer, request, id)
	}
}

func (handler *Handler) share(writer http.ResponseWriter, request *http.Request, id productport.ID) {
	if !readAuthorized(request) {
		writeAuthorizationError(writer, request)
		return
	}
	if request.URL.RawQuery != "" || requireEmptyBody(request) != nil {
		writeMalformed(writer, request)
		return
	}
	product, err := handler.application.GetServicePeriodProduct(request.Context(), id)
	if err != nil {
		writeApplicationError(writer, request, err)
		return
	}
	if product.ServiceProductID != id || !product.Enabled || product.Archived || product.Lifecycle != productport.ServicePeriodEnabled {
		writeApplicationError(writer, request, productapp.ErrNotFound)
		return
	}
	writeJSON(writer, http.StatusOK, shareResponse{
		OK: true, ServiceProductID: id, PublicPath: "/p/service_period/" + strconv.FormatInt(int64(id), 10),
		LocalOnly: true, RealExternalCallExecuted: false,
	})
}

func (handler *Handler) list(writer http.ResponseWriter, request *http.Request) {
	if !readAuthorized(request) {
		writeAuthorizationError(writer, request)
		return
	}
	if err := requireEmptyBody(request); err != nil {
		writeMalformed(writer, request)
		return
	}
	limit, offset, err := parseListQuery(request.URL.RawQuery)
	if err != nil {
		writeMalformed(writer, request)
		return
	}
	page, err := handler.application.ListServicePeriodProducts(request.Context(), limit, offset)
	if err != nil {
		writeApplicationError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (handler *Handler) get(writer http.ResponseWriter, request *http.Request, id productport.ID) {
	if !readAuthorized(request) {
		writeAuthorizationError(writer, request)
		return
	}
	if request.URL.RawQuery != "" || requireEmptyBody(request) != nil {
		writeMalformed(writer, request)
		return
	}
	product, err := handler.application.GetServicePeriodProduct(request.Context(), id)
	if err != nil {
		writeApplicationError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, productResponse{OK: true, Product: product})
}

func (handler *Handler) create(writer http.ResponseWriter, request *http.Request) {
	principal, ok := writePrincipal(request)
	if !ok {
		writeAuthorizationError(writer, request)
		return
	}
	key, ok := idempotencyKey(request)
	if !ok {
		writeMalformed(writer, request)
		return
	}
	if request.URL.RawQuery != "" {
		writeMalformed(writer, request)
		return
	}
	var body createRequest
	if err := decodeStrictJSON(writer, request, &body); err != nil || body.ProductCode == nil || body.Name == nil || body.PriceMinor == nil || body.Currency == nil || body.StockQuantity == nil {
		writeMalformed(writer, request)
		return
	}
	product, err := handler.application.CreateServicePeriodProduct(request.Context(), productport.CreateServicePeriodProductCommand{
		ProductCode:    *body.ProductCode,
		Name:           *body.Name,
		Description:    body.Description,
		PriceMinor:     *body.PriceMinor,
		Currency:       *body.Currency,
		StockQuantity:  *body.StockQuantity,
		Actor:          principal.AdminUserID,
		IdempotencyKey: key,
	})
	if err != nil {
		writeApplicationError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, productResponse{OK: true, Product: product})
}

func (handler *Handler) update(writer http.ResponseWriter, request *http.Request, id productport.ID) {
	principal, ok := writePrincipal(request)
	if !ok {
		writeAuthorizationError(writer, request)
		return
	}
	key, ok := idempotencyKey(request)
	if !ok {
		writeMalformed(writer, request)
		return
	}
	if request.URL.RawQuery != "" {
		writeMalformed(writer, request)
		return
	}
	var body updateRequest
	if err := decodeStrictJSON(writer, request, &body); err != nil || body.ExpectedVersion == nil || body.Name == nil || body.Description == nil || body.PriceMinor == nil || body.Currency == nil || body.StockQuantity == nil {
		writeMalformed(writer, request)
		return
	}
	product, err := handler.application.UpdateServicePeriodProduct(request.Context(), productport.UpdateServicePeriodProductCommand{
		ID:              id,
		ExpectedVersion: *body.ExpectedVersion,
		Name:            *body.Name,
		Description:     *body.Description,
		PriceMinor:      *body.PriceMinor,
		Currency:        *body.Currency,
		StockQuantity:   *body.StockQuantity,
		Actor:           principal.AdminUserID,
		IdempotencyKey:  key,
	})
	if err != nil {
		writeApplicationError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, productResponse{OK: true, Product: product})
}

func (handler *Handler) setEnabled(writer http.ResponseWriter, request *http.Request, id productport.ID, enabled bool) {
	principal, ok := writePrincipal(request)
	if !ok {
		writeAuthorizationError(writer, request)
		return
	}
	key, ok := idempotencyKey(request)
	if !ok {
		writeMalformed(writer, request)
		return
	}
	version, ok := decodeVersionRequest(writer, request)
	if !ok {
		return
	}
	product, err := handler.application.SetServicePeriodProductEnabled(request.Context(), productport.SetServicePeriodProductEnabledCommand{
		ID:              id,
		ExpectedVersion: version,
		Enabled:         enabled,
		Actor:           principal.AdminUserID,
		IdempotencyKey:  key,
	})
	if err != nil {
		writeApplicationError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, productResponse{OK: true, Product: product})
}

func (handler *Handler) copyProduct(writer http.ResponseWriter, request *http.Request, id productport.ID) {
	principal, ok := writePrincipal(request)
	if !ok {
		writeAuthorizationError(writer, request)
		return
	}
	key, ok := idempotencyKey(request)
	if !ok {
		writeMalformed(writer, request)
		return
	}
	version, ok := decodeVersionRequest(writer, request)
	if !ok {
		return
	}
	product, err := handler.application.CopyServicePeriodProduct(request.Context(), productport.CopyServicePeriodProductCommand{
		ID:              id,
		ExpectedVersion: version,
		Actor:           principal.AdminUserID,
		IdempotencyKey:  key,
	})
	if err != nil {
		writeApplicationError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, productResponse{OK: true, Product: product})
}

func (handler *Handler) archive(writer http.ResponseWriter, request *http.Request, id productport.ID) {
	principal, ok := writePrincipal(request)
	if !ok {
		writeAuthorizationError(writer, request)
		return
	}
	key, ok := idempotencyKey(request)
	if !ok {
		writeMalformed(writer, request)
		return
	}
	version, ok := decodeVersionRequest(writer, request)
	if !ok {
		return
	}
	product, err := handler.application.ArchiveServicePeriodProduct(request.Context(), productport.ArchiveServicePeriodProductCommand{
		ID:              id,
		ExpectedVersion: version,
		Actor:           principal.AdminUserID,
		IdempotencyKey:  key,
	})
	if err != nil {
		writeApplicationError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, productResponse{OK: true, Product: product})
}

func decodeVersionRequest(writer http.ResponseWriter, request *http.Request) (int64, bool) {
	if request.URL.RawQuery != "" {
		writeMalformed(writer, request)
		return 0, false
	}
	var body versionRequest
	if err := decodeStrictJSON(writer, request, &body); err != nil || body.ExpectedVersion == nil {
		writeMalformed(writer, request)
		return 0, false
	}
	return *body.ExpectedVersion, true
}

type createRequest struct {
	ProductCode   *string `json:"product_code"`
	Name          *string `json:"name"`
	Description   string  `json:"description"`
	PriceMinor    *int64  `json:"price_minor"`
	Currency      *string `json:"currency"`
	StockQuantity *int32  `json:"stock_quantity"`
}

type updateRequest struct {
	ExpectedVersion *int64  `json:"expected_version"`
	Name            *string `json:"name"`
	Description     *string `json:"description"`
	PriceMinor      *int64  `json:"price_minor"`
	Currency        *string `json:"currency"`
	StockQuantity   *int32  `json:"stock_quantity"`
}

type versionRequest struct {
	ExpectedVersion *int64 `json:"expected_version"`
}

type productResponse struct {
	OK      bool                             `json:"ok"`
	Product productport.ServicePeriodProduct `json:"product"`
}

type shareResponse struct {
	OK                       bool           `json:"ok"`
	ServiceProductID         productport.ID `json:"service_product_id"`
	PublicPath               string         `json:"public_path"`
	LocalOnly                bool           `json:"local_only"`
	RealExternalCallExecuted bool           `json:"real_external_call_executed"`
}

func readAuthorized(request *http.Request) bool {
	if request == nil {
		return false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && validReadRole(principal.Role) && authorizationOK &&
		authorization.Capability == authport.CapabilityProductsRead && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func writePrincipal(request *http.Request) (authport.Principal, bool) {
	if request == nil {
		return authport.Principal{}, false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	if !principalOK || principal.AdminUserID < 1 || !validWriteRole(principal.Role) || !authorizationOK ||
		authorization.Capability != authport.CapabilityProductsWrite || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return authport.Principal{}, false
	}
	return principal, true
}

func idempotencyKey(request *http.Request) (string, bool) {
	if request == nil {
		return "", false
	}
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || len(values[0]) < 16 || len(values[0]) > 128 || strings.TrimSpace(values[0]) != values[0] {
		return "", false
	}
	return values[0], true
}

func validReadRole(role authport.Role) bool {
	return role == authport.RoleAdmin || role == authport.RoleOps
}

func validWriteRole(role authport.Role) bool {
	return role == authport.RoleAdmin || role == authport.RoleOps
}

func parseListQuery(raw string) (int32, int32, error) {
	limit := int32(productapp.DefaultLimit)
	offset := int32(0)
	if raw == "" {
		return limit, offset, nil
	}
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, "&") {
		if part == "" || strings.Count(part, "=") != 1 {
			return 0, 0, errMalformedServicePeriodRequest
		}
		key, value, _ := strings.Cut(part, "=")
		if key == "" || value == "" || strings.ContainsAny(key+value, "+%") || seen[key] {
			return 0, 0, errMalformedServicePeriodRequest
		}
		seen[key] = true
		switch key {
		case "limit":
			parsed, ok := canonicalNonNegativeInt32(value)
			if !ok || parsed < 1 || parsed > productapp.MaximumLimit {
				return 0, 0, errMalformedServicePeriodRequest
			}
			limit = parsed
		case "offset":
			parsed, ok := canonicalNonNegativeInt32(value)
			if !ok || parsed > productapp.MaximumLegacyOffset {
				return 0, 0, errMalformedServicePeriodRequest
			}
			offset = parsed
		default:
			return 0, 0, errMalformedServicePeriodRequest
		}
	}
	return limit, offset, nil
}

func canonicalPositiveID(value string) (int64, bool) {
	if value == "" || value[0] == '0' {
		return 0, false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}

func canonicalNonNegativeInt32(value string) (int32, bool) {
	if value == "" || len(value) > 1 && value[0] == '0' {
		return 0, false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	return int32(parsed), err == nil && parsed >= 0 && strconv.FormatInt(parsed, 10) == value
}

func decodeStrictJSON(_ http.ResponseWriter, request *http.Request, destination any) error {
	if request == nil || request.Body == nil || destination == nil {
		return errMalformedServicePeriodRequest
	}
	mediaType, parameters, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || len(parameters) > 1 {
		return errMalformedServicePeriodRequest
	}
	for name, value := range parameters {
		if name != "charset" || !strings.EqualFold(value, "utf-8") {
			return errMalformedServicePeriodRequest
		}
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maximumRequestBytes+1))
	if err != nil || len(body) == 0 || len(body) > maximumRequestBytes || !utf8.Valid(body) {
		return errMalformedServicePeriodRequest
	}
	allowed, ok := allowedJSONFields(destination)
	if !ok {
		return errMalformedServicePeriodRequest
	}

	keys := json.NewDecoder(bytes.NewReader(body))
	keys.UseNumber()
	opening, err := keys.Token()
	if err != nil || opening != json.Delim('{') {
		return errMalformedServicePeriodRequest
	}
	seen := make(map[string]struct{}, len(allowed))
	for keys.More() {
		token, tokenErr := keys.Token()
		key, keyOK := token.(string)
		if tokenErr != nil || !keyOK {
			return errMalformedServicePeriodRequest
		}
		if _, known := allowed[key]; !known {
			return errMalformedServicePeriodRequest
		}
		if _, duplicate := seen[key]; duplicate {
			return errMalformedServicePeriodRequest
		}
		seen[key] = struct{}{}
		var value json.RawMessage
		if keys.Decode(&value) != nil {
			return errMalformedServicePeriodRequest
		}
	}
	closing, err := keys.Token()
	if err != nil || closing != json.Delim('}') {
		return errMalformedServicePeriodRequest
	}
	if trailing, trailingErr := keys.Token(); trailingErr != io.EOF || trailing != nil {
		return errMalformedServicePeriodRequest
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(destination) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return errMalformedServicePeriodRequest
	}
	return nil
}

func allowedJSONFields(destination any) (map[string]struct{}, bool) {
	valueOfDestination := reflect.ValueOf(destination)
	if valueOfDestination.Kind() != reflect.Pointer || valueOfDestination.IsNil() {
		return nil, false
	}
	typeOfDestination := valueOfDestination.Type().Elem()
	if typeOfDestination.Kind() != reflect.Struct {
		return nil, false
	}
	fields := make(map[string]struct{}, typeOfDestination.NumField())
	for index := 0; index < typeOfDestination.NumField(); index++ {
		field := typeOfDestination.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" {
			name = field.Name
		}
		if name == "-" {
			continue
		}
		fields[name] = struct{}{}
	}
	return fields, len(fields) > 0
}

func requireEmptyBody(request *http.Request) error {
	if request == nil || request.Body == nil || request.Body == http.NoBody {
		return nil
	}
	var buffer [1]byte
	read, err := request.Body.Read(buffer[:])
	if read == 0 && errors.Is(err, io.EOF) {
		return nil
	}
	return errMalformedServicePeriodRequest
}

func writeApplicationError(writer http.ResponseWriter, request *http.Request, err error) {
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, productapp.ErrInvalidProduct), errors.Is(err, productapp.ErrInvalidCursor):
		code = platformhttp.CodeValidationFailed
	case errors.Is(err, productapp.ErrNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, productapp.ErrConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func writeAuthorizationError(writer http.ResponseWriter, request *http.Request) {
	if request == nil {
		return
	}
	if _, ok := authport.PrincipalFromContext(request.Context()); !ok {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated))
		return
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
}

func writeMalformed(writer http.ResponseWriter, request *http.Request) {
	platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, errMalformedServicePeriodRequest))
}

func writeNotFound(writer http.ResponseWriter, request *http.Request) {
	platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, productapp.ErrNotFound))
}

func writeMethodNotAllowed(writer http.ResponseWriter, request *http.Request, allow string) {
	writer.Header().Set("Allow", allow)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusMethodNotAllowed)
	_ = json.NewEncoder(writer).Encode(struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: "METHOD_NOT_ALLOWED", Message: "The method is not allowed."})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func nilApplication(application productport.ServicePeriodApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
