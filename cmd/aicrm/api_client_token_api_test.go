package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
)

func TestAPIClientTokenRouteInvokesConfiguredPublicHandler(t *testing.T) {
	credential := apiClientTokenCredential(t)
	reader := &apiClientCredentialStub{credential: credential}
	secrets := &apiClientSecretVerifierStub{secretRef: credential.SecretRef, secret: "client-secret"}
	tokenHandler := newAPIClientTokenHandler(reader, secrets, []byte("01234567890123456789012345678901"), false)
	authService := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(authService)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, &candidateHandler{
		Handler:        authHandler,
		apiClientToken: tokenHandler,
	})
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, apiClientTokenRequest(t, apiClientTokenForm(), "identity.reader", "client-secret"))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Pragma") != "no-cache" || reader.calls != 1 || secrets.calls != 1 {
		t.Fatalf("token route status=%d headers=%#v reader=%d verifier=%d body=%s", response.Code, response.Header(), reader.calls, secrets.calls, response.Body.String())
	}
	if capabilities := authService.capabilities(); len(capabilities) != 0 {
		t.Fatalf("public token route must not invoke browser-session authorization: %v", capabilities)
	}

	wrongMethod := httptest.NewRecorder()
	router.ServeHTTP(wrongMethod, httptest.NewRequest(http.MethodGet, "https://service.test/oauth/token", nil))
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != http.MethodPost || reader.calls != 1 || secrets.calls != 1 {
		t.Fatalf("token route method guard status=%d allow=%q reader=%d verifier=%d body=%s", wrongMethod.Code, wrongMethod.Header().Get("Allow"), reader.calls, secrets.calls, wrongMethod.Body.String())
	}
}

func TestAPIClientTokenRouteFailsClosedWithoutComposedHandler(t *testing.T) {
	authHandler, err := authhttp.NewHandler(&recordingAuth{})
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, &candidateHandler{Handler: authHandler})
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, apiClientTokenRequest(t, apiClientTokenForm(), "identity.reader", "client-secret"))
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("unconfigured token route status=%d headers=%#v body=%s", response.Code, response.Header(), response.Body.String())
	}
}
