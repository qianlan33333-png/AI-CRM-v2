package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentConfigTicketClientFetchesBothRequiredTicketTypes(t *testing.T) {
	now := time.Date(2026, time.August, 26, 8, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		ticketType := request.URL.Query().Get("type")
		isConfig := request.URL.Path == "/cgi-bin/get_jsapi_ticket" && ticketType == ""
		isAgentConfig := request.URL.Path == "/cgi-bin/ticket/get" && ticketType == "agent_config"
		if (!isConfig && !isAgentConfig) || request.URL.Query().Get("access_token") != "access-fixture" {
			t.Fatalf("request = %s?%s", request.URL.Path, request.URL.RawQuery)
		}
		if isConfig {
			ticketType = "jsapi"
		}
		_, _ = writer.Write([]byte(`{"errcode":0,"ticket":"` + ticketType + `-ticket","expires_in":7200}`))
	}))
	defer server.Close()
	client, err := NewAgentConfigTicketClient(AgentConfigTicketClientConfig{
		BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: staticTokenProvider{token: AccessToken{value: "access-fixture"}}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	configTicket, err := client.FetchConfigTicket(context.Background())
	if err != nil || configTicket.Value != "jsapi-ticket" || !configTicket.ExpiresAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("FetchConfigTicket() = %+v, %v", configTicket, err)
	}
	agentTicket, err := client.FetchAgentConfigTicket(context.Background())
	if err != nil || agentTicket.Value != "agent_config-ticket" || !agentTicket.ExpiresAt.Equal(now.Add(2*time.Hour)) || calls.Load() != 2 {
		t.Fatalf("FetchAgentConfigTicket() = %+v, %v, calls=%d", agentTicket, err, calls.Load())
	}
}

func TestAgentConfigTicketClientRejectsInvalidProviderPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"errcode":0,"ticket":"bad ticket","expires_in":7200}`))
	}))
	defer server.Close()
	client, err := NewAgentConfigTicketClient(AgentConfigTicketClientConfig{
		BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: staticTokenProvider{token: AccessToken{value: "access-fixture"}}, Now: time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.FetchAgentConfigTicket(context.Background()); err != ErrUnexpectedResponse {
		t.Fatalf("FetchAgentConfigTicket() error = %v", err)
	}
}
