package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type runtimeConfigAuthSpy struct {
	principal           authport.Principal
	authenticationError error
	authorization       authport.Authorization
	authorizationError  error
	authenticateCalls   int
	authorizeCalls      int
	csrfCalls           int
}

func (spy *runtimeConfigAuthSpy) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	spy.authenticateCalls++
	if spy.authenticationError != nil {
		return authport.Principal{}, spy.authenticationError
	}
	return spy.principal, nil
}

func (spy *runtimeConfigAuthSpy) Authorize(_ context.Context, _ authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	spy.authorizeCalls++
	if spy.authorizationError != nil {
		return authport.Authorization{}, spy.authorizationError
	}
	if spy.authorization.Capability == "" {
		return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
	}
	return spy.authorization, nil
}

func (spy *runtimeConfigAuthSpy) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	spy.csrfCalls++
	return nil
}

func (*runtimeConfigAuthSpy) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func runtimeConfigRouter(t *testing.T, service authport.Service, adminOps legacyAdminOps, declaration runtimeConfigDeclaration) http.Handler {
	t.Helper()
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	legacy := &Handler{auth: service, adminOps: adminOps, runtimeConfig: declaration}
	router, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy,
	)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func runtimeConfigRequest(method string, session bool) *http.Request {
	request := httptest.NewRequest(method, legacyRuntimeConfigPath, nil)
	if session {
		request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(58)})
	}
	return request
}

func TestRuntimeConfigDeclarationFromConfigUsesOnlyFrozenValues(t *testing.T) {
	for _, testCase := range []struct {
		name, releaseSHA string
		callback, oauth  bool
		wantSHA          string
	}{
		{name: "disabled and blank release", wantSHA: "unknown"},
		{name: "callback enabled", callback: true, wantSHA: "release-fixture"},
		{name: "oauth enabled", oauth: true, wantSHA: "release-fixture"},
		{name: "both enabled", callback: true, oauth: true, wantSHA: "release-fixture"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			releaseSHA := testCase.releaseSHA
			if releaseSHA == "" && testCase.wantSHA == "release-fixture" {
				releaseSHA = "  release-fixture  "
			}
			declaration := runtimeConfigDeclarationFromConfig(appconfig.Root{
				Release: appconfig.Release{SHA: releaseSHA},
				WeCom: appconfig.WeCom{
					Callback: appconfig.WeComCallback{Enabled: testCase.callback},
					OAuth:    appconfig.WeComOAuth{Enabled: testCase.oauth},
				},
			})
			wantCallback, wantOAuth := "missing", "missing"
			if testCase.callback {
				wantCallback = "configured"
			}
			if testCase.oauth {
				wantOAuth = "configured"
			}
			if declaration.DatabaseMode != "postgres" || declaration.ProductionDataReady != "unknown" || declaration.ReleaseSHA != testCase.wantSHA ||
				declaration.WeChatCallbackToken != wantCallback || declaration.WeChatPayConfig != "unknown" || declaration.OAuthConfig != wantOAuth {
				t.Fatalf("declaration=%+v", declaration)
			}
		})
	}
}

func TestRuntimeConfigPageRendersOnlyEscapedFrozenTable(t *testing.T) {
	declaration := runtimeConfigDeclaration{
		DatabaseMode: "postgres", ProductionDataReady: "unknown", ReleaseSHA: "<release&sha>",
		WeChatCallbackToken: "configured", WeChatPayConfig: "unknown", OAuthConfig: "missing",
	}
	response := httptest.NewRecorder()
	(&Handler{runtimeConfig: declaration}).runtimeConfigPage(response)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/html; charset=utf-8" || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("status/headers=%d/%v", response.Code, response.Header())
	}
	body := response.Body.String()
	if strings.Count(body, "<tr>") != 7 || strings.Contains(body, "<release&sha>") || !strings.Contains(body, "&lt;release&amp;sha&gt;") {
		t.Fatalf("table/escaping body=%s", body)
	}
	last := -1
	for _, value := range []string{
		"database_mode</td><td>postgres", "production_data_ready</td><td>unknown", "release_sha</td><td>&lt;release&amp;sha&gt;",
		"wechat_callback_token</td><td>configured", "wechat_pay_config</td><td>unknown", "oauth_config</td><td>missing",
	} {
		index := strings.Index(body, value)
		if index <= last {
			t.Fatalf("missing or unordered table value %q in %s", value, body)
		}
		last = index
	}
	for _, forbidden := range []string{"provider_execution", "raw-database-url", "raw-callback-secret", "raw-oauth-secret", "merchant-secret", "provider_result", "event_log"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("forbidden material %q in %s", forbidden, body)
		}
	}
}

