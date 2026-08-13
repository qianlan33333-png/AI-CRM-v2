// Package client owns the narrow, read-only WeCom HTTP boundary.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const maxResponseBytes = 1 << 20

var (
	ErrInvalidConfig      = errors.New("invalid WeCom client configuration")
	ErrRequestTimeout     = errors.New("WeCom request timed out")
	ErrTransport          = errors.New("WeCom transport failure")
	ErrUnexpectedResponse = errors.New("invalid WeCom response")
	ErrUpstream           = errors.New("WeCom API rejected request")
)

// CorpID identifies the enterprise and is safe to use as a request parameter.
type CorpID string

// CorpSecret is opaque so it cannot be accidentally formatted into logs.
type CorpSecret struct{ value string }

func (secret CorpSecret) Value() string { return secret.value }
func (CorpSecret) String() string       { return "[REDACTED]" }
func (CorpSecret) GoString() string     { return "[REDACTED]" }

// AccessToken is opaque; callers can only obtain it from a TokenProvider.
type AccessToken struct{ value string }

func (token AccessToken) Value() string { return token.value }
func (AccessToken) String() string      { return "[REDACTED]" }
func (AccessToken) GoString() string    { return "[REDACTED]" }

// Credentials keeps the token grant inputs typed and separate from callback
// credentials. It is deliberately not wired into process startup in this
// read-client slice.
type Credentials struct {
	CorpID     CorpID
	CorpSecret CorpSecret
}

func NewCredentials(corpID, corpSecret string) (Credentials, error) {
	if strings.TrimSpace(corpID) != corpID || corpID == "" || len(corpID) > 128 ||
		strings.TrimSpace(corpSecret) != corpSecret || corpSecret == "" || len(corpSecret) > 256 {
		return Credentials{}, ErrInvalidConfig
	}
	return Credentials{CorpID: CorpID(corpID), CorpSecret: CorpSecret{value: corpSecret}}, nil
}

// TokenProvider supplies an access token to read-only API calls.
type TokenProvider interface {
	Token(context.Context) (AccessToken, error)
}

// TokenProviderConfig contains only the inputs required for the gettoken
// grant. BaseURL is explicit so constructing this package never performs a
// provider call or chooses a live endpoint implicitly.
type TokenProviderConfig struct {
	BaseURL     string
	Credentials Credentials
	HTTPClient  *http.Client
	Now         func() time.Time
}

// CachingTokenProvider caches the token until shortly before its advertised
// expiry. Network calls occur only when Token is called by a future reader.
type CachingTokenProvider struct {
	baseURL     *url.URL
	credentials Credentials
	httpClient  *http.Client
	now         func() time.Time

	mu        sync.Mutex
	cached    AccessToken
	refreshAt time.Time
}

func NewTokenProvider(config TokenProviderConfig) (*CachingTokenProvider, error) {
	baseURL, err := parseBaseURL(config.BaseURL)
	if err != nil || config.HTTPClient == nil || config.Now == nil || config.Credentials.CorpID == "" || config.Credentials.CorpSecret.Value() == "" {
		return nil, ErrInvalidConfig
	}
	return &CachingTokenProvider{
		baseURL: baseURL, credentials: config.Credentials, httpClient: config.HTTPClient, now: config.Now,
	}, nil
}

func (provider *CachingTokenProvider) Token(ctx context.Context) (AccessToken, error) {
	if provider == nil {
		return AccessToken{}, ErrInvalidConfig
	}
	now := provider.now()
	provider.mu.Lock()
	if provider.cached.Value() != "" && now.Before(provider.refreshAt) {
		token := provider.cached
		provider.mu.Unlock()
		return token, nil
	}
	provider.mu.Unlock()

	endpoint := provider.baseURL.ResolveReference(&url.URL{Path: "/cgi-bin/gettoken"})
	query := url.Values{}
	query.Set("corpid", string(provider.credentials.CorpID))
	query.Set("corpsecret", provider.credentials.CorpSecret.Value())
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return AccessToken{}, ErrInvalidConfig
	}
	response, err := provider.httpClient.Do(request)
	if err != nil {
		return AccessToken{}, mapRequestError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return AccessToken{}, fmt.Errorf("%w: status %d", ErrUnexpectedResponse, response.StatusCode)
	}
	var payload struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err = decodeResponse(response.Body, &payload); err != nil {
		return AccessToken{}, err
	}
	if payload.ErrCode != 0 {
		return AccessToken{}, apiError(payload.ErrCode, payload.ErrMsg)
	}
	if payload.AccessToken == "" || payload.ExpiresIn <= 0 {
		return AccessToken{}, ErrUnexpectedResponse
	}

	refreshAt := now.Add(time.Duration(payload.ExpiresIn) * time.Second)
	margin := minDuration(time.Minute, time.Duration(payload.ExpiresIn)*time.Second/10)
	refreshAt = refreshAt.Add(-margin)
	provider.mu.Lock()
	provider.cached = AccessToken{value: payload.AccessToken}
	provider.refreshAt = refreshAt
	provider.mu.Unlock()
	return AccessToken{value: payload.AccessToken}, nil
}

func parseBaseURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, ErrInvalidConfig
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, ErrInvalidConfig
	}
	return parsed, nil
}

func decodeResponse(body io.Reader, target any) error {
	data, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		return ErrUnexpectedResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return ErrUnexpectedResponse
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return ErrUnexpectedResponse
	}
	return nil
}

func mapRequestError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrRequestTimeout, context.DeadlineExceeded)
	}
	return fmt.Errorf("%w: %w", ErrTransport, err)
}

// APIError exposes WeCom's numeric code without exposing request credentials.
type APIError struct {
	Code    int
	Message string
}

func (err *APIError) Error() string {
	return fmt.Sprintf("WeCom API error %d: %s", err.Code, err.Message)
}

func apiError(code int, message string) error {
	return fmt.Errorf("%w: %w", ErrUpstream, &APIError{Code: code, Message: message})
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}
