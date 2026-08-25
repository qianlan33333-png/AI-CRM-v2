package surveyhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
)

type externalPushReconcileApplicationStub struct {
	value   surveyapp.ExternalPushBinding
	err     error
	calls   int
	command surveyapp.ExternalPushReconcileCommand
}

func (s *externalPushReconcileApplicationStub) Reconcile(_ context.Context, command surveyapp.ExternalPushReconcileCommand) (surveyapp.ExternalPushBinding, error) {
	s.calls++
	s.command = command
	return s.value, s.err
}

func TestExternalPushReconcileHandlerRequiresWriteRBACAndKeepsReceiptFactsSeparate(t *testing.T) {
	app := &externalPushReconcileApplicationStub{value: surveyapp.ExternalPushBinding{SubmissionID: 12, CustomerID: 77, EffectID: "eer_123", State: eer.StateReconciled, ProviderAccepted: true, DeliveryProven: false}}
	handler := &ExternalPushReconcileHandler{Application: app}

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, ExternalPushReconcilePath, strings.NewReader(externalPushReconcileJSON())),
		externalPushReconcileRequestWithAuthorization(t, authport.Authorization{Capability: authport.CapabilityQuestionnairesRead, Scope: authport.ScopeGlobal}),
	} {
		response := httptest.NewRecorder()
		handler.Reconcile(response, request, 9, 12)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	}
	if app.calls != 0 {
		t.Fatalf("unauthorized calls=%d", app.calls)
	}

	request := externalPushReconcileRequestWithAuthorization(t, authport.Authorization{Capability: authport.CapabilityQuestionnairesWrite, Scope: authport.ScopeGlobal})
	response := httptest.NewRecorder()
	handler.Reconcile(response, request, 9, 12)
	if response.Code != http.StatusOK || app.calls != 1 {
		t.Fatalf("status/calls=%d/%d body=%s", response.Code, app.calls, response.Body.String())
	}
	if app.command.Lease.EffectID != "eer_123" || app.command.Lease.Generation != 2 || app.command.Lease.Fence != 5 || !app.command.ProviderAccepted || app.command.DeliveryProven || len(app.command.IdempotencyKey) < 16 {
		t.Fatalf("command=%+v", app.command)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 5 || body["state"] != "reconciled" || body["provider_accepted"] != true || body["delivery_proven"] != false {
		t.Fatalf("body=%v", body)
	}
	for _, forbidden := range []string{"digest", "identity", "receipt", "customer_id"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("response exposed %q: %v", forbidden, body)
		}
	}
}

func TestExternalPushReconcileHandlerRejectsMalformedAndNonUnknownReconciliation(t *testing.T) {
	app := &externalPushReconcileApplicationStub{}
	handler := &ExternalPushReconcileHandler{Application: app}
	validAuthorization := authport.Authorization{Capability: authport.CapabilityQuestionnairesWrite, Scope: authport.ScopeGlobal}
	for _, body := range []string{
		`{"effect_id":"eer_123","generation":2,"fence":5,"lease_expires_at":"2026-08-25T12:00:00Z","evidence_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","provider_accepted":true}`,
		`{"effect_id":"eer_123","generation":2,"fence":5,"lease_expires_at":"2026-08-25T12:00:00Z","evidence_digest":"not-a-digest","provider_accepted":true,"delivery_proven":false}`,
	} {
		request := externalPushReconcileRequestWithBodyAndAuthorization(t, body, validAuthorization)
		response := httptest.NewRecorder()
		handler.Reconcile(response, request, 9, 12)
		if response.Code != http.StatusBadRequest || app.calls != 0 {
			t.Fatalf("status/calls=%d/%d body=%s", response.Code, app.calls, response.Body.String())
		}
	}
	app.err = surveyapp.ErrExternalPushReconcileRequired
	response := httptest.NewRecorder()
	handler.Reconcile(response, externalPushReconcileRequestWithAuthorization(t, validAuthorization), 9, 12)
	if response.Code != http.StatusConflict || app.calls != 1 {
		t.Fatalf("conflict status/calls=%d/%d", response.Code, app.calls)
	}
	app.err = surveyapp.ErrExternalPushNotFound
	response = httptest.NewRecorder()
	handler.Reconcile(response, externalPushReconcileRequestWithAuthorization(t, validAuthorization), 9, 12)
	if response.Code != http.StatusNotFound || app.calls != 2 {
		t.Fatalf("not found status/calls=%d/%d", response.Code, app.calls)
	}
}

func TestParseExternalPushReconcilePath(t *testing.T) {
	questionnaireID, submissionID, ok := ParseExternalPushReconcilePath("/api/admin/questionnaires/9/submissions/12/external-push/reconcile")
	if !ok || questionnaireID != 9 || submissionID != 12 {
		t.Fatalf("parsed=%d/%d/%t", questionnaireID, submissionID, ok)
	}
	for _, path := range []string{
		"/api/admin/questionnaires/9/submissions/12/external-push/retry",
		"/api/admin/questionnaires/9/submissions/12/external-push//reconcile",
	} {
		if _, _, ok := ParseExternalPushReconcilePath(path); ok {
			t.Fatalf("unsafe path accepted as reconcile: %s", path)
		}
	}
}

func externalPushReconcileRequestWithAuthorization(t *testing.T, authorization authport.Authorization) *http.Request {
	t.Helper()
	return externalPushReconcileRequestWithBodyAndAuthorization(t, externalPushReconcileJSON(), authorization)
}

func externalPushReconcileRequestWithBodyAndAuthorization(t *testing.T, body string, authorization authport.Authorization) *http.Request {
	t.Helper()
	ctx, err := authport.WithAuthorization(context.Background(), authorization)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, ExternalPushReconcilePath, strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "survey-external-push-reconcile-key")
	return request
}

func externalPushReconcileJSON() string {
	return `{"effect_id":"eer_123","generation":2,"fence":5,"lease_expires_at":"` + time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano) + `","evidence_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","provider_accepted":true,"delivery_proven":false}`
}
