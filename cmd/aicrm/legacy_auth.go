package main

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strings"
	"time"
	"unicode"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
)

const (
	legacyLoginPath        = "/login"
	legacyLogoutPath       = "/logout"
	weComOAuthStartPath    = "/auth/wecom/start"
	weComOAuthCallbackPath = "/auth/wecom/callback"
	oauthStateCookieName   = "aicrm_oauth_state"
	defaultLegacyNext      = "/admin"
)

var loginTemplate = template.Must(template.New("legacy-login").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>登录运营工作台</title></head><body><main><h1>登录运营工作台</h1>
{{if .Failed}}<p role="alert">登录未完成，请重新发起企业微信登录。</p>{{end}}
<p><a href="{{.LoginURL}}">企业微信登录</a></p></main></body></html>`))

type humanOAuthProvider interface {
	CorpID() string
	AuthorizationURL(string) (string, error)
	Exchange(context.Context, string) (wecomclient.HumanIdentity, error)
}

type HumanAuthHandler struct {
	auth     authport.Service
	issuer   authport.Issuer
	states   authport.OAuthStateManager
	provider humanOAuthProvider
	clock    func() time.Time
	logger   *slog.Logger
}

type HumanAuthOptions struct {
	Clock  func() time.Time
	Logger *slog.Logger
}

func NewHumanAuthHandler(auth authport.Service, issuer authport.Issuer, states authport.OAuthStateManager, provider humanOAuthProvider, options HumanAuthOptions) (*HumanAuthHandler, error) {
	if nilHumanDependency(auth) || nilHumanDependency(issuer) || nilHumanDependency(states) {
		return nil, authport.ErrAuthenticationUnavailable
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	return &HumanAuthHandler{auth: auth, issuer: issuer, states: states, provider: provider, clock: options.Clock, logger: options.Logger}, nil
}

func (handler *HumanAuthHandler) Login(writer http.ResponseWriter, request *http.Request) {
	nextPath, err := legacyNextFromRequest(request)
	if err != nil {
		writeHumanAuthError(writer, request, platformhttp.CodeMalformedRequest, err)
		return
	}
	if session, sessionErr := strictBrowserSession(request); sessionErr == nil {
		principal, authenticateErr := handler.auth.Authenticate(request.Context(), session)
		if authenticateErr == nil {
			if _, authorizeErr := handler.auth.Authorize(request.Context(), principal, authport.CapabilityAuthSessionRead); authorizeErr != nil {
				writeHumanAuthError(writer, request, platformhttp.CodeUnauthorized, authorizeErr)
				return
			}
			noStore(writer)
			http.Redirect(writer, request, nextPath, http.StatusFound)
			return
		}
		if !errors.Is(authenticateErr, authport.ErrUnauthenticated) {
			writeHumanAuthError(writer, request, platformhttp.CodeDependencyUnavailable, authenticateErr)
			return
		}
		clearHumanSession(writer)
	} else if !errors.Is(sessionErr, http.ErrNoCookie) {
		writeHumanAuthError(writer, request, platformhttp.CodeMalformedRequest, sessionErr)
		return
	}
	start := &url.URL{Path: weComOAuthStartPath}
	start.RawQuery = url.Values{"next": []string{nextPath}}.Encode()
	noStore(writer)
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_ = loginTemplate.Execute(writer, struct {
		LoginURL string
		Failed   bool
	}{LoginURL: start.String(), Failed: request.URL.Query().Get("auth_error") != ""})
}

func (handler *HumanAuthHandler) Start(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilHumanDependency(handler.provider) || handler.provider.CorpID() == "" {
		writeHumanAuthError(writer, request, platformhttp.CodeDependencyUnavailable, authport.ErrAuthenticationUnavailable)
		return
	}
	nextPath, err := legacyNextFromRequest(request)
	if err != nil {
		writeHumanAuthError(writer, request, platformhttp.CodeMalformedRequest, err)
		return
	}
	attempt, err := handler.states.Begin(request.Context(), authport.ProviderWeCom, nextPath)
	if err != nil {
		code := platformhttp.CodeMalformedRequest
		if errors.Is(err, authport.ErrOAuthStateUnavailable) {
			code = platformhttp.CodeDependencyUnavailable
		}
		writeHumanAuthError(writer, request, code, err)
		return
	}
	authorizationURL, err := handler.provider.AuthorizationURL(string(attempt.State))
	if err != nil {
		writeHumanAuthError(writer, request, platformhttp.CodeDependencyUnavailable, err)
		return
	}
	if err = writeOAuthStateCookie(writer, attempt, handler.clock()); err != nil {
		writeHumanAuthError(writer, request, platformhttp.CodeDependencyUnavailable, err)
		return
	}
	noStore(writer)
	http.Redirect(writer, request, authorizationURL, http.StatusFound)
}

func (handler *HumanAuthHandler) Callback(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilHumanDependency(handler.provider) {
		writeHumanAuthError(writer, request, platformhttp.CodeDependencyUnavailable, authport.ErrAuthenticationUnavailable)
		return
	}
	state, code, err := callbackInputs(request)
	clearOAuthState(writer)
	if err != nil {
		writeHumanAuthError(writer, request, platformhttp.CodeMalformedRequest, err)
		return
	}
	claim, err := handler.states.Claim(request.Context(), authport.ProviderWeCom, authport.OAuthState(state))
	if err != nil {
		responseCode := platformhttp.CodeMalformedRequest
		if errors.Is(err, authport.ErrOAuthStateUnavailable) {
			responseCode = platformhttp.CodeDependencyUnavailable
		}
		writeHumanAuthError(writer, request, responseCode, err)
		return
	}
	identity, err := handler.provider.Exchange(request.Context(), code)
	if err != nil {
		handler.logProviderExchangeFailure(request.Context(), err)
		redirectLoginError(writer, request, "provider_failed")
		return
	}
	if string(identity.CorpID) != handler.provider.CorpID() || claim.Provider != authport.ProviderWeCom {
		redirectLoginError(writer, request, "account_blocked")
		return
	}
	session, err := handler.issuer.IssueVerified(request.Context(), authport.VerifiedLogin{
		Provider: authport.ProviderWeCom, CorpID: handler.provider.CorpID(), SubjectID: identity.UserID,
	})
	if err != nil {
		if errors.Is(err, authport.ErrUnauthenticated) || errors.Is(err, authport.ErrInvalidVerifiedLogin) {
			redirectLoginError(writer, request, "account_blocked")
			return
		}
		writeHumanAuthError(writer, request, platformhttp.CodeDependencyUnavailable, err)
		return
	}
	if err = writeHumanBrowserSession(writer, session); err != nil {
		writeHumanAuthError(writer, request, platformhttp.CodeDependencyUnavailable, err)
		return
	}
	noStore(writer)
	http.Redirect(writer, request, claim.NextPath, http.StatusFound)
}

func (handler *HumanAuthHandler) logProviderExchangeFailure(ctx context.Context, err error) {
	if handler == nil || handler.logger == nil || err == nil {
		return
	}
	failureClass := "unexpected_response"
	switch {
	case errors.Is(err, wecomclient.ErrRequestTimeout):
		failureClass = "timeout"
	case errors.Is(err, wecomclient.ErrTransport):
		failureClass = "transport"
	case errors.Is(err, wecomclient.ErrInvalidConfig):
		failureClass = "invalid_config"
	case errors.Is(err, wecomclient.ErrUpstream):
		failureClass = "upstream_rejected"
	}
	attributes := []any{"failure_class", failureClass}
	var providerError *wecomclient.APIError
	if errors.As(err, &providerError) {
		attributes = append(attributes, "provider_code", providerError.Code)
	}
	handler.logger.WarnContext(ctx, "wecom_oauth_exchange_failed", attributes...)
}

func (handler *HumanAuthHandler) Logout(writer http.ResponseWriter, request *http.Request) {
	session, csrf, err := pairedBrowserCredentials(request)
	if errors.Is(err, http.ErrNoCookie) {
		clearHumanSession(writer)
		noStore(writer)
		http.Redirect(writer, request, legacyLoginPath, http.StatusFound)
		return
	}
	if err != nil {
		writeHumanAuthError(writer, request, platformhttp.CodeUnauthorized, err)
		return
	}
	principal, err := handler.auth.Authenticate(request.Context(), session)
	if err != nil {
		code := platformhttp.CodeUnauthenticated
		if errors.Is(err, authport.ErrAuthenticationUnavailable) {
			code = platformhttp.CodeDependencyUnavailable
		}
		writeHumanAuthError(writer, request, code, err)
		return
	}
	if _, err = handler.auth.Authorize(request.Context(), principal, authport.CapabilityAuthSessionLogout); err != nil {
		writeHumanAuthError(writer, request, platformhttp.CodeUnauthorized, err)
		return
	}
	if err = handler.auth.Invalidate(request.Context(), session, csrf); err != nil {
		code := platformhttp.CodeUnauthorized
		if errors.Is(err, authport.ErrUnauthenticated) {
			code = platformhttp.CodeUnauthenticated
		} else if errors.Is(err, authport.ErrAuthenticationUnavailable) {
			code = platformhttp.CodeDependencyUnavailable
		}
		writeHumanAuthError(writer, request, code, err)
		return
	}
	clearHumanSession(writer)
	noStore(writer)
	http.Redirect(writer, request, legacyLoginPath, http.StatusFound)
}

func (*HumanAuthHandler) Options(writer http.ResponseWriter, _ *http.Request) {
	noStore(writer)
	writer.Header().Set("Allow", "GET, OPTIONS")
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write([]byte("{}\n"))
}

func legacyNextFromRequest(request *http.Request) (string, error) {
	if request == nil || request.URL == nil {
		return "", errInvalidLegacyQuery
	}
	values, present := request.URL.Query()["next"]
	if !present || len(values) == 1 && values[0] == "" {
		return defaultLegacyNext, nil
	}
	if len(values) != 1 {
		return "", errInvalidLegacyQuery
	}
	return safeLegacyNext(values[0])
}

func safeLegacyNext(value string) (string, error) {
	if len(value) < 1 || len(value) > 2048 {
		return "", errInvalidLegacyQuery
	}
	original := value
	for {
		if !safeLegacyNextLayer(value) {
			return "", errInvalidLegacyQuery
		}
		decoded, err := url.PathUnescape(value)
		if err != nil {
			return "", errInvalidLegacyQuery
		}
		if decoded == value {
			return original, nil
		}
		value = decoded
	}
}

func safeLegacyNextLayer(value string) bool {
	if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.ContainsAny(value, "\\#") || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.Opaque != "" || parsed.Fragment != "" ||
		!strings.HasPrefix(parsed.Path, "/") || strings.HasPrefix(parsed.Path, "//") {
		return false
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return path.Clean(parsed.Path) == parsed.Path || strings.HasSuffix(parsed.Path, "/") && path.Clean(parsed.Path)+"/" == parsed.Path
}

func callbackInputs(request *http.Request) (string, string, error) {
	if request == nil || request.URL == nil {
		return "", "", authport.ErrOAuthStateInvalid
	}
	states := request.URL.Query()["state"]
	codes := request.URL.Query()["code"]
	if len(states) != 1 || len(codes) != 1 || !validToken(states[0]) || len(codes[0]) < 1 || len(codes[0]) > 512 || strings.TrimSpace(codes[0]) != codes[0] {
		return "", "", authport.ErrOAuthStateInvalid
	}
	cookies := namedCookies(request, oauthStateCookieName)
	if len(cookies) != 1 || cookies[0].Value != states[0] || !validToken(cookies[0].Value) {
		return "", "", authport.ErrOAuthStateInvalid
	}
	for _, character := range codes[0] {
		if unicode.IsControl(character) {
			return "", "", authport.ErrOAuthStateInvalid
		}
	}
	return states[0], codes[0], nil
}

func strictBrowserSession(request *http.Request) (authport.SessionRef, error) {
	current, currentPresent, err := oneBrowserToken(request, authhttp.SessionCookieName)
	if err != nil {
		return "", err
	}
	legacy, legacyPresent, err := oneBrowserToken(request, LegacySessionCookieName)
	if err != nil {
		return "", err
	}
	if currentPresent && legacyPresent && current != legacy {
		return "", authport.ErrUnauthenticated
	}
	if currentPresent {
		return authport.SessionRef(current), nil
	}
	if legacyPresent {
		return authport.SessionRef(legacy), nil
	}
	return "", http.ErrNoCookie
}

func pairedBrowserCredentials(request *http.Request) (authport.SessionRef, authport.CSRFToken, error) {
	currentSession, currentSessionPresent, err := oneBrowserToken(request, authhttp.SessionCookieName)
	if err != nil {
		return "", "", err
	}
	currentCSRF, currentCSRFPresent, err := oneBrowserToken(request, authhttp.CSRFCookieName)
	if err != nil {
		return "", "", err
	}
	legacySession, legacySessionPresent, err := oneBrowserToken(request, LegacySessionCookieName)
	if err != nil {
		return "", "", err
	}
	legacyCSRF, legacyCSRFPresent, err := oneBrowserToken(request, LegacyCSRFCookieName)
	if err != nil {
		return "", "", err
	}
	if currentSessionPresent != currentCSRFPresent || legacySessionPresent != legacyCSRFPresent {
		return "", "", authport.ErrCSRFInvalid
	}
	if currentSessionPresent && legacySessionPresent && (currentSession != legacySession || currentCSRF != legacyCSRF) {
		return "", "", authport.ErrCSRFInvalid
	}
	if currentSessionPresent {
		return authport.SessionRef(currentSession), authport.CSRFToken(currentCSRF), nil
	}
	if legacySessionPresent {
		return authport.SessionRef(legacySession), authport.CSRFToken(legacyCSRF), nil
	}
	return "", "", http.ErrNoCookie
}

func oneBrowserToken(request *http.Request, name string) (string, bool, error) {
	cookies := namedCookies(request, name)
	if len(cookies) == 0 {
		return "", false, nil
	}
	if len(cookies) != 1 || !validToken(cookies[0].Value) {
		return "", false, authport.ErrUnauthenticated
	}
	return cookies[0].Value, true, nil
}

func namedCookies(request *http.Request, name string) []*http.Cookie {
	if request == nil {
		return nil
	}
	result := make([]*http.Cookie, 0, 1)
	for _, cookie := range request.Cookies() {
		if cookie.Name == name {
			result = append(result, cookie)
		}
	}
	return result
}

func writeOAuthStateCookie(writer http.ResponseWriter, attempt authport.OAuthAttempt, now time.Time) error {
	if writer == nil || !validToken(string(attempt.State)) || now.IsZero() || !attempt.ExpiresAt.After(now) || attempt.ExpiresAt.After(now.Add(15*time.Minute)) {
		return authport.ErrOAuthStateUnavailable
	}
	maxAge := int(attempt.ExpiresAt.Sub(now).Seconds())
	if maxAge < 1 {
		return authport.ErrOAuthStateUnavailable
	}
	http.SetCookie(writer, &http.Cookie{
		Name: oauthStateCookieName, Value: string(attempt.State), Path: weComOAuthCallbackPath,
		Expires: attempt.ExpiresAt, MaxAge: maxAge, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func clearOAuthState(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{
		Name: oauthStateCookieName, Value: "", Path: weComOAuthCallbackPath, Expires: time.Unix(1, 0).UTC(),
		MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

func clearHumanSession(writer http.ResponseWriter) {
	authhttp.ClearBrowserSession(writer)
	http.SetCookie(writer, expiredLegacyCookie(LegacySessionCookieName, true))
	http.SetCookie(writer, expiredLegacyCookie(LegacyCSRFCookieName, false))
}

func writeHumanBrowserSession(writer http.ResponseWriter, session authport.BrowserSession) error {
	now := time.Now().UTC()
	if writer == nil || !validToken(string(session.Session)) || !validToken(string(session.CSRF)) ||
		!session.ExpiresAt.After(now) || session.ExpiresAt.After(now.Add(24*time.Hour)) {
		return authport.ErrAuthenticationUnavailable
	}
	maxAge := int(session.ExpiresAt.Sub(now).Seconds())
	if maxAge < 1 {
		return authport.ErrAuthenticationUnavailable
	}
	if err := authhttp.WriteBrowserSession(writer, session); err != nil {
		return err
	}
	// The old UI and the canonical middleware receive the same opaque values;
	// neither legacy cookie creates a second session or CSRF identity.
	http.SetCookie(writer, &http.Cookie{
		Name: LegacySessionCookieName, Value: string(session.Session), Path: "/", Expires: session.ExpiresAt,
		MaxAge: maxAge, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(writer, &http.Cookie{
		Name: LegacyCSRFCookieName, Value: string(session.CSRF), Path: "/", Expires: session.ExpiresAt,
		MaxAge: maxAge, Secure: true, HttpOnly: false, SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func expiredLegacyCookie(name string, httpOnly bool) *http.Cookie {
	sameSite := http.SameSiteLaxMode
	if !httpOnly {
		sameSite = http.SameSiteStrictMode
	}
	return &http.Cookie{Name: name, Value: "", Path: "/", Expires: time.Unix(1, 0).UTC(), MaxAge: -1, Secure: true, HttpOnly: httpOnly, SameSite: sameSite}
}

func redirectLoginError(writer http.ResponseWriter, request *http.Request, code string) {
	target := &url.URL{Path: legacyLoginPath}
	target.RawQuery = url.Values{"auth_error": []string{code}}.Encode()
	noStore(writer)
	http.Redirect(writer, request, target.String(), http.StatusFound)
}

func writeHumanAuthError(writer http.ResponseWriter, request *http.Request, code platformhttp.ErrorCode, err error) {
	noStore(writer)
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func noStore(writer http.ResponseWriter) { writer.Header().Set("Cache-Control", "no-store") }

func nilHumanDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}
