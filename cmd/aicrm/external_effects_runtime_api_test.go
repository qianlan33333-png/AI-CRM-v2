package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
)

func TestExternalEffectsRuntimeAdapterFailsClosedWithoutDependency(t *testing.T) {
	handler := &candidateHandler{}
	effectID := api.ExternalEffectRuntimeID("1")
	checks := []func(http.ResponseWriter, *http.Request){
		func(w http.ResponseWriter, r *http.Request) {
			handler.ListExternalEffectsRuntime(w, r, api.ListExternalEffectsRuntimeParams{})
		},
		func(w http.ResponseWriter, r *http.Request) { handler.GetExternalEffectRuntime(w, r, effectID) },
		handler.GetExternalEffectsDiagnostics,
		func(w http.ResponseWriter, r *http.Request) {
			handler.CancelExternalEffectRuntime(w, r, effectID, api.CancelExternalEffectRuntimeParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.RetryExternalEffectRuntime(w, r, effectID, api.RetryExternalEffectRuntimeParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.ReconcileExternalEffectRuntime(w, r, effectID, api.ReconcileExternalEffectRuntimeParams{})
		},
	}
	for index, check := range checks {
		response := httptest.NewRecorder()
		check(response, httptest.NewRequest(http.MethodPost, "/api/admin/external-effects/1", nil))
		if response.Code != http.StatusServiceUnavailable || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("operation %d = %d cache=%q, want 503/no-store", index, response.Code, response.Header().Get("Cache-Control"))
		}
	}
}
