package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetupWizardLegacyCarrierRemainsHTML(t *testing.T) {
	legacy := &Handler{}
	response := httptest.NewRecorder()
	legacy.adminOpsPage(response, httptest.NewRequest(http.MethodGet, "/setup/wizard", nil))
	if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Type"), "text/html") || !strings.Contains(response.Body.String(), "配置控制面") {
		t.Fatalf("legacy carrier status/content-type/body=%d/%q/%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
}
