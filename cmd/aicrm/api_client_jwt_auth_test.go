package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	adminopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/app"
	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

type apiClientCredentialStub struct {
	credential adminopsport.Credential
	err        error
	calls      int
}

func (stub *apiClientCredentialStub) GetCredential(_ context.Context, kind adminopsport.CredentialKind, clientID string) (adminopsport.Credential, error) {
	stub.calls++
	if kind != adminopsport.CredentialAPIClient || clientID != "identity.reader" {
		return adminopsport.Credential{}, errors.New("unexpected credential query")
	}
	return stub.credential, stub.err
}

func TestAPIClientJWTAuthenticatorAcceptsExactActiveIdentityPolicy(t *testing.T) {
	now := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	reader := &apiClientCredentialStub{credential: apiClientIdentityCredential(t)}
	authenticator := newAPIClientJWTAuthenticator(reader, key).(*apiClientJWTAuthenticator)
	authenticator.now = func() time.Time { return now }
	request := httptest.NewRequest("GET", "/api/identity/resolve?unionid=fixture", nil)
	request.Header.Set("Authorization", "Bearer "+apiClientIdentityJWT(t, key, apiClientJWTClaims{
		Subject: "identity.reader", Audience: "identity", Purpose: "identity", Capability: "identity.resolve",
		CredentialVersion: 3, IssuedAt: now.Add(-time.Minute).Unix(), NotBefore: now.Add(-time.Minute).Unix(), ExpiresAt: now.Add(4 * time.Minute).Unix(),
	}))
	principal, err := authenticator.AuthenticateOperation(context.Background(), request, "identity")
	if err != nil || principal != (operationServicePrincipal{ClientID: "identity.reader", PrincipalID: "api-client:identity.reader"}) || reader.calls != 1 {
		t.Fatalf("AuthenticateOperation() = %#v, %v calls=%d", principal, err, reader.calls)
	}
}

func TestAPIClientJWTAuthenticatorRejectsBeforeOrAfterCredentialRead(t *testing.T) {
	now := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	base := apiClientJWTClaims{Subject: "identity.reader", Audience: "identity", Purpose: "identity", Capability: "identity.resolve", CredentialVersion: 3,
		IssuedAt: now.Add(-time.Minute).Unix(), NotBefore: now.Add(-time.Minute).Unix(), ExpiresAt: now.Add(4 * time.Minute).Unix()}
	tests := []struct {
		name       string
		mutate     func(*apiClientJWTClaims, *adminopsport.Credential)
		authHeader string
		purpose    string
		readerErr  error
		want       error
		wantCalls  int
	}{
		{name: "missing bearer", authHeader: "", purpose: "identity", want: authport.ErrUnauthorized},
		{name: "wrong purpose argument", purpose: "operations", want: authport.ErrAuthenticationUnavailable},
		{name: "expired", purpose: "identity", mutate: func(claims *apiClientJWTClaims, _ *adminopsport.Credential) {
			claims.ExpiresAt = now.Add(-time.Minute).Unix()
		}, want: authport.ErrUnauthorized},
		{name: "wrong claim purpose", purpose: "identity", mutate: func(claims *apiClientJWTClaims, _ *adminopsport.Credential) { claims.Purpose = "operations" }, want: authport.ErrUnauthorized},
		{name: "disabled", purpose: "identity", mutate: func(_ *apiClientJWTClaims, credential *adminopsport.Credential) { credential.State = "disabled" }, want: authport.ErrUnauthorized, wantCalls: 1},
		{name: "version mismatch", purpose: "identity", mutate: func(_ *apiClientJWTClaims, credential *adminopsport.Credential) { credential.Version = 4 }, want: authport.ErrUnauthorized, wantCalls: 1},
		{name: "policy mismatch", purpose: "identity", mutate: func(_ *apiClientJWTClaims, credential *adminopsport.Credential) {
			credential.Metadata = []byte(`{"purpose":"identity","capability":"customers.read","audience":"identity","token_ttl_minutes":5}`)
		}, want: authport.ErrUnauthorized, wantCalls: 1},
		{name: "credential unavailable", purpose: "identity", readerErr: adminopsapp.ErrUnavailable, want: authport.ErrAuthenticationUnavailable, wantCalls: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims, credential := base, apiClientIdentityCredential(t)
			if test.mutate != nil {
				test.mutate(&claims, &credential)
			}
			reader := &apiClientCredentialStub{credential: credential, err: test.readerErr}
			authenticator := newAPIClientJWTAuthenticator(reader, key).(*apiClientJWTAuthenticator)
			authenticator.now = func() time.Time { return now }
			request := httptest.NewRequest("GET", "/api/identity/resolve", nil)
			header := test.authHeader
			if header == "" && test.name != "missing bearer" {
				header = "Bearer " + apiClientIdentityJWT(t, key, claims)
			}
			if header != "" {
				request.Header.Set("Authorization", header)
			}
			_, err := authenticator.AuthenticateOperation(context.Background(), request, test.purpose)
			if !errors.Is(err, test.want) || reader.calls != test.wantCalls {
				t.Fatalf("error=%v calls=%d, want %v/%d", err, reader.calls, test.want, test.wantCalls)
			}
		})
	}
}

