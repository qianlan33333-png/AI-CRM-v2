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

func TestReleaseSHAMiddlewarePublishesOnlyCompleteConfiguredSHA(t *testing.T) {
	sha := strings.Repeat("a", 40)
	leaf := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	for _, testCase := range []struct {
		name, configured, expected string
	}{
		{name: "complete", configured: "  " + sha + "  ", expected: sha},
		{name: "missing", configured: ""},
		{name: "incomplete", configured: "abc"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			releaseSHAMiddleware(testCase.configured, leaf).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
			if response.Code != http.StatusNoContent || response.Header().Get("X-AICRM-Release-SHA") != testCase.expected {
				t.Fatalf("response=%d header=%q", response.Code, response.Header().Get("X-AICRM-Release-SHA"))
			}
		})
	}
}

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
		{http.MethodGet, "/api/sidebar/v2/workbench", authport.CapabilityCustomersRead},
		{http.MethodPut, "/api/sidebar/v2/profile", authport.CapabilityCustomersWrite},
		{http.MethodGet, "/api/sidebar/v2/questionnaires", authport.CapabilityCustomersRead},
		{http.MethodGet, "/api/sidebar/v2/orders", authport.CapabilityCustomersRead},
		{http.MethodGet, "/api/sidebar/v2/periodic-orders", authport.CapabilityCustomersRead},
		{http.MethodPut, "/api/sidebar/v2/periodic-orders/71/members/spm_abcdefghijklmnopqrstuv/remark", authport.CapabilityCustomersWrite},
		{http.MethodGet, "/api/sidebar/v2/materials", authport.CapabilityCustomersRead},
		{http.MethodGet, "/api/sidebar/v2/materials/image/1/thumbnail", authport.CapabilityCustomersRead},
		{http.MethodGet, "/api/v1/customers", authport.CapabilityCustomersRead},
		{http.MethodGet, "/api/v1/admin/release-candidates", authport.CapabilityReleaseRead},
		{http.MethodGet, "/api/v1/admin/release-candidates/1", authport.CapabilityReleaseRead},
		{http.MethodPost, "/api/v1/admin/release-candidates", authport.CapabilityReleaseManage},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/prerequisites", authport.CapabilityReleaseManage},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/prepare", authport.CapabilityReleaseManage},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/cutover/start", authport.CapabilityReleaseManage},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/cutover/restart", authport.CapabilityReleaseManage},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/cutover/steps/announce/complete", authport.CapabilityReleaseManage},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/activate", authport.CapabilityReleaseManage},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/rollback-checks", authport.CapabilityReleaseManage},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/rollback/request", authport.CapabilityReleaseManage},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/rollback/complete", authport.CapabilityReleaseManage},
		{http.MethodGet, "/api/admin/external-effects", authport.CapabilityOperationsRead},
		{http.MethodGet, "/api/admin/external-effects/1", authport.CapabilityOperationsRead},
		{http.MethodGet, "/api/admin/external-effects/diagnostics", authport.CapabilityOperationsRead},
		{http.MethodPost, "/api/admin/external-effects/1/cancel", authport.CapabilityOperationsManage},
		{http.MethodPost, "/api/admin/external-effects/1/retry", authport.CapabilityOperationsManage},
		{http.MethodPost, "/api/admin/external-effects/1/reconcile", authport.CapabilityOperationsManage},
		{http.MethodPost, "/api/v1/customer-exports", authport.CapabilityCustomersRead},
		{http.MethodGet, "/api/v1/customer-exports/cse_0123456789abcdef0123456789abcdef", authport.CapabilityCustomersRead},
		{http.MethodGet, "/api/v1/customer-exports/cse_0123456789abcdef0123456789abcdef/download", authport.CapabilityCustomersRead},
		{http.MethodGet, "/api/v1/customers/1", authport.CapabilityCustomersRead},
		{http.MethodPatch, "/api/v1/customers/1", authport.CapabilityCustomersWrite},
		{http.MethodPut, "/api/v1/customers/1/stage", authport.CapabilityCustomersWrite},
		{http.MethodPut, "/api/v1/customers/1/tags/2", authport.CapabilityCustomersWrite},
		{http.MethodDelete, "/api/v1/customers/1/tags/2", authport.CapabilityCustomersWrite},
		{http.MethodGet, "/api/v1/customers/1/events", authport.CapabilityCustomerEventsRead},
		{http.MethodGet, "/api/v1/customers/1/context", authport.CapabilityCustomerEventsRead},
		{http.MethodGet, "/api/v1/customers/1/merge-history", authport.CapabilityIdentityReviewRead},
		{http.MethodGet, "/api/v1/customers/1/chat-activity", authport.CapabilityCustomerEventsRead},
		{http.MethodGet, "/api/v1/customers/1/activity-analytics", authport.CapabilityCustomerEventsRead},
		{http.MethodGet, "/api/v1/tags", authport.CapabilityCustomersRead},
		{http.MethodPost, "/api/v1/tags", authport.CapabilityCustomersWrite},
		{http.MethodPut, "/api/v1/tags/reorder", authport.CapabilityCustomersWrite},
		{http.MethodPatch, "/api/v1/tags/2", authport.CapabilityCustomersWrite},
		{http.MethodDelete, "/api/v1/tags/2", authport.CapabilityCustomersWrite},
		{http.MethodGet, "/api/v1/tag-groups", authport.CapabilityCustomersRead},
		{http.MethodPost, "/api/v1/tag-groups", authport.CapabilityCustomersWrite},
		{http.MethodPut, "/api/v1/tag-groups/reorder", authport.CapabilityCustomersWrite},
		{http.MethodPatch, "/api/v1/tag-groups/1", authport.CapabilityCustomersWrite},
		{http.MethodDelete, "/api/v1/tag-groups/1", authport.CapabilityCustomersWrite},
		{http.MethodPost, "/api/v1/identity/bind", authport.CapabilityIdentityBind},
		{http.MethodPost, "/api/v1/identity/ingest", authport.CapabilityIdentityIngest},
		{http.MethodPost, "/api/v1/identity/resolve", authport.CapabilityIdentityResolve},
		{http.MethodGet, "/api/v1/identity/merge-reviews", authport.CapabilityIdentityReviewRead},
		{http.MethodPost, "/api/v1/identity/merge-reviews/1/approve", authport.CapabilityIdentityReviewWrite},
		{http.MethodPost, "/api/v1/identity/merge-reviews/1/reject", authport.CapabilityIdentityReviewWrite},
		{http.MethodGet, "/api/v1/stages", authport.CapabilityStagesRead},
		{http.MethodGet, "/api/v1/segments", authport.CapabilitySegmentsRead},
		{http.MethodPost, "/api/v1/segments", authport.CapabilitySegmentsWrite},
		{http.MethodGet, "/api/v1/segments/1", authport.CapabilitySegmentsRead},
		{http.MethodPatch, "/api/v1/segments/1", authport.CapabilitySegmentsWrite},
		{http.MethodDelete, "/api/v1/segments/1", authport.CapabilitySegmentsWrite},
		{http.MethodGet, "/api/v1/segments/1/members", authport.CapabilitySegmentsRead},
		{http.MethodPost, "/api/v1/segments/1/refresh", authport.CapabilitySegmentsWrite},
		{http.MethodGet, "/api/v1/products", authport.CapabilityProductsRead},
		{http.MethodPost, "/api/v1/products", authport.CapabilityProductsWrite},
		{http.MethodGet, "/api/v1/products/1", authport.CapabilityProductsRead},
		{http.MethodPut, "/api/v1/products/1", authport.CapabilityProductsWrite},
		{http.MethodGet, "/api/v1/products/1/local-entitlements", authport.CapabilityEntitlementsRead},
		{http.MethodPost, "/api/v1/products/1/local-entitlements", authport.CapabilityEntitlementsWrite},
		{http.MethodGet, "/api/v1/product-entitlements/1", authport.CapabilityEntitlementsRead},
		{http.MethodPost, "/api/v1/product-entitlements/1/revoke", authport.CapabilityEntitlementsWrite},
		{http.MethodPost, "/api/v1/stages", authport.CapabilityStagesWrite},
		{http.MethodPut, "/api/v1/stages/reorder", authport.CapabilityStagesWrite},
		{http.MethodDelete, "/api/v1/stages/1", authport.CapabilityStagesWrite},
		{http.MethodPatch, "/api/v1/stages/1", authport.CapabilityStagesWrite},
		{http.MethodGet, "/api/admin/cloud-orchestrator/campaigns/spring-campaign/touch-plans", authport.CapabilityOperationsRead},
		{http.MethodPost, "/api/admin/cloud-orchestrator/campaigns/spring-campaign/touch-plans", authport.CapabilityOperationsManage},
		{http.MethodGet, "/api/admin/cloud-orchestrator/campaigns/spring-campaign/touch-plans/ctp_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", authport.CapabilityOperationsRead},
		{http.MethodPost, "/api/admin/questionnaires/1/public-publish", authport.CapabilityQuestionnairesWrite},
		{http.MethodPost, "/api/admin/questionnaires/1/public-disable", authport.CapabilityQuestionnairesWrite},
		{http.MethodGet, "/api/admin/questionnaires/1/public-analytics?definition_version=1", authport.CapabilityQuestionnairesRead},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			service.reset()
			request := httptest.NewRequest(test.method, test.path, nil)
			request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "router-test-session"})
			if test.capability == authport.CapabilityAuthSessionLogout {
				request.Header.Set("X-CSRF-Token", "router-test-csrf")
			}
			if test.method == http.MethodPost && strings.Contains(test.path, "/customer-exports") {
				request.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
				request.Header.Set("Idempotency-Key", "router-customer-export-key")
				request.Header.Set("Content-Type", "application/json")
			}
			if test.capability == authport.CapabilityStagesWrite ||
				test.capability == authport.CapabilitySegmentsWrite ||
				test.capability == authport.CapabilityCustomersWrite ||
				test.capability == authport.CapabilityIdentityBind ||
				test.capability == authport.CapabilityIdentityIngest ||
				test.capability == authport.CapabilityIdentityReviewWrite {
				request.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
			}
			if test.capability == authport.CapabilityStagesWrite || test.capability == authport.CapabilitySegmentsWrite || test.capability == authport.CapabilityCustomersWrite {
				request.Header.Set("Idempotency-Key", "router-classification-key")
			}
			if test.capability == authport.CapabilityProductsWrite || test.capability == authport.CapabilityEntitlementsWrite {
				request.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
				request.Header.Set("Idempotency-Key", "router-product-key")
			}
			if test.capability == authport.CapabilityQuestionnairesWrite {
				request.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
				request.Header.Set("Idempotency-Key", "router-public-survey-key")
				request.Header.Set("Content-Type", "application/json")
			}
			if test.capability == authport.CapabilityIdentityReviewWrite {
				request.Header.Set("Idempotency-Key", "router-review-key")
			}
			if test.method == http.MethodPost && strings.Contains(test.path, "/touch-plans") {
				request.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
				request.Header.Set("Idempotency-Key", "router-touch-plan-key")
			}
			if test.capability == authport.CapabilityReleaseManage {
				request.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
				request.Header.Set("Idempotency-Key", "router-release-plane-key")
			}
			if test.method == http.MethodPost && strings.Contains(test.path, "/external-effects/") {
				request.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
				request.Header.Set("Idempotency-Key", "router-external-effects-key")
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if got := service.capabilities(); len(got) != 1 || got[0] != test.capability {
				t.Fatalf("authorized capabilities = %v, want [%s]", got, test.capability)
			}
		})
	}
}

