package http

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
)

const bodyLimit = 32 << 10

const (
	sidebarOAuthBindingCookie = "aicrm_sidebar_oauth_binding"
	sidebarOAuthCookiePath    = "/api/sidebar/v2/oauth/"
	defaultSidebarOAuthNext   = "/sidebar/"
)

type Handler struct{ service *sidebarapp.Service }

type sidebarShareableProductResponse struct {
	Items  []sidebarShareableProductJSON `json:"items"`
	Safety sidebarapp.Safety             `json:"safety"`
}

type sidebarShareableProductJSON struct {
	Kind          sidebarapp.ShareableProductKind `json:"kind"`
	ProductID     int64                           `json:"product_id"`
	ProductCode   string                          `json:"product_code"`
	Name          string                          `json:"name"`
	Description   string                          `json:"description"`
	PriceMinor    int64                           `json:"price_minor"`
	Currency      string                          `json:"currency"`
	StockQuantity int32                           `json:"stock_quantity"`
	PublicPath    string                          `json:"public_path"`
}

// OAuthHandler exposes the sidebar-only browser OAuth boundary. A nil service
// is an intentional disabled configuration and fails closed without a provider
// call.
type OAuthHandler struct {
	service      *sidebarapp.OAuthGrantService
	writeSession func(http.ResponseWriter, authport.BrowserSession) error
}

func NewOAuthHandler(service *sidebarapp.OAuthGrantService, writeSession func(http.ResponseWriter, authport.BrowserSession) error) *OAuthHandler {
	return &OAuthHandler{service: service, writeSession: writeSession}
}

