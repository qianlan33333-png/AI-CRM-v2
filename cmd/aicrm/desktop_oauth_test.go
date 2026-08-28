package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

func TestDesktopOAuthHandlerUsesDesktopURLAndCompletesCallback(t *testing.T) {
	now := time.Date(2026, time.August, 29, 9, 0, 0, 0, time.UTC)
	state := browserToken(0x61)
	application := &humanAuthStub{
		attempt: authport.OAuthAttempt{State: authport.OAuthState(state), ExpiresAt: now.Add(5 * time.Minute)},
		claim:   authport.OAuthClaim{Provider: authport.ProviderWeCom, NextPath: "/admin/config"},
		session: authport.BrowserSession{Session: authport.SessionRef(browserToken(0x62)), CSRF: authport.CSRFToken(browserToken(0x63)), ExpiresAt: now.Add(time.Hour)},
	}
	var tokenCalls, identityCalls int
	providerServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			tokenCalls++
			_, _ = writer.Write([]byte(`{"errcode":0,"access_token":"desktop-access-token","expires_in":7200}`))
		case "/cgi-bin/auth/getuserinfo":
			identityCalls++
			query, err := url.ParseQuery(request.URL.RawQuery)
			if err != nil || query.Get("access_token") != "desktop-access-token" || query.Get("code") != "desktop-code" {
				t.Fatalf("identity query = %q", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"errcode":0,"userid":"desktop-member"}`))
		default:
			t.Fatalf("unexpected provider request %s", request.URL.Path)
		}
	}))
	defer providerServer.Close()
	provider := newDesktopOAuthProvider(t, providerServer)
	handler, err := NewHumanAuthHandler(application, application, application, provider, HumanAuthOptions{Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}

	startResponse := httptest.NewRecorder()
	handler.Start(startResponse, httptest.NewRequest(http.MethodGet, "/auth/wecom/start?next=%2Fadmin%2Fconfig", nil))
	if startResponse.Code != http.StatusFound || application.beginNext != "/admin/config" {
		t.Fatalf("start status/next = %d/%q", startResponse.Code, application.beginNext)
	}
	desktopURL, err := url.Parse(startResponse.Header().Get("Location"))
	if err != nil || desktopURL.Scheme != "https" || desktopURL.Host != "login.work.weixin.qq.com" || desktopURL.Path != "/wwlogin/sso/login" || desktopURL.Fragment != "" {
		t.Fatalf("desktop redirect = %q err=%v", startResponse.Header().Get("Location"), err)
	}
	query, err := url.ParseQuery(desktopURL.RawQuery)
	if err != nil || len(query) != 5 || query.Get("login_type") != "CorpApp" || query.Get("appid") != "corp-fixture" || query.Get("agentid") != "1000025" ||
		query.Get("redirect_uri") != "https://crm.example.test/auth/wecom/callback" || query.Get("state") != state {
		t.Fatalf("desktop redirect query = %q", desktopURL.RawQuery)
	}
	cookies := startResponse.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != oauthStateCookieName || cookies[0].Value != state || !cookies[0].Secure || !cookies[0].HttpOnly ||
		cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != weComOAuthCallbackPath {
		t.Fatalf("state cookie = %#v", cookies)
	}

	callbackRequest := httptest.NewRequest(http.MethodGet, "/auth/wecom/callback?code=desktop-code&state="+url.QueryEscape(state), nil)
	callbackRequest.AddCookie(cookies[0])
	callbackResponse := httptest.NewRecorder()
	handler.Callback(callbackResponse, callbackRequest)
	if callbackResponse.Code != http.StatusFound || callbackResponse.Header().Get("Location") != "/admin/config" ||
		application.claimState != state || application.issued.Provider != authport.ProviderWeCom || application.issued.CorpID != "corp-fixture" || application.issued.SubjectID != "desktop-member" ||
		tokenCalls != 1 || identityCalls != 1 {
		t.Fatalf("callback status/location/claim/login/calls=%d/%q/%q/%+v/%d/%d", callbackResponse.Code, callbackResponse.Header().Get("Location"), application.claimState, application.issued, tokenCalls, identityCalls)
	}
	callbackCookies := cookiesByName(callbackResponse.Result().Cookies())
	if callbackCookies[authhttp.SessionCookieName] == nil || callbackCookies[authhttp.CSRFCookieName] == nil || callbackCookies[oauthStateCookieName] == nil || callbackCookies[oauthStateCookieName].MaxAge >= 0 {
		t.Fatalf("callback cookies = %#v", callbackResponse.Result().Cookies())
	}
}

func newDesktopOAuthProvider(t *testing.T, providerServer *httptest.Server) *wecomclient.HumanOAuthClient {
	t.Helper()
	credentials, err := wecomclient.NewCredentials("corp-fixture", "desktop-secret")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{
		BaseURL: providerServer.URL, Credentials: credentials, HTTPClient: providerServer.Client(), Now: time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := wecomclient.NewHumanOAuthClient(wecomclient.HumanOAuthConfig{
		BaseURL: providerServer.URL, AuthorizeURL: providerServer.URL + "/unused", CallbackURL: "https://crm.example.test/auth/wecom/callback",
		CorpID: "corp-fixture", DesktopAgentID: 1000025, HTTPClient: providerServer.Client(), TokenProvider: tokens,
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
