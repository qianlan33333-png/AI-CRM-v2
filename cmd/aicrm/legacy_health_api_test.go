package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	legacyhealth "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/legacyhealth"
)

func TestLegacyHealthRootRoutePreservesFrozenRuntimeSnapshot(t *testing.T) {
	for _, test := range []struct {
		name     string
		snapshot legacyhealth.RuntimeSnapshot
	}{
		{
			name: "postgres normal",
			snapshot: legacyhealth.RuntimeSnapshot{
				DatabaseIsPostgres: true, SecretKeyPresent: true, WeChatShopCallbackTokenPresent: true,
			},
		},
		{name: "fixture", snapshot: legacyhealth.RuntimeSnapshot{}},
		{
			name:     "production fixture degraded",
			snapshot: legacyhealth.RuntimeSnapshot{ProductionEnvironment: true},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := legacyHealthRouter(t, test.snapshot)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "" {
				t.Fatalf("GET /health status/type/cache = %d/%q/%q", response.Code, response.Header().Get("Content-Type"), response.Header().Get("Cache-Control"))
			}
			var got legacyhealth.Payload
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if want := legacyhealth.NewQuery(test.snapshot).Execute(); got != want {
				t.Fatalf("GET /health payload = %#v, want %#v", got, want)
			}
		})
	}
}

func TestLegacyHealthRegistersOnlyGETAndCannotCaptureReservedRoutes(t *testing.T) {
	router := legacyHealthRouter(t, legacyhealth.RuntimeSnapshot{DatabaseIsPostgres: true})
	post := httptest.NewRecorder()
	router.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/health", nil))
	if post.Code != http.StatusBadRequest {
		t.Fatalf("POST /health = %d, want existing router method-not-allowed response", post.Code)
	}

	for _, test := range []struct {
		path string
		want int
	}{
		{"/healthz", http.StatusOK},
		{"/login", http.StatusOK},
		{"/logout", http.StatusFound},
		{"/admin", http.StatusNotFound},
		{"/api", http.StatusNotFound},
		{"/auth/wecom/start", http.StatusServiceUnavailable},
		{"/auth/wecom/callback", http.StatusServiceUnavailable},
		{"/wecom/external-contact/callback", http.StatusNoContent},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.want {
			t.Fatalf("GET %s = %d, want %d", test.path, response.Code, test.want)
		}
	}
}

func legacyHealthRouter(t *testing.T, snapshot legacyhealth.RuntimeSnapshot) http.Handler {
	t.Helper()
	service := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	candidate := &candidateHandler{
		Handler:      authHandler,
		legacyHealth: legacyhealth.NewHandler(legacyhealth.NewQuery(snapshot)),
	}
	router, err := newAPIHandlerWithAll(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) }),
		authHandler, candidate, nil, &HumanAuthHandler{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return router
}
