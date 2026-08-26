package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
	sidebarhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/http"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type sidebarRouteAuth struct {
	principal      authport.Principal
	authorization  authport.Authorization
	authorizeErr   error
	authorizeCalls int
}

func (service *sidebarRouteAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return service.principal, nil
}
func (service *sidebarRouteAuth) Authorize(context.Context, authport.Principal, authport.Capability) (authport.Authorization, error) {
	service.authorizeCalls++
	return service.authorization, service.authorizeErr
}
func (*sidebarRouteAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}
func (*sidebarRouteAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

type sidebarRouteCorp struct{}

func (sidebarRouteCorp) CorpID(context.Context) (string, error) { return "corp-1", nil }

type sidebarRouteIdentity struct {
	status identityport.ResolveStatus
	calls  int
}

type sidebarRoutePhones struct{}

func (sidebarRoutePhones) BindPhone(context.Context, sidebarapp.PhoneBindingCommand) (string, error) {
	return "already_bound", nil
}

func (resolver *sidebarRouteIdentity) Resolve(context.Context, identityport.IDRef) (identityport.ResolveResult, error) {
	resolver.calls++
	if resolver.status == identityport.ResolveFound {
		return identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 41}, nil
	}
	return identityport.ResolveResult{Status: resolver.status}, nil
}

type sidebarRouteProfiles struct{ profile contactport.SidebarProfile }

func (profiles *sidebarRouteProfiles) ResolveSidebarProfile(context.Context, contactport.CustomerID) (contactport.SidebarProfile, error) {
	return profiles.profile, nil
}
func (profiles *sidebarRouteProfiles) ReadSidebarProfile(context.Context, contactport.CustomerID, int64) (contactport.SidebarProfile, error) {
	return profiles.profile, nil
}
func (profiles *sidebarRouteProfiles) UpdateSidebarProfile(context.Context, contactport.SidebarProfileUpdateCommand) (contactport.SidebarProfile, error) {
	return profiles.profile, nil
}

type sidebarRouteSurveys struct{}

func (sidebarRouteSurveys) ListCustomerSurveyAnswers(context.Context, contactport.CustomerID, int32) (surveyport.CustomerSurveyAnswerPage, error) {
	return surveyport.CustomerSurveyAnswerPage{}, nil
}

type sidebarRouteOrders struct{}

func (sidebarRouteOrders) List(context.Context, orderport.Filter) (orderport.Page, error) {
	return orderport.Page{}, nil
}

type sidebarRouteMembers struct{}

func (sidebarRouteMembers) Get(context.Context, int64, string) (sidebarapp.PeriodicMember, error) {
	return sidebarapp.PeriodicMember{}, sidebarapp.ErrNotFound
}
func (sidebarRouteMembers) UpdateRemark(context.Context, sidebarapp.PeriodicRemarkCommand) (sidebarapp.PeriodicMember, error) {
	return sidebarapp.PeriodicMember{}, sidebarapp.ErrNotFound
}
func (sidebarRouteMembers) ListCustomer(context.Context, sidebarapp.PeriodicListQuery) (sidebarapp.PeriodicListResult, error) {
	return sidebarapp.PeriodicListResult{}, nil
}

type sidebarRouteMedia struct{}

func (sidebarRouteMedia) ListImages(context.Context, mediaport.ImageListQuery) (mediaport.ImageListPage, error) {
	return mediaport.ImageListPage{}, nil
}
func (sidebarRouteMedia) Facets(context.Context) (mediaport.ImageFacets, error) {
	return mediaport.ImageFacets{}, nil
}
func (sidebarRouteMedia) LocalImageExists(context.Context, int64) (bool, error) { return false, nil }

type sidebarPublicMethodCandidate struct {
	api.Unimplemented
	startCalls, callbackCalls, agentConfigCalls int
}

func (candidate *sidebarPublicMethodCandidate) StartSidebarOAuth(http.ResponseWriter, *http.Request, api.StartSidebarOAuthParams) {
	candidate.startCalls++
}

func (candidate *sidebarPublicMethodCandidate) CompleteSidebarOAuth(http.ResponseWriter, *http.Request, api.CompleteSidebarOAuthParams) {
	candidate.callbackCalls++
}

func (candidate *sidebarPublicMethodCandidate) GetSidebarAgentConfig(http.ResponseWriter, *http.Request, api.GetSidebarAgentConfigParams) {
	candidate.agentConfigCalls++
}

