package main

import (
	"net/http"

	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

// surveyOperationsHTTP stays at the transport boundary so the compatibility
// router can enforce its existing session, capability, and CSRF middleware.
// The concrete handler owns only local Survey Operations data.
type surveyOperationsHTTP interface {
	GetOperations(http.ResponseWriter, *http.Request)
	GetOperationsPage(http.ResponseWriter, *http.Request)
	SaveCompletion(http.ResponseWriter, *http.Request)
	SaveExternalPush(http.ResponseWriter, *http.Request)
	QueueExternalPushTest(http.ResponseWriter, *http.Request)
	ListGlobalExternalPushLogs(http.ResponseWriter, *http.Request)
	ListQuestionnaireExternalPushLogs(http.ResponseWriter, *http.Request)
}

func (handler *Handler) GetSurveyOperations(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.surveyOperations == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.surveyOperations.GetOperations(writer, request)
}

func (handler *Handler) GetSurveyOperationsPage(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.surveyOperations == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.surveyOperations.GetOperationsPage(writer, request)
}

func (handler *Handler) SaveSurveyCompletionOperations(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.surveyOperations == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.surveyOperations.SaveCompletion(writer, request)
}

func (handler *Handler) SaveSurveyExternalPushOperations(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.surveyOperations == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.surveyOperations.SaveExternalPush(writer, request)
}

func (handler *Handler) QueueSurveyExternalPushTest(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.surveyOperations == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.surveyOperations.QueueExternalPushTest(writer, request)
}

func (handler *Handler) ListSurveyExternalPushLogs(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.surveyOperations == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.surveyOperations.ListGlobalExternalPushLogs(writer, request)
}

func (handler *Handler) ListSurveyQuestionnaireExternalPushLogs(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.surveyOperations == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, nil))
		return
	}
	handler.surveyOperations.ListQuestionnaireExternalPushLogs(writer, request)
}
