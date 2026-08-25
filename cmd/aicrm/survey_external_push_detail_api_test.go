package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/http"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type surveyExternalPushDetailRouteStub struct {
	binding surveyapp.ExternalPushBinding
	err     error
	calls   int
}

func (s *surveyExternalPushDetailRouteStub) Detail(_ context.Context, _ surveyport.ID, _ int64) (surveyapp.ExternalPushBinding, error) {
	s.calls++
	return s.binding, s.err
}

func TestSurveyExternalPushDetailRouteUsesQuestionnaireReadAndKeepsResponseSafe(t *testing.T) {
	authService := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(authService)
	if err != nil {
		t.Fatal(err)
	}
	app := &surveyExternalPushDetailRouteStub{binding: surveyapp.ExternalPushBinding{
		QuestionnaireID: 9, SubmissionID: 12, CustomerID: 77, EffectID: "eer_123", State: eer.StateAccepted,
		SourceRefDigest: "sha256:source", TargetRefDigest: "sha256:target", PayloadDigest: "sha256:payload", PolicyVersionHash: "sha256:policy",
	}}
	candidate := &candidateHandler{Handler: authHandler, surveyExternalPushDetail: &surveyhttp.ExternalPushDetailHandler{Application: app}}
	router, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, candidate)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/questionnaires/9/submissions/12/external-push", nil)
	request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "survey-detail-session"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || app.calls != 1 {
		t.Fatalf("status/calls=%d/%d body=%s", response.Code, app.calls, response.Body.String())
	}
	if got := authService.capabilities(); len(got) != 1 || got[0] != authport.CapabilityQuestionnairesRead {
		t.Fatalf("capabilities=%v", got)
	}
	for _, forbidden := range []string{"digest", "identity", "receipt", "customer_id"} {
		if body := response.Body.String(); strings.Contains(body, forbidden) {
			t.Fatalf("response exposed %q: %s", forbidden, body)
		}
	}
}

func TestSurveyExternalPushDetailRouteRejectsAnonymousInvalidAndMissingBinding(t *testing.T) {
	authService := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(authService)
	if err != nil {
		t.Fatal(err)
	}
	app := &surveyExternalPushDetailRouteStub{}
	candidate := &candidateHandler{Handler: authHandler, surveyExternalPushDetail: &surveyhttp.ExternalPushDetailHandler{Application: app}}
	router, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, candidate)
	if err != nil {
		t.Fatal(err)
	}

	anonymous := httptest.NewRecorder()
	router.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/admin/questionnaires/9/submissions/12/external-push", nil))
	if anonymous.Code != http.StatusUnauthorized || app.calls != 0 {
		t.Fatalf("anonymous status/calls=%d/%d", anonymous.Code, app.calls)
	}

	for _, test := range []struct {
		path string
		want int
	}{
		{"/api/admin/questionnaires/0/submissions/12/external-push", http.StatusBadRequest},
		{"/api/admin/questionnaires/9/submissions/12/external-push", http.StatusNotFound},
	} {
		if test.want == http.StatusNotFound {
			app.err = errors.New("missing binding")
		}
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "survey-detail-session"})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("%s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
}
