package main

import (
	"net/http"

	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type externalEffectsHTTP interface {
	Jobs(http.ResponseWriter, *http.Request)
	Diagnostics(http.ResponseWriter, *http.Request)
}

func (handler *Handler) ExternalEffectsJobs(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.externalEffects == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.externalEffects.Jobs(writer, request)
}

func (handler *Handler) ExternalEffectsDiagnostics(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.externalEffects == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.externalEffects.Diagnostics(writer, request)
}
