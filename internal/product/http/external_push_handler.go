package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

const (
	WeChatPayExternalPushPathPattern     = "/api/admin/wechat-pay/products/{product_id}/external-push"
	ServicePeriodExternalPushPathPattern = "/api/admin/service-period-products/{product_id}/external-push"
)

// ExternalPushAuthorizer and ExternalPushCSRFVerifier are injected by the
// central composition lane. The fragment is deliberately not registered here.
type ExternalPushAuthorizer interface {
	Authorize(context.Context, authport.Capability) (authport.Principal, error)
}

type ExternalPushCSRFVerifier interface {
	Verify(*http.Request, authport.Principal) error
}

type ExternalPushHandler struct {
	application productport.CommerceExternalPushApplication
	authorizer  ExternalPushAuthorizer
	csrf        ExternalPushCSRFVerifier
}

func NewExternalPushHandler(application productport.CommerceExternalPushApplication, authorizer ExternalPushAuthorizer, csrf ExternalPushCSRFVerifier) (*ExternalPushHandler, error) {
	if nilInterface(application) || nilInterface(authorizer) || nilInterface(csrf) {
		return nil, productapp.ErrUnavailable
	}
	return &ExternalPushHandler{application: application, authorizer: authorizer, csrf: csrf}, nil
}

// NewExternalPushRouteFragment exposes this unregistered Product-owned route
// set to the later central composition lane.
func NewExternalPushRouteFragment(handler *ExternalPushHandler) (http.Handler, error) {
	if handler == nil || nilInterface(handler.application) || nilInterface(handler.authorizer) || nilInterface(handler.csrf) {
		return nil, productapp.ErrUnavailable
	}
	return handler, nil
}

type ExternalPushRoute struct {
	Method, Pattern string
	RequiresCSRF    bool
}

func (handler *ExternalPushHandler) Routes() []ExternalPushRoute {
	return []ExternalPushRoute{
		{http.MethodGet, WeChatPayExternalPushPathPattern, false},
		{http.MethodPut, WeChatPayExternalPushPathPattern, true},
		{http.MethodPost, WeChatPayExternalPushPathPattern + "/test", true},
		{http.MethodGet, ServicePeriodExternalPushPathPattern, false},
		{http.MethodPut, ServicePeriodExternalPushPathPattern, true},
		{http.MethodPost, ServicePeriodExternalPushPathPattern + "/test", true},
	}
}

func (handler *ExternalPushHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilInterface(handler.application) || nilInterface(handler.authorizer) || nilInterface(handler.csrf) || request == nil || request.URL == nil {
		writeLocalError(writer, request, productapp.ErrUnavailable)
		return
	}
	if request.URL.RawQuery != "" || request.URL.ForceQuery || request.URL.RawPath != "" || request.URL.EscapedPath() != request.URL.Path ||
		strings.Contains(request.URL.Path, "\\") || strings.HasSuffix(request.URL.Path, "/") {
		writeLocalError(writer, request, productapp.ErrInvalidProduct)
		return
	}
	kind, productID, action, ok := externalPushPath(request.URL.Path)
	if !ok {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, productapp.ErrNotFound))
		return
	}
	switch action {
	case "configuration":
		switch request.Method {
		case http.MethodGet:
			handler.get(writer, request, productID, kind)
		case http.MethodPut:
			handler.save(writer, request, productID, kind)
		default:
			writeExternalPushMethodNotAllowed(writer, http.MethodGet+", "+http.MethodPut)
		}
	case "test":
		if request.Method != http.MethodPost {
			writeExternalPushMethodNotAllowed(writer, http.MethodPost)
			return
		}
		handler.test(writer, request, productID, kind)
	default:
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeNotFound, productapp.ErrNotFound))
	}
}

func (handler *ExternalPushHandler) get(writer http.ResponseWriter, request *http.Request, productID productport.ID, kind productport.ExternalPushProductKind) {
	if _, _, ok := handler.authorize(writer, request, authport.CapabilityProductsRead, false); !ok {
		return
	}
	if !externalPushEmptyBody(request) {
		writeLocalError(writer, request, productapp.ErrInvalidProduct)
		return
	}
	configuration, err := handler.application.GetExternalPushConfiguration(request.Context(), productID, kind)
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	response, err := externalPushConfigurationResponse(configuration)
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	write(writer, http.StatusOK, response)
}