func TestSidebarPublicProtocolRoutesRejectWrongMethodsWithoutCallingEndpoint(t *testing.T) {
	authHandler, err := authhttp.NewHandler(&sidebarRouteAuth{})
	if err != nil {
		t.Fatal(err)
	}
	candidate := &sidebarPublicMethodCandidate{}
	router, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, candidate)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/sidebar/v2/oauth/start"},
		{method: http.MethodPut, path: "/api/sidebar/v2/oauth/callback"},
		{method: http.MethodDelete, path: "/api/sidebar/v2/jssdk/agent-config"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))

			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("status/allow=%d/%q", response.Code, response.Header().Get("Allow"))
			}
			if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("unsafe method guard headers=%v", response.Header())
			}
		})
	}
	if candidate.startCalls != 0 || candidate.callbackCalls != 0 || candidate.agentConfigCalls != 0 {
		t.Fatalf("start/callback/agent-config calls=%d/%d/%d", candidate.startCalls, candidate.callbackCalls, candidate.agentConfigCalls)
	}
}

func TestFinalSidebarContextRouteOptionalSessionRBACAndEnumerationSafety(t *testing.T) {
	staffID := int64(7)
	authService := &sidebarRouteAuth{
		principal:     authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin},
		authorization: authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
	}
	authHandler, err := authhttp.NewHandler(authService)
	if err != nil {
		t.Fatal(err)
	}
	identity := &sidebarRouteIdentity{status: identityport.ResolveFound}
	profiles := &sidebarRouteProfiles{profile: contactport.SidebarProfile{CustomerID: 41, OwnerStaffID: staffID, Name: "customer", UpdatedAt: time.Now().UTC()}}
	service, err := sidebarapp.NewService(sidebarRouteCorp{}, identity, sidebarRoutePhones{}, profiles, sidebarRouteSurveys{}, sidebarRouteOrders{}, sidebarRouteMembers{}, sidebarRouteMedia{}, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	sidebarHandler, err := sidebarhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, &candidateHandler{sidebar: sidebarHandler})
	if err != nil {
		t.Fatal(err)
	}

	call := func(cookie bool, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/sidebar/context-token", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if cookie {
			request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "sidebar-session"})
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	decodeState := func(t *testing.T, response *httptest.ResponseRecorder) string {
		t.Helper()
		var payload struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response %d: %v body=%s", response.Code, err, response.Body.String())
		}
		return payload.State
	}

	missing := call(false, `{"external_userid":"wm_external_41"}`)
	if missing.Code != http.StatusOK || decodeState(t, missing) != "viewer_session_required" || authService.authorizeCalls != 0 || identity.calls != 0 {
		t.Fatalf("missing session status/auth/identity/body=%d/%d/%d/%s", missing.Code, authService.authorizeCalls, identity.calls, missing.Body.String())
	}
	malformed := call(false, `{}`)
	if malformed.Code != http.StatusBadRequest || authService.authorizeCalls != 0 || identity.calls != 0 {
		t.Fatalf("malformed status/auth/identity=%d/%d/%d", malformed.Code, authService.authorizeCalls, identity.calls)
	}
	ready := call(true, `{"external_userid":"wm_external_41"}`)
	if ready.Code != http.StatusOK || decodeState(t, ready) != "ready" || authService.authorizeCalls != 1 || identity.calls != 1 {
		t.Fatalf("ready status/auth/identity/body=%d/%d/%d/%s", ready.Code, authService.authorizeCalls, identity.calls, ready.Body.String())
	}

	authService.authorizeErr = authport.ErrUnauthorized
	denied := call(true, `{"external_userid":"wm_external_41"}`)
	if denied.Code != http.StatusForbidden || authService.authorizeCalls != 2 || identity.calls != 1 {
		t.Fatalf("denied status/auth/identity=%d/%d/%d", denied.Code, authService.authorizeCalls, identity.calls)
	}
	authService.authorizeErr = nil
	otherStaffID := int64(8)
	authService.principal = authport.Principal{AdminUserID: 10, Role: authport.RoleSales, StaffID: &otherStaffID}
	authService.authorization = authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 8}
	otherOwner := call(true, `{"external_userid":"wm_external_41"}`)
	if otherOwner.Code != http.StatusOK || decodeState(t, otherOwner) != "customer_not_bound" {
		t.Fatalf("other owner response=%d/%s", otherOwner.Code, otherOwner.Body.String())
	}
	identity.status = identityport.ResolveNotFound
	unbound := call(true, `{"external_userid":"wm_external_41"}`)
	if unbound.Code != http.StatusOK || decodeState(t, unbound) != "customer_not_bound" || otherOwner.Body.String() != unbound.Body.String() {
		t.Fatalf("other-owner/unbound differ: %d %s / %d %s", otherOwner.Code, otherOwner.Body.String(), unbound.Code, unbound.Body.String())
	}
}
