package client

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHumanOAuthProviderAdapterUsesFrozenAuthorizationAndIdentityExchange(t *testing.T) {
	var tokenCalls, identityCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			tokenCalls.Add(1)
			if request.URL.Query().Get("corpid") != "corp-fixture" || request.URL.Query().Get("corpsecret") != "secret-fixture" {
				t.Fatalf("token query = %q", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"access-fixture","expires_in":7200}`))
		case "/cgi-bin/auth/getuserinfo":
			identityCalls.Add(1)
			if request.URL.Query().Get("access_token") != "access-fixture" || request.URL.Query().Get("code") != "provider-code-fixture" {
				t.Fatalf("identity query = %q", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"userid":"member-fixture"}`))
		default:
			t.Fatalf("unexpected provider path %q", request.URL.Path)
		}
	}))
	defer server.Close()
	client := newHumanOAuthFixture(t, server)
	state := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	authorization, err := client.AuthorizationURL(state)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authorization)
	if err != nil || parsed.Path != "/connect/oauth2/authorize" || parsed.Fragment != "wechat_redirect" {
		t.Fatalf("authorization URL = %q err=%v", authorization, err)
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		t.Fatal(err)
	}
	if query.Get("appid") != "corp-fixture" || query.Get("redirect_uri") != "https://crm.example.test/auth/wecom/callback" ||
		query.Get("response_type") != "code" || query.Get("scope") != "snsapi_base" || query.Get("state") != state || query.Get("agentid") != "" {
		t.Fatalf("authorization query = %q", parsed.RawQuery)
	}
	identity, err := client.Exchange(context.Background(), "provider-code-fixture")
	if err != nil || identity.CorpID != "corp-fixture" || identity.UserID != "member-fixture" || tokenCalls.Load() != 1 || identityCalls.Load() != 1 {
		t.Fatalf("Exchange()=%+v err=%v calls=%d/%d", identity, err, tokenCalls.Load(), identityCalls.Load())
	}
}

func TestHumanOAuthProviderAdapterBuildsDesktopAuthorizationAndExchangesCode(t *testing.T) {
	var tokenCalls, identityCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			tokenCalls.Add(1)
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"access-fixture","expires_in":7200}`))
		case "/cgi-bin/auth/getuserinfo":
			identityCalls.Add(1)
			if request.URL.Query().Get("access_token") != "access-fixture" || request.URL.Query().Get("code") != "desktop-code" {
				t.Fatalf("identity query=%q", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"userid":"member-fixture"}`))
		default:
			t.Fatalf("unexpected provider path %q", request.URL.Path)
		}
	}))
	defer server.Close()
	credentials, err := NewCredentials("corp-fixture", "secret-fixture")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := NewTokenProvider(TokenProviderConfig{BaseURL: server.URL, Credentials: credentials, HTTPClient: server.Client(), Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewHumanOAuthClient(HumanOAuthConfig{
		BaseURL: server.URL, AuthorizeURL: server.URL + "/connect/oauth2/authorize", CallbackURL: "https://crm.example.test/auth/wecom/callback",
		CorpID: "corp-fixture", DesktopAgentID: 1000025, HTTPClient: server.Client(), TokenProvider: tokens,
	})
	if err != nil {
		t.Fatal(err)
	}
	state := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	authorization, err := client.AuthorizationURL(state)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authorization)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "login.work.weixin.qq.com" || parsed.Path != "/wwlogin/sso/login" || parsed.Fragment != "" {
		t.Fatalf("desktop authorization=%q err=%v", authorization, err)
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query) != 5 || query.Get("login_type") != "CorpApp" || query.Get("appid") != "corp-fixture" || query.Get("agentid") != "1000025" ||
		query.Get("redirect_uri") != "https://crm.example.test/auth/wecom/callback" || query.Get("state") != state || query.Get("scope") != "" || query.Get("response_type") != "" {
		t.Fatalf("desktop authorization query=%q err=%v", parsed.RawQuery, err)
	}
	identity, err := client.Exchange(context.Background(), "desktop-code")
	if err != nil || identity.CorpID != "corp-fixture" || identity.UserID != "member-fixture" || tokenCalls.Load() != 1 || identityCalls.Load() != 1 {
		t.Fatalf("desktop Exchange=%+v err=%v calls=%d/%d", identity, err, tokenCalls.Load(), identityCalls.Load())
	}
}

