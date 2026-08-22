package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

type setupWizardRouteAuth struct {
	recordingAuth
	csrfCalls int
}

func (service *setupWizardRouteAuth) ValidateCSRF(_ context.Context, _ authport.SessionRef, token authport.CSRFToken) error {
	service.csrfCalls++
	return nil
}

func TestSetupWizardRouteUsesCentralAuthAndSingleCSRF(t *testing.T) {
	service := &setupWizardRouteAuth{}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	leafCalls := 0
	legacy := &Handler{auth: service, setupWizard: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		leafCalls++
		principal, ok := authport.PrincipalFromContext(request.Context())
		if !ok || principal.AdminUserID != 1 {
			t.Fatalf("principal at setup-wizard leaf = %#v, %t", principal, ok)
		}
		authorization, ok := authport.AuthorizationFromContext(request.Context())
		if !ok {
			t.Fatal("setup-wizard leaf missing central authorization")
		}
		want := authport.CapabilityConfigOverviewRead
		if request.Method == http.MethodPost {
			want = authport.CapabilityConfigSettingsManage
		}
		if authorization.Capability != want || authorization.Scope != authport.ScopeGlobal {
			t.Fatalf("authorization at setup-wizard leaf = %#v, want global %q", authorization, want)
		}
		writer.WriteHeader(http.StatusNoContent)
	})}
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

	get := httptest.NewRequest(http.MethodGet, "/setup/wizard", nil)
	get.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(1)})
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusServiceUnavailable || leafCalls != 0 || service.csrfCalls != 0 {
		t.Fatalf("legacy GET status/json/csrf = %d/%d/%d", getResponse.Code, leafCalls, service.csrfCalls)
	}
	if got := service.capabilities(); len(got) != 1 || got[0] != authport.CapabilityConfigOverviewRead {
		t.Fatalf("GET capabilities = %#v", got)
	}

	service.reset()
	legacyPost := httptest.NewRequest(http.MethodPost, "/setup/wizard/save", nil)
	legacyPost.Header.Set("X-CSRF-Token", legacyToken(2))
	legacyPost.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(1)})
	legacyPostResponse := httptest.NewRecorder()
	router.ServeHTTP(legacyPostResponse, legacyPost)
	if legacyPostResponse.Code != http.StatusServiceUnavailable || leafCalls != 0 || service.csrfCalls != 1 {
		t.Fatalf("legacy POST status/json/csrf = %d/%d/%d", legacyPostResponse.Code, leafCalls, service.csrfCalls)
	}
	if got := service.capabilities(); len(got) != 1 || got[0] != authport.CapabilityConfigSettingsManage {
		t.Fatalf("legacy POST capabilities = %#v", got)
	}

	service.reset()
	canonicalGet := httptest.NewRequest(http.MethodGet, "/api/admin/setup-wizard", nil)
	canonicalGet.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(1)})
	canonicalGetResponse := httptest.NewRecorder()
	router.ServeHTTP(canonicalGetResponse, canonicalGet)
	if canonicalGetResponse.Code != http.StatusNoContent || leafCalls != 1 || service.csrfCalls != 1 {
		t.Fatalf("canonical GET status/json/csrf = %d/%d/%d", canonicalGetResponse.Code, leafCalls, service.csrfCalls)
	}
	if got := service.capabilities(); len(got) != 1 || got[0] != authport.CapabilityConfigOverviewRead {
		t.Fatalf("canonical GET capabilities = %#v", got)
	}

	service.reset()
	post := httptest.NewRequest(http.MethodPost, "/api/admin/setup-wizard", nil)
	post.Header.Set("X-CSRF-Token", legacyToken(2))
	post.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(1)})
	postResponse := httptest.NewRecorder()
	router.ServeHTTP(postResponse, post)
	if postResponse.Code != http.StatusNoContent || leafCalls != 2 || service.csrfCalls != 2 {
		t.Fatalf("canonical POST status/calls/csrf = %d/%d/%d, want 204/2/2", postResponse.Code, leafCalls, service.csrfCalls)
	}
	if got := service.capabilities(); len(got) != 1 || got[0] != authport.CapabilityConfigSettingsManage {
		t.Fatalf("POST capabilities = %#v", got)
	}

	service.reset()
	missingCSRF := httptest.NewRequest(http.MethodPost, "/api/admin/setup-wizard", nil)
	missingCSRF.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(1)})
	missingCSRFResponse := httptest.NewRecorder()
	router.ServeHTTP(missingCSRFResponse, missingCSRF)
	if missingCSRFResponse.Code != http.StatusForbidden || leafCalls != 2 || service.csrfCalls != 2 {
		t.Fatalf("POST without CSRF status/calls/csrf = %d/%d/%d, want 403/2/2", missingCSRFResponse.Code, leafCalls, service.csrfCalls)
	}
	if got := service.capabilities(); len(got) != 0 {
		t.Fatalf("POST without CSRF capabilities = %#v, want none", got)
	}
}
