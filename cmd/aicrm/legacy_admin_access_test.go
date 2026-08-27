package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestAdminAccessRouteUsesCentralAdminAndCSRFGuards(t *testing.T) {
	service := &setupWizardRouteAuth{}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	leafCalls := 0
	legacy := &Handler{auth: service, adminAccess: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		leafCalls++
		authorization, ok := authport.AuthorizationFromContext(request.Context())
		if !ok || authorization.Capability != authport.CapabilityConfigSettingsManage || authorization.Scope != authport.ScopeGlobal {
			t.Fatalf("authorization=%#v present=%t", authorization, ok)
		}
		writer.WriteHeader(http.StatusNoContent)
	})}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, authhttp.AdminAccessPath, nil)
	get.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(1)})
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusNoContent || leafCalls != 1 || service.csrfCalls != 0 || len(service.capabilities()) != 1 || service.capabilities()[0] != authport.CapabilityConfigSettingsManage {
		t.Fatalf("GET status/calls/csrf/capability=%d/%d/%d/%v", getResponse.Code, leafCalls, service.csrfCalls, service.capabilities())
	}
	service.reset()
	put := httptest.NewRequest(http.MethodPut, authhttp.AdminAccessPath, nil)
	put.Header.Set("X-CSRF-Token", legacyToken(2))
	put.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(1)})
	putResponse := httptest.NewRecorder()
	router.ServeHTTP(putResponse, put)
	if putResponse.Code != http.StatusNoContent || leafCalls != 2 || service.csrfCalls != 1 || len(service.capabilities()) != 1 || service.capabilities()[0] != authport.CapabilityConfigSettingsManage {
		t.Fatalf("PUT status/calls/csrf/capability=%d/%d/%d/%v", putResponse.Code, leafCalls, service.csrfCalls, service.capabilities())
	}
	service.reset()
	missing := httptest.NewRequest(http.MethodPut, authhttp.AdminAccessPath, nil)
	missing.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: legacyToken(1)})
	missingResponse := httptest.NewRecorder()
	router.ServeHTTP(missingResponse, missing)
	if missingResponse.Code != http.StatusForbidden || leafCalls != 2 || service.csrfCalls != 1 || len(service.capabilities()) != 0 {
		t.Fatalf("missing csrf status/calls/csrf/capability=%d/%d/%d/%v", missingResponse.Code, leafCalls, service.csrfCalls, service.capabilities())
	}
}
