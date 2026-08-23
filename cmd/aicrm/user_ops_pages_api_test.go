package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFinalRouterDoesNotMountCancelledUserOpsPageCarriers(t *testing.T) {
	router, auth := legacySurveyRouter(t, &legacySurveyStub{item: legacySurveyItem()})
	for _, path := range []string{"/admin/user-ops", "/admin/user-ops/ui"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("GET %s status=%d", path, response.Code)
		}
		if response.Header().Get("Location") != "" {
			t.Fatalf("GET %s location=%q", path, response.Header().Get("Location"))
		}
	}
	if capabilities := auth.capabilities(); len(capabilities) != 0 {
		t.Fatalf("capabilities=%v", capabilities)
	}
}