func (handler *OAuthHandler) Start(writer http.ResponseWriter, request *http.Request) {
	oauthResponseHeaders(writer)
	externalUserID, nextPath, err := sidebarOAuthStartInput(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	if handler == nil || nilValue(handler.service) || handler.writeSession == nil {
		writeError(writer, request, sidebarapp.ErrOAuthUnavailable)
		return
	}
	start, err := handler.service.Begin(request.Context(), externalUserID, nextPath)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	http.SetCookie(writer, &http.Cookie{Name: sidebarOAuthBindingCookie, Value: start.Binding, Path: sidebarOAuthCookiePath, Expires: start.ExpiresAt, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(writer, request, start.AuthorizationURL, http.StatusFound)
}

func (handler *OAuthHandler) Callback(writer http.ResponseWriter, request *http.Request) {
	oauthResponseHeaders(writer)
	code, state, err := sidebarOAuthCallbackInput(request)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	cookie, err := request.Cookie(sidebarOAuthBindingCookie)
	if err != nil || cookie.Value == "" {
		writeError(writer, request, sidebarapp.ErrOAuthAttemptInvalid)
		return
	}
	clearSidebarOAuthBinding(writer)
	if handler == nil || nilValue(handler.service) || handler.writeSession == nil {
		writeError(writer, request, sidebarapp.ErrOAuthUnavailable)
		return
	}
	completed, err := handler.service.Complete(request.Context(), code, authport.OAuthState(state), cookie.Value)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	if err = handler.writeSession(writer, completed.Session); err != nil {
		if revokeErr := handler.service.RevokeCompletedSession(request.Context(), completed.Session); revokeErr != nil {
			writeError(writer, request, revokeErr)
			return
		}
		writeError(writer, request, sidebarapp.ErrOAuthUnavailable)
		return
	}
	http.Redirect(writer, request, completed.NextPath, http.StatusFound)
}

// JSSDKHandler signs only agent_config payloads. A nil or disabled service is
// deliberately observable as unavailable, without any provider request.
type JSSDKHandler struct{ service *sidebarapp.JSSDKService }

func NewJSSDKHandler(service *sidebarapp.JSSDKService) *JSSDKHandler {
	return &JSSDKHandler{service: service}
}

func (handler *JSSDKHandler) AgentConfig(writer http.ResponseWriter, request *http.Request) {
	if request == nil || request.URL == nil || len(request.URL.Query()) != 1 || len(request.URL.Query()["url"]) != 1 {
		writeError(writer, request, sidebarapp.ErrJSSDKInvalid)
		return
	}
	if handler == nil || nilValue(handler.service) {
		writeError(writer, request, sidebarapp.ErrJSSDKDisabled)
		return
	}
	result, err := handler.service.AgentConfig(request.Context(), request.URL.Query().Get("url"))
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func NewHandler(service *sidebarapp.Service) (*Handler, error) {
	if nilValue(service) {
		return nil, sidebarapp.ErrUnavailable
	}
	return &Handler{service: service}, nil
}

func (handler *Handler) MintContext(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		ExternalUserID string `json:"external_userid"`
	}
	if err := decodeBody(writer, request, &body); err != nil {
		writeError(writer, request, err)
		return
	}
	principal, authenticated := authport.PrincipalFromContext(request.Context())
	session, _ := authport.SessionFromContext(request.Context())
	result, err := handler.service.MintContext(request.Context(), principal, session, authenticated, body.ExternalUserID)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) Bootstrap(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		ExternalUserID string `json:"external_userid"`
	}
	if err := decodeBody(writer, request, &body); err != nil {
		writeError(writer, request, err)
		return
	}
	principal, authenticated := authport.PrincipalFromContext(request.Context())
	session, _ := authport.SessionFromContext(request.Context())
	result, err := handler.service.Bootstrap(request.Context(), principal, session, authenticated, body.ExternalUserID)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) Workbench(writer http.ResponseWriter, request *http.Request, token string) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersRead)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.Workbench(request.Context(), scope)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) UpdateProfile(writer http.ResponseWriter, request *http.Request, token, idempotencyKey string) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersWrite)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	var body struct {
		ExpectedUpdatedAt time.Time                       `json:"expected_updated_at"`
		Patch             contactport.SidebarProfilePatch `json:"patch"`
	}
	if err = decodeBody(writer, request, &body); err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.UpdateProfile(request.Context(), scope, body.ExpectedUpdatedAt, body.Patch, idempotencyKey)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) BindPhone(writer http.ResponseWriter, request *http.Request, token, idempotencyKey string) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersWrite)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	var body struct {
		Mobile string `json:"mobile"`
	}
	if err = decodeBody(writer, request, &body); err != nil {
		writeError(writer, request, err)
		return
	}
	status, err := handler.service.BindPhone(request.Context(), scope, body.Mobile, idempotencyKey)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		Status string            `json:"status"`
		Safety sidebarapp.Safety `json:"safety"`
	}{status, sidebarapp.Safety{LocalOnly: true}})
}

func (handler *Handler) Questionnaires(writer http.ResponseWriter, request *http.Request, token string, limit int32) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersRead)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.Questionnaires(request.Context(), scope, limit)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) Orders(writer http.ResponseWriter, request *http.Request, token string, limit, offset int32) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersRead)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.Orders(request.Context(), scope, limit, offset)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) PeriodicOrders(writer http.ResponseWriter, request *http.Request, token string, limit, offset int) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersRead)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.PeriodicOrders(request.Context(), scope, limit, offset)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) UpdatePeriodicRemark(writer http.ResponseWriter, request *http.Request, token, idempotencyKey string, serviceProductID int64, memberRef string) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersWrite)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	var body struct {
		ExpectedVersion int64  `json:"expected_version"`
		Remark          string `json:"remark"`
	}
	if err = decodeBody(writer, request, &body); err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.UpdatePeriodicRemark(request.Context(), scope, serviceProductID, memberRef, body.ExpectedVersion, &body.Remark, idempotencyKey)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) Materials(writer http.ResponseWriter, request *http.Request, token string, query mediaport.ImageListQuery) {
	if _, err := handler.scope(request, token, authport.CapabilityCustomersRead); err != nil {
		writeError(writer, request, err)
		return
	}
	query.EnabledOnly = true
	result, err := handler.service.Materials(request.Context(), query)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) ShareableProducts(writer http.ResponseWriter, request *http.Request, token string, limit int32) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersRead)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.ShareableProducts(request.Context(), scope, limit)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	items := make([]sidebarShareableProductJSON, 0, len(result.Items))
	for _, product := range result.Items {
		items = append(items, sidebarShareableProductJSON{
			Kind: product.Kind, ProductID: product.ProductID, ProductCode: product.ProductCode, Name: product.Name,
			Description: product.Description, PriceMinor: product.PriceMinor, Currency: product.Currency, StockQuantity: product.StockQuantity,
			PublicPath: "/p/" + string(product.Kind) + "/" + strconv.FormatInt(product.ProductID, 10),
		})
	}
	writeJSON(writer, http.StatusOK, sidebarShareableProductResponse{Items: items, Safety: result.Safety})
}

