package main

import (
	"net/http"
	"net/url"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
)

func (handler *candidateHandler) StartSurveyH5OAuth(writer http.ResponseWriter, request *http.Request, params api.StartSurveyH5OAuthParams) {
	if handler == nil || handler.surveyH5OAuth == nil {
		writeSurveyPublicUnavailable(writer)
		return
	}
	request.URL.RawQuery = url.Values{"next": []string{params.Next}}.Encode()
	handler.surveyH5OAuth.Start(writer, request)
}

func (handler *candidateHandler) CallbackSurveyH5OAuth(writer http.ResponseWriter, request *http.Request, params api.CallbackSurveyH5OAuthParams) {
	if handler == nil || handler.surveyH5OAuth == nil {
		writeSurveyPublicUnavailable(writer)
		return
	}
	request.URL.RawQuery = url.Values{"state": []string{params.State}, "code": []string{params.Code}}.Encode()
	handler.surveyH5OAuth.Callback(writer, request)
}