func TestHumanOAuthProviderAdapterRejectsExternalOrMalformedIdentity(t *testing.T) {
	for _, payload := range []string{
		`{"errcode":0,"openid":"external-only"}`,
		`{"errcode":0,"userid":"member with spaces"}`,
		`{"errcode":40029,"errmsg":"invalid code"}`,
		`not-json`,
	} {
		t.Run(payload, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/cgi-bin/gettoken" {
					_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"access-fixture","expires_in":7200}`))
					return
				}
				_, _ = writer.Write([]byte(payload))
			}))
			defer server.Close()
			_, err := newHumanOAuthFixture(t, server).Exchange(context.Background(), "provider-code")
			if !errors.Is(err, ErrUnexpectedResponse) && !errors.Is(err, ErrUpstream) {
				t.Fatalf("Exchange() error = %v", err)
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	client := newHumanOAuthFixture(t, server)
	if _, err := client.Exchange(context.Background(), " bad-code"); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("unsafe code error = %v", err)
	}
	if _, err := client.AuthorizationURL(strings.Repeat("A", 42)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("unsafe state error = %v", err)
	}
}

func TestHumanOAuthProviderAdapterAcceptsOnlyExplicitSidebarCallbackPath(t *testing.T) {
	var identityPath string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"access-fixture","expires_in":7200}`))
		default:
			identityPath = request.URL.Path
			_, _ = writer.Write([]byte(`{"errcode":0,"userid":"sidebar-member"}`))
		}
	}))
	defer server.Close()
	credentials, err := NewCredentials("corp-fixture", "secret-fixture")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := NewTokenProvider(TokenProviderConfig{BaseURL: server.URL, Credentials: credentials, HTTPClient: server.Client(), Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewHumanOAuthClient(HumanOAuthConfig{
		BaseURL: server.URL, AuthorizeURL: server.URL + "/connect/oauth2/authorize", CallbackURL: "https://crm.example.test/api/sidebar/v2/oauth/callback", CallbackPath: "/api/sidebar/v2/oauth/callback",
		CorpID: "corp-fixture", HTTPClient: server.Client(), TokenProvider: tokens,
	})
	if err != nil {
		t.Fatal(err)
	}
	state := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	redirect, err := client.AuthorizationURL(state)
	if err != nil || !strings.Contains(redirect, "redirect_uri=https%3A%2F%2Fcrm.example.test%2Fapi%2Fsidebar%2Fv2%2Foauth%2Fcallback") {
		t.Fatalf("sidebar redirect/error = %q/%v", redirect, err)
	}
	identity, err := client.Exchange(context.Background(), "sidebar-code")
	if err != nil || identity.UserID != "sidebar-member" || identityPath != "/cgi-bin/user/getuserinfo" {
		t.Fatalf("sidebar Exchange=%+v path=%q err=%v", identity, identityPath, err)
	}
	if _, err = NewHumanOAuthClient(HumanOAuthConfig{
		BaseURL: server.URL, AuthorizeURL: server.URL + "/connect/oauth2/authorize", CallbackURL: "https://crm.example.test/api/sidebar/v2/oauth/callback", CallbackPath: "/auth/wecom/callback",
		CorpID: "corp-fixture", HTTPClient: server.Client(), TokenProvider: tokens,
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("wrong callback-path error = %v", err)
	}
	if _, err = NewHumanOAuthClient(HumanOAuthConfig{
		BaseURL: server.URL, AuthorizeURL: server.URL + "/connect/oauth2/authorize", CallbackURL: "https://crm.example.test/api/sidebar/v2/oauth/callback", CallbackPath: "/api/sidebar/v2/oauth/callback",
		CorpID: "corp-fixture", DesktopAgentID: 1000025, HTTPClient: server.Client(), TokenProvider: tokens,
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("desktop sidebar combination error = %v", err)
	}
	if _, err = NewHumanOAuthClient(HumanOAuthConfig{
		BaseURL: server.URL, AuthorizeURL: server.URL + "/connect/oauth2/authorize", CallbackURL: "https://crm.example.test/auth/wecom/callback",
		CorpID: "corp-fixture", DesktopAgentID: -1, HTTPClient: server.Client(), TokenProvider: tokens,
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("negative desktop agent error = %v", err)
	}
}

func newHumanOAuthFixture(t *testing.T, server *httptest.Server) *HumanOAuthClient {
	t.Helper()
	credentials, err := NewCredentials("corp-fixture", "secret-fixture")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewTokenProvider(TokenProviderConfig{
		BaseURL: server.URL, Credentials: credentials, HTTPClient: server.Client(), Now: time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewHumanOAuthClient(HumanOAuthConfig{
		BaseURL: server.URL, AuthorizeURL: server.URL + "/connect/oauth2/authorize",
		CallbackURL: "https://crm.example.test/auth/wecom/callback", CorpID: "corp-fixture",
		HTTPClient: server.Client(), TokenProvider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
