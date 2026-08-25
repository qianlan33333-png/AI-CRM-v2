package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
)

func TestReleasePlaneAdapterFailsClosedWithoutDependency(t *testing.T) {
	handler := &candidateHandler{}
	checks := []func(http.ResponseWriter, *http.Request){
		func(w http.ResponseWriter, r *http.Request) {
			handler.ListReleaseCandidates(w, r, api.ListReleaseCandidatesParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.RegisterReleaseCandidate(w, r, api.RegisterReleaseCandidateParams{})
		},
		func(w http.ResponseWriter, r *http.Request) { handler.GetReleaseCandidate(w, r, 1) },
		func(w http.ResponseWriter, r *http.Request) {
			handler.RecordReleasePrerequisite(w, r, 1, api.RecordReleasePrerequisiteParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.PrepareReleaseCandidate(w, r, 1, api.PrepareReleaseCandidateParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.StartReleaseCutover(w, r, 1, api.StartReleaseCutoverParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.RestartReleaseCutover(w, r, 1, api.RestartReleaseCutoverParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.CompleteReleaseCutoverStep(w, r, 1, api.ReleaseCutoverStep("announce"), api.CompleteReleaseCutoverStepParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.ActivateReleaseCandidate(w, r, 1, api.ActivateReleaseCandidateParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.RecordReleaseRollbackCheck(w, r, 1, api.RecordReleaseRollbackCheckParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.RequestReleaseRollback(w, r, 1, api.RequestReleaseRollbackParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handler.CompleteReleaseRollback(w, r, 1, api.CompleteReleaseRollbackParams{})
		},
	}
	for index, check := range checks {
		response := httptest.NewRecorder()
		check(response, httptest.NewRequest(http.MethodPost, "/api/v1/admin/release-candidates/1", nil))
		if response.Code != http.StatusServiceUnavailable || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("operation %d = %d cache=%q, want 503/no-store", index, response.Code, response.Header().Get("Cache-Control"))
		}
	}
}
