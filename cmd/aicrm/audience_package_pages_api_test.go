package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	automationhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/http"
)

func TestFinalRouterMountsAudiencePackagePageCarriersWithOperationsRead(t *testing.T) {
	router, auth := legacySurveyRouter(t, &legacySurveyStub{item: legacySurveyItem()})
	tests := []struct {
		path     string
		location string
	}{
		{path: automationhttp.AudiencePackagesPath, location: "/?legacy_admin_path=%2Fadmin%2Fautomation-conversion"},
		{path: "/admin/automation-conversion/packages/42", location: "/?legacy_admin_path=%2Fadmin%2Fautomation-conversion%2Fpackages%2F42"},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, test.path, legacyToken(193)))
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
		if capability != authport.CapabilityOperationsRead {
			t.Fatalf("capabilities=%v", capabilities)
		}
	}
}

func TestFinalRouterKeepsCloudCarrierCapabilitiesRouteSpecific(t *testing.T) {
	router, auth := legacySurveyRouter(t, &legacySurveyStub{item: legacySurveyItem()})
	tests := []struct {
		path       string
		location   string
		capability authport.Capability
	}{
		{path: automationhttp.CloudOrchestratorRootPath, location: automationhttp.CloudOrchestratorPlansPath, capability: authport.CapabilityAdminRead},
		{path: automationhttp.CloudOrchestratorPlansPath, location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fplans", capability: authport.CapabilityAdminRead},
		{path: automationhttp.CloudOrchestratorPlansPath + "/plan_A-42", location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fplans%2Fplan_A-42", capability: authport.CapabilityAdminRead},
		{path: automationhttp.CloudOrchestratorCampaignsPath, location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fcampaigns", capability: authport.CapabilityOperationsRead},
		{path: automationhttp.CloudOrchestratorCampaignsPath + "?source_kind=segment_members&source_id=7", location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fcampaigns%3Fsource_kind%3Dsegment_members%26source_id%3D7", capability: authport.CapabilityOperationsRead},
		{path: automationhttp.CloudOrchestratorObservabilityPath, location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fobservability", capability: authport.CapabilityAdminRead},
	}
	for index, test := range tests {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, test.path, legacyToken(194)))
		if response.Code != http.StatusFound || response.Header().Get("Location") != test.location {
			t.Fatalf("GET %s status/location=%d/%q", test.path, response.Code, response.Header().Get("Location"))
		}
		capabilities := auth.capabilities()
		if len(capabilities) != index+1 || capabilities[index] != test.capability {
			t.Fatalf("GET %s capabilities=%v", test.path, capabilities)
		}
	}
}

func TestFinalRouterRejectsMalformedCloudCampaignCarrierLaunchContext(t *testing.T) {
	router, auth := legacySurveyRouter(t, &legacySurveyStub{item: legacySurveyItem()})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, automationhttp.CloudOrchestratorCampaignsPath+"?source_kind=segment_members", legacyToken(195)))

	if response.Code != http.StatusBadRequest || response.Header().Get("Location") != "" {
		t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
	}
	capabilities := auth.capabilities()
	if len(capabilities) != 1 || capabilities[0] != authport.CapabilityOperationsRead {
		t.Fatalf("capabilities=%v", capabilities)
	}
}

func TestFinalRouterRejectsAudiencePackagePageMethodsBeforeAuthentication(t *testing.T) {
	router, auth := legacySurveyRouter(t, &legacySurveyStub{item: legacySurveyItem()})
	for _, path := range []string{automationhttp.AudiencePackagesPath, "/admin/automation-conversion/packages/42"} {
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
