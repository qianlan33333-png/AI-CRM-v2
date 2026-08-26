package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	adminopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/app"
	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
)

const (
	apiClientTokenMaximumFormSize = 8 << 10
	apiClientTokenWWWAuthenticate = `Basic realm="oauth2/client"`
)

// apiClientSecretVerifier is deliberately local to the composition root.
// Admin Ops persists a secret reference, not secret material or a hash, so a
// production route must inject the approved secret-store verifier.
type apiClientSecretVerifier interface {
	VerifyAPIClientSecret(context.Context, string, string) (bool, error)
}

// apiClientTokenHandler is an unregistered client-credentials endpoint. Route
// registration and the public protocol contract remain separately owned.
type apiClientTokenHandler struct {
	credentials apiClientCredentialReader
	secrets     apiClientSecretVerifier
	key         []byte
	production  bool
	now         func() time.Time
}

func newAPIClientTokenHandler(credentials apiClientCredentialReader, secrets apiClientSecretVerifier, key []byte, production bool) *apiClientTokenHandler {
	if nilLegacyDependency(credentials) || nilLegacyDependency(secrets) || len(key) != sha256.Size {
		return nil
	}
	return &apiClientTokenHandler{
		credentials: credentials,
		secrets:     secrets,
		key:         append([]byte(nil), key...),
		production:  production,
		now:         time.Now,
	}
}

func (handler *apiClientTokenHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setAPIClientTokenNoStore(writer)
	if handler == nil || request == nil || nilLegacyDependency(handler.credentials) || nilLegacyDependency(handler.secrets) || len(handler.key) != sha256.Size || handler.now == nil {
		writeAPIClientTokenError(writer, http.StatusServiceUnavailable, "temporarily_unavailable", false)
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeAPIClientTokenError(writer, http.StatusMethodNotAllowed, "invalid_request", false)
		return
	}
	if handler.production && request.TLS == nil {
		writeAPIClientTokenError(writer, http.StatusBadRequest, "invalid_request", false)
		return
	}
	clientID, clientSecret, input, ok := parseAPIClientTokenRequest(writer, request)
	if !ok {
		return
	}
	credential, err := handler.credentials.GetCredential(request.Context(), adminopsport.CredentialAPIClient, clientID)
	if err != nil {
		if errors.Is(err, adminopsapp.ErrUnavailable) {
			writeAPIClientTokenError(writer, http.StatusServiceUnavailable, "temporarily_unavailable", false)
			return
		}
		writeAPIClientTokenError(writer, http.StatusUnauthorized, "invalid_client", true)
		return
	}
	policy, policyOK := apiClientPolicy(credential, apiClientIdentityExpectation)
	if !policyOK || credential.State != "active" || credential.ClientID != clientID || credential.SecretRef == "" {
		writeAPIClientTokenError(writer, http.StatusUnauthorized, "invalid_client", true)
		return
	}
	verified, err := handler.secrets.VerifyAPIClientSecret(request.Context(), credential.SecretRef, clientSecret)
	if err != nil {
		writeAPIClientTokenError(writer, http.StatusServiceUnavailable, "temporarily_unavailable", false)
		return
	}
	if !verified {
		writeAPIClientTokenError(writer, http.StatusUnauthorized, "invalid_client", true)
		return
	}
	if !allowedAPIClientSource(policy.AllowedSourceCIDRs, request.RemoteAddr) {
		writeAPIClientTokenError(writer, http.StatusForbidden, "access_denied", false)
		return
	}
	if input.audience != policy.Audience {
		writeAPIClientTokenError(writer, http.StatusBadRequest, "invalid_target", false)
		return
	}
	if input.scope != policy.Scope {
		writeAPIClientTokenError(writer, http.StatusBadRequest, "invalid_scope", false)
		return
	}
	now := handler.now().UTC()
	claims := apiClientJWTClaims{
		Subject:           credential.ClientID,
		Audience:          policy.Audience,
		Purpose:           policy.Purpose,
		Capability:        policy.Capability,
		CredentialVersion: credential.Version,
		IssuedAt:          now.Unix(),
		NotBefore:         now.Unix(),
		ExpiresAt:         now.Add(time.Duration(policy.TTLMinutes) * time.Minute).Unix(),
	}
	token, ok := signAPIClientJWT(handler.key, claims)
	if !ok {
		writeAPIClientTokenError(writer, http.StatusServiceUnavailable, "temporarily_unavailable", false)
		return
	}
	writeAPIClientTokenJSON(writer, http.StatusOK, map[string]any{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   policy.TTLMinutes * 60,
		"scope":        policy.Scope,
	})
}

type apiClientTokenInput struct{ audience, scope string }

