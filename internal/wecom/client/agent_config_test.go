package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentConfigTicketClientFetchesOnlyAgentConfigTicket(t *testing.T) {
	now := time.Date(2026, time.August, 26, 8, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path != "/cgi-bin/ticket/get" || request.URL.Query().Get("type") != "agent_config" || request.URL.Query().Get("access_token") != "access-fixture" {
			t.Fatalf("request = %s?%s", request.URL.Path, request.URL.RawQuery)
		}
		_, _ = writer.Write([]byte(`{"errcode":0,"ticket":"agent-config-ticket","expires_in":7200}`))
	}))
	defer server.Close()
	client, err := NewAgentConfigTicketClient(AgentConfigTicketClientConfig{
		BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: staticTokenProvider{token: AccessToken{value: "access-fixture"}}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := client.FetchAgentConfigTicket(context.Background())
	if err != nil || ticket.Value != "agent-config-ticket" || !ticket.ExpiresAt.Equal(now.Add(2*time.Hour)) || calls.Load() != 1 {
		t.Fatalf("FetchAgentConfigTicket() = %+v, %v, calls=%d", ticket, err, calls.Load())
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
