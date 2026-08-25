package surveyhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	surveyapp "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/app"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type externalPushDetailApplicationStub struct {
	binding surveyapp.ExternalPushBinding
	err     error
	calls   int
}

func (s *externalPushDetailApplicationStub) Detail(_ context.Context, _ surveyport.ID, _ int64) (surveyapp.ExternalPushBinding, error) {
	s.calls++
	return s.binding, s.err
}

func TestExternalPushDetailHandlerKeepsReadRBACAndResponsePIIFree(t *testing.T) {
	app := &externalPushDetailApplicationStub{binding: surveyapp.ExternalPushBinding{
		QuestionnaireID: 9, SubmissionID: 12, CustomerID: 77, EffectID: "eer_123", State: eer.StateAccepted,
		SourceRefDigest: "sha256:source", TargetRefDigest: "sha256:target", PayloadDigest: "sha256:payload", PolicyVersionHash: "sha256:policy",
	}}
	handler := &ExternalPushDetailHandler{Application: app}

	wrongCapability, err := authport.WithAuthorization(context.Background(), authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, ExternalPushDetailPath, nil),
		httptest.NewRequest(http.MethodGet, ExternalPushDetailPath, nil).WithContext(wrongCapability),
	} {
		response := httptest.NewRecorder()
		handler.Get(response, request, 9, 12)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	}
	if app.calls != 0 {
		t.Fatalf("forbidden calls=%d", app.calls)
	}

	ctx, err := authport.WithAuthorization(context.Background(), authport.Authorization{Capability: authport.CapabilityQuestionnairesRead, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.Get(response, httptest.NewRequest(http.MethodGet, ExternalPushDetailPath, nil).WithContext(ctx), 9, 12)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 5 || body["submission_id"] != float64(12) || body["effect_id"] != "eer_123" || body["state"] != "accepted" || body["provider_accepted"] != false || body["delivery_proven"] != false {
		t.Fatalf("response=%v", body)
	}
	for _, forbidden := range []string{"customer_id", "source_ref_digest", "target_ref_digest", "payload_digest", "policy_version_hash", "identity", "receipt"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("response exposed %q: %v", forbidden, body)
		}
	}
}

func TestExternalPushDetailHandlerRejectsInvalidAndReturnsNotFound(t *testing.T) {
	ctx, err := authport.WithAuthorization(context.Background(), authport.Authorization{Capability: authport.CapabilityQuestionnairesRead, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	app := &externalPushDetailApplicationStub{}
	handler := &ExternalPushDetailHandler{Application: app}
	invalid := httptest.NewRecorder()
	handler.Get(invalid, httptest.NewRequest(http.MethodGet, ExternalPushDetailPath, nil).WithContext(ctx), 0, 12)
	if invalid.Code != http.StatusBadRequest || app.calls != 0 {
		t.Fatalf("invalid status/calls=%d/%d", invalid.Code, app.calls)
	}
	app.err = errors.New("missing binding")
	notFound := httptest.NewRecorder()
	handler.Get(notFound, httptest.NewRequest(http.MethodGet, ExternalPushDetailPath, nil).WithContext(ctx), 9, 12)
	if notFound.Code != http.StatusNotFound || app.calls != 1 {
		t.Fatalf("not found status/calls=%d/%d", notFound.Code, app.calls)
	}
}

func TestParseExternalPushDetailPath(t *testing.T) {
	questionnaireID, submissionID, ok := ParseExternalPushDetailPath("/api/admin/questionnaires/9/submissions/12/external-push")
	if !ok || questionnaireID != 9 || submissionID != 12 {
		t.Fatalf("parsed=%d/%d/%t", questionnaireID, submissionID, ok)
	}
	for _, path := range []string{
		"/api/admin/questionnaires/9/submissions/12",
		"/api/admin/questionnaires/0/submissions/12/external-push",
		"/api/admin/questionnaires/9/submissions/nope/external-push",
		"/api/admin/questionnaires/9/submissions/12/external-push/extra",
	} {
		if _, _, ok := ParseExternalPushDetailPath(path); ok {
			t.Fatalf("unsafe path accepted: %q", path)
		}
	}
}
