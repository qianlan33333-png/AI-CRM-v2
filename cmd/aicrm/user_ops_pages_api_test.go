package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	automationhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/http"
)

func TestFinalRouterMountsUserOpsPageCarriersWithAdminRead(t *testing.T) {
	router, auth := legacySurveyRouter(t, &legacySurveyStub{item: legacySurveyItem()})
	tests := []struct {
		path     string
		location string
	}{
		{path: automationhttp.UserOpsPath, location: "/?legacy_admin_path=%2Fadmin%2Fuser-ops"},
		{path: automationhttp.UserOpsUIPath, location: "/?legacy_admin_path=%2Fadmin%2Fuser-ops%2Fui"},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, test.path, legacyToken(194)))
		if response.Code != http.StatusFound || response.Header().Get("Location") != test.location {
			t.Fatalf("GET %s status/location=%d/%q", test.path, response.Code, response.Header().Get("Location"))
		}
		if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("GET %s headers=%q/%q", test.path, response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
		}
	}
	capabilities := auth.capabilities()
	if len(capabilities) != len(tests) {
		t.Fatalf("capabilities=%v", capabilities)
	}
	for _, capability := range capabilities {
		if capability != authport.CapabilityAdminRead {
			t.Fatalf("capabilities=%v", capabilities)
		}
	}
}

func TestFinalRouterRejectsUserOpsPageMethodsBeforeAuthentication(t *testing.T) {
	router, auth := legacySurveyRouter(t, &legacySurveyStub{item: legacySurveyItem()})
	for _, path := range []string{automationhttp.UserOpsPath, automationhttp.UserOpsUIPath} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(method, path, nil))
			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("%s %s status/allow=%d/%q", method, path, response.Code, response.Header().Get("Allow"))
			}
			if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Location") != "" {
				t.Fatalf("%s %s headers/location=%q/%q/%q", method, path, response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"), response.Header().Get("Location"))
			}
		}
	}
	if capabilities := auth.capabilities(); len(capabilities) != 0 {
		t.Fatalf("capabilities=%v", capabilities)
	}
}