func TestFinalRouterRejectsProtectedWriteWithoutValidCSRF(t *testing.T) {
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
		body         string
	}{
		{http.MethodPatch, "/api/v1/customers/1", `{"name":"安全写入"}`},
		{http.MethodPut, "/api/sidebar/v2/profile", `{"expected_updated_at":"2026-08-24T00:00:00Z","patch":{"needs":"renewal"}}`},
		{http.MethodPut, "/api/sidebar/v2/periodic-orders/71/members/spm_abcdefghijklmnopqrstuv/remark", `{"expected_version":1,"remark":"renewal"}`},
		{http.MethodPut, "/api/v1/customers/1/stage", `{"stage_id":null}`},
		{http.MethodPut, "/api/v1/customers/1/tags/2", ""},
		{http.MethodDelete, "/api/v1/customers/1/tags/2", ""},
		{http.MethodPost, "/api/v1/identity/bind", `{}`},
		{http.MethodPost, "/api/v1/customer-exports", `{}`},
		{http.MethodPost, "/api/v1/identity/ingest", `{}`},
		{http.MethodPost, "/api/v1/admin/release-candidates", `{}`},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/prerequisites", `{}`},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/prepare", ``},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/cutover/start", ``},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/cutover/restart", `{}`},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/cutover/steps/announce/complete", `{}`},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/activate", `{}`},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/rollback-checks", `{}`},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/rollback/request", ``},
		{http.MethodPost, "/api/v1/admin/release-candidates/1/rollback/complete", ``},
		{http.MethodPost, "/api/admin/external-effects/1/cancel", ``},
		{http.MethodPost, "/api/admin/external-effects/1/retry", `{}`},
		{http.MethodPost, "/api/admin/external-effects/1/reconcile", `{}`},
		{http.MethodPost, "/api/v1/identity/merge-reviews/1/approve", `{"expected_version":1,"primary_customer_id":1,"reason":"confirm"}`},
		{http.MethodPost, "/api/v1/identity/merge-reviews/1/reject", `{"expected_version":1,"reason":"reject"}`},
		{http.MethodPost, "/api/v1/products", `{"product_code":"sku","name":"商品","description":"","price_minor":1,"currency":"CNY","stock_quantity":0,"images":[]}`},
		{http.MethodPut, "/api/v1/products/1", `{"expected_version":1,"name":"商品","description":"","price_minor":1,"currency":"CNY","stock_quantity":0}`},
		{http.MethodPost, "/api/v1/products/1/local-entitlements", `{"order_id":1}`},
		{http.MethodPost, "/api/v1/product-entitlements/1/revoke", `{"expected_version":1}`},
		{http.MethodPost, "/api/v1/tag-groups", `{"name":"Lifecycle","first_tag_name":"Warm"}`},
		{http.MethodPut, "/api/v1/tag-groups/reorder", `{"ids":[1]}`},
		{http.MethodPatch, "/api/v1/tag-groups/1", `{"name":"Lifecycle"}`},
		{http.MethodDelete, "/api/v1/tag-groups/1", ``},
		{http.MethodPost, "/api/v1/tags", `{"group_id":1,"name":"Warm"}`},
		{http.MethodPut, "/api/v1/tags/reorder", `{"ids":[1]}`},
		{http.MethodPatch, "/api/v1/tags/1", `{"name":"Warm"}`},
		{http.MethodDelete, "/api/v1/tags/1", ``},
		{http.MethodPost, "/api/v1/segments", `{"name":"High intent","definition":{"field":"is_deleted","op":"eq","value":false},"refresh_mode":"manual"}`},
		{http.MethodPatch, "/api/v1/segments/1", `{"name":"High intent"}`},
		{http.MethodDelete, "/api/v1/segments/1", ``},
		{http.MethodPost, "/api/v1/segments/1/refresh", ``},
		{http.MethodPost, "/api/v1/stages", `{"name":"New"}`},
		{http.MethodPut, "/api/v1/stages/reorder", `{"ids":[1]}`},
		{http.MethodPatch, "/api/v1/stages/1", `{"name":"New"}`},
		{http.MethodDelete, "/api/v1/stages/1", ``},
		{http.MethodPost, "/api/admin/questionnaires/1/public-publish", `{"expected_questionnaire_version":1}`},
		{http.MethodPost, "/api/admin/questionnaires/1/public-disable", `{"expected_definition_version":1}`},
	} {
		service.reset()
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "router-test-session"})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || len(service.capabilities()) != 0 {
			t.Fatalf("%s %s status/capabilities = %d/%v, want 403/none", test.method, test.path, response.Code, service.capabilities())
		}
	}
}

func TestFinalRouterMountsBothWeComCallbackPathsOutsideAdminAuthentication(t *testing.T) {
	service := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, authHandler)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/wecom/events", "/wecom/external-contact/callback"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusServiceUnavailable || len(service.capabilities()) != 0 {
			t.Fatalf("GET %s = %d with capabilities %v, want unavailable/no auth", path, response.Code, service.capabilities())
		}
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

func TestFinalRouterWithoutLegacyDoesNotServeAPIDocsPage(t *testing.T) {
	service := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, authHandler)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/admin/api-docs", "/admin/config/mcp-tools", "/admin/config/mcp-tools/save"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d without legacy composition, want 404", path, response.Code)
		}
	}
}
