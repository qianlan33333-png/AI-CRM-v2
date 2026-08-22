package main

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudiencemembers"
)

func TestFinalRouterMountsAIAudienceMembersRead(t *testing.T) {
	router, auth := legacyAIAudienceMembersTestRouter(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := authport.PrincipalFromContext(request.Context()); !ok {
			t.Fatal("principal missing from mounted member handler")
		}
		if authorization, ok := authport.AuthorizationFromContext(request.Context()); !ok ||
			authorization.Capability != authport.CapabilitySegmentsRead || authorization.Scope != authport.ScopeGlobal {
			t.Fatalf("authorization=%#v ok=%v", authorization, ok)
		}
		writer.WriteHeader(http.StatusOK)
	}))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/ai-audience/packages/42/members?limit=20&offset=0", legacyToken(211)))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	capabilities := auth.capabilities()
	if len(capabilities) != 1 || capabilities[0] != authport.CapabilitySegmentsRead {
		t.Fatalf("capabilities=%v", capabilities)
	}
}

func TestFinalRouterRejectsAIAudienceMembersMethodsBeforeAuthentication(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		router, auth := legacyAIAudienceMembersTestRouter(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("wrong method reached member handler")
		}))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/api/admin/ai-audience/packages/42/members", nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("%s status/allow=%d/%q body=%s", method, response.Code, response.Header().Get("Allow"), response.Body.String())
		}
		if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s security headers=%q/%q", method, response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
		}
		if capabilities := auth.capabilities(); len(capabilities) != 0 {
			t.Fatalf("%s capabilities=%v", method, capabilities)
		}
	}
}

func legacyAIAudienceMembersTestRouter(t *testing.T, endpoint http.Handler) (http.Handler, *recordingAuth) {
	t.Helper()
	service := &recordingAuth{}
	legacy, err := NewHandlerWithOutboundProductsMediaAndSurvey(
		service,
		&legacyCustomerStub{result: legacyCustomerResult()},
		&legacyOutboundQueryStub{},
		&legacyCancelStub{},
		&legacyRetryStub{},
		&legacyProductStub{},
		&legacyMediaStub{},
		&legacySurveyStub{item: legacySurveyItem()},
	)
	if err != nil {
		t.Fatal(err)
	}
	legacy.aiAudienceMembers = endpoint
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		authHandler,
		authHandler,
		legacy,
	)
	if err != nil {
		t.Fatal(err)
	}
	return router, service
}

func TestAIAudienceMembersRoutePatternRemainsOwned(t *testing.T) {
	if legacyaudiencemembers.RoutePattern != "/api/admin/ai-audience/packages/{package_id}/members" {
		t.Fatalf("route pattern=%q", legacyaudiencemembers.RoutePattern)
	}
}

func TestAIAudienceMembersSecurityRequiresGlobalSegmentsRead(t *testing.T) {
	security := legacyAIAudienceMembersSecurity{}
	request := httptest.NewRequest(http.MethodGet, "/api/admin/ai-audience/packages/1/members", nil)
	if err := security.Authorize(request, legacyaudiencemembers.AccessRequirement{Capability: legacyaudiencemembers.CapabilitySegmentsRead}); !errors.Is(err, legacyaudiencemembers.ErrUnauthenticated) {
		t.Fatalf("anonymous error=%v", err)
	}
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authport.SessionRef(legacyToken(212)))
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilitySegmentsRead, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	if err = security.Authorize(request.WithContext(ctx), legacyaudiencemembers.AccessRequirement{Capability: legacyaudiencemembers.CapabilitySegmentsRead}); err != nil {
		t.Fatalf("authorized error=%v", err)
	}
	if err = security.Authorize(request.WithContext(ctx), legacyaudiencemembers.AccessRequirement{Capability: legacyaudiencemembers.CapabilitySegmentsRead, RequireCSRF: true}); !errors.Is(err, legacyaudiencemembers.ErrForbidden) {
		t.Fatalf("unexpected CSRF contract error=%v", err)
	}
}
