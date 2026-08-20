package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/http"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type routerSurveyService struct {
	publish surveyport.PublishPublicDefinitionCommand
}

func (*routerSurveyService) Definition(context.Context, string) (surveyport.PublicQuestionnaire, error) {
	return surveyport.PublicQuestionnaire{ID: 1, Slug: "public", Title: "匿名问卷", AnswerDisplayMode: surveyport.AllInOne, Version: 1, Questions: []surveyport.PublicQuestion{{ID: 2, Type: surveyport.SingleChoice, Title: "请选择", Required: true, SortOrder: 0, Minimum: 1, Maximum: 1, Options: []surveyport.PublicOption{{ID: 3, OptionText: "A", SortOrder: 0}}}}}, nil
}
func (*routerSurveyService) Submit(context.Context, surveyport.PublicSubmissionCommand) (surveyport.PublicSubmissionReceipt, string, error) {
	return surveyport.PublicSubmissionReceipt{}, "", surveyapp.ErrPublicUnavailable
}
func (*routerSurveyService) Result(context.Context, string) (surveyport.PublicSubmissionResult, error) {
	return surveyport.PublicSubmissionResult{}, surveyapp.ErrNotFound
}
func (service *routerSurveyService) Publish(_ context.Context, command surveyport.PublishPublicDefinitionCommand) (surveyapp.PublicDefinitionRecord, error) {
	service.publish = command
	return surveyapp.PublicDefinitionRecord{State: "public", View: surveyport.PublicQuestionnaire{ID: command.QuestionnaireID, Slug: "public", Version: 1}}, nil
}
func (*routerSurveyService) Disable(_ context.Context, command surveyport.DisablePublicDefinitionCommand) (surveyapp.PublicDefinitionRecord, error) {
	return surveyapp.PublicDefinitionRecord{State: "disabled", View: surveyport.PublicQuestionnaire{ID: command.QuestionnaireID, Slug: "public", Version: command.ExpectedDefinitionVersion}}, nil
}
func (*routerSurveyService) Analytics(_ context.Context, questionnaireID surveyport.ID, version int64) (surveyport.PublicAnalytics, error) {
	return surveyport.PublicAnalytics{QuestionnaireID: questionnaireID, DefinitionVersion: version, Slug: "public", State: "public", Questions: []surveyport.PublicAnalyticsQuestion{}}, nil
}

func TestDeriveSurveyPublicKeysSeparatesDomainsAndFailsClosed(t *testing.T) {
	zeroToken, zeroCookie, zeroAbuse := deriveSurveyPublicKeys(make([]byte, 32))
	if zeroToken != [32]byte{} || zeroCookie != [32]byte{} || zeroAbuse != [32]byte{} {
		t.Fatal("zero Survey root key enabled public operations")
	}
	root := bytes.Repeat([]byte{7}, 32)
	token, cookie, abuse := deriveSurveyPublicKeys(root)
	if token == [32]byte{} || cookie == [32]byte{} || abuse == [32]byte{} || token == cookie || token == abuse || cookie == abuse {
		t.Fatalf("Survey subkeys were not nonzero and domain-separated: %x %x %x", token, cookie, abuse)
	}
}

func TestSurveyPublicMissingServiceKeepsPublicErrorContract(t *testing.T) {
	response := httptest.NewRecorder()
	(&candidateHandler{}).GetPublicSurveyDefinition(response, httptest.NewRequest(http.MethodGet, "/api/public/questionnaires/public", nil), "public")
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"code\":\"unavailable\"}\n" || response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("status/body/headers=%d/%q/%v", response.Code, response.Body.String(), response.Header())
	}
}

func TestSurveyPublicRoutesStayOutsideAuthenticationAndKeepExactMethodGuard(t *testing.T) {
	service := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	var key [32]byte
	key[0] = 1
	candidate := &candidateHandler{Handler: authHandler, surveyPublic: surveyhttp.NewPublicHandler(&routerSurveyService{}, key, key)}
	handler, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, candidate)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		method, path string
		want         int
		allow        string
	}{
		{http.MethodGet, "/api/public/questionnaires/public", http.StatusOK, ""},
		{http.MethodPost, "/api/public/questionnaires/public", http.StatusMethodNotAllowed, http.MethodGet},
		{http.MethodGet, "/q/public", http.StatusFound, ""},
		{http.MethodGet, "/q/Public", http.StatusBadRequest, ""},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != test.want || response.Header().Get("Allow") != test.allow || len(service.capabilities()) != 0 {
			t.Fatalf("%s %s status/allow/auth=%d/%q/%v", test.method, test.path, response.Code, response.Header().Get("Allow"), service.capabilities())
		}
		if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("unsafe public headers=%v", response.Header())
		}
		if test.path == "/q/public" && (response.Header().Get("Location") != "/?public_survey_slug=public" || response.Body.Len() != 0) {
			t.Fatalf("carrier location/body=%q/%q", response.Header().Get("Location"), response.Body.String())
		}
	}
}

func TestSurveyPublicAdminRoutesUseQuestionnaireAuthCSRFAndActor(t *testing.T) {
	authService := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(authService)
	if err != nil {
		t.Fatal(err)
	}
	surveyService := &routerSurveyService{}
	var key [32]byte
	key[0] = 1
	candidate := &candidateHandler{Handler: authHandler, surveyPublic: surveyhttp.NewPublicHandler(surveyService, key, key)}
	handler, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, candidate)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/admin/questionnaires/7/public-publish", strings.NewReader(`{"expected_questionnaire_version":2}`))
	request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "survey-router-session"})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
	request.Header.Set("Idempotency-Key", "survey-router-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || surveyService.publish.Actor != 1 || surveyService.publish.QuestionnaireID != 7 || surveyService.publish.ExpectedQuestionnaireVersion != 2 || surveyService.publish.IdempotencyKey != "survey-router-key" {
		t.Fatalf("publish status/command=%d/%+v body=%s", response.Code, surveyService.publish, response.Body.String())
	}
	if got := authService.capabilities(); len(got) != 1 || got[0] != "questionnaires.write" {
		t.Fatalf("capabilities=%v", got)
	}
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("Referrer-Policy") != "same-origin" {
		t.Fatalf("unsafe admin headers=%v", response.Header())
	}

	authService.reset()
	analytics := httptest.NewRequest(http.MethodGet, "/api/admin/questionnaires/7/public-analytics?definition_version=1", nil)
	analytics.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "survey-router-session"})
	analyticsResponse := httptest.NewRecorder()
	handler.ServeHTTP(analyticsResponse, analytics)
	if analyticsResponse.Code != http.StatusOK || len(authService.capabilities()) != 1 || authService.capabilities()[0] != "questionnaires.read" {
		t.Fatalf("analytics status/auth=%d/%v", analyticsResponse.Code, authService.capabilities())
	}
}
