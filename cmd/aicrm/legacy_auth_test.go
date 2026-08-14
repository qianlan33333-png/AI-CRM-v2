package main

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

func TestSafeLegacyNextPermanentlyRejectsCrossOriginAndNormalizationBypasses(t *testing.T) {
	for _, safe := range []string{"/admin", "/admin/customers?tab=active", "/admin/%E5%AE%A2%E6%88%B7", "/"} {
		if got, err := safeLegacyNext(safe); err != nil || got != safe {
			t.Fatalf("safeLegacyNext(%q)=%q,%v", safe, got, err)
		}
	}
	unsafe := []string{
		"https://evil.example/path", "http://evil.example/path", "//evil.example/path",
		`/\evil.example/path`, `/%5cevil.example/path`, `/%255cevil.example/path`,
		"/%2f%2fevil.example/path", "/%252f%252fevil.example/path",
		"javascript:alert(1)", "admin", "/../evil", "/%2e%2e/evil", "/./evil",
		"/line\nbreak", "/line\rbreak", "/%0d%0aLocation:%20https://evil.example",
		" /admin", "/admin ", "/admin#fragment",
	}
	for _, candidate := range unsafe {
		t.Run(candidate, func(t *testing.T) {
			if got, err := safeLegacyNext(candidate); err == nil {
				t.Fatalf("safeLegacyNext(%q) accepted %q", candidate, got)
			}
		})
	}
}

func TestHumanOAuthBrowserFlowIssuesCanonicalAndLegacySessionAndRevokesWithBoundCSRF(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	state := browserToken(0x31)
	session := browserToken(0x41)
	csrf := browserToken(0x42)
	application := &humanAuthStub{
		principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin},
		attempt:   authport.OAuthAttempt{State: authport.OAuthState(state), ExpiresAt: now.Add(5 * time.Minute)},
		claim:     authport.OAuthClaim{Provider: authport.ProviderWeCom, NextPath: "/admin/customers"},
		session:   authport.BrowserSession{Session: authport.SessionRef(session), CSRF: authport.CSRFToken(csrf), ExpiresAt: now.Add(time.Hour)},
	}
	provider := &humanOAuthStub{identity: wecomclient.HumanIdentity{CorpID: "corp-fixture", UserID: "member-fixture"}}
	handler, err := NewHumanAuthHandler(application, application, application, provider, HumanAuthOptions{Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}

	startRequest := httptest.NewRequest(http.MethodGet, "/auth/wecom/start?next=%2Fadmin%2Fcustomers", nil)
	startResponse := httptest.NewRecorder()
	handler.Start(startResponse, startRequest)
	if startResponse.Code != http.StatusFound || application.beginNext != "/admin/customers" || provider.authorizationState != state {
		t.Fatalf("start status/next/state=%d/%q/%q body=%s", startResponse.Code, application.beginNext, provider.authorizationState, startResponse.Body.String())
	}
	stateCookies := startResponse.Result().Cookies()
	if len(stateCookies) != 1 || stateCookies[0].Name != oauthStateCookieName || !stateCookies[0].Secure || !stateCookies[0].HttpOnly ||
		stateCookies[0].SameSite != http.SameSiteLaxMode || stateCookies[0].Path != weComOAuthCallbackPath {
		t.Fatalf("state cookie = %#v", stateCookies)
	}

	callbackRequest := httptest.NewRequest(http.MethodGet, "/auth/wecom/callback?code=provider-code&state="+url.QueryEscape(state), nil)
	callbackRequest.AddCookie(stateCookies[0])
	callbackRequest.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: browserToken(0x51)})
	callbackRequest.AddCookie(&http.Cookie{Name: LegacyCSRFCookieName, Value: browserToken(0x52)})
	callbackResponse := httptest.NewRecorder()
	handler.Callback(callbackResponse, callbackRequest)
	if callbackResponse.Code != http.StatusFound || callbackResponse.Header().Get("Location") != "/admin/customers" ||
		application.claimState != state || provider.exchangeCode != "provider-code" ||
		application.issued.Provider != authport.ProviderWeCom || application.issued.TenantID != "corp-fixture" || application.issued.SubjectID != "member-fixture" {
		t.Fatalf("callback status/location/claim/exchange/issued=%d/%q/%q/%q/%+v body=%s", callbackResponse.Code, callbackResponse.Header().Get("Location"), application.claimState, provider.exchangeCode, application.issued, callbackResponse.Body.String())
	}
	callbackCookies := cookiesByName(callbackResponse.Result().Cookies())
	if callbackCookies[authhttp.SessionCookieName] == nil || callbackCookies[authhttp.CSRFCookieName] == nil ||
		callbackCookies[LegacySessionCookieName] == nil || callbackCookies[LegacyCSRFCookieName] == nil || callbackCookies[oauthStateCookieName] == nil ||
		callbackCookies[authhttp.SessionCookieName].Value != session || callbackCookies[authhttp.CSRFCookieName].Value != csrf ||
		callbackCookies[LegacySessionCookieName].Value != session || callbackCookies[LegacyCSRFCookieName].Value != csrf ||
		!callbackCookies[LegacySessionCookieName].HttpOnly || callbackCookies[LegacySessionCookieName].SameSite != http.SameSiteLaxMode ||
		callbackCookies[LegacyCSRFCookieName].HttpOnly || callbackCookies[LegacyCSRFCookieName].SameSite != http.SameSiteStrictMode ||
		callbackCookies[oauthStateCookieName].MaxAge >= 0 {
		t.Fatalf("callback cookies = %#v", callbackResponse.Result().Cookies())
	}

	wrongLogout := httptest.NewRequest(http.MethodGet, "/logout", nil)
	wrongLogout.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: session})
	wrongLogout.AddCookie(&http.Cookie{Name: authhttp.CSRFCookieName, Value: browserToken(0x43)})
	wrongResponse := httptest.NewRecorder()
	application.invalidateErr = authport.ErrCSRFInvalid
	handler.Logout(wrongResponse, wrongLogout)
	if wrongResponse.Code != http.StatusForbidden || application.invalidateCalls != 1 {
		t.Fatalf("wrong-CSRF logout status/calls=%d/%d", wrongResponse.Code, application.invalidateCalls)
	}

	logout := httptest.NewRequest(http.MethodGet, "/logout", nil)
	logout.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: session})
	logout.AddCookie(&http.Cookie{Name: authhttp.CSRFCookieName, Value: csrf})
	logout.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: session})
	logout.AddCookie(&http.Cookie{Name: LegacyCSRFCookieName, Value: csrf})
	logoutResponse := httptest.NewRecorder()
	application.invalidateErr = nil
	handler.Logout(logoutResponse, logout)
	if logoutResponse.Code != http.StatusFound || logoutResponse.Header().Get("Location") != "/login" || application.invalidatedSession != session || application.invalidatedCSRF != csrf {
		t.Fatalf("logout status/location/session/csrf=%d/%q/%q/%q", logoutResponse.Code, logoutResponse.Header().Get("Location"), application.invalidatedSession, application.invalidatedCSRF)
	}
	cleared := cookiesByName(logoutResponse.Result().Cookies())
	for _, name := range []string{authhttp.SessionCookieName, authhttp.CSRFCookieName, LegacySessionCookieName, LegacyCSRFCookieName} {
		if cleared[name] == nil || cleared[name].MaxAge >= 0 || cleared[name].Value != "" {
			t.Fatalf("cookie %q not cleared: %#v", name, cleared[name])
		}
	}
}