func TestAPIClientJWTAuthenticatorRejectsBadSignatureAndDuplicateHeaders(t *testing.T) {
	now := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	reader := &apiClientCredentialStub{credential: apiClientIdentityCredential(t)}
	authenticator := newAPIClientJWTAuthenticator(reader, key).(*apiClientJWTAuthenticator)
	authenticator.now = func() time.Time { return now }
	claims := apiClientJWTClaims{Subject: "identity.reader", Audience: "identity", Purpose: "identity", Capability: "identity.resolve", CredentialVersion: 3,
		IssuedAt: now.Add(-time.Minute).Unix(), NotBefore: now.Add(-time.Minute).Unix(), ExpiresAt: now.Add(4 * time.Minute).Unix()}
	bad := httptest.NewRequest("GET", "/api/identity/resolve", nil)
	bad.Header.Set("Authorization", "Bearer "+apiClientIdentityJWT(t, []byte("different-key-different-key-123456"), claims))
	if _, err := authenticator.AuthenticateOperation(context.Background(), bad, "identity"); !errors.Is(err, authport.ErrUnauthorized) || reader.calls != 0 {
		t.Fatalf("bad signature error=%v calls=%d", err, reader.calls)
	}
	duplicate := httptest.NewRequest("GET", "/api/identity/resolve", nil)
	value := "Bearer " + apiClientIdentityJWT(t, key, claims)
	duplicate.Header.Add("Authorization", value)
	duplicate.Header.Add("Authorization", value)
	if _, err := authenticator.AuthenticateOperation(context.Background(), duplicate, "identity"); !errors.Is(err, authport.ErrUnauthorized) || reader.calls != 0 {
		t.Fatalf("duplicate header error=%v calls=%d", err, reader.calls)
	}
}

func apiClientIdentityCredential(t *testing.T) adminopsport.Credential {
	t.Helper()
	metadata, err := json.Marshal(map[string]any{"purpose": "identity", "capability": "identity.resolve", "audience": "identity", "token_ttl_minutes": 5})
	if err != nil {
		t.Fatal(err)
	}
	return adminopsport.Credential{Kind: adminopsport.CredentialAPIClient, ClientID: "identity.reader", State: "active", Metadata: metadata, Version: 3}
}

func apiClientIdentityJWT(t *testing.T, key []byte, claims apiClientJWTClaims) string {
	t.Helper()
	header, err := json.Marshal(apiClientJWTHeader{Algorithm: "HS256", Type: "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(encodedHeader + "." + encodedPayload))
	return encodedHeader + "." + encodedPayload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