type externalPushSaveRequest struct {
	Enabled                *bool   `json:"enabled"`
	ConfigurationReference *string `json:"configuration_reference"`
}

func (handler *ExternalPushHandler) save(writer http.ResponseWriter, request *http.Request, productID productport.ID, kind productport.ExternalPushProductKind) {
	principal, key, ok := handler.authorize(writer, request, authport.CapabilityProductsWrite, true)
	if !ok {
		return
	}
	var body externalPushSaveRequest
	if err := decodeLocalBody(writer, request, &body); err != nil || body.Enabled == nil {
		writeLocalError(writer, request, productapp.ErrInvalidProduct)
		return
	}
	reference := ""
	if body.ConfigurationReference != nil {
		reference = *body.ConfigurationReference
	}
	configuration, err := handler.application.SaveExternalPushConfiguration(request.Context(), productport.SaveExternalPushConfigurationCommand{
		ProductID: productID, ProductKind: kind, Enabled: *body.Enabled, ConfigurationReference: reference,
		Actor: principal.AdminUserID, IdempotencyKey: key,
	})
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	response, err := externalPushConfigurationResponse(configuration)
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	write(writer, http.StatusOK, response)
}

func (handler *ExternalPushHandler) test(writer http.ResponseWriter, request *http.Request, productID productport.ID, kind productport.ExternalPushProductKind) {
	principal, key, ok := handler.authorize(writer, request, authport.CapabilityProductsWrite, true)
	if !ok {
		return
	}
	if !externalPushEmptyBody(request) {
		writeLocalError(writer, request, productapp.ErrInvalidProduct)
		return
	}
	result, err := handler.application.QueueExternalPushTest(request.Context(), productport.QueueExternalPushTestCommand{
		ProductID: productID, ProductKind: kind, Actor: principal.AdminUserID, IdempotencyKey: key,
	})
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	response, err := externalPushTestResponse(result)
	if err != nil {
		writeLocalError(writer, request, err)
		return
	}
	write(writer, http.StatusAccepted, response)
}

func (handler *ExternalPushHandler) authorize(writer http.ResponseWriter, request *http.Request, capability authport.Capability, csrf bool) (authport.Principal, string, bool) {
	principal, err := handler.authorizer.Authorize(request.Context(), capability)
	if err != nil || principal.AdminUserID < 1 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
		writeExternalPushAuthorizationError(writer, request, err)
		return authport.Principal{}, "", false
	}
	if !csrf {
		return principal, "", true
	}
	if err := handler.csrf.Verify(request, principal); err != nil {
		writeExternalPushAuthorizationError(writer, request, err)
		return authport.Principal{}, "", false
	}
	keys := request.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !validLocalIdempotencyKey(keys[0]) {
		writeLocalError(writer, request, productapp.ErrInvalidProduct)
		return authport.Principal{}, "", false
	}
	return principal, keys[0], true
}

type ExternalPushConfigurationResponse struct {
	ProductID              int64  `json:"product_id"`
	ProductKind            string `json:"product_kind"`
	Enabled                bool   `json:"enabled"`
	ConfigurationReference string `json:"configuration_reference,omitempty"`
	UpdatedAt              string `json:"updated_at"`
}

type ExternalPushTestResponse struct {
	ProductID                int64  `json:"product_id"`
	ProductKind              string `json:"product_kind"`
	EffectID                 string `json:"effect_id"`
	State                    string `json:"state"`
	ProviderAccepted         bool   `json:"provider_accepted"`
	DeliveryProven           bool   `json:"delivery_proven"`
	RealExternalCallExecuted bool   `json:"real_external_call_executed"`
	AutoRetryAllowed         bool   `json:"auto_retry_allowed"`
	CreatedAt                string `json:"created_at"`
}

