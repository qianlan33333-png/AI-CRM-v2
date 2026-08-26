package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudience"
)

func signAIAudienceWebhook(key, body []byte, timestamp, eventID string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(timestamp + "\n" + eventID + "\n"))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestAIAudienceProtocolAuthenticatorVerifiesExactRawBody(t *testing.T) {
	now := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	body := []byte(`{"external_event_id":"business-event-0001"}`)
	timestamp, eventID := "1787731200", "transport-event-0001"
	request := httptest.NewRequest("POST", "/api/ai/audience/packages/42/webhook", nil)
	request.Header.Set("X-AICRM-Client-Id", legacyaudience.AIAudienceWebhookClientID)
	request.Header.Set("X-AICRM-Timestamp", timestamp)
	request.Header.Set("X-AICRM-Event-Id", eventID)
	request.Header.Set("X-AICRM-Signature", signAIAudienceWebhook(key, body, timestamp, eventID))
	authenticator := &aiAudienceProtocolAuthenticator{key: key, now: func() time.Time { return now }}

	identity, err := authenticator.Authenticate(context.Background(), request, body)
	if err != nil || identity.ClientID != legacyaudience.AIAudienceWebhookClientID || identity.TransportEventID != eventID {
		t.Fatalf("identity=%+v err=%v", identity, err)
	}
	if _, err = authenticator.Authenticate(context.Background(), request, append(body, ' ')); err == nil {
		t.Fatal("changed raw body authenticated")
	}
}

func TestAIAudienceProtocolAuthenticatorFailsClosed(t *testing.T) {
	now := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	body := []byte(`{}`)
	request := func(timestamp string) *http.Request {
		eventID := "transport-event-0001"
		r := httptest.NewRequest("POST", "/", nil)
		r.Header.Set("X-AICRM-Client-Id", legacyaudience.AIAudienceWebhookClientID)
		r.Header.Set("X-AICRM-Timestamp", timestamp)
		r.Header.Set("X-AICRM-Event-Id", eventID)
		r.Header.Set("X-AICRM-Signature", signAIAudienceWebhook(key, body, timestamp, eventID))
		return r
	}
	tests := []struct {
		name    string
		auth    *aiAudienceProtocolAuthenticator
		req     *http.Request
		wantErr error
	}{
		{name: "missing key", auth: &aiAudienceProtocolAuthenticator{now: func() time.Time { return now }}, req: request("1787731200"), wantErr: legacyaudience.ErrUnavailable},
		{name: "expired", auth: &aiAudienceProtocolAuthenticator{key: key, now: func() time.Time { return now }}, req: request("1787730899"), wantErr: legacyaudience.ErrUnauthenticated},
		{name: "future", auth: &aiAudienceProtocolAuthenticator{key: key, now: func() time.Time { return now }}, req: request("1787731261"), wantErr: legacyaudience.ErrUnauthenticated},
	}
	wrongClient := request("1787731200")
	wrongClient.Header.Set("X-AICRM-Client-Id", "other")
	tests = append(tests, struct {
		name    string
		auth    *aiAudienceProtocolAuthenticator
		req     *http.Request
		wantErr error
	}{name: "wrong client", auth: &aiAudienceProtocolAuthenticator{key: key, now: func() time.Time { return now }}, req: wrongClient, wantErr: legacyaudience.ErrUnauthenticated})
	duplicate := request("1787731200")
	duplicate.Header.Add("X-AICRM-Event-Id", "transport-event-0002")
	tests = append(tests, struct {
		name    string
		auth    *aiAudienceProtocolAuthenticator
		req     *http.Request
		wantErr error
	}{name: "duplicate header", auth: &aiAudienceProtocolAuthenticator{key: key, now: func() time.Time { return now }}, req: duplicate, wantErr: legacyaudience.ErrUnauthenticated})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.auth.Authenticate(context.Background(), test.req, body); !errors.Is(err, test.wantErr) {
				t.Fatalf("err=%v want=%v", err, test.wantErr)
			}
		})
	}
}