func (handler *Handler) PrepareTemporaryImageMedia(writer http.ResponseWriter, request *http.Request, token, idempotencyKey string, imageID int64) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersRead)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.PrepareTemporaryImageMedia(request.Context(), scope, imageID, idempotencyKey)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	status := http.StatusOK
	if result.UploadState != "ready" {
		status = http.StatusAccepted
	}
	writeJSON(writer, status, struct {
		ImageID                  int64     `json:"image_id"`
		MediaID                  string    `json:"media_id,omitempty"`
		MediaExpiresAt           time.Time `json:"media_expires_at,omitempty"`
		UploadState              string    `json:"upload_state"`
		ProviderCallDispatched   bool      `json:"provider_call_dispatched"`
		RealExternalCallExecuted bool      `json:"real_external_call_executed"`
		ClientCallback           string    `json:"client_callback"`
		DeliveryState            string    `json:"delivery_state"`
	}{
		ImageID: result.ImageID, MediaID: result.MediaID, MediaExpiresAt: result.MediaExpiresAt,
		UploadState: result.UploadState, ProviderCallDispatched: result.ProviderCallDispatched,
		RealExternalCallExecuted: result.ProviderCallDispatched, ClientCallback: "not_called", DeliveryState: "not_sent_yet",
	})
}

func (handler *Handler) ThumbnailStatus(writer http.ResponseWriter, request *http.Request, token string, imageID int64) {
	if _, err := handler.scope(request, token, authport.CapabilityCustomersRead); err != nil {
		writeError(writer, request, err)
		return
	}
	status, err := handler.service.ThumbnailStatus(request.Context(), imageID)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writer.Header().Set("X-Thumbnail-Status", status)
	writeJSON(writer, http.StatusAccepted, struct {
		Status string            `json:"status"`
		Safety sidebarapp.Safety `json:"safety"`
	}{status, sidebarapp.Safety{LocalOnly: true}})
}

func (handler *Handler) ThumbnailPreview(writer http.ResponseWriter, request *http.Request, token string, imageID int64) {
	if _, err := handler.scope(request, token, authport.CapabilityCustomersRead); err != nil {
		writeError(writer, request, err)
		return
	}
	variant, err := handler.service.ThumbnailPreview(request.Context(), imageID)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writer.Header().Set("ETag", variant.ETag)
	writer.Header().Set("Cache-Control", "private, no-cache")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if request.Header.Get("If-None-Match") == variant.ETag {
		writer.WriteHeader(http.StatusNotModified)
		return
	}
	writer.Header().Set("Content-Type", variant.MediaType)
	writer.Header().Set("Content-Length", strconv.Itoa(len(variant.Content)))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(variant.Content)
}