func TestHumanAuthRejectsStateReplayCorpIDMismatchAndUnsafeNextBeforeProvider(t *testing.T) {
	application := &humanAuthStub{
		principal: authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin},
		attempt:   authport.OAuthAttempt{State: authport.OAuthState(browserToken(1)), ExpiresAt: time.Now().Add(time.Minute)},
		claim:     authport.OAuthClaim{Provider: authport.ProviderWeCom, NextPath: "/admin"},
	}
	provider := &humanOAuthStub{identity: wecomclient.HumanIdentity{CorpID: "other-corp", UserID: "member-fixture"}}
	handler, err := NewHumanAuthHandler(application, application, application, provider, HumanAuthOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		"/login?next=https%3A%2F%2Fevil.example%2Fpath",
		"/auth/wecom/start?next=%2F%5Cevil.example%2Fpath",
		"/auth/wecom/start?next=%252F%252Fevil.example%252Fpath",
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, target, nil)
		if strings.HasPrefix(target, "/login") {
			handler.Login(response, request)
		} else {
			handler.Start(response, request)
		}
		if response.Code != http.StatusBadRequest || application.beginCalls != 0 || provider.authorizationCalls != 0 {
			t.Fatalf("unsafe target %q status/begin/provider=%d/%d/%d", target, response.Code, application.beginCalls, provider.authorizationCalls)
		}
	}

	state := browserToken(1)
	request := httptest.NewRequest(http.MethodGet, "/auth/wecom/callback?code=provider-code&state="+state, nil)
	request.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: state})
	response := httptest.NewRecorder()
	handler.Callback(response, request)
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/login?auth_error=account_blocked" || application.issueCalls != 0 {
		t.Fatalf("CorpID mismatch status/location/issue=%d/%q/%d", response.Code, response.Header().Get("Location"), application.issueCalls)
	}
	application.claimErr = authport.ErrOAuthStateInvalid
	replay := httptest.NewRecorder()
	handler.Callback(replay, request)
	if replay.Code != http.StatusBadRequest || provider.exchangeCalls != 1 || application.issueCalls != 0 {
		t.Fatalf("replay status/provider/issue=%d/%d/%d", replay.Code, provider.exchangeCalls, application.issueCalls)
	}
}

