package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	adminopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/app"
	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

const (
	apiClientIdentityPurpose    = "identity"
	apiClientIdentityAudience   = "identity"
	apiClientIdentityCapability = "identity.resolve"
	apiClientJWTMaximumSize     = 8 << 10
	apiClientJWTClockSkew       = 30 * time.Second
)

type apiClientCredentialReader interface {
	GetCredential(context.Context, adminopsport.CredentialKind, string) (adminopsport.Credential, error)
}

type apiClientJWTAuthenticator struct {
	credentials apiClientCredentialReader
	key         []byte
	now         func() time.Time
}

type apiClientJWTHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

type apiClientJWTClaims struct {
	Subject           string `json:"sub"`
	Audience          string `json:"aud"`
	Purpose           string `json:"purpose"`
	Capability        string `json:"capability"`
	CredentialVersion int64  `json:"credential_version"`
	IssuedAt          int64  `json:"iat"`
	NotBefore         int64  `json:"nbf"`
	ExpiresAt         int64  `json:"exp"`
}

type apiClientCredentialPolicy struct {
	Audience           string   `json:"audience"`
	Purpose            string   `json:"purpose"`
	Capability         string   `json:"capability"`
	Scope              string   `json:"scope,omitempty"`
	AllowedSourceCIDRs []string `json:"allowed_source_cidrs,omitempty"`
	TTLMinutes         int64    `json:"token_ttl_minutes"`
}

type apiClientJWTExpectation struct{ Audience, Purpose, Capability, Scope string }

var apiClientIdentityExpectation = apiClientJWTExpectation{Audience: apiClientIdentityAudience, Purpose: apiClientIdentityPurpose, Capability: apiClientIdentityCapability}

func newAPIClientJWTAuthenticator(credentials apiClientCredentialReader, key []byte) operationServiceAuthenticator {
	if nilLegacyDependency(credentials) || len(key) != sha256.Size {
		return nil
	}
	return &apiClientJWTAuthenticator{credentials: credentials, key: append([]byte(nil), key...), now: time.Now}
}

func (authenticator *apiClientJWTAuthenticator) AuthenticateOperation(ctx context.Context, request *http.Request, purpose string) (operationServicePrincipal, error) {
	if authenticator == nil || ctx == nil || request == nil || purpose != apiClientIdentityPurpose ||
		nilLegacyDependency(authenticator.credentials) || len(authenticator.key) != sha256.Size || authenticator.now == nil {
		return operationServicePrincipal{}, authport.ErrAuthenticationUnavailable
	}
	return authenticator.authenticate(ctx, request, apiClientIdentityExpectation)
}

func (authenticator *apiClientJWTAuthenticator) authenticate(ctx context.Context, request *http.Request, expected apiClientJWTExpectation) (operationServicePrincipal, error) {
	claims, err := authenticator.verify(request, expected)
	if err != nil {
		return operationServicePrincipal{}, err
	}
	credential, err := authenticator.credentials.GetCredential(ctx, adminopsport.CredentialAPIClient, claims.Subject)
	if err != nil {
		if errors.Is(err, adminopsapp.ErrUnavailable) {
			return operationServicePrincipal{}, authport.ErrAuthenticationUnavailable
		}
		return operationServicePrincipal{}, authport.ErrUnauthorized
	}
	policy, ok := apiClientPolicy(credential, expected)
	if !ok || credential.State != "active" || credential.ClientID != claims.Subject || credential.Version != claims.CredentialVersion ||
		claims.Purpose != policy.Purpose || claims.Audience != policy.Audience || claims.Capability != policy.Capability || policy.Scope != expected.Scope || claims.ExpiresAt-claims.IssuedAt > policy.TTLMinutes*60 {
		return operationServicePrincipal{}, authport.ErrUnauthorized
	}
	return operationServicePrincipal{ClientID: credential.ClientID, PrincipalID: "api-client:" + credential.ClientID}, nil
}

func (authenticator *apiClientJWTAuthenticator) verify(request *http.Request, expected apiClientJWTExpectation) (apiClientJWTClaims, error) {
	values := request.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") || strings.TrimSpace(values[0]) != values[0] {
		return apiClientJWTClaims{}, authport.ErrUnauthorized
	}
	token := strings.TrimPrefix(values[0], "Bearer ")
	if token == "" || len(token) > apiClientJWTMaximumSize || strings.TrimSpace(token) != token {
		return apiClientJWTClaims{}, authport.ErrUnauthorized
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return apiClientJWTClaims{}, authport.ErrUnauthorized
	}
	var header apiClientJWTHeader
	var claims apiClientJWTClaims
	if !decodeAPIClientJWTSegment(parts[0], &header) || !decodeAPIClientJWTSegment(parts[1], &claims) || header.Algorithm != "HS256" || header.Type != "JWT" {
		return apiClientJWTClaims{}, authport.ErrUnauthorized
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil || len(signature) != sha256.Size || base64.RawURLEncoding.EncodeToString(signature) != parts[2] {
		return apiClientJWTClaims{}, authport.ErrUnauthorized
	}
	mac := hmac.New(sha256.New, authenticator.key)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if subtle.ConstantTimeCompare(signature, mac.Sum(nil)) != 1 {
		return apiClientJWTClaims{}, authport.ErrUnauthorized
	}
	now := authenticator.now().UTC().Unix()
	skew := int64(apiClientJWTClockSkew / time.Second)
	if !validAPIClientID(claims.Subject) || claims.Audience != expected.Audience || claims.Purpose != expected.Purpose || claims.Capability != expected.Capability || claims.CredentialVersion < 1 || claims.IssuedAt < 1 || claims.NotBefore < 1 || claims.ExpiresAt < 1 ||
		claims.NotBefore < claims.IssuedAt || claims.ExpiresAt <= claims.NotBefore || claims.IssuedAt > now+skew || claims.NotBefore > now+skew || claims.ExpiresAt <= now-skew {
		return apiClientJWTClaims{}, authport.ErrUnauthorized
	}
	return claims, nil
}

func apiClientPolicy(credential adminopsport.Credential, expected apiClientJWTExpectation) (apiClientCredentialPolicy, bool) {
	if credential.Kind != adminopsport.CredentialAPIClient || !validAPIClientID(credential.ClientID) || credential.Version < 1 || len(credential.Metadata) == 0 {
		return apiClientCredentialPolicy{}, false
	}
	var policy apiClientCredentialPolicy
	decoder := json.NewDecoder(strings.NewReader(string(credential.Metadata)))
	if decoder.Decode(&policy) != nil || decoder.Decode(&struct{}{}) != io.EOF || policy.Purpose != expected.Purpose || policy.Audience != expected.Audience || policy.Capability != expected.Capability || policy.Scope != expected.Scope || policy.TTLMinutes < 1 || policy.TTLMinutes > 1440 {
		return apiClientCredentialPolicy{}, false
	}
	return policy, true
}

func decodeAPIClientJWTSegment(value string, target any) bool {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(decoded) == 0 || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(string(decoded)))
	return decoder.Decode(target) == nil && decoder.Decode(&struct{}{}) == io.EOF
}

func validAPIClientID(value string) bool {
	if value == "" || value == "." || value == ".." || strings.TrimSpace(value) != value || len(value) > 120 {
		return false
	}
	for _, character := range value {
		if !(character == '-' || character == '_' || character == '.' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}
