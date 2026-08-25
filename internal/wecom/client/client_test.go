package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenProviderCachesAndRefreshesAtExpiry(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/cgi-bin/gettoken" || request.URL.Query().Get("corpid") != "wx-corp" || request.URL.Query().Get("corpsecret") != "secret-value" {
			t.Fatalf("token request = %s?%s", request.URL.Path, request.URL.RawQuery)
		}
		requests.Add(1)
		_, _ = writer.Write([]byte(`{"errcode":0,"errmsg":"ok","access_token":"token-value","expires_in":2}`))
	}))
	defer server.Close()
	credentials, err := NewCredentials("wx-corp", "secret-value")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 13, 0, 0, 0, 0, time.UTC)
	provider, err := NewTokenProvider(TokenProviderConfig{BaseURL: server.URL, Credentials: credentials, HTTPClient: server.Client(), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		token, err := provider.Token(context.Background())
		if err != nil || token.Value() != "token-value" {
			t.Fatalf("Token() = %v, %v", token, err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("gettoken requests = %d, want 1", got)
	}
	now = now.Add(2 * time.Second)
	if _, err = provider.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("gettoken requests after expiry = %d, want 2", got)
	}
}

func TestTokenProviderMapsTimeoutAndUpstreamErrors(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) { <-request.Context().Done() }))
		defer server.Close()
		provider := testProvider(t, server.URL, server.Client())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, err := provider.Token(ctx)
		if !errors.Is(err, ErrRequestTimeout) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Token() error = %v, want timeout mapping", err)
		}
	})
	t.Run("upstream", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`{"errcode":40001,"errmsg":"invalid credential"}`))
		}))
		defer server.Close()
		_, err := testProvider(t, server.URL, server.Client()).Token(context.Background())
		var apiErr *APIError
		if !errors.Is(err, ErrUpstream) || !errors.As(err, &apiErr) || apiErr.Code != 40001 {
			t.Fatalf("Token() error = %v, want mapped API error", err)
		}
	})
}

func TestExternalContactReaderReturnsFixtureCursorPage(t *testing.T) {
	fixture, err := os.ReadFile("testdata/externalcontact_get_page.json")
	if err != nil {
		t.Fatal(err)
	}
	provider := staticTokenProvider{token: AccessToken{value: "fixture-token"}}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/cgi-bin/externalcontact/get" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("access_token") != "fixture-token" || query.Get("external_userid") != "external-fixture" || query.Get("cursor") != "prior-cursor" {
			t.Fatalf("query = %q", request.URL.RawQuery)
		}
		_, _ = writer.Write(fixture)
	}))
	defer server.Close()
	reader, err := NewExternalContactReader(ReaderConfig{BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: provider})
	if err != nil {
		t.Fatal(err)
	}
	page, err := reader.FollowUsers(context.Background(), "external-fixture", "prior-cursor")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(page.UserIDs, []string{"owner-a", "owner-b"}) || page.NextCursor != "fixture-next-cursor" {
		t.Fatalf("FollowUsers() = %#v", page)
	}
}

func TestExternalContactReaderListsExternalContactsFromFixture(t *testing.T) {
	fixture, err := os.ReadFile("testdata/externalcontact_list_page.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/cgi-bin/externalcontact/list" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("access_token") != "fixture-token" || query.Get("userid") != "owner-fixture" || query.Get("cursor") != "prior-cursor" {
			t.Fatalf("query = %q", request.URL.RawQuery)
		}
		_, _ = writer.Write(fixture)
	}))
	defer server.Close()
	reader, err := NewExternalContactReader(ReaderConfig{BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: staticTokenProvider{token: AccessToken{value: "fixture-token"}}})
	if err != nil {
		t.Fatal(err)
	}
	page, err := reader.ListExternalContacts(context.Background(), "owner-fixture", "prior-cursor")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(page.ExternalUserIDs, []string{"woEXTERNALUSERID-1", "woEXTERNALUSERID-2"}) || page.NextCursor != "external-contact-next-cursor" {
		t.Fatalf("ListExternalContacts() = %#v", page)
	}
}

