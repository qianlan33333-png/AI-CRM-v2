package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/riverqueue/river"
)

func TestWhitelistGatewayExposesCurrentCapabilitiesWithoutExternalExecution(t *testing.T) {
	next := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) })
	handler := whitelistGateway(next, func(context.Context) error { return nil }, whitelistCapabilities{weComTagCatalogSync: true})
	for _, test := range []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/healthz", http.StatusNoContent},
		{http.MethodGet, "/readyz", http.StatusOK},
		{http.MethodGet, "/api/admin/wechat-pay/products", http.StatusNoContent},
		{http.MethodPut, "/api/v1/products/41", http.StatusNoContent},
		{http.MethodGet, "/api/v1/segments/42/members", http.StatusNoContent},
		{http.MethodPost, "/api/v1/segments/42/refresh", http.StatusNotFound},
		{http.MethodPatch, "/api/admin/wechat-pay/products/41", http.StatusNoContent},
		{http.MethodGet, "/api/admin/ai-audience/packages/42/members", http.StatusNoContent},
		{http.MethodPost, "/api/admin/automation-conversion/group-ops/plans/42/run-due/preview", http.StatusNoContent},
		{http.MethodPost, "/api/admin/automation-conversion/group-ops/plans/42/run-due", http.StatusNotFound},
		{http.MethodPost, "/api/admin/automation-agents/7/activate", http.StatusNoContent},
		{http.MethodGet, "/api/admin/orders", http.StatusNoContent},
		{http.MethodGet, "/api/admin/hxc-current", http.StatusNoContent},
		{http.MethodPost, "/api/admin/hxc-current", http.StatusNotFound},
		{http.MethodGet, "/api/v1/customers", http.StatusNoContent},
		{http.MethodGet, "/api/v1/tag-groups", http.StatusNoContent},
		{http.MethodPost, "/api/admin/wecom/tags/sync", http.StatusNoContent},
		{http.MethodGet, "/api/admin/wecom/tags", http.StatusNoContent},
		{http.MethodGet, "/api/admin/coupons", http.StatusNoContent},
		{http.MethodGet, "/api/admin/image-library", http.StatusNoContent},
		{http.MethodGet, "/api/admin/attachment-library", http.StatusNoContent},
		{http.MethodGet, "/api/admin/miniprogram-library", http.StatusNoContent},
		{http.MethodGet, "/api/admin/automation-conversion/group-ops/plans", http.StatusNoContent},
		{http.MethodGet, "/api/admin/common/operation-members", http.StatusNoContent},
		{http.MethodPost, "/api/admin/automation-conversion/group-ops/plans/1/run-due", http.StatusNotFound},
		{http.MethodGet, "/api/admin/config/app-settings", http.StatusNoContent},
		{http.MethodGet, "/api/sidebar/v2/jssdk/agent-config", http.StatusNoContent},
		{http.MethodPost, "/api/sidebar/v2/bootstrap", http.StatusNoContent},
		{http.MethodGet, "/api/sidebar/v2/timeline", http.StatusNoContent},
		{http.MethodGet, "/api/sidebar/v2/chat-activity", http.StatusNoContent},
		{http.MethodGet, "/api/sidebar/v2/other-staff-chats", http.StatusNoContent},
		{http.MethodGet, "/api/sidebar/v2/workbench", http.StatusNoContent},
		{http.MethodPut, "/api/sidebar/v2/profile", http.StatusNoContent},
		{http.MethodPost, "/api/sidebar/v2/phone-binding", http.StatusNoContent},
		{http.MethodGet, "/api/sidebar/v2/materials", http.StatusNoContent},
		{http.MethodGet, "/api/sidebar/v2/materials/image/7/preview", http.StatusNoContent},
		{http.MethodPost, "/api/sidebar/v2/materials/image/7/temporary-media", http.StatusNotFound},
		{http.MethodGet, "/api/admin/orders/legacy-order-1", http.StatusNotFound},
		{http.MethodPost, "/api/admin/orders", http.StatusNotFound},
		{http.MethodGet, "/api/admin/campaigns", http.StatusNotFound},
		{http.MethodGet, "/api/admin/messages", http.StatusNotFound},
		{http.MethodPost, "/api/admin/wechat-pay/products/41/external-push", http.StatusNotFound},
		{http.MethodGet, "/api/admin/channels/3/contacts", http.StatusNoContent},
		{http.MethodGet, "/api/admin/channels/3/acquisition-preview", http.StatusNoContent},
		{http.MethodGet, "/api/admin/channels/3/acquisition-staff", http.StatusNoContent},
		{http.MethodGet, "/api/admin/channels/3/acquisition-assets", http.StatusNoContent},
		{http.MethodPost, "/api/admin/channels/3/qrcode/generate", http.StatusNoContent},
		{http.MethodGet, "/api/admin/channels/3/qrcode/download", http.StatusNoContent},
		{http.MethodGet, "/api/admin/audience-history/packages", http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != test.want {
			t.Errorf("%s %s: status=%d want=%d", test.method, test.path, response.Code, test.want)
		}
	}
}

