package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

func TestLegacyHXCSenderReadContractAndCarrier(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 1, 2, 3, 4, time.FixedZone("CST", 8*3600))
	source := &hxcReadStub{projection: hxcapp.Projection{SendConfigs: []hxcport.SenderConfig{{ID: "one", SenderUserID: "alice", DisplayName: "", Priority: 2, IsActive: true, CreatedAt: stamp, UpdatedAt: stamp}}, Directory: []hxcapp.Candidate{{WeComUserID: "alice", DisplayName: "Alice", IsSender: true, Priority: 2, IsActive: true}, {WeComUserID: "orphan", DisplayName: "Orphan", IsSender: true, Priority: 1, IsActive: false}}, ActiveSenderCount: 1, LastSyncedAt: stamp}}
	router := hxcSenderRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, source)
	page := httptest.NewRecorder()
	router.ServeHTTP(page, legacyRequest(http.MethodGet, legacyHXCSenderPagePath, legacyToken(1)))
	if page.Code != http.StatusFound || page.Header().Get("Location") != "/?legacy_admin_path=%2Fadmin%2Fhxc-send-config" {
		t.Fatalf("carrier=%d %q", page.Code, page.Header().Get("Location"))
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, legacyHXCSenderReadPath, legacyToken(2)))
	if response.Code != http.StatusOK {
		t.Fatalf("read=%d %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["source_status"] != "v2_local_staff" || body["active_sender_count"] != float64(1) || body["last_synced_at"] != stamp.UTC().Format(time.RFC3339Nano) || body["real_external_call_executed"] != false || body["empty_state"] != false {
		t.Fatalf("body=%#v", body)
	}
	if got := body["members"].([]any); len(got) != 2 || got[1].(map[string]any)["is_active"] != false {
		t.Fatalf("members=%#v", got)
	}
	assertNoHXCSenderSensitiveKeys(t, body)
	if source.calls != 1 {
		t.Fatalf("calls=%d", source.calls)
	}
}

func TestLegacyHXCSenderReadEmptyProjectionIsAvailableAndClosed(t *testing.T) {
	source := &hxcReadStub{}
	response := httptest.NewRecorder()
	hxcSenderRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, source).ServeHTTP(response, legacyRequest(http.MethodGet, legacyHXCSenderReadPath, legacyToken(21)))
	if response.Code != http.StatusOK {
		t.Fatalf("empty read=%d %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 15 || body["empty_state"] != true || body["last_synced_at"] != "" || len(body["send_configs"].([]any)) != 0 || len(body["directory_candidates"].([]any)) != 0 || len(body["members"].([]any)) != 0 || body["directory_count"] != float64(0) || body["sender_count"] != float64(0) || body["active_sender_count"] != float64(0) {
		t.Fatalf("empty body=%#v", body)
	}
	assertNoHXCSenderSensitiveKeys(t, body)
	if source.calls != 1 {
		t.Fatalf("empty calls=%d", source.calls)
	}
}

