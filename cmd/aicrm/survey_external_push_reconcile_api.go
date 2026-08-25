package main

import (
	"net/http"

	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	surveyhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/http"
)

func (handler *candidateHandler) ReconcileSurveyExternalPush(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.surveyPushReconcile == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	questionnaireID, submissionID, ok := surveyhttp.ParseExternalPushReconcilePath(request.URL.Path)
	if !ok {
		handler.surveyPushReconcile.Reconcile(writer, request, 0, 0)
		return
	}
	handler.surveyPushReconcile.Reconcile(writer, request, questionnaireID, submissionID)
}
