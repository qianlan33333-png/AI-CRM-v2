package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/http"
)

type surveyExternalPushReconcileRouteStub struct {
	calls   int
	command surveyapp.ExternalPushReconcileCommand
}

func (s *surveyExternalPushReconcileRouteStub) Reconcile(_ context.Context, command surveyapp.ExternalPushReconcileCommand) (surveyapp.ExternalPushBinding, error) {
	s.calls++
	s.command = command
	return surveyapp.ExternalPushBinding{QuestionnaireID: command.QuestionnaireID, SubmissionID: command.SubmissionID, EffectID: command.Lease.EffectID, State: eer.StateReconciled, ProviderAccepted: command.ProviderAccepted, DeliveryProven: command.DeliveryProven}, nil
}

func TestSurveyExternalPushReconcileRouteUsesWriteAuthCSRFAndIdempotency(t *testing.T) {
	authService := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(authService)
	if err != nil {
		t.Fatal(err)
	}
	app := &surveyExternalPushReconcileRouteStub{}
	candidate := &candidateHandler{Handler: authHandler, surveyPushReconcile: &surveyhttp.ExternalPushReconcileHandler{Application: app}}
	router, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, candidate)
	if err != nil {
		t.Fatal(err)
	}

	request := surveyExternalPushReconcileRouteRequest(false)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || app.calls != 0 {
		t.Fatalf("missing csrf status/calls=%d/%d body=%s", response.Code, app.calls, response.Body.String())
	}

	request = surveyExternalPushReconcileRouteRequest(true)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || app.calls != 1 || app.command.IdempotencyKey != "survey-external-push-reconcile-key" {
		t.Fatalf("status/calls/command=%d/%d/%+v body=%s", response.Code, app.calls, app.command, response.Body.String())
	}
	if got := authService.capabilities(); len(got) != 1 || got[0] != authport.CapabilityQuestionnairesWrite {
		t.Fatalf("capabilities=%v", got)
	}
	for _, forbidden := range []string{"digest", "identity", "receipt", "customer_id"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("response exposed %q: %s", forbidden, response.Body.String())
		}
	}
}

func surveyExternalPushReconcileRouteRequest(withCSRF bool) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/admin/questionnaires/9/submissions/12/external-push/reconcile", strings.NewReader(`{"effect_id":"eer_123","generation":2,"fence":5,"lease_expires_at":"2026-08-25T12:00:00Z","evidence_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","provider_accepted":true,"delivery_proven":false}`))
	request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "survey-reconcile-session"})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "survey-external-push-reconcile-key")
	if withCSRF {
		request.Header.Set("X-CSRF-Token", strings.Repeat("A", 43))
	}
	return request
}
