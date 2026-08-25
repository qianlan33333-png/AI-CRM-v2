package main

import (
	"net/http"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func (handler *candidateHandler) ReconcileSurveyExternalPush(writer http.ResponseWriter, request *http.Request, questionnaireID api.QuestionnaireID, submissionID api.SurveySubmissionID, _ api.ReconcileSurveyExternalPushParams) {
	if handler == nil || handler.surveyPushReconcile == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.surveyPushReconcile.Reconcile(writer, request, surveyport.ID(questionnaireID), submissionID)
}
