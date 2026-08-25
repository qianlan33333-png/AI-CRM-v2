package main

import (
	"fmt"
	"net/http"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func (handler *candidateHandler) ListExternalEffectsRuntime(writer http.ResponseWriter, request *http.Request, params api.ListExternalEffectsRuntimeParams) {
	if !handler.externalEffectsRuntimeAvailable(writer, request) {
		return
	}
	limit := int32(50)
	if params.Limit != nil {
		limit = *params.Limit
	}
	handler.externalEffectsRuntime.List(writer, request, limit)
}

func (handler *candidateHandler) GetExternalEffectRuntime(writer http.ResponseWriter, request *http.Request, effectID api.ExternalEffectRuntimeID) {
	if !handler.externalEffectsRuntimeAvailable(writer, request) {
		return
	}
	handler.externalEffectsRuntime.Detail(writer, request, effectID)
}

func (handler *candidateHandler) GetExternalEffectsDiagnostics(writer http.ResponseWriter, request *http.Request) {
	if !handler.externalEffectsRuntimeAvailable(writer, request) {
		return
	}
	handler.externalEffectsRuntime.Diagnostics(writer, request)
}

func (handler *candidateHandler) CancelExternalEffectRuntime(writer http.ResponseWriter, request *http.Request, effectID api.ExternalEffectRuntimeID, _ api.CancelExternalEffectRuntimeParams) {
	request, ok := handler.externalEffectsRuntimeMutation(writer, request)
	if !ok {
		return
	}
	handler.externalEffectsRuntime.Cancel(writer, request, effectID)
}

func (handler *candidateHandler) RetryExternalEffectRuntime(writer http.ResponseWriter, request *http.Request, effectID api.ExternalEffectRuntimeID, _ api.RetryExternalEffectRuntimeParams) {
	request, ok := handler.externalEffectsRuntimeMutation(writer, request)
	if !ok {
		return
	}
	handler.externalEffectsRuntime.Retry(writer, request, effectID)
}

func (handler *candidateHandler) ReconcileExternalEffectRuntime(writer http.ResponseWriter, request *http.Request, effectID api.ExternalEffectRuntimeID, _ api.ReconcileExternalEffectRuntimeParams) {
	request, ok := handler.externalEffectsRuntimeMutation(writer, request)
	if !ok {
		return
	}
	handler.externalEffectsRuntime.Reconcile(writer, request, effectID)
}

func (handler *candidateHandler) externalEffectsRuntimeAvailable(writer http.ResponseWriter, request *http.Request) bool {
	if handler != nil && handler.externalEffectsRuntime != nil {
		return true
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
	return false
}

func (handler *candidateHandler) externalEffectsRuntimeMutation(writer http.ResponseWriter, request *http.Request) (*http.Request, bool) {
	if !handler.externalEffectsRuntimeAvailable(writer, request) {
		return request, false
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	keys := request.Header.Values("Idempotency-Key")
	if !ok || principal.AdminUserID < 1 || len(keys) != 1 || keys[0] == "" {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, nil))
		return request, false
	}
	bound := request.Clone(request.Context())
	bound.Header = request.Header.Clone()
	bound.Header.Set("Idempotency-Key", fmt.Sprintf("admin:%d:%s", principal.AdminUserID, keys[0]))
	return bound, true
}
