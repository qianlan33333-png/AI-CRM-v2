package outboundhttp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
)

type campaignHandoffHTTPApplication struct {
	result                outbound.CampaignHandoffSummary
	acceptCalls, getCalls int
}

func (application *campaignHandoffHTTPApplication) Accept(_ context.Context, command outboundapp.AcceptCampaignHandoffCommand) (outbound.CampaignHandoffSummary, error) {
	application.acceptCalls++
	if command.ActorID != 7 || command.ExpectedReviewVersion != 3 {
		return outbound.CampaignHandoffSummary{}, outbound.ErrCampaignHandoffInvalid
	}
	return application.result, nil
}
func (application *campaignHandoffHTTPApplication) Get(context.Context, string, string) (outbound.CampaignHandoffSummary, error) {
	application.getCalls++
	return application.result, nil
}

func TestCampaignHandoffHTTPReturnsCountOnlyLocalSafety(t *testing.T) {
	application := &campaignHandoffHTTPApplication{result: campaignHandoffHTTPSummary()}
	handler, err := NewCampaignHandoffHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/admin/outbound/campaign-handoffs/spring-campaign/"+application.result.PlanID+"/accept", bytes.NewBufferString(`{"expected_review_version":3}`))
	request.Header.Set("Idempotency-Key", "aaaaaaaaaaaaaaaa")
	request = authorizedCampaignHandoffRequest(t, request, authport.CapabilityOperationsManage)
	response := httptest.NewRecorder()
	handler.Accept(response, request, "spring-campaign", application.result.PlanID)
	if response.Code != http.StatusOK || application.acceptCalls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, application.acceptCalls, response.Body)
	}
	body := response.Body.String()
	for _, forbidden := range []string{"customer_id", "content", "provider_message_id", "provider_code", "last_error", "unionid", "external_userid", "mobile", "payload"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
	for _, expected := range []string{`"held_count":2`, `"not_evaluated_count":2`, `"local_only":true`, `"provider_execution_eligible":false`, `"real_external_call_executed":false`, `"delivery_proven":false`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("missing %s: %s", expected, body)
		}
	}
}

func TestCampaignHandoffHTTPRejectsDuplicateKeyUnknownBodyAndQuery(t *testing.T) {
	application := &campaignHandoffHTTPApplication{result: campaignHandoffHTTPSummary()}
	handler, _ := NewCampaignHandoffHandler(application)
	for _, test := range []struct {
		name, body, query string
		duplicate         bool
	}{
		{name: "duplicate key", body: `{"expected_review_version":3}`, duplicate: true},
		{name: "unknown body", body: `{"expected_review_version":3,"customer_ids":[7]}`},
		{name: "query", body: `{"expected_review_version":3}`, query: "?release=true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/accept"+test.query, strings.NewReader(test.body))
			request.Header.Set("Idempotency-Key", "aaaaaaaaaaaaaaaa")
			if test.duplicate {
				request.Header.Add("Idempotency-Key", "bbbbbbbbbbbbbbbb")
			}
			request = authorizedCampaignHandoffRequest(t, request, authport.CapabilityOperationsManage)
			response := httptest.NewRecorder()
			handler.Accept(response, request, "spring-campaign", application.result.PlanID)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
		})
	}
	if application.acceptCalls != 0 {
		t.Fatalf("accept calls=%d", application.acceptCalls)
	}
}

func authorizedCampaignHandoffRequest(t *testing.T, request *http.Request, capability authport.Capability) *http.Request {
	t.Helper()
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleOps}, "session")
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return request.WithContext(ctx)
}
func campaignHandoffHTTPSummary() outbound.CampaignHandoffSummary {
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	return outbound.CampaignHandoffSummary{ID: 9, CampaignCode: "spring-campaign", PlanID: "ctp_" + digest, ReviewVersion: 3, Status: outbound.CampaignHandoffHeld, TargetCount: 2, StepCount: 1, HeldCount: 2, NotEvaluatedCount: 2, AcceptedAt: time.Date(2026, 8, 23, 3, 4, 5, 0, time.UTC), Safety: outbound.LocalCampaignHandoffSafety()}
}
