package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	adminopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/app"
	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	appruntime "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/runtime"
)

type apiClientSecretVerifierStub struct {
	secretRef string
	secret    string
	err       error
	calls     int
}

func (stub *apiClientSecretVerifierStub) VerifyAPIClientSecret(_ context.Context, secretRef, secret string) (bool, error) {
	stub.calls++
	if stub.err != nil {
		return false, stub.err
	}
	return secretRef == stub.secretRef && secret == stub.secret, nil
}

func TestAPIClientTokenHandlerIssuesReusableIdentityJWTWithBasicCredentials(t *testing.T) {
	now := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	credential := apiClientTokenCredential(t)
	reader := &apiClientCredentialStub{credential: credential}
	secrets := &apiClientSecretVerifierStub{secretRef: credential.SecretRef, secret: "client-secret"}
	handler := newAPIClientTokenHandler(reader, secrets, key, true)
	handler.now = func() time.Time { return now }

	response := httptest.NewRecorder()
	request := apiClientTokenRequest(t, url.Values{"grant_type": {"client_credentials"}, "audience": {"identity"}}, "identity.reader", "client-secret")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Pragma") != "no-cache" || secrets.calls != 1 || reader.calls != 1 {
		t.Fatalf("token response status=%d headers=%#v reader=%d secret=%d body=%s", response.Code, response.Header(), reader.calls, secrets.calls, response.Body.String())
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
		Scope       string `json:"scope"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload.AccessToken == "" || payload.TokenType != "Bearer" || payload.ExpiresIn != 300 || payload.Scope != "" {
		t.Fatalf("token payload=%#v err=%v", payload, err)
	}
	authenticator := newAPIClientJWTAuthenticator(reader, key).(*apiClientJWTAuthenticator)
	authenticator.now = func() time.Time { return now }
	protected := httptest.NewRequest(http.MethodGet, "/api/identity/resolve?unionid=fixture", nil)
	protected.Header.Set("Authorization", "Bearer "+payload.AccessToken)
	principal, err := authenticator.AuthenticateOperation(context.Background(), protected, apiClientIdentityPurpose)
	if err != nil || principal != (operationServicePrincipal{ClientID: "identity.reader", PrincipalID: "api-client:identity.reader"}) || reader.calls != 2 {
		t.Fatalf("issued token authentication=%#v err=%v reader=%d", principal, err, reader.calls)
	}
}

func TestAPIClientTokenHandlerAcceptsFormCredentialsOnly(t *testing.T) {
	credential := apiClientTokenCredential(t)
	reader := &apiClientCredentialStub{credential: credential}
	secrets := &apiClientSecretVerifierStub{secretRef: credential.SecretRef, secret: "client-secret"}
	handler := newAPIClientTokenHandler(reader, secrets, []byte("01234567890123456789012345678901"), false)
	form := url.Values{"grant_type": {"client_credentials"}, "audience": {"identity"}, "client_id": {"identity.reader"}, "client_secret": {"client-secret"}}
	request := httptest.NewRequest(http.MethodPost, "http://service.test/oauth/token", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.RemoteAddr = "198.51.100.8:443"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || secrets.calls != 1 {
		t.Fatalf("form client credentials status=%d verifier=%d body=%s", response.Code, secrets.calls, response.Body.String())
	}
}

func TestAPIClientTokenHandlerIssuesWithConfiguredEnvironmentSecretMap(t *testing.T) {
	credential := apiClientTokenCredential(t)
	clientSecret := base64.RawURLEncoding.EncodeToString([]byte("abcdefghijklmnopqrstuvwxyzABCDEF"))
	jwtSecret := base64.RawURLEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	t.Setenv("AICRM_DATABASE_URL", "postgres://db/aicrm")
	t.Setenv("AICRM_HTTP_LISTEN_ADDRESS", "127.0.0.1:8080")
	t.Setenv("AICRM_API_PGX_MAX_CONNS", "1")
	t.Setenv("AICRM_IDENTITY_HMAC_KEY", strings.Repeat("A", 43))
	t.Setenv("AICRM_ENV", "production")
	t.Setenv("AICRM_API_CLIENT_JWT_SECRET", jwtSecret)
	t.Setenv("AICRM_API_CLIENT_SECRET_MAP", `{"`+credential.SecretRef+`":"`+clientSecret+`"}`)
	config, err := appconfig.Load(appruntime.RoleAPI)
	if err != nil || !config.APIClient.JWTSecret.Configured() || !config.APIClient.SecretMap.Configured() {
		t.Fatalf("API-client environment config = %#v, %v", config.APIClient, err)
	}
	handler := newAPIClientTokenHandler(&apiClientCredentialStub{credential: credential}, config.APIClient.SecretMap, config.APIClient.JWTSecret.Value(), true)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, apiClientTokenRequest(t, apiClientTokenForm(), credential.ClientID, clientSecret))
	if response.Code != http.StatusOK {
		t.Fatalf("environment-configured token issue status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAPIClientTokenHandlerFailsClosedForTransportCredentialsAndPolicy(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	base := apiClientTokenCredential(t)
	tests := []struct {
		name            string
		mutate          func(*adminopsport.Credential, *apiClientSecretVerifierStub)
		readerErr       error
		form            url.Values
		basic           bool
		setTLS          bool
		remoteAddr      string
		wantStatus      int
		wantError       string
		wantSecretCalls int
		wantReaderCalls int
		wantChallenge   bool
	}{
		{name: "production HTTP", form: apiClientTokenForm(), basic: true, setTLS: false, remoteAddr: "198.51.100.7:443", wantStatus: http.StatusBadRequest, wantError: "invalid_request"},
		{name: "wrong method", form: apiClientTokenForm(), basic: true, setTLS: true, remoteAddr: "198.51.100.7:443", wantStatus: http.StatusMethodNotAllowed, wantError: "invalid_request"},
		{name: "malformed basic", form: apiClientTokenForm(), basic: false, setTLS: true, remoteAddr: "198.51.100.7:443", wantStatus: http.StatusUnauthorized, wantError: "invalid_client", wantChallenge: true},
		{name: "basic and form credentials", form: url.Values{"grant_type": {"client_credentials"}, "audience": {"identity"}, "client_id": {"identity.reader"}, "client_secret": {"client-secret"}}, basic: true, setTLS: true, remoteAddr: "198.51.100.7:443", wantStatus: http.StatusBadRequest, wantError: "invalid_request"},
		{name: "wrong secret", form: apiClientTokenForm(), basic: true, setTLS: true, remoteAddr: "198.51.100.7:443", mutate: func(_ *adminopsport.Credential, verifier *apiClientSecretVerifierStub) { verifier.secret = "different" }, wantStatus: http.StatusUnauthorized, wantError: "invalid_client", wantSecretCalls: 1, wantReaderCalls: 1, wantChallenge: true},
		{name: "disabled credential", form: apiClientTokenForm(), basic: true, setTLS: true, remoteAddr: "198.51.100.7:443", mutate: func(credential *adminopsport.Credential, _ *apiClientSecretVerifierStub) {
			credential.State = "disabled"
		}, wantStatus: http.StatusUnauthorized, wantError: "invalid_client", wantReaderCalls: 1, wantChallenge: true},
		{name: "wrong source IP", form: apiClientTokenForm(), basic: true, setTLS: true, remoteAddr: "203.0.113.9:443", wantStatus: http.StatusForbidden, wantError: "access_denied", wantSecretCalls: 1, wantReaderCalls: 1},
		{name: "forwarded source is not trusted", form: apiClientTokenForm(), basic: true, setTLS: true, remoteAddr: "203.0.113.9:443", wantStatus: http.StatusForbidden, wantError: "access_denied", wantSecretCalls: 1, wantReaderCalls: 1},
		{name: "invalid audience", form: url.Values{"grant_type": {"client_credentials"}, "audience": {"operations"}}, basic: true, setTLS: true, remoteAddr: "198.51.100.7:443", wantStatus: http.StatusBadRequest, wantError: "invalid_target", wantSecretCalls: 1, wantReaderCalls: 1},
		{name: "invalid scope", form: url.Values{"grant_type": {"client_credentials"}, "audience": {"identity"}, "scope": {"customers.read"}}, basic: true, setTLS: true, remoteAddr: "198.51.100.7:443", wantStatus: http.StatusBadRequest, wantError: "invalid_scope", wantSecretCalls: 1, wantReaderCalls: 1},
		{name: "unsupported grant", form: url.Values{"grant_type": {"authorization_code"}, "audience": {"identity"}}, basic: true, setTLS: true, remoteAddr: "198.51.100.7:443", wantStatus: http.StatusBadRequest, wantError: "unsupported_grant_type"},
		{name: "credential service unavailable", form: apiClientTokenForm(), basic: true, setTLS: true, remoteAddr: "198.51.100.7:443", readerErr: adminopsapp.ErrUnavailable, wantStatus: http.StatusServiceUnavailable, wantError: "temporarily_unavailable", wantReaderCalls: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credential := base
			verifier := &apiClientSecretVerifierStub{secretRef: credential.SecretRef, secret: "client-secret"}
			if test.mutate != nil {
				test.mutate(&credential, verifier)
			}
			reader := &apiClientCredentialStub{credential: credential, err: test.readerErr}
			handler := newAPIClientTokenHandler(reader, verifier, key, true)
			response := httptest.NewRecorder()
			request := apiClientTokenRequest(t, test.form, "identity.reader", "client-secret")
			if !test.basic {
				request.Header.Del("Authorization")
			}
			if test.name == "malformed basic" {
				request.Header.Set("Authorization", "Basic not-a-canonical-value")
			}
			if test.name == "wrong method" {
				request.Method = http.MethodGet
			}
			if !test.setTLS {
				request.TLS = nil
			}
			request.RemoteAddr = test.remoteAddr
			if test.name == "forwarded source is not trusted" {
				request.Header.Set("X-Forwarded-For", "198.51.100.7")
			}
			handler.ServeHTTP(response, request)
			var payload map[string]string
			if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || response.Code != test.wantStatus || payload["error"] != test.wantError || verifier.calls != test.wantSecretCalls || reader.calls != test.wantReaderCalls {
				t.Fatalf("status=%d payload=%#v decode=%v verifier=%d reader=%d", response.Code, payload, err, verifier.calls, reader.calls)
			}
			if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Pragma") != "no-cache" || (test.wantChallenge && response.Header().Get("WWW-Authenticate") != apiClientTokenWWWAuthenticate) || (!test.wantChallenge && response.Header().Get("WWW-Authenticate") != "") {
				t.Fatalf("headers=%#v", response.Header())
			}
		})
	}
}

func TestAPIClientTokenHandlerMapsSecretVerifierFailureToUnavailable(t *testing.T) {
	credential := apiClientTokenCredential(t)
	reader := &apiClientCredentialStub{credential: credential}
	verifier := &apiClientSecretVerifierStub{secretRef: credential.SecretRef, secret: "client-secret", err: errors.New("secret store unavailable")}
	handler := newAPIClientTokenHandler(reader, verifier, []byte("01234567890123456789012345678901"), false)
	response := httptest.NewRecorder()
	request := apiClientTokenRequest(t, apiClientTokenForm(), "identity.reader", "client-secret")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || verifier.calls != 1 {
		t.Fatalf("secret verifier failure status=%d calls=%d", response.Code, verifier.calls)
	}
}

func TestAllowedAPIClientSourceFailsClosedForMalformedPolicy(t *testing.T) {
	if allowedAPIClientSource([]string{"198.51.100.0/24", "not-a-cidr"}, "198.51.100.7:443") {
		t.Fatal("a malformed CIDR must not be bypassed by an earlier matching CIDR")
	}
}

func apiClientTokenCredential(t *testing.T) adminopsport.Credential {
	t.Helper()
	metadata, err := json.Marshal(map[string]any{
		"purpose":              "identity",
		"capability":           "identity.resolve",
		"audience":             "identity",
		"token_ttl_minutes":    5,
		"allowed_source_cidrs": []string{"198.51.100.0/24"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return adminopsport.Credential{Kind: adminopsport.CredentialAPIClient, ClientID: "identity.reader", State: "active", SecretRef: "secret://adminops/api_client/identity.reader/current", Metadata: metadata, Version: 3}
}

func apiClientTokenForm() url.Values {
	return url.Values{"grant_type": {"client_credentials"}, "audience": {"identity"}}
}

func apiClientTokenRequest(t *testing.T, form url.Values, clientID, clientSecret string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "https://service.test/oauth/token", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(clientID, clientSecret)
	request.RemoteAddr = "198.51.100.7:443"
	request.TLS = &tls.ConnectionState{}
	return request
}

var _ apiClientSecretVerifier = (*apiClientSecretVerifierStub)(nil)
