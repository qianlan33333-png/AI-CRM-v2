package http

import (
	"context"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eventapp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/app"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type cloudAuditApplicationStub struct{ filter eventport.CloudAuditFilter }

func (application *cloudAuditApplicationStub) List(_ context.Context, filter eventport.CloudAuditFilter) (eventapp.CloudAuditResult, error) {
	application.filter = filter
	return eventapp.CloudAuditResult{Filter: filter, Items: []eventport.CloudAuditFact{{EventID: 9, EventType: "cloud_campaign.fact", OccurredAt: time.Now().UTC(), OutcomeUnknown: 1}}, ObservedAt: time.Now().UTC(), LocalFactsOnly: true}, nil
}

func TestCloudAuditHTTPFiltersSessionAndDeniesDeliveryProof(t *testing.T) {
	application := &cloudAuditApplicationStub{}
	handler, err := NewCloudAuditHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/admin/cloud-orchestrator/audit?session_id=session-1&limit=25", nil)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, "session")
	ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.List(response, request.WithContext(ctx))
	if response.Code != stdhttp.StatusOK || application.filter.SessionID != "session-1" || application.filter.Limit != 25 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d filter=%+v body=%s", response.Code, application.filter, response.Body)
	}
	for _, expected := range []string{`"outcome_unknown":1`, `"local_facts_only":true`, `"real_external_call_executed":false`, `"delivery_proven":false`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("missing %s in %s", expected, response.Body)
		}
	}
}
