package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/riverqueue/river"
)

func TestWhitelistGatewayExposesOnlyFrozenBusinessRoutes(t *testing.T) {
	next := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) })
	handler := whitelistGateway(next, func(context.Context) error { return nil })
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
		{http.MethodPost, "/api/admin/automation-agents/7/activate", http.StatusNoContent},
		{http.MethodGet, "/api/admin/orders", http.StatusNoContent},
		{http.MethodGet, "/api/admin/hxc-current", http.StatusNoContent},
		{http.MethodPost, "/api/admin/hxc-current", http.StatusNotFound},
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
		{http.MethodPost, "/api/admin/channels/3/qrcode/generate", http.StatusNotFound},
		{http.MethodGet, "/api/admin/audience-history/packages", http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != test.want {
			t.Errorf("%s %s: status=%d want=%d", test.method, test.path, response.Code, test.want)
		}
	}
}

func TestWhitelistWorkerHasOnlyInertFailClosedKind(t *testing.T) {
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
	handler := whitelistGateway(next, func(context.Context) error { return errors.New("wrong schema") })
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
		if whitelistRouteAllowed(http.MethodPost, path) {
			t.Fatalf("external execution route is allowed: %s", path)
		}
	}
}