func TestLegacyHXCSenderReadFailsClosedAndStrictMethod(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		source := &hxcReadStub{}
		response := httptest.NewRecorder()
		hxcSenderRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, source).ServeHTTP(response, legacyRequest(method, legacyHXCSenderReadPath, legacyToken(3)))
		if response.Code != http.StatusMethodNotAllowed || source.calls != 0 {
			t.Fatalf("%s status/calls=%d/%d", method, response.Code, source.calls)
		}
	}
	for _, principal := range []authport.Principal{{AdminUserID: 9, Role: authport.RoleOps}, {AdminUserID: 9, Role: authport.RoleSales}} {
		source := &hxcReadStub{}
		response := httptest.NewRecorder()
		hxcSenderRouter(t, principal, source).ServeHTTP(response, legacyRequest(http.MethodGet, legacyHXCSenderReadPath, legacyToken(4)))
		if (response.Code != http.StatusUnauthorized && response.Code != http.StatusForbidden) || source.calls != 0 {
			t.Fatalf("principal=%+v status/calls=%d/%d", principal, response.Code, source.calls)
		}
	}
	source := &hxcReadStub{err: context.DeadlineExceeded}
	response := httptest.NewRecorder()
	hxcSenderRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, source).ServeHTTP(response, legacyRequest(http.MethodGet, legacyHXCSenderReadPath, legacyToken(5)))
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error_code\":\"hxc_send_config_unavailable\",\"ok\":false,\"real_external_call_executed\":false,\"status_code\":503}\n" {
		t.Fatalf("unavailable=%d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	hxcSenderRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, nil).ServeHTTP(response, legacyRequest(http.MethodGet, legacyHXCSenderReadPath, legacyToken(6)))
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error_code\":\"hxc_send_config_unavailable\",\"ok\":false,\"real_external_call_executed\":false,\"status_code\":503}\n" {
		t.Fatalf("nil unavailable=%d %s", response.Code, response.Body.String())
	}
}

func TestLegacyHXCSenderReadRequiresGlobalHumanAdmin(t *testing.T) {
	unauthenticatedSource := &hxcReadStub{}
	unauthenticated := httptest.NewRecorder()
	hxcSenderRouter(t, authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, unauthenticatedSource).ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, legacyHXCSenderReadPath, nil))
	if unauthenticated.Code != http.StatusUnauthorized || unauthenticatedSource.calls != 0 {
		t.Fatalf("unauthenticated status/calls=%d/%d", unauthenticated.Code, unauthenticatedSource.calls)
	}

	for _, test := range []struct {
		name    string
		request *http.Request
		service authport.Service
	}{
		{"owner scoped admin", legacyRequest(http.MethodGet, legacyHXCSenderReadPath, legacyToken(7)), &dataHealthAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 9}}},
		{"bearer", httptest.NewRequest(http.MethodGet, legacyHXCSenderReadPath, nil), &legacyAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}}},
		{"service", legacyRequest(http.MethodGet, legacyHXCSenderReadPath, legacyToken(8)), &dataHealthAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.Role("service")}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "bearer" {
				test.request.Header.Set("Authorization", "Bearer opaque-service-token")
			}
			source := &hxcReadStub{}
			response := httptest.NewRecorder()
			hxcSenderRouterWithService(t, test.service, source).ServeHTTP(response, test.request)
			if (response.Code != http.StatusUnauthorized && response.Code != http.StatusForbidden) || source.calls != 0 {
				t.Fatalf("status/calls=%d/%d body=%s", response.Code, source.calls, response.Body.String())
			}
		})
	}
}

func hxcSenderRouter(t *testing.T, principal authport.Principal, source hxcSenderRead) http.Handler {
	t.Helper()
	service := &legacyAuthStub{principal: principal}
	return hxcSenderRouterWithService(t, service, source)
}

func hxcSenderRouterWithService(t *testing.T, service authport.Service, source hxcSenderRead) http.Handler {
	t.Helper()
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewHandler(service, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.hxcSender = &hxcSenderHandler{reader: source}
	router, err := newAPIHandlerWithAll(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy, nil)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func assertNoHXCSenderSensitiveKeys(t *testing.T, value any) {
	t.Helper()
	forbidden := map[string]bool{"mobile": true, "avatar": true, "department": true, "token": true, "secret": true, "provider": true, "payload": true, "event": true, "job": true, "receipt": true, "error": true}
	var walk func(any)
	walk = func(node any) {
		switch current := node.(type) {
		case map[string]any:
			for key, child := range current {
				if forbidden[key] {
					t.Fatalf("sensitive key %q in %#v", key, current)
				}
				walk(child)
			}
		case []any:
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(value)
}

type hxcReadStub struct {
	projection hxcapp.Projection
	err        error
	calls      int
}

func (s *hxcReadStub) Read(context.Context) (hxcapp.Projection, error) {
	s.calls++
	return s.projection, s.err
}
