package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
)

func TestSidebarOAuthHTTPFailsClosedWhenDisabled(t *testing.T) {
	handler := NewOAuthHandler(nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/oauth/start?external_userid=wm_external_41", nil)
	response := httptest.NewRecorder()
	handler.Start(response, request)
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Set-Cookie") != "" || response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("disabled start status/cookie/referrer = %d/%q/%q", response.Code, response.Header().Get("Set-Cookie"), response.Header().Get("Referrer-Policy"))
	}

	response = httptest.NewRecorder()
	handler.Callback(response, httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/oauth/callback?code=code&state=state", nil))
	if response.Code != http.StatusBadRequest || response.Header().Get("Set-Cookie") != "" {
		t.Fatalf("callback without binding status/cookie = %d/%q", response.Code, response.Header().Get("Set-Cookie"))
	}
}

func TestSidebarOAuthHTTPRejectsUnknownStartQueryBeforeService(t *testing.T) {
	response := httptest.NewRecorder()
	NewOAuthHandler(nil, nil).Start(response, httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/oauth/start?external_userid=wm_external_41&unexpected=1", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown query status = %d", response.Code)
	}
}

func TestSidebarAgentConfigHTTPUsesOnlyAgentConfigSignature(t *testing.T) {
	now := time.Date(2026, time.August, 26, 8, 0, 0, 0, time.UTC)
	provider := &httpAgentConfigProvider{ticket: sidebarapp.AgentConfigTicket{Value: "agent-config-ticket", ExpiresAt: now.Add(2 * time.Hour)}}
	service, err := sidebarapp.NewJSSDKService(sidebarapp.JSSDKServiceConfig{
		Enabled: true, CorpID: "corp-1", AgentID: 73, AllowedHosts: []string{"crm.example.test"},
	}, provider, sidebarapp.JSSDKOptions{Clock: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{1}, 16))})
	if err != nil {
		t.Fatal(err)
	}
	query := url.Values{"url": {"https://crm.example.test/sidebar#fragment"}}
	response := httptest.NewRecorder()
	NewJSSDKHandler(service).AgentConfig(response, httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/jssdk/agent-config?"+query.Encode(), nil))
	if response.Code != http.StatusOK || provider.calls != 1 || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("agent config status/calls/cache = %d/%d/%q", response.Code, provider.calls, response.Header().Get("Cache-Control"))
	}
	body := append([]byte(nil), response.Body.Bytes()...)
	var got sidebarapp.AgentConfigSignature
	if err = json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.SignatureType != "agent_config" || got.URL != "https://crm.example.test/sidebar" || !bytes.Contains(body, []byte(`"signature_type":"agent_config"`)) || bytes.Contains(body, []byte("agent-config-ticket")) {
		t.Fatalf("agent config response = %+v body=%q", got, body)
	}
}

func TestSidebarAgentConfigHTTPDisabledOrInvalidNeverCallsProvider(t *testing.T) {
	disabled, err := sidebarapp.NewJSSDKService(sidebarapp.JSSDKServiceConfig{}, nil, sidebarapp.JSSDKOptions{})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	NewJSSDKHandler(disabled).AgentConfig(response, httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/jssdk/agent-config?url=https%3A%2F%2Fcrm.example.test%2Fsidebar", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled status = %d", response.Code)
	}
	provider := &httpAgentConfigProvider{}
	service, err := sidebarapp.NewJSSDKService(sidebarapp.JSSDKServiceConfig{Enabled: true, CorpID: "corp-1", AgentID: 1, AllowedHosts: []string{"crm.example.test"}}, provider, sidebarapp.JSSDKOptions{})
	if err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	NewJSSDKHandler(service).AgentConfig(response, httptest.NewRequest(http.MethodGet, "/api/sidebar/v2/jssdk/agent-config?url=https%3A%2F%2Fevil.example%2Fsidebar", nil))
	if response.Code != http.StatusBadRequest || provider.calls != 0 {
		t.Fatalf("invalid status/calls = %d/%d", response.Code, provider.calls)
	}
}

type httpAgentConfigProvider struct {
	ticket sidebarapp.AgentConfigTicket
	calls  int
}

func (provider *httpAgentConfigProvider) FetchAgentConfigTicket(context.Context) (sidebarapp.AgentConfigTicket, error) {
	provider.calls++
	return provider.ticket, nil
}