func TestReadersFailClosedForMalformedAndUpstreamResponses(t *testing.T) {
	for _, response := range []string{`{"errcode":0,"follow_user":[{}]}`, `{"errcode":48002,"errmsg":"forbidden"}`, `not-json`} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { _, _ = writer.Write([]byte(response)) }))
			defer server.Close()
			reader, err := NewExternalContactReader(ReaderConfig{BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: staticTokenProvider{token: AccessToken{value: "fixture-token"}}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = reader.FollowUsers(context.Background(), "external-fixture", "")
			if !errors.Is(err, ErrUnexpectedResponse) && !errors.Is(err, ErrUpstream) {
				t.Fatalf("FollowUsers() error = %v", err)
			}
		})
	}
	if _, err := NewExternalContactReader(ReaderConfig{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewExternalContactReader() error = %v", err)
	}
	credentials, _ := NewCredentials("wx-corp", "secret-value")
	if _, err := NewTokenProvider(TokenProviderConfig{BaseURL: "https://example.test?x=1", Credentials: credentials, HTTPClient: http.DefaultClient, Now: time.Now}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewTokenProvider() error = %v", err)
	}
	if strings.Contains((&CorpSecret{value: "secret-value"}).String(), "secret-value") || strings.Contains((&AccessToken{value: "token-value"}).String(), "token-value") {
		t.Fatal("credential formatting leaked a value")
	}
}

func TestExternalContactReaderRejectsDuplicateExternalUserIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"errcode":0,"external_userid":["wo-1","wo-1"]}`))
	}))
	defer server.Close()
	reader, err := NewExternalContactReader(ReaderConfig{BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: staticTokenProvider{token: AccessToken{value: "fixture-token"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reader.ListExternalContacts(context.Background(), "owner-fixture", ""); !errors.Is(err, ErrUnexpectedResponse) {
		t.Fatalf("ListExternalContacts() error = %v, want duplicate rejection", err)
	}
}

func TestExternalContactReaderRejectsControlCharacterExternalUserID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"errcode":0,"external_userid":["wo-\u0001"]}`))
	}))
	defer server.Close()
	reader, err := NewExternalContactReader(ReaderConfig{BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: staticTokenProvider{token: AccessToken{value: "fixture-token"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reader.ListExternalContacts(context.Background(), "owner-fixture", ""); !errors.Is(err, ErrUnexpectedResponse) {
		t.Fatalf("ListExternalContacts() error = %v, want malformed provider response", err)
	}
}

func TestDisabledExternalContactReaderMakesNoHTTPCall(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	reader := NewDisabledExternalContactReader()
	if _, err := reader.ListExternalContacts(context.Background(), "owner-fixture", "cursor-1"); !errors.Is(err, ErrExternalContactReadDisabled) {
		t.Fatalf("ListExternalContacts() error = %v, want disabled", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("disabled reader made %d HTTP calls", got)
	}
}

func TestReadersRejectOversizedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"errcode":0,"follow_user":[],"padding":"` + strings.Repeat("x", maxResponseBytes) + `"}`))
	}))
	defer server.Close()
	reader, err := NewExternalContactReader(ReaderConfig{BaseURL: server.URL, HTTPClient: server.Client(), TokenProvider: staticTokenProvider{token: AccessToken{value: "fixture-token"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = reader.FollowUsers(context.Background(), "external-fixture", "")
	if !errors.Is(err, ErrUnexpectedResponse) {
		t.Fatalf("FollowUsers() error = %v, want oversized response rejection", err)
	}
}

type staticTokenProvider struct{ token AccessToken }

func (provider staticTokenProvider) Token(context.Context) (AccessToken, error) {
	return provider.token, nil
}

func testProvider(t *testing.T, baseURL string, httpClient *http.Client) *CachingTokenProvider {
	t.Helper()
	credentials, err := NewCredentials("wx-corp", "secret-value")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewTokenProvider(TokenProviderConfig{BaseURL: baseURL, Credentials: credentials, HTTPClient: httpClient, Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
