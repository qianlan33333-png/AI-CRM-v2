package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	authacceptance "github.com/qianlan33333-png/AI-CRM-v2/acceptance/auth"
	authapp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/app"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authstore "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

var a01DatabaseURL = flag.String("p4-a01-database-url", "", "isolated PostgreSQL 16.14 acceptance database URL")

func TestA01HumanOAuthSessionRBACCSRFFullChainOnPostgreSQL(t *testing.T) {
	if *a01DatabaseURL == "" {
		t.Skip("-p4-a01-database-url is not set")
	}
	ctx := context.Background()
	fixture, err := authacceptance.OpenPostgreSQL(ctx, *a01DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	pool := fixture.Pool()

	serverVersion, err := fixture.ServerVersion(ctx)
	if err != nil || serverVersion != 160014 {
		t.Fatalf("PostgreSQL server version=%d err=%v, want 160014", serverVersion, err)
	}
	userID, err := fixture.SeedAdmin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.Reset(ctx, userID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = fixture.Reset(context.Background(), userID)
	}()

	var tokenCalls, identityCalls atomic.Int32
	providerServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			tokenCalls.Add(1)
			if request.URL.Query().Get("corpid") != "corp-a01-fixture" || request.URL.Query().Get("corpsecret") != "secret-a01-fixture" {
				t.Fatalf("token query=%q", request.URL.RawQuery)
			}
			_, _ = io.WriteString(writer, `{"errcode":0,"access_token":"non-production-token","expires_in":7200}`)
		case "/cgi-bin/auth/getuserinfo":
			identityCalls.Add(1)
			if request.URL.Query().Get("access_token") != "non-production-token" || request.URL.Query().Get("code") != "one-time-code" {
				t.Fatalf("identity query=%q", request.URL.RawQuery)
			}
			_, _ = io.WriteString(writer, `{"errcode":0,"userid":"member-a01-fixture"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer providerServer.Close()

	credentials, err := wecomclient.NewCredentials("corp-a01-fixture", "secret-a01-fixture")
	if err != nil {
		t.Fatal(err)
	}
	providerToken, err := wecomclient.NewTokenProvider(wecomclient.TokenProviderConfig{
		BaseURL: providerServer.URL, Credentials: credentials, HTTPClient: providerServer.Client(), Now: time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := wecomclient.NewHumanOAuthClient(wecomclient.HumanOAuthConfig{
		BaseURL: providerServer.URL, AuthorizeURL: providerServer.URL + "/authorize",
		CallbackURL: "https://a01.example.test/auth/wecom/callback", CorpID: "corp-a01-fixture",
		HTTPClient: providerServer.Client(), TokenProvider: providerToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := authstore.NewRepository()
	uow := platformstore.NewUnitOfWork(pool)
	service, err := authapp.NewService(uow, repository, authapp.Options{})
	if err != nil {
		t.Fatal(err)
	}
	states, err := authapp.NewOAuthStateService(uow, repository, authapp.OAuthStateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	human, err := NewHumanAuthHandler(service, service, states, provider, HumanAuthOptions{})
	if err != nil {
		t.Fatal(err)
	}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithAll(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, nil, human,
	)
	if err != nil {
		t.Fatal(err)
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(http.MethodGet, "/auth/wecom/start?next=%2Fadmin%2Fcustomers%3Ftab%3Dactive", nil))
	if start.Code != http.StatusFound {
		t.Fatalf("start status=%d body=%s", start.Code, start.Body.String())
	}
	stateCookie := cookieNamed(t, start.Result().Cookies(), oauthStateCookieName)
	location, err := url.Parse(start.Header().Get("Location"))
	locationQuery, queryErr := url.ParseQuery(location.RawQuery)
	if err != nil || queryErr != nil || locationQuery.Get("state") != stateCookie.Value || locationQuery.Get("redirect_uri") != "https://a01.example.test/auth/wecom/callback" {
		t.Fatalf("authorization location=%q err=%v", start.Header().Get("Location"), err)
	}

	callbackRequest := httptest.NewRequest(http.MethodGet, "/auth/wecom/callback?code=one-time-code&state="+url.QueryEscape(stateCookie.Value), nil)
	callbackRequest.AddCookie(stateCookie)
	callbackRequest.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: browserToken(0xf1)})
	callbackRequest.AddCookie(&http.Cookie{Name: LegacyCSRFCookieName, Value: browserToken(0xf2)})
	callback := httptest.NewRecorder()
	router.ServeHTTP(callback, callbackRequest)
	if callback.Code != http.StatusFound || callback.Header().Get("Location") != "/admin/customers?tab=active" {
		t.Fatalf("callback status/location=%d/%q body=%s", callback.Code, callback.Header().Get("Location"), callback.Body.String())
	}
	sessionCookie := cookieNamed(t, callback.Result().Cookies(), authhttp.SessionCookieName)
	csrfCookie := cookieNamed(t, callback.Result().Cookies(), authhttp.CSRFCookieName)
	legacySessionCookie := cookieNamed(t, callback.Result().Cookies(), LegacySessionCookieName)
	legacyCSRFCookie := cookieNamed(t, callback.Result().Cookies(), LegacyCSRFCookieName)
	if !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteLaxMode || csrfCookie.HttpOnly || csrfCookie.SameSite != http.SameSiteStrictMode ||
		legacySessionCookie.Value != sessionCookie.Value || legacyCSRFCookie.Value != csrfCookie.Value ||
		!legacySessionCookie.HttpOnly || legacySessionCookie.SameSite != http.SameSiteLaxMode || legacyCSRFCookie.HttpOnly || legacyCSRFCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("canonical/legacy session/csrf cookies=%#v/%#v/%#v/%#v", sessionCookie, csrfCookie, legacySessionCookie, legacyCSRFCookie)
	}
	if tokenCalls.Load() != 1 || identityCalls.Load() != 1 {
		t.Fatalf("provider token/identity calls=%d/%d", tokenCalls.Load(), identityCalls.Load())
	}
	stateConsumed, rawStateStored, rawSessionStored, err := fixture.Persistence(ctx, stateCookie.Value, sessionCookie.Value)
	if err != nil || !stateConsumed || rawStateStored || rawSessionStored {
		t.Fatalf("state/session persistence consumed/raw-state/raw-session=%v/%v/%v err=%v", stateConsumed, rawStateStored, rawSessionStored, err)
	}

	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	sessionRequest.AddCookie(sessionCookie)
	session := httptest.NewRecorder()
	router.ServeHTTP(session, sessionRequest)
	if session.Code != http.StatusOK {
		t.Fatalf("session status=%d body=%s", session.Code, session.Body.String())
	}
	var sessionPayload map[string]any
	if err = json.Unmarshal(session.Body.Bytes(), &sessionPayload); err != nil || sessionPayload["role"] != "admin" {
		t.Fatalf("session payload=%s err=%v", session.Body.String(), err)
	}

	loggedInRequest := httptest.NewRequest(http.MethodGet, "/login?next=%2Fadmin%2Fcustomers", nil)
	loggedInRequest.AddCookie(sessionCookie)
	loggedInRequest.AddCookie(legacySessionCookie)
	loggedIn := httptest.NewRecorder()
	router.ServeHTTP(loggedIn, loggedInRequest)
	if loggedIn.Code != http.StatusFound || loggedIn.Header().Get("Location") != "/admin/customers" {
		t.Fatalf("logged-in login status/location=%d/%q", loggedIn.Code, loggedIn.Header().Get("Location"))
	}

	wrongCSRF := *csrfCookie
	wrongCSRF.Value = browserToken(0xee)
	badLogoutRequest := httptest.NewRequest(http.MethodGet, "/logout", nil)
	badLogoutRequest.AddCookie(sessionCookie)
	badLogoutRequest.AddCookie(&wrongCSRF)
	badLogout := httptest.NewRecorder()
	router.ServeHTTP(badLogout, badLogoutRequest)
	if badLogout.Code != http.StatusForbidden {
		t.Fatalf("bad logout status=%d body=%s", badLogout.Code, badLogout.Body.String())
	}
	revoked, err := fixture.SessionRevoked(ctx, userID)
	if err != nil || revoked {
		t.Fatalf("session revoked after wrong CSRF=%v err=%v", revoked, err)
	}

	logoutRequest := httptest.NewRequest(http.MethodGet, "/logout", nil)
	logoutRequest.AddCookie(sessionCookie)
	logoutRequest.AddCookie(csrfCookie)
	logoutRequest.AddCookie(legacySessionCookie)
	logoutRequest.AddCookie(legacyCSRFCookie)
	logout := httptest.NewRecorder()
	router.ServeHTTP(logout, logoutRequest)
	if logout.Code != http.StatusFound || logout.Header().Get("Location") != "/login" {
		t.Fatalf("logout status/location=%d/%q body=%s", logout.Code, logout.Header().Get("Location"), logout.Body.String())
	}
	for _, name := range []string{authhttp.SessionCookieName, authhttp.CSRFCookieName, LegacySessionCookieName, LegacyCSRFCookieName} {
		if cookieNamed(t, logout.Result().Cookies(), name).MaxAge != -1 {
			t.Fatalf("logout cookie %s was not cleared", name)
		}
	}
	revoked, err = fixture.SessionRevoked(ctx, userID)
	if err != nil || !revoked {
		t.Fatalf("session revoked=%v err=%v", revoked, err)
	}

	revokedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	revokedRequest.AddCookie(sessionCookie)
	revokedResponse := httptest.NewRecorder()
	router.ServeHTTP(revokedResponse, revokedRequest)
	if revokedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status=%d body=%s", revokedResponse.Code, revokedResponse.Body.String())
	}

	replayRequest := httptest.NewRequest(http.MethodGet, "/auth/wecom/callback?code=one-time-code&state="+url.QueryEscape(stateCookie.Value), nil)
	replayRequest.AddCookie(stateCookie)
	replay := httptest.NewRecorder()
	router.ServeHTTP(replay, replayRequest)
	if replay.Code != http.StatusBadRequest || tokenCalls.Load() != 1 || identityCalls.Load() != 1 {
		t.Fatalf("replay status/provider calls=%d/%d/%d body=%s", replay.Code, tokenCalls.Load(), identityCalls.Load(), replay.Body.String())
	}
}

func TestA01S200KOAuthClaimPlanHasNoIllegalSequentialScan(t *testing.T) {
	if *a01DatabaseURL == "" {
		t.Skip("-p4-a01-database-url is not set")
	}
	ctx := context.Background()
	fixture, err := authacceptance.OpenPostgreSQL(ctx, *a01DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	if err = fixture.Reset(ctx, 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fixture.Reset(context.Background(), 0) }()
	if err = fixture.PreparePlanCorpus(ctx); err != nil {
		t.Fatal(err)
	}
	claimPlan, err := fixture.ClaimPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(claimPlan, "Seq Scan on admin_oauth_states") || !strings.Contains(claimPlan, "Index Scan using admin_oauth_states_pkey") {
		t.Fatalf("illegal OAuth claim plan:\n%s", claimPlan)
	}
	expiryPlan, err := fixture.ExpiryPlan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(expiryPlan, "Seq Scan on admin_oauth_states") || !strings.Contains(expiryPlan, "Index Scan using idx_admin_oauth_states_expiry") {
		t.Fatalf("illegal OAuth expiry cleanup plan:\n%s", expiryPlan)
	}
}

func cookieNamed(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %q absent from %#v", name, cookies)
	return nil
}