func (handler *Handler) scope(request *http.Request, token string, capability authport.Capability) (sidebarapp.Scope, error) {
	if handler == nil || nilValue(handler.service) || request == nil {
		return sidebarapp.Scope{}, sidebarapp.ErrUnavailable
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok {
		return sidebarapp.Scope{}, sidebarapp.ErrViewerSession
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != capability {
		return sidebarapp.Scope{}, sidebarapp.ErrForbidden
	}
	session, ok := authport.SessionFromContext(request.Context())
	if !ok {
		return sidebarapp.Scope{}, sidebarapp.ErrViewerSession
	}
	scope, err := handler.service.VerifyContext(request.Context(), principal, session, token)
	if err != nil {
		return sidebarapp.Scope{}, err
	}
	if !authorization.AllowsOwner(scope.OwnerStaffID) {
		return sidebarapp.Scope{}, sidebarapp.ErrForbidden
	}
	return scope, nil
}

func decodeBody(writer http.ResponseWriter, request *http.Request, target any) error {
	if request == nil || request.Body == nil || target == nil {
		return sidebarapp.ErrInvalidInput
	}
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return sidebarapp.ErrInvalidInput
	}
	request.Body = http.MaxBytesReader(writer, request.Body, bodyLimit)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return sidebarapp.ErrInvalidInput
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return sidebarapp.ErrInvalidInput
	}
	return nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, request *http.Request, err error) {
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, sidebarapp.ErrInvalidInput):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, sidebarapp.ErrOAuthAttemptInvalid), errors.Is(err, sidebarapp.ErrJSSDKInvalid):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, sidebarapp.ErrViewerSession), errors.Is(err, sidebarapp.ErrTokenInvalid), errors.Is(err, sidebarapp.ErrTokenExpired):
		code = platformhttp.CodeUnauthenticated
	case errors.Is(err, sidebarapp.ErrForbidden):
		code = platformhttp.CodeUnauthorized
	case errors.Is(err, sidebarapp.ErrNotFound), errors.Is(err, sidebarapp.ErrCustomerNotBound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, sidebarapp.ErrConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func sidebarOAuthStartInput(request *http.Request) (externalUserID, nextPath string, err error) {
	if request == nil || request.URL == nil || len(request.URL.Query()["external_userid"]) != 1 || len(request.URL.Query()) > 2 {
		return "", "", sidebarapp.ErrOAuthAttemptInvalid
	}
	externalUserID = request.URL.Query().Get("external_userid")
	nextPath = request.URL.Query().Get("next")
	if nextPath == "" {
		nextPath = defaultSidebarOAuthNext
	}
	if len(request.URL.Query()) == 2 && len(request.URL.Query()["next"]) != 1 {
		return "", "", sidebarapp.ErrOAuthAttemptInvalid
	}
	if !validSidebarNextPath(nextPath) {
		return "", "", sidebarapp.ErrOAuthAttemptInvalid
	}
	return externalUserID, nextPath, nil
}

func validSidebarNextPath(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Fragment != "" {
		return false
	}
	return parsed.Path == "/sidebar/" || parsed.Path == "/sidebar/index.html"
}

func sidebarOAuthCallbackInput(request *http.Request) (code, state string, err error) {
	if request == nil || request.URL == nil || len(request.URL.Query()) != 2 || len(request.URL.Query()["code"]) != 1 || len(request.URL.Query()["state"]) != 1 {
		return "", "", sidebarapp.ErrOAuthAttemptInvalid
	}
	return request.URL.Query().Get("code"), request.URL.Query().Get("state"), nil
}

func clearSidebarOAuthBinding(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{Name: sidebarOAuthBindingCookie, Value: "", Path: sidebarOAuthCookiePath, MaxAge: -1, Expires: time.Unix(1, 0).UTC(), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func oauthResponseHeaders(writer http.ResponseWriter) {
	if writer == nil {
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
}

func nilValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface) && reflected.IsNil()
}

func ValidContextToken(value string) bool {
	return len(value) >= 64 && len(value) <= 4096 && strings.TrimSpace(value) == value
}
