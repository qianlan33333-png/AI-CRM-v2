package main

import (
	"net/http"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

func (handler *candidateHandler) GetSurveyExternalPushDetail(writer http.ResponseWriter, request *http.Request, questionnaireID api.QuestionnaireID, submissionID api.SurveySubmissionID) {
	if handler == nil || handler.surveyExternalPushDetail == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.surveyExternalPushDetail.Get(writer, request, surveyport.ID(questionnaireID), submissionID)
}
