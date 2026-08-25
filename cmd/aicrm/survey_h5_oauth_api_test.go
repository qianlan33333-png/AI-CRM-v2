package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/http"
)

type surveyH5OAuthRouteStub struct{}

func (surveyH5OAuthRouteStub) Start(context.Context, string) (string, time.Time, error) {
	return "https://provider.example/authorize", time.Now().UTC().Add(time.Minute), nil
}

func (surveyH5OAuthRouteStub) Callback(context.Context, string, string) (surveyapp.H5CanonicalIdentity, string, error) {
	return surveyapp.H5CanonicalIdentity{CustomerID: 1, ExpiresAt: time.Now().UTC().Add(time.Minute)}, "/s/survey-1", nil
}

func TestSurveyH5OAuthRoutesArePublicGeneratedCandidateRoutes(t *testing.T) {
	authHandler, err := authhttp.NewHandler(&recordingAuth{})
	if err != nil {
		t.Fatal(err)
	}
	candidate := &candidateHandler{Handler: authHandler, surveyH5OAuth: surveyhttp.NewH5OAuthHandler(surveyH5OAuthRouteStub{}, [32]byte{1})}
	router, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, candidate)
	if err != nil {
		t.Fatal(err)
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(http.MethodGet, "/api/h5/surveys/oauth/start?next=/s/survey-1", nil))
	if start.Code != http.StatusFound || start.Header().Get("Location") != "https://provider.example/authorize" {
		t.Fatalf("start status/location=%d/%q", start.Code, start.Header().Get("Location"))
	}
	callback := httptest.NewRecorder()
	router.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/api/h5/surveys/oauth/callback?state=state&code=code", nil))
	if callback.Code != http.StatusFound || callback.Header().Get("Location") != "/s/survey-1" || len(callback.Result().Cookies()) != 1 {
		t.Fatalf("callback status/location/cookies=%d/%q/%v", callback.Code, callback.Header().Get("Location"), callback.Result().Cookies())
	}

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/h5/surveys/oauth/start", nil))
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing next status=%d body=%s", missing.Code, missing.Body.String())
	}
}