func parseAPIClientTokenRequest(writer http.ResponseWriter, request *http.Request) (string, string, apiClientTokenInput, bool) {
	if request.URL == nil || request.URL.RawQuery != "" {
		writeAPIClientTokenError(writer, http.StatusBadRequest, "invalid_request", false)
		return "", "", apiClientTokenInput{}, false
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		writeAPIClientTokenError(writer, http.StatusBadRequest, "invalid_request", false)
		return "", "", apiClientTokenInput{}, false
	}
	request.Body = http.MaxBytesReader(writer, request.Body, apiClientTokenMaximumFormSize)
	if err := request.ParseForm(); err != nil {
		writeAPIClientTokenError(writer, http.StatusBadRequest, "invalid_request", false)
		return "", "", apiClientTokenInput{}, false
	}
	grantType, ok := singleAPIClientTokenFormValue(request.PostForm, "grant_type", true)
	if !ok {
		writeAPIClientTokenError(writer, http.StatusBadRequest, "invalid_request", false)
		return "", "", apiClientTokenInput{}, false
	}
	if grantType != "client_credentials" {
		writeAPIClientTokenError(writer, http.StatusBadRequest, "unsupported_grant_type", false)
		return "", "", apiClientTokenInput{}, false
	}
	audience, ok := singleAPIClientTokenFormValue(request.PostForm, "audience", true)
	if !ok || !validAPIClientAudience(audience) {
		writeAPIClientTokenError(writer, http.StatusBadRequest, "invalid_request", false)
		return "", "", apiClientTokenInput{}, false
	}
	scope, ok := singleAPIClientTokenFormValue(request.PostForm, "scope", false)
	if !ok || !validAPIClientScope(scope) {
		writeAPIClientTokenError(writer, http.StatusBadRequest, "invalid_request", false)
		return "", "", apiClientTokenInput{}, false
	}
	clientID, clientSecret, basic, basicOK := basicAPIClientTokenCredentials(request.Header.Values("Authorization"))
	if !basicOK {
		writeAPIClientTokenError(writer, http.StatusUnauthorized, "invalid_client", true)
		return "", "", apiClientTokenInput{}, false
	}
	formID, formIDOK := singleAPIClientTokenFormValue(request.PostForm, "client_id", false)
	formSecret, formSecretOK := singleAPIClientTokenFormValue(request.PostForm, "client_secret", false)
	if !formIDOK || !formSecretOK || basic && (formID != "" || formSecret != "") {
		writeAPIClientTokenError(writer, http.StatusBadRequest, "invalid_request", false)
		return "", "", apiClientTokenInput{}, false
	}
	if !basic {
		clientID, clientSecret = formID, formSecret
	}
	if !validAPIClientID(clientID) || !validAPIClientSecret(clientSecret) {
		writeAPIClientTokenError(writer, http.StatusUnauthorized, "invalid_client", true)
		return "", "", apiClientTokenInput{}, false
	}
	return clientID, clientSecret, apiClientTokenInput{audience: audience, scope: scope}, true
}

func basicAPIClientTokenCredentials(values []string) (string, string, bool, bool) {
	if len(values) == 0 {
		return "", "", false, true
	}
	if len(values) != 1 || !strings.HasPrefix(values[0], "Basic ") || strings.TrimSpace(values[0]) != values[0] {
		return "", "", false, false
	}
	encoded := strings.TrimPrefix(values[0], "Basic ")
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.StdEncoding.EncodeToString(decoded) != encoded {
		return "", "", false, false
	}
	pair := strings.SplitN(string(decoded), ":", 2)
	if len(pair) != 2 || pair[0] == "" || pair[1] == "" {
		return "", "", false, false
	}
	return pair[0], pair[1], true, true
}

func singleAPIClientTokenFormValue(form map[string][]string, key string, required bool) (string, bool) {
	values, present := form[key]
	if !present {
		return "", !required
	}
	if len(values) != 1 || strings.TrimSpace(values[0]) != values[0] {
		return "", false
	}
	return values[0], !required || values[0] != ""
}

func validAPIClientAudience(value string) bool {
	return value != "" && len(value) <= 120 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n\t ")
}

func validAPIClientScope(value string) bool {
	if len(value) > 500 || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n\t") {
		return false
	}
	for _, scope := range strings.Fields(value) {
		if len(scope) > 120 || strings.ContainsAny(scope, "\"\\") {
			return false
		}
	}
	return true
}

func validAPIClientSecret(value string) bool {
	return value != "" && len(value) <= 1024 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}

func allowedAPIClientSource(allowed []string, remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	address, err := netip.ParseAddr(host)
	if err != nil || len(allowed) == 0 {
		return false
	}
	matched := false
	for _, raw := range allowed {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || prefix.String() != raw {
			return false
		}
		if prefix.Contains(address) {
			matched = true
		}
	}
	return matched
}

func signAPIClientJWT(key []byte, claims apiClientJWTClaims) (string, bool) {
	if len(key) != sha256.Size {
		return "", false
	}
	header, err := json.Marshal(apiClientJWTHeader{Algorithm: "HS256", Type: "JWT"})
	if err != nil {
		return "", false
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", false
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(encodedHeader + "." + encodedPayload))
	return encodedHeader + "." + encodedPayload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), true
}

func setAPIClientTokenNoStore(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Pragma", "no-cache")
}

func writeAPIClientTokenError(writer http.ResponseWriter, status int, code string, basicChallenge bool) {
	if basicChallenge {
		writer.Header().Set("WWW-Authenticate", apiClientTokenWWWAuthenticate)
	}
	writeAPIClientTokenJSON(writer, status, map[string]string{"error": code})
}

func writeAPIClientTokenJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

var _ http.Handler = (*apiClientTokenHandler)(nil)
