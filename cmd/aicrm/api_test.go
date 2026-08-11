package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestFinalRouterBindsEveryFrozenOperationCapability(t *testing.T) {
	service := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, authHandler)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		method, path string
		capability   authport.Capability
	}{
		{http.MethodGet, "/api/v1/admin/config/overview", authport.CapabilityConfigOverviewRead},
		{http.MethodPost, "/api/v1/auth/logout", authport.CapabilityAuthSessionLogout},
		{http.MethodGet, "/api/v1/auth/session", authport.CapabilityAuthSessionRead},
		{http.MethodGet, "/api/v1/customers", authport.CapabilityCustomersRead},
		{http.MethodGet, "/api/v1/customers/1", authport.CapabilityCustomersRead},
		{http.MethodPatch, "/api/v1/customers/1", authport.CapabilityCustomersWrite},
		{http.MethodGet, "/api/v1/customers/1/events", authport.CapabilityCustomerEventsRead},
		{http.MethodPost, "/api/v1/identity/bind", authport.CapabilityIdentityBind},
		{http.MethodPost, "/api/v1/identity/ingest", authport.CapabilityIdentityIngest},
		{http.MethodPost, "/api/v1/identity/resolve", authport.CapabilityIdentityResolve},
		{http.MethodGet, "/api/v1/stages", authport.CapabilityStagesRead},
		{http.MethodPost, "/api/v1/stages", authport.CapabilityStagesWrite},
		{http.MethodPatch, "/api/v1/stages/1", authport.CapabilityStagesWrite},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			service.reset()
			request := httptest.NewRequest(test.method, test.path, nil)
			request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "router-test-session"})
			if test.capability == authport.CapabilityAuthSessionLogout {
				request.Header.Set("X-CSRF-Token", "router-test-csrf")
			}
			if test.capability == authport.CapabilityStagesWrite {
				request.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if got := service.capabilities(); len(got) != 1 || got[0] != test.capability {
				t.Fatalf("authorized capabilities = %v, want [%s]", got, test.capability)
			}
		})
	}
}

func TestFinalRouterKeepsHealthPublicAndUnknownRoutesUnified(t *testing.T) {
	service := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, authHandler)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/api/v1/not-real", http.StatusNotFound},
		{http.MethodPost, "/healthz", http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != test.want || len(service.capabilities()) != 0 {
			t.Fatalf("%s %s status/capabilities = %d/%v, want %d/none", test.method, test.path, response.Code, service.capabilities(), test.want)
		}
	}
}

func TestFinalRouterLogsTemplateWhenAuthenticationShortCircuits(t *testing.T) {
	service := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	handler, err := newAPIHandler(slog.New(slog.NewJSONHandler(&logs, nil)), authHandler, authHandler)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/customers/987?email=private@example.test", nil)
	request.Header.Set("X-Request-ID", "router-log-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	var entry map[string]any
	if err = json.NewDecoder(&logs).Decode(&entry); err != nil {
		t.Fatal(err)
	}
	if entry["path"] != "/api/v1/customers/{customer_id}" || entry["status"] != float64(http.StatusUnauthorized) ||
		entry["err"] != "UNAUTHENTICATED" || entry["request_id"] != "router-log-1" {
		t.Fatalf("access log = %#v", entry)
	}
	if bytes.Contains(logs.Bytes(), []byte("987")) || bytes.Contains(logs.Bytes(), []byte("private@example.test")) {
		t.Fatalf("access log leaked raw path/query: %s", logs.String())
	}
}

type recordingAuth struct {
	mu   sync.Mutex
	seen []authport.Capability
}

func (*recordingAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, nil
}

func (service *recordingAuth) Authorize(_ context.Context, _ authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	service.mu.Lock()
	service.seen = append(service.seen, capability)
	service.mu.Unlock()
	scope := authport.ScopeGlobal
	if capability == authport.CapabilityAuthSessionRead || capability == authport.CapabilityAuthSessionLogout {
		scope = authport.ScopeSelf
	}
	return authport.Authorization{Capability: capability, Scope: scope}, nil
}

func (*recordingAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func (*recordingAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func (service *recordingAuth) reset() {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.seen = nil
}

func (service *recordingAuth) capabilities() []authport.Capability {
	service.mu.Lock()
	defer service.mu.Unlock()
	return append([]authport.Capability(nil), service.seen...)
}
