package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const maximumIdentityConsoleBodyBytes = 1 << 20

// ConsoleApplication deliberately excludes ingest. An operator-entered
// free-form event payload has no closed browser contract in this package.
type ConsoleApplication interface {
	Resolve(context.Context, identityport.IDRef) (identityport.ResolveResult, error)
	Bind(context.Context, identityport.BindCommand) (identityport.BindResult, error)
}

type ConsoleHandler struct{ application ConsoleApplication }

func NewConsoleHandler(application ConsoleApplication) (*ConsoleHandler, error) {
	if nilConsoleApplication(application) {
		return nil, identityapp.ErrIdentityResolveFailed
	}
	return &ConsoleHandler{application: application}, nil
}

func (handler *ConsoleHandler) ResolveIdentity(writer http.ResponseWriter, request *http.Request) {
	if _, err := handler.operation(request, authport.CapabilityIdentityResolve); err != nil {
		writeConsoleError(writer, request, err)
		return
	}
	if request.Method != http.MethodPost {
		consoleMethod(writer, http.MethodPost)
		return
	}
	var body generated.ResolveIdentityRequest
	if err := decodeConsoleBody(writer, request, &body); err != nil {
		writeConsoleError(writer, request, err)
		return
	}
	ref, err := consoleIdentityRef(body.Ref)
	if err != nil {
		writeConsoleError(writer, request, err)
		return
	}
	result, err := handler.application.Resolve(request.Context(), ref)
	if err != nil {
		writeConsoleError(writer, request, err)
		return
	}
	response, err := resolveConsoleResponse(result)
	if err != nil {
		writeConsoleError(writer, request, err)
		return
	}
	writeConsoleJSON(writer, http.StatusOK, response)
}

func (handler *ConsoleHandler) BindIdentity(writer http.ResponseWriter, request *http.Request, params generated.BindIdentityParams) {
	principal, err := handler.operation(request, authport.CapabilityIdentityBind)
	if err != nil {
		writeConsoleError(writer, request, err)
		return
	}
	if request.Method != http.MethodPost {
		consoleMethod(writer, http.MethodPost)
		return
	}
	if !validConsoleKey(string(params.IdempotencyKey)) {
		writeConsoleError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, identityapp.ErrIdentityBindFailed))
		return
	}
	var body generated.BindIdentityRequest
	if err = decodeConsoleBody(writer, request, &body); err != nil {
		writeConsoleError(writer, request, err)
		return
	}
	ref, err := consoleIdentityRef(body.Ref)
	if err != nil {
		writeConsoleError(writer, request, err)
		return
	}
	result, err := handler.application.Bind(request.Context(), identityport.BindCommand{
		CustomerID:     contactport.CustomerID(body.CustomerId),
		Ref:            ref,
		Actor:          contactport.Actor(fmt.Sprintf("admin:%d", principal.AdminUserID)),
		IdempotencyKey: string(params.IdempotencyKey),
	})
	if err != nil {
		writeConsoleError(writer, request, err)
		return
	}
	response, err := bindConsoleResponse(result)
	if err != nil {
		writeConsoleError(writer, request, err)
		return
	}
	writeConsoleJSON(writer, http.StatusOK, response)
}

func (handler *ConsoleHandler) operation(request *http.Request, capability authport.Capability) (authport.Principal, error) {
	if request == nil {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrIdentityResolveFailed)
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != capability || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID <= 0 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	if handler == nil || nilConsoleApplication(handler.application) {
		return authport.Principal{}, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrIdentityResolveFailed)
	}
	return principal, nil
}

func decodeConsoleBody(writer http.ResponseWriter, request *http.Request, target any) error {
	if request == nil || request.Body == nil || target == nil || !strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "application/json") {
		return platformhttp.NewError(platformhttp.CodeMalformedRequest, identityapp.ErrInvalidIdentity)
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maximumIdentityConsoleBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return platformhttp.NewError(platformhttp.CodeMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return platformhttp.NewError(platformhttp.CodeMalformedRequest, err)
	}
	return nil
}

func consoleIdentityRef(value generated.IdentityRef) (identityport.IDRef, error) {
	kind, ok := map[generated.IdentityRefType]identityport.IDKind{
		generated.IdentityRefTypeWecomExternalUserid: identityport.KindWeComExternalUserID,
		generated.IdentityRefTypeUnionid:             identityport.KindUnionID,
		generated.IdentityRefTypeMpOpenid:            identityport.KindMPOpenID,
		generated.IdentityRefTypeOaOpenid:            identityport.KindOAOpenID,
		generated.IdentityRefTypeAlipayUserId:        identityport.KindAlipayUserID,
		generated.IdentityRefTypePhone:               identityport.KindPhone,
		generated.IdentityRefTypeExt:                 identityport.KindExtension,
	}[value.Type]
	if !ok {
		return identityport.IDRef{}, platformhttp.NewError(platformhttp.CodeValidationFailed, identityapp.ErrInvalidIdentity)
	}
	return identityport.IDRef{Kind: kind, Scope: value.Scope, Value: value.Value, Assurance: identityport.AssuranceDeclared, Source: "admin"}, nil
}

func resolveConsoleResponse(result identityport.ResolveResult) (any, error) {
	switch result.Status {
	case identityport.ResolveFound:
		if result.CustomerID < 1 {
			break
		}
		return struct {
			Status     string `json:"status"`
			CustomerID int64  `json:"customer_id"`
		}{Status: "found", CustomerID: int64(result.CustomerID)}, nil
	case identityport.ResolveNotFound:
		if result.CustomerID == 0 {
			return struct {
				Status string `json:"status"`
			}{Status: "not_found"}, nil
		}
	case identityport.ResolveConflict:
		if result.CustomerID == 0 {
			return struct {
				Status string `json:"status"`
			}{Status: "conflict"}, nil
		}
	}
	return nil, identityapp.ErrIdentityResolveFailed
}

func bindConsoleResponse(result identityport.BindResult) (any, error) {
	switch result.Status {
	case identityport.BindBound, identityport.BindAlreadyBound:
		if result.CustomerID < 1 || result.PrimaryCustomerID != 0 || result.MergeAuditID != 0 || result.ReviewID != 0 {
			break
		}
		return struct {
			Status     string `json:"status"`
			CustomerID int64  `json:"customer_id"`
		}{Status: string(result.Status), CustomerID: int64(result.CustomerID)}, nil
	case identityport.BindRejected:
		if result.CustomerID == 0 && result.PrimaryCustomerID == 0 && result.MergeAuditID == 0 && result.ReviewID == 0 {
			return struct {
				Status string `json:"status"`
			}{Status: "rejected"}, nil
		}
	}
	return nil, identityapp.ErrIdentityBindFailed
}

func validConsoleKey(value string) bool {
	return value == strings.TrimSpace(value) && len(value) >= 16 && len(value) <= 128
}

func writeConsoleError(writer http.ResponseWriter, request *http.Request, err error) {
	if request == nil {
		return
	}
	if platformhttp.ErrorCodeOf(err) != platformhttp.CodeInternal {
		platformhttp.WriteError(writer, request, err)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, identityapp.ErrInvalidIdentity):
		code = platformhttp.CodeValidationFailed
	case errors.Is(err, identityapp.ErrIdentityBindIdempotencyConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func writeConsoleJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func consoleMethod(writer http.ResponseWriter, allow string) {
	writer.Header().Set("Allow", allow)
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func nilConsoleApplication(application ConsoleApplication) bool {
	if application == nil {
		return true
	}
	value := reflect.ValueOf(application)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
