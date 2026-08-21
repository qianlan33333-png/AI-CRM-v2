package main

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestExternalEffectsReadRoutesUseOperationsReadWithoutCSRF(t *testing.T) {
	application := &externalEffectsHTTPStub{}
	auth := &legacyAuthStub{csrfErr: errors.New("GET must not validate csrf")}
	router := externalEffectsRouter(t, auth, application)

	for _, target := range []string{
		"/api/admin/external-effects/jobs?limit=50",
		"/api/admin/external-effects/diagnostics",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, target, legacyToken(81)))
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", target, response.Code, response.Body.String())
		}
	}
	if application.jobsCalls != 1 || application.diagnosticsCalls != 1 {
		t.Fatalf("calls jobs=%d diagnostics=%d", application.jobsCalls, application.diagnosticsCalls)
	}
}

type externalEffectsHTTPStub struct {
	jobsCalls        int
	diagnosticsCalls int
}

func (stub *externalEffectsHTTPStub) Jobs(writer http.ResponseWriter, _ *http.Request) {
	stub.jobsCalls++
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write([]byte(`{"ok":true}`))
}

func (stub *externalEffectsHTTPStub) Diagnostics(writer http.ResponseWriter, _ *http.Request) {
	stub.diagnosticsCalls++
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write([]byte(`{"ok":true}`))
}

func externalEffectsRouter(t *testing.T, auth authport.Service, externalEffects externalEffectsHTTP) http.Handler {
	t.Helper()
	legacy := &Handler{auth: auth, externalEffects: externalEffects}
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		authHandler,
		authHandler,
		legacy,
	)
	if err != nil {
		t.Fatal(err)
	}
	return router
}
