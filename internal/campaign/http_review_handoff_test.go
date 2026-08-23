package campaign

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type reviewHTTPApplicationStub struct {
	submit  TouchPlanReview
	approve TouchPlanReviewResult
}

func (s reviewHTTPApplicationStub) Submit(context.Context, SubmitTouchPlanReviewCommand) (TouchPlanReview, error) {
	return s.submit, nil
}
func (s reviewHTTPApplicationStub) Approve(context.Context, DecideTouchPlanReviewCommand) (TouchPlanReviewResult, error) {
	return s.approve, nil
}
func (s reviewHTTPApplicationStub) Reject(context.Context, DecideTouchPlanReviewCommand) (TouchPlanReviewResult, error) {
	return s.approve, nil
}
func (reviewHTTPApplicationStub) ListRecipients(context.Context, string, string, *TouchPlanRecipientKeyset, int32) (TouchPlanRecipientPage, error) {
	return TouchPlanRecipientPage{}, ErrNotFound
}
func (reviewHTTPApplicationStub) GetRecipient(context.Context, string, string, int64) (TouchPlanRecipient, error) {
	return TouchPlanRecipient{}, ErrNotFound
}
func (reviewHTTPApplicationStub) GetReview(context.Context, string, string) (TouchPlanReviewResult, error) {
	return TouchPlanReviewResult{}, ErrNotFound
}

func TestReviewHandoffRouteRejectsCrossCampaignMutationProjection(t *testing.T) {
	planID := DraftTouchPlanID(7, "spring-campaign", "review-http-projection")
	now := time.Date(2026, time.August, 23, 2, 3, 4, 0, time.UTC)
	pending := TouchPlanReview{PlanID: planID, CampaignCode: "spring-campaign", Status: TouchPlanReviewPending, Version: 2, SubmittedByActorID: 7, SubmittedAt: now}
	approved := pending
	approved.Status, approved.Version, approved.ReviewedByActorID, approved.ReviewedAt, approved.ConfirmationDigest = TouchPlanReviewApproved, 3, 8, now, ReviewConfirmationDigest(ReviewConfirmation("approve", planID))
	handoff := TouchPlanHandoff{PlanID: planID, CampaignCode: "spring-campaign", ReviewVersion: 3, Status: HandoffPendingOutboundAccept, CreatedAt: now, LocalOnly: true}
	if !ValidTouchPlanReview(pending) || !ValidTouchPlanReview(approved) || !ValidTouchPlanHandoff(handoff) {
		t.Fatal("invalid review fixture")
	}
	for _, test := range []struct {
		name, operation, body string
		application           reviewHTTPApplicationStub
	}{
		{name: "submit review campaign", operation: "submit", body: `{"expected_version":1}`, application: reviewHTTPApplicationStub{submit: withReviewCampaign(pending, "other-campaign")}},
		{name: "approve review campaign", operation: "approve", body: `{"expected_version":2,"confirmation":"APPROVE ` + planID + `"}`, application: reviewHTTPApplicationStub{approve: TouchPlanReviewResult{Review: withReviewCampaign(approved, "other-campaign"), Handoff: &handoff, EventIDs: []int64{1, 2}}}},
		{name: "approve handoff campaign", operation: "approve", body: `{"expected_version":2,"confirmation":"APPROVE ` + planID + `"}`, application: reviewHTTPApplicationStub{approve: TouchPlanReviewResult{Review: approved, Handoff: withHandoffCampaign(handoff, "other-campaign"), EventIDs: []int64{1, 2}}}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			handler, err := NewReviewHandoffRouteFragment(test.application, &initiationHTTPAuthorizerStub{actor: Actor{ID: 7}})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, RoutePrefix+"/spring-campaign/touch-plans/"+planID+"/review/"+test.operation, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "review-http-projection-key")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func withReviewCampaign(value TouchPlanReview, campaignCode string) TouchPlanReview {
	value.CampaignCode = campaignCode
	return value
}

func withHandoffCampaign(value TouchPlanHandoff, campaignCode string) *TouchPlanHandoff {
	value.CampaignCode = campaignCode
	return &value
}
