package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	commercehttp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/http"
)

func TestFinalRouterMountsCommerceWorkspaceCarriersWithAdminRead(t *testing.T) {
	router, auth := legacySurveyRouter(t, &legacySurveyStub{item: legacySurveyItem()})
	paths := []string{
		commercehttp.AlipayTransactionsPath,
		commercehttp.ServiceProductsPath,
		commercehttp.ServiceProductNewPath,
		commercehttp.ServiceProductsPath + "/service_A-42/edit",
		commercehttp.ServiceProductsPath + "/service_A-42/data",
		commercehttp.WeChatPayProductNewPath,
		commercehttp.WeChatPayProductsPath + "/product_A-42/edit",
		commercehttp.WeChatPayTransactionsPath,
		commercehttp.WeChatPayTransactionsPath + "/order_A-42",
		commercehttp.WeChatShopTransactionsPath,
		commercehttp.WeChatShopTransactionsPath + "/order_A-42",
	}
	for _, path := range paths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(191)))
		if response.Code != http.StatusFound || response.Header().Get("Location") == "" {
			t.Fatalf("GET %s status/location=%d/%q", path, response.Code, response.Header().Get("Location"))
		}
		if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("GET %s headers=%q/%q", path, response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
		}
	}
	capabilities := auth.capabilities()
	if len(capabilities) != len(paths) {
		t.Fatalf("capabilities=%v", capabilities)
	}
	for _, capability := range capabilities {
		if capability != authport.CapabilityAdminRead {
			t.Fatalf("capabilities=%v", capabilities)
		}
	}
}

func TestFinalRouterRejectsCommerceWorkspaceMethodsBeforeAuthentication(t *testing.T) {
	router, auth := legacySurveyRouter(t, &legacySurveyStub{item: legacySurveyItem()})
	for _, path := range []string{commercehttp.ServiceProductsPath, commercehttp.WeChatPayTransactionsPath + "/order_A-42"} {
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
