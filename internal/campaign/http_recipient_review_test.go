package campaign

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type recipientReviewHTTPStub struct {
	value   TouchPlanRecipientReview
	result  TouchPlanRecipientReviewResult
	saved   SaveTouchPlanRecipientMessageOverrideCommand
	decided DecideTouchPlanRecipientCommand
}

func (s *recipientReviewHTTPStub) Get(context.Context, string, string, int64) (TouchPlanRecipientReview, error) {
	return s.value, nil
}
func (s *recipientReviewHTTPStub) SaveMessageOverride(_ context.Context, command SaveTouchPlanRecipientMessageOverrideCommand) (TouchPlanRecipientReviewResult, error) {
	s.saved = command
	return s.result, nil
}
func (s *recipientReviewHTTPStub) Approve(_ context.Context, command DecideTouchPlanRecipientCommand) (TouchPlanRecipientReviewResult, error) {
	s.decided = command
	return s.result, nil
}
func (s *recipientReviewHTTPStub) Reject(_ context.Context, command DecideTouchPlanRecipientCommand) (TouchPlanRecipientReviewResult, error) {
	s.decided = command
	return s.result, nil
}

func TestRecipientReviewRouteMutatesOnlyLocalReview(t *testing.T) {
	planID := DraftTouchPlanID(3, "spring-campaign", "recipient-review-http")
	value := TouchPlanRecipientReview{
		PlanID: planID, CampaignCode: "spring-campaign", CustomerID: 19, MessageOverride: "您好，请查收。",
		Status: TouchPlanRecipientReviewPending, Version: 1, UpdatedByActorID: 7,
		UpdatedAt: time.Date(2026, time.August, 27, 1, 2, 3, 456000, time.UTC), Safety: LocalInitiationSafety(),
	}
	stub := &recipientReviewHTTPStub{result: TouchPlanRecipientReviewResult{Review: value, EventID: 41}}
	handler, err := NewRecipientReviewRouteFragment(stub, &initiationHTTPAuthorizerStub{actor: Actor{ID: 7}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, RoutePrefix+"/spring-campaign/touch-plans/"+planID+"/recipients/19/review/message", strings.NewReader(`{"expected_plan_version":2,"expected_recipient_version":0,"message_override":"您好，请查收。"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "recipient-review-http-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if stub.saved.CustomerID != 19 || stub.saved.ExpectedPlanVersion != 2 || stub.saved.ExpectedRecipientVersion != 0 || stub.saved.Actor.ID != 7 || stub.saved.IdempotencyKey != "recipient-review-http-key" {
		t.Fatalf("command=%+v", stub.saved)
	}
	body := response.Body.String()
	for _, expected := range []string{`"status":"pending_review"`, `"event_id":41`, `"local_only":true`, `"provider_execution_eligible":false`, `"real_external_call_executed":false`, `"delivery_proven":false`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("missing %s in %s", expected, body)
		}
	}
}

func TestRecipientReviewRouteReadsScopedProjectionAndRejectsMessageOnDecision(t *testing.T) {
	planID := DraftTouchPlanID(3, "spring-campaign", "recipient-review-http-read")
	value := TouchPlanRecipientReview{
		PlanID: planID, CampaignCode: "spring-campaign", CustomerID: 19, Status: TouchPlanRecipientReviewApproved,
		Version: 2, UpdatedByActorID: 7, UpdatedAt: time.Date(2026, time.August, 27, 1, 2, 3, 0, time.UTC), Safety: LocalInitiationSafety(),
	}
	stub := &recipientReviewHTTPStub{value: value, result: TouchPlanRecipientReviewResult{Review: value, EventID: 42}}
	handler, err := NewRecipientReviewRouteFragment(stub, &initiationHTTPAuthorizerStub{actor: Actor{ID: 7}})
	if err != nil {
		t.Fatal(err)
	}
	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, RoutePrefix+"/spring-campaign/touch-plans/"+planID+"/recipients/19/review", nil))
	if read.Code != http.StatusOK || strings.Contains(read.Body.String(), "event_id") || !strings.Contains(read.Body.String(), `"status":"approved"`) {
		t.Fatalf("status=%d body=%s", read.Code, read.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, RoutePrefix+"/spring-campaign/touch-plans/"+planID+"/recipients/19/review/approve", strings.NewReader(`{"expected_plan_version":2,"expected_recipient_version":1,"message_override":"unexpected"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "recipient-review-decision-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || stub.decided.CustomerID != 0 {
		t.Fatalf("status=%d command=%+v body=%s", response.Code, stub.decided, response.Body.String())
	}
}

func TestRecipientReviewRouteRejectsCrossScopeProjection(t *testing.T) {
	planID := DraftTouchPlanID(3, "spring-campaign", "recipient-review-http-scope")
	value := TouchPlanRecipientReview{
		PlanID: planID, CampaignCode: "other-campaign", CustomerID: 19, Status: TouchPlanRecipientReviewRejected,
		Version: 1, UpdatedByActorID: 7, UpdatedAt: time.Date(2026, time.August, 27, 1, 2, 3, 0, time.UTC), Safety: LocalInitiationSafety(),
	}
	stub := &recipientReviewHTTPStub{result: TouchPlanRecipientReviewResult{Review: value, EventID: 43}}
	handler, err := NewRecipientReviewRouteFragment(stub, &initiationHTTPAuthorizerStub{actor: Actor{ID: 7}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, RoutePrefix+"/spring-campaign/touch-plans/"+planID+"/recipients/19/review/reject", strings.NewReader(`{"expected_plan_version":2,"expected_recipient_version":0}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "recipient-review-scope-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
