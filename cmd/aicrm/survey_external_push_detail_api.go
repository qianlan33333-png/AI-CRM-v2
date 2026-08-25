package main

import (
	"net/http"

	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	surveyhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/http"
)

func (handler *candidateHandler) GetSurveyExternalPushDetail(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.surveyExternalPushDetail == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	questionnaireID, submissionID, ok := surveyhttp.ParseExternalPushDetailPath(request.URL.Path)
	if !ok {
		handler.surveyExternalPushDetail.Get(writer, request, 0, 0)
		return
	}
	handler.surveyExternalPushDetail.Get(writer, request, questionnaireID, submissionID)
}