func externalPushConfigurationResponse(value productport.ExternalPushConfiguration) (ExternalPushConfigurationResponse, error) {
	if value.ProductID < 1 || (value.ProductKind != productport.ExternalPushWeChatPay && value.ProductKind != productport.ExternalPushServicePeriod) || value.UpdatedAt.IsZero() ||
		(!value.Enabled && value.ConfigurationReference != "") || (value.Enabled && value.ConfigurationReference == "") {
		return ExternalPushConfigurationResponse{}, productapp.ErrUnavailable
	}
	return ExternalPushConfigurationResponse{ProductID: int64(value.ProductID), ProductKind: string(value.ProductKind), Enabled: value.Enabled,
		ConfigurationReference: value.ConfigurationReference, UpdatedAt: value.UpdatedAt.UTC().Format(timeRFC3339Nano)}, nil
}

func externalPushTestResponse(value productport.ExternalPushTest) (ExternalPushTestResponse, error) {
	if value.ProductID < 1 || (value.ProductKind != productport.ExternalPushWeChatPay && value.ProductKind != productport.ExternalPushServicePeriod) ||
		!externalPushEffectID(value.EffectID) || (value.State != "accepted" && value.State != "queued") || value.ProviderAccepted || value.DeliveryProven ||
		value.RealExternalCallExecuted || value.AutoRetryAllowed || value.CreatedAt.IsZero() {
		return ExternalPushTestResponse{}, productapp.ErrUnavailable
	}
	return ExternalPushTestResponse{ProductID: int64(value.ProductID), ProductKind: string(value.ProductKind), EffectID: value.EffectID,
		State: value.State, ProviderAccepted: false, DeliveryProven: false, RealExternalCallExecuted: false, AutoRetryAllowed: false,
		CreatedAt: value.CreatedAt.UTC().Format(timeRFC3339Nano)}, nil
}

func externalPushPath(path string) (productport.ExternalPushProductKind, productport.ID, string, bool) {
	for _, candidate := range []struct {
		prefix string
		kind   productport.ExternalPushProductKind
	}{
		{"/api/admin/wechat-pay/products/", productport.ExternalPushWeChatPay},
		{"/api/admin/service-period-products/", productport.ExternalPushServicePeriod},
	} {
		if !strings.HasPrefix(path, candidate.prefix) {
			continue
		}
		tail := strings.TrimPrefix(path, candidate.prefix)
		action := "configuration"
		if strings.HasSuffix(tail, "/external-push/test") {
			tail, action = strings.TrimSuffix(tail, "/external-push/test"), "test"
		} else if strings.HasSuffix(tail, "/external-push") {
			tail = strings.TrimSuffix(tail, "/external-push")
		} else {
			continue
		}
		if !externalPushID(tail) {
			return "", 0, "", false
		}
		id, err := strconv.ParseInt(tail, 10, 64)
		if err == nil && id > 0 {
			return candidate.kind, productport.ID(id), action, true
		}
	}
	return "", 0, "", false
}

func externalPushID(value string) bool {
	if value == "" || value[0] == '0' || len(value) > 19 {
		return false
	}
	for _, rune := range value {
		if rune < '0' || rune > '9' {
			return false
		}
	}
	return true
}

func externalPushEffectID(value string) bool {
	if !strings.HasPrefix(value, "eer_") || len(value) == 4 || value[4] == '0' {
		return false
	}
	for _, rune := range value[4:] {
		if rune < '0' || rune > '9' {
			return false
		}
	}
	return true
}

func externalPushEmptyBody(request *http.Request) bool {
	if request == nil || request.Body == nil || request.Body == http.NoBody {
		return true
	}
	var byte [1]byte
	count, err := request.Body.Read(byte[:])
	return count == 0 && errors.Is(err, io.EOF)
}

func writeExternalPushAuthorizationError(writer http.ResponseWriter, request *http.Request, err error) {
	code := platformhttp.CodeUnauthorized
	if errors.Is(err, authport.ErrUnauthenticated) {
		code = platformhttp.CodeUnauthenticated
	} else if err != nil && !errors.Is(err, authport.ErrUnauthorized) && !errors.Is(err, authport.ErrCSRFInvalid) {
		code = platformhttp.CodeDependencyUnavailable
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, authport.ErrUnauthorized))
}

func writeExternalPushMethodNotAllowed(writer http.ResponseWriter, allow string) {
	writer.Header().Set("Allow", allow)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}
