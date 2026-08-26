package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
)

type groupOpsProtocolReplayStub struct {
	created         bool
	err             error
	resource, event string
	payload         [32]byte
}

func (stub *groupOpsProtocolReplayStub) Reserve(_ context.Context, resource, event string, payload [32]byte) (bool, error) {
	stub.resource, stub.event, stub.payload = resource, event, payload
	return stub.created, stub.err
}

func TestGroupOpsProtocolAuthenticatorAcceptsExactBroadcastPolicy(t *testing.T) {
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	metadata, err := json.Marshal(map[string]any{
		"purpose": "group_broadcast", "audience": "external_integration",
		"capability": "group_broadcast_execute", "scope": "write", "token_ttl_minutes": 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := &apiClientCredentialStub{credential: adminopsport.Credential{
		Kind: adminopsport.CredentialAPIClient, ClientID: "identity.reader", State: "active", Metadata: metadata, Version: 3,
	}}
	jwt := newAPIClientJWTAuthenticator(reader, key).(*apiClientJWTAuthenticator)
	jwt.now = func() time.Time { return now }
	authenticator := &groupOpsProtocolAuthenticator{jwt: jwt, now: func() time.Time { return now }}
	request := httptest.NewRequest("POST", "/api/automation/group-ops/broadcast", nil)
	request.Header.Set("Authorization", "Bearer "+apiClientIdentityJWT(t, key, apiClientJWTClaims{
		Subject: "identity.reader", Audience: "external_integration", Purpose: "group_broadcast", Capability: "group_broadcast_execute",
		CredentialVersion: 3, IssuedAt: now.Add(-time.Minute).Unix(), NotBefore: now.Add(-time.Minute).Unix(), ExpiresAt: now.Add(4 * time.Minute).Unix(),
	}))
	principal, err := authenticator.Authenticate(context.Background(), request, "group_ops_broadcast", "service", nil)
	if err != nil || principal.ID != "identity.reader" || reader.calls != 1 {
		t.Fatalf("principal=%#v err=%v calls=%d", principal, err, reader.calls)
	}

	reader.credential.Metadata = []byte(`{"purpose":"group_broadcast","audience":"external_integration","capability":"group_broadcast_execute","scope":"read","token_ttl_minutes":5}`)
	if _, err = authenticator.Authenticate(context.Background(), request, "group_ops_broadcast", "service", nil); err == nil {
		t.Fatal("read-scoped credential authorized a write broadcast")
	}
}

func TestGroupOpsProtocolAuthenticatorVerifiesWebhookAndReservesExactReplayPayload(t *testing.T) {
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	replay := &groupOpsProtocolReplayStub{created: true}
	authenticator := &groupOpsProtocolAuthenticator{webhookKey: key, replay: replay, now: func() time.Time { return now }}
	body := []byte(`{"event":"accepted-only"}`)
	eventID := "event-0123456789abcdef"
	request := httptest.NewRequest("POST", "/api/automation/group-ops/webhooks/hook-1", nil)
	request.Header.Set("X-AICRM-Client-Id", groupOpsWebhookClientID)
	request.Header.Set("X-AICRM-Timestamp", "1787738400")
	request.Header.Set("X-AICRM-Event-Id", eventID)
	request.Header.Set("X-AICRM-Signature", "sha256="+groupOpsWebhookSignature(key, "1787738400", eventID, body))
	principal, err := authenticator.Authenticate(context.Background(), request, "group_ops_webhook", "hook-1", body)
	wantPayload := sha256.Sum256(append([]byte("hook-1\n"), body...))
	if err != nil || principal.ID != eventID || replay.resource != "hook-1" || replay.event != eventID || replay.payload != wantPayload {
		t.Fatalf("principal=%#v err=%v replay=%#v", principal, err, replay)
	}
}

func TestGroupOpsProtocolAuthenticatorRejectsWebhookDriftAndReplay(t *testing.T) {
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	body := []byte(`{"event":"accepted-only"}`)
	eventID := "event-0123456789abcdef"
	request := httptest.NewRequest("POST", "/api/automation/group-ops/webhooks/hook-1", nil)
	request.Header.Set("X-AICRM-Client-Id", groupOpsWebhookClientID)
	request.Header.Set("X-AICRM-Timestamp", "1787738400")
	request.Header.Set("X-AICRM-Event-Id", eventID)
	request.Header.Set("X-AICRM-Signature", groupOpsWebhookSignature(key, "1787738400", eventID, body))

	tests := []struct {
		name   string
		mutate func(*groupOpsProtocolAuthenticator, []byte)
	}{
		{name: "body drift", mutate: func(_ *groupOpsProtocolAuthenticator, value []byte) { value[0] = '[' }},
		{name: "expired", mutate: func(value *groupOpsProtocolAuthenticator, _ []byte) {
			value.now = func() time.Time { return now.Add(6 * time.Minute) }
		}},
		{name: "future", mutate: func(value *groupOpsProtocolAuthenticator, _ []byte) {
			value.now = func() time.Time { return now.Add(-2 * time.Minute) }
		}},
		{name: "replay", mutate: func(value *groupOpsProtocolAuthenticator, _ []byte) {
			value.replay = &groupOpsProtocolReplayStub{created: false}
		}},
		{name: "store unavailable", mutate: func(value *groupOpsProtocolAuthenticator, _ []byte) {
			value.replay = &groupOpsProtocolReplayStub{err: errors.New("db unavailable")}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidateBody := append([]byte(nil), body...)
			authenticator := &groupOpsProtocolAuthenticator{webhookKey: key, replay: &groupOpsProtocolReplayStub{created: true}, now: func() time.Time { return now }}
			test.mutate(authenticator, candidateBody)
			if _, err := authenticator.Authenticate(context.Background(), request, "group_ops_webhook", "hook-1", candidateBody); err == nil {
				t.Fatal("invalid webhook authenticated")
			}
		})
	}
}

func groupOpsWebhookSignature(key []byte, timestamp, eventID string, body []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(timestamp + "\n" + eventID + "\n"))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