func TestWhitelistChannelAcquisitionRoutesReachFailClosedHandlers(t *testing.T) {
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/channels/3/acquisition-staff"},
		{http.MethodGet, "/api/admin/channels/3/acquisition-assets"},
		{http.MethodPost, "/api/admin/channels/3/qrcode/generate"},
		{http.MethodGet, "/api/admin/channels/3/qrcode/download"},
	} {
		if !whitelistRouteAllowed(route.method, route.path, whitelistCapabilities{}) {
			t.Fatalf("channel acquisition route is blocked: %s", route.path)
		}
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/channels/3/contacts"},
		{http.MethodGet, "/api/admin/channels/3/acquisition-preview"},
		{http.MethodPut, "/api/admin/channels/3/assignees"},
	} {
		if !whitelistRouteAllowed(route.method, route.path, whitelistCapabilities{}) {
			t.Fatalf("local channel route is blocked: %s %s", route.method, route.path)
		}
	}
}

func TestWhitelistGatewayRejectedRouteReturnsJSON(t *testing.T) {
	handler := whitelistGateway(http.NotFoundHandler(), func(context.Context) error { return nil }, whitelistCapabilities{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/admin/messages", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type=%q", got)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not json: %v", err)
	}
	if body["code"] != "NOT_FOUND" || body["message"] == "" {
		t.Fatalf("body=%v", body)
	}
}

func TestWhitelistWorkerAlwaysHasInertFailClosedKind(t *testing.T) {
	if got := (whitelistInertJobArgs{}).Kind(); got != "aicrm_whitelist_inert_v1" {
		t.Fatalf("kind=%q", got)
	}
	err := (&whitelistInertWorker{}).Work(context.Background(), nil)
	var cancelled *river.JobCancelError
	if !errors.As(err, &cancelled) {
		t.Fatalf("work error=%v", err)
	}
}

func TestWhitelistReadinessFailsClosed(t *testing.T) {
	next := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) })
	handler := whitelistGateway(next, func(context.Context) error { return errors.New("wrong schema") }, whitelistCapabilities{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"status\":\"not_ready\"}\n" {
		t.Fatalf("response=%d %q", response.Code, response.Body.String())
	}
}

func TestWhitelistRouteAllowlistHasNoExternalExecution(t *testing.T) {
	for _, path := range []string{
		"/api/admin/wechat-pay/products/1/external-push",
		"/api/admin/questionnaires/1/submissions/2/external-push",
		"/api/admin/automations/1/activate",
		"/api/admin/ai-audience/packages/1/send-records",
		"/api/sidebar/v2/materials/image/1/temporary-media",
	} {
		if whitelistRouteAllowed(http.MethodPost, path, whitelistCapabilities{weComTagCatalogSync: true}) {
			t.Fatalf("external execution route is allowed: %s", path)
		}
	}
}

func TestWhitelistTagCatalogSyncRequiresExplicitReadOnlyCapability(t *testing.T) {
	if whitelistRouteAllowed(http.MethodPost, "/api/admin/wecom/tags/sync", whitelistCapabilities{}) {
		t.Fatal("tag catalog sync is allowed while the read-only capability is disabled")
	}
	if !whitelistRouteAllowed(http.MethodPost, "/api/admin/wecom/tags/sync", whitelistCapabilities{weComTagCatalogSync: true}) {
		t.Fatal("tag catalog sync is blocked while the read-only capability is enabled")
	}
	for _, path := range []string{
		"/api/admin/wecom/tags/live/mark",
		"/api/admin/wecom/tags/live/unmark",
		"/api/admin/wecom/tag-effects/effect-1/reconcile",
	} {
		if whitelistRouteAllowed(http.MethodPost, path, whitelistCapabilities{weComTagCatalogSync: true}) {
			t.Fatalf("tag mutation route is allowed: %s", path)
		}
	}
}
