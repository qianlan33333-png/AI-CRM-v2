package main

import (
	"net/http"

	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type surveySafeAdminHTTP interface {
	Results(http.ResponseWriter, *http.Request)
	ExportPreview(http.ResponseWriter, *http.Request)
}

func (handler *Handler) GetQuestionnaireSafeAnalysis(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.surveySafeAdmin == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.surveySafeAdmin.Results(writer, request)
}

func (handler *Handler) PreviewQuestionnaireSafeExport(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.surveySafeAdmin == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.surveySafeAdmin.ExportPreview(writer, request)
}