func TestRuntimeConfigRouteEnforcesFrozenHTTPAndAuthContract(t *testing.T) {
	declaration := runtimeConfigDeclaration{DatabaseMode: "postgres", ProductionDataReady: "unknown", ReleaseSHA: "unknown", WeChatCallbackToken: "missing", WeChatPayConfig: "unknown", OAuthConfig: "missing"}
	admin := authport.Principal{AdminUserID: 58, Role: authport.RoleAdmin}

	t.Run("global admin GET without CSRF", func(t *testing.T) {
		service := &runtimeConfigAuthSpy{principal: admin}
		response := httptest.NewRecorder()
		runtimeConfigRouter(t, service, &legacyAdminOpsTransportStub{}, declaration).ServeHTTP(response, runtimeConfigRequest(http.MethodGet, true))
		if response.Code != http.StatusOK || service.authenticateCalls != 1 || service.authorizeCalls != 1 || service.csrfCalls != 0 {
			t.Fatalf("status/calls=%d/%d/%d/%d", response.Code, service.authenticateCalls, service.authorizeCalls, service.csrfCalls)
		}
	})

	t.Run("anonymous is unauthorized", func(t *testing.T) {
		service := &runtimeConfigAuthSpy{principal: admin}
		response := httptest.NewRecorder()
		runtimeConfigRouter(t, service, &legacyAdminOpsTransportStub{}, declaration).ServeHTTP(response, runtimeConfigRequest(http.MethodGet, false))
		assertLegacyError(t, response, http.StatusUnauthorized, platformhttp.CodeUnauthenticated)
		if service.authenticateCalls != 0 || service.authorizeCalls != 0 {
			t.Fatalf("status/calls=%d/%d/%d", response.Code, service.authenticateCalls, service.authorizeCalls)
		}
	})

	for _, testCase := range []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
		authorizeErr  error
	}{
		{name: "wrong capability", principal: admin, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}},
		{name: "wrong scope", principal: admin, authorization: authport.Authorization{Capability: authport.CapabilityConfigOverviewRead, Scope: authport.ScopeSelf}},
		{name: "sales", principal: authport.Principal{AdminUserID: 59, Role: authport.RoleSales}, authorizeErr: authport.ErrUnauthorized},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &runtimeConfigAuthSpy{principal: testCase.principal, authorization: testCase.authorization, authorizationError: testCase.authorizeErr}
			response := httptest.NewRecorder()
			runtimeConfigRouter(t, service, &legacyAdminOpsTransportStub{}, declaration).ServeHTTP(response, runtimeConfigRequest(http.MethodGet, true))
			assertLegacyError(t, response, http.StatusForbidden, platformhttp.CodeUnauthorized)
		})
	}

	t.Run("non GET is rejected before auth", func(t *testing.T) {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
			service := &runtimeConfigAuthSpy{principal: admin}
			response := httptest.NewRecorder()
			runtimeConfigRouter(t, service, &legacyAdminOpsTransportStub{}, declaration).ServeHTTP(response, runtimeConfigRequest(method, false))
			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || service.authenticateCalls != 0 || service.authorizeCalls != 0 || service.csrfCalls != 0 {
				t.Fatalf("method=%s status/allow/calls=%d/%q/%d/%d/%d", method, response.Code, response.Header().Get("Allow"), service.authenticateCalls, service.authorizeCalls, service.csrfCalls)
			}
		}
	})

	t.Run("missing admin ops composition is safe 503", func(t *testing.T) {
		service := &runtimeConfigAuthSpy{principal: admin}
		response := httptest.NewRecorder()
		runtimeConfigRouter(t, service, nil, declaration).ServeHTTP(response, runtimeConfigRequest(http.MethodGet, true))
		if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error\":\"admin_ops_unavailable\",\"ok\":false}\n" {
			t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
		}
	})
}
