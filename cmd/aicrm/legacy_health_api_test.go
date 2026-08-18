package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/platform/legacyhealth"
)

func TestLegacyHealthRouteServesExactRuntimeSnapshots(t *testing.T) {
	tests := []struct {
		name     string
		snapshot legacyhealth.RuntimeSnapshot
		wantBody string
	}{
		{
			name:     "postgres with configuration present",
			snapshot: legacyhealth.RuntimeSnapshot{DatabaseIsPostgres: true, SecretKeyPresent: true, WeChatShopCallbackTokenPresent: true},
			wantBody: `{"ok":true,"status":"ok","service":"aicrm-next","secret_key_present":true,"wechat_shop_callback_token_present":true,"wechat_shop_callback_token_required":false,"database":"postgres","database_mode":"postgres","fixture_mode":false,"production_data_ready":true,"production_data_mode":true,"repository_policy":"production_repositories_required","runtime_owner":"ai_crm_next","legacy_runtime_enabled":false,"warning":""}`,
		},
		{
			name:     "postgres with missing configuration",
			snapshot: legacyhealth.RuntimeSnapshot{DatabaseIsPostgres: true},
			wantBody: `{"ok":true,"status":"ok","service":"aicrm-next","secret_key_present":false,"wechat_shop_callback_token_present":false,"wechat_shop_callback_token_required":false,"database":"postgres","database_mode":"postgres","fixture_mode":false,"production_data_ready":true,"production_data_mode":true,"repository_policy":"production_repositories_required","runtime_owner":"ai_crm_next","legacy_runtime_enabled":false,"warning":""}`,
		},
		{
			name:     "fixture outside production",
			snapshot: legacyhealth.RuntimeSnapshot{},
			wantBody: `{"ok":true,"status":"ok","service":"aicrm-next","secret_key_present":false,"wechat_shop_callback_token_present":false,"wechat_shop_callback_token_required":false,"database":"fixture","database_mode":"fixture","fixture_mode":true,"production_data_ready":false,"production_data_mode":false,"repository_policy":"fixture_repositories_allowed","runtime_owner":"ai_crm_next","legacy_runtime_enabled":false,"warning":"fixture data mode"}`,
		},
		{
			name:     "production fixture degraded",
			snapshot: legacyhealth.RuntimeSnapshot{ProductionEnvironment: true},
			wantBody: `{"ok":false,"status":"degraded","service":"aicrm-next","secret_key_present":false,"wechat_shop_callback_token_present":false,"wechat_shop_callback_token_required":true,"database":"fixture","database_mode":"fixture","fixture_mode":true,"production_data_ready":false,"production_data_mode":false,"repository_policy":"production_repositories_required","runtime_owner":"ai_crm_next","legacy_runtime_enabled":false,"warning":"production runtime is using fixture data; production data is not ready"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, service := legacyHealthRouter(t, test.snapshot)
			request := httptest.NewRequest(http.MethodGet, "/health", nil)
			// Session material must be ignored by the public route.
			request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "router-test-session"})
			request.Header.Set("X-CSRF-Token", "router-test-csrf")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("GET /health = %d, want 200; body=%s", response.Code, response.Body.String())
			}
			if got := response.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q", got)
			}
			if got := response.Header().Get("Cache-Control"); got != "" {
				t.Fatalf("Cache-Control = %q, want absent", got)
			}
			if response.Body.String() != test.wantBody {
				t.Fatalf("body = %s\nwant %s", response.Body.String(), test.wantBody)
			}
			if len(service.capabilities()) != 0 {
				t.Fatalf("public route authorized capabilities %v", service.capabilities())
			}
		})
	}
}

func TestLegacyHealthRouteRejectsNonGETWithExactLegacyMethodError(t *testing.T) {
	router, service := legacyHealthRouter(t, legacyhealth.RuntimeSnapshot{DatabaseIsPostgres: true})
	for _, method := range []string{
		http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodOptions, http.MethodHead,
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/health", nil))
		if response.Code != http.StatusMethodNotAllowed ||
			response.Header().Get("Allow") != http.MethodGet ||
			response.Header().Get("Content-Type") != "application/json" ||
			response.Body.String() != `{"detail":"Method Not Allowed"}` {
			t.Fatalf("%s /health = status %d allow %q type %q body %q", method,
				response.Code, response.Header().Get("Allow"), response.Header().Get("Content-Type"), response.Body.String())
		}
		if len(service.capabilities()) != 0 {
			t.Fatalf("%s /health authorized capabilities %v", method, service.capabilities())
		}
	}
}

func TestLegacyHealthRouteDoesNotCaptureReservedPaths(t *testing.T) {
	router, service := legacyHealthRouter(t, legacyhealth.RuntimeSnapshot{DatabaseIsPostgres: true})
	for _, test := range []struct {
		method, path string
		wantStatus   int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodPost, "/healthz", http.StatusBadRequest},
		{http.MethodGet, "/api/system/health", http.StatusNotFound},
		{http.MethodGet, "/WW_verify_missing.txt", http.StatusNotFound},
		{http.MethodGet, "/login", http.StatusOK},
		{http.MethodGet, "/logout", http.StatusFound},
		{http.MethodGet, "/admin", http.StatusNotFound},
		{http.MethodGet, "/api/v1/customers", http.StatusUnauthorized},
		{http.MethodGet, "/auth/wecom/start", http.StatusServiceUnavailable},
		{http.MethodGet, "/wecom/external-contact/callback", http.StatusNoContent},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != test.wantStatus {
			t.Fatalf("%s %s = %d, want %d; body=%s", test.method, test.path, response.Code, test.wantStatus, response.Body.String())
		}
	}
	if len(service.capabilities()) != 0 {
		t.Fatalf("reserved path probe authorized capabilities %v", service.capabilities())
	}
}

func legacyHealthRouter(t *testing.T, snapshot legacyhealth.RuntimeSnapshot) (http.Handler, *recordingAuth) {
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
	return router, service
}