func TestFinalRouterMountsFrozenHumanAuthMethodsOutsideSessionMiddleware(t *testing.T) {
	application := &humanAuthStub{principal: authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, attempt: authport.OAuthAttempt{State: authport.OAuthState(browserToken(2)), ExpiresAt: time.Now().Add(time.Minute)}}
	human, err := NewHumanAuthHandler(application, application, application, &humanOAuthStub{}, HumanAuthOptions{})
	if err != nil {
		t.Fatal(err)
	}
	authHandler, err := authhttp.NewHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithAll(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, nil, human)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/login", http.StatusOK},
		{http.MethodOptions, "/login", http.StatusOK},
		{http.MethodGet, "/logout", http.StatusFound},
		{http.MethodOptions, "/logout", http.StatusOK},
		{http.MethodGet, "/auth/wecom/start", http.StatusFound},
		{http.MethodOptions, "/auth/wecom/start", http.StatusOK},
		{http.MethodOptions, "/auth/wecom/callback", http.StatusOK},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != test.want {
			t.Fatalf("%s %s = %d, want %d body=%s", test.method, test.path, response.Code, test.want, response.Body.String())
		}
	}
}

func browserToken(fill byte) string {
	return base64.RawURLEncoding.EncodeToString(bytesOf(fill, 32))
}

func bytesOf(fill byte, count int) []byte {
	result := make([]byte, count)
	for index := range result {
		result[index] = fill
	}
	return result
}

func cookiesByName(cookies []*http.Cookie) map[string]*http.Cookie {
	result := make(map[string]*http.Cookie, len(cookies))
	for _, cookie := range cookies {
		result[cookie.Name] = cookie
	}
	return result
}

type humanOAuthStub struct {
	identity           wecomclient.HumanIdentity
	authorizationState string
	exchangeCode       string
	authorizationCalls int
	exchangeCalls      int
}

func (*humanOAuthStub) CorpID() string { return "corp-fixture" }

func (provider *humanOAuthStub) AuthorizationURL(state string) (string, error) {
	provider.authorizationCalls++
	provider.authorizationState = state
	return "https://provider.example.test/oauth?state=" + url.QueryEscape(state), nil
}

func (provider *humanOAuthStub) Exchange(_ context.Context, code string) (wecomclient.HumanIdentity, error) {
	provider.exchangeCalls++
	provider.exchangeCode = code
	if provider.identity.UserID == "" {
		return wecomclient.HumanIdentity{CorpID: "corp-fixture", UserID: "member-fixture"}, nil
	}
	return provider.identity, nil
}

type humanAuthStub struct {
	principal authport.Principal
	attempt   authport.OAuthAttempt
	claim     authport.OAuthClaim
	session   authport.BrowserSession

	beginCalls         int
	beginNext          string
	claimState         string
	claimErr           error
	issued             authport.VerifiedLogin
	issueCalls         int
	invalidateErr      error
	invalidateCalls    int
	invalidatedSession string
	invalidatedCSRF    string
}

func (stub *humanAuthStub) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	if stub.principal.AdminUserID == 0 {
		return authport.Principal{}, authport.ErrUnauthenticated
	}
	return stub.principal, nil
}

func (*humanAuthStub) Authorize(_ context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if principal.AdminUserID < 1 || capability != authport.CapabilityAuthSessionRead && capability != authport.CapabilityAuthSessionLogout {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	return authport.Authorization{Capability: capability, Scope: authport.ScopeSelf}, nil
}

func (*humanAuthStub) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func (stub *humanAuthStub) Invalidate(_ context.Context, session authport.SessionRef, csrf authport.CSRFToken) error {
	stub.invalidateCalls++
	stub.invalidatedSession = string(session)
	stub.invalidatedCSRF = string(csrf)
	return stub.invalidateErr
}

func (stub *humanAuthStub) IssueVerified(_ context.Context, login authport.VerifiedLogin) (authport.BrowserSession, error) {
	stub.issueCalls++
	stub.issued = login
	if stub.session.Session == "" {
		now := time.Now()
		return authport.BrowserSession{Session: authport.SessionRef(browserToken(3)), CSRF: authport.CSRFToken(browserToken(4)), ExpiresAt: now.Add(time.Hour)}, nil
	}
	return stub.session, nil
}

func (stub *humanAuthStub) Begin(_ context.Context, _ authport.Provider, nextPath string) (authport.OAuthAttempt, error) {
	stub.beginCalls++
	stub.beginNext = nextPath
	return stub.attempt, nil
}

func (stub *humanAuthStub) Claim(_ context.Context, _ authport.Provider, state authport.OAuthState) (authport.OAuthClaim, error) {
	stub.claimState = string(state)
	return stub.claim, stub.claimErr
}

var (
	_ authport.Service           = (*humanAuthStub)(nil)
	_ authport.Issuer            = (*humanAuthStub)(nil)
	_ authport.OAuthStateManager = (*humanAuthStub)(nil)
	_ humanOAuthProvider         = (*humanOAuthStub)(nil)
)
