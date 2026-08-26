package campaign

import (
	"context"
	"errors"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"
)

type RecipientReviewApplication interface {
	Get(context.Context, string, string, int64) (TouchPlanRecipientReview, error)
	SaveMessageOverride(context.Context, SaveTouchPlanRecipientMessageOverrideCommand) (TouchPlanRecipientReviewResult, error)
	Approve(context.Context, DecideTouchPlanRecipientCommand) (TouchPlanRecipientReviewResult, error)
	Reject(context.Context, DecideTouchPlanRecipientCommand) (TouchPlanRecipientReviewResult, error)
}

type RecipientReviewRouteFragment struct {
	application RecipientReviewApplication
	authorizer  Authorizer
}

func NewRecipientReviewRouteFragment(application RecipientReviewApplication, authorizer Authorizer) (*RecipientReviewRouteFragment, error) {
	if nilish(application) || nilish(authorizer) {
		return nil, ErrUnavailable
	}
	return &RecipientReviewRouteFragment{application: application, authorizer: authorizer}, nil
}

func (h *RecipientReviewRouteFragment) Routes() []Route {
	base := RoutePrefix + "/{campaign_code}/touch-plans/{plan_id}/recipients/{customer_id}/review"
	return []Route{
		{Method: stdhttp.MethodGet, Pattern: base, Capability: CapabilityOperationsRead},
		{Method: stdhttp.MethodPost, Pattern: base + "/{operation}", Capability: CapabilityManageAutomation, RequiresCSRF: true},
	}
}

func (h *RecipientReviewRouteFragment) ServeHTTP(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if h == nil || nilish(h.application) || nilish(h.authorizer) || r == nil || r.URL == nil {
		writeHTTPError(w, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	if r.URL.EscapedPath() != r.URL.Path || strings.Contains(r.URL.Path, "\\") {
		writeHTTPError(w, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, RoutePrefix+"/"), "/")
	if len(parts) < 6 || len(parts) > 7 || !validCode(parts[0]) || parts[1] != "touch-plans" || !ValidTouchPlanReviewID(parts[2]) || parts[3] != "recipients" || parts[5] != "review" {
		writeHTTPError(w, stdhttp.StatusNotFound, "NOT_FOUND")
		return
	}
	customerID, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil || customerID < 1 {
		writeHTTPError(w, stdhttp.StatusNotFound, "NOT_FOUND")
		return
	}
	if len(parts) == 6 && r.Method == stdhttp.MethodGet {
		h.get(w, r, parts[0], parts[2], customerID)
		return
	}
	if len(parts) == 7 && r.Method == stdhttp.MethodPost && (parts[6] == "message" || parts[6] == "approve" || parts[6] == "reject") {
		h.mutate(w, r, parts[0], parts[2], customerID, parts[6])
		return
	}
	writeHTTPError(w, stdhttp.StatusNotFound, "NOT_FOUND")
}

func (h *RecipientReviewRouteFragment) authorize(w stdhttp.ResponseWriter, r *stdhttp.Request, capability string) (Actor, bool) {
	actor, err := h.authorizer.Authorize(r, AccessRequirement{Capability: capability})
	if errors.Is(err, ErrUnauthenticated) {
		writeHTTPError(w, stdhttp.StatusUnauthorized, "UNAUTHENTICATED")
		return Actor{}, false
	}
	if err != nil || actor.ID < 1 {
		writeHTTPError(w, stdhttp.StatusForbidden, "FORBIDDEN")
		return Actor{}, false
	}
	return actor, true
}

func (h *RecipientReviewRouteFragment) get(w stdhttp.ResponseWriter, r *stdhttp.Request, campaignCode, planID string, customerID int64) {
	if !emptyBody(r) || r.URL.RawQuery != "" {
		writeHTTPError(w, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	if _, ok := h.authorize(w, r, CapabilityOperationsRead); !ok {
		return
	}
	value, err := h.application.Get(r.Context(), campaignCode, planID, customerID)
	if err != nil {
		mapInitiationError(w, err)
		return
	}
	if !ValidTouchPlanRecipientReview(value) || value.CampaignCode != campaignCode || value.PlanID != planID || value.CustomerID != customerID {
		writeHTTPError(w, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	writeJSON(w, stdhttp.StatusOK, recipientReviewMutationResponse{Review: recipientReviewProjection(value), ReviewSafety: localReviewSafety()})
}

func (h *RecipientReviewRouteFragment) mutate(w stdhttp.ResponseWriter, r *stdhttp.Request, campaignCode, planID string, customerID int64, operation string) {
	if r.URL.RawQuery != "" {
		writeHTTPError(w, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	actor, ok := h.authorize(w, r, CapabilityManageAutomation)
	if !ok {
		return
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !validKey(keys[0]) {
		writeHTTPError(w, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	var body recipientReviewMutationRequest
	if !decodeJSON(r, &body) || body.ExpectedPlanVersion < 1 || body.ExpectedRecipientVersion < 0 || (operation == "message") != (body.MessageOverride != "") {
		writeHTTPError(w, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	var result TouchPlanRecipientReviewResult
	var err error
	if operation == "message" {
		result, err = h.application.SaveMessageOverride(r.Context(), SaveTouchPlanRecipientMessageOverrideCommand{
			CampaignCode: campaignCode, PlanID: planID, CustomerID: customerID, ExpectedPlanVersion: body.ExpectedPlanVersion,
			ExpectedRecipientVersion: body.ExpectedRecipientVersion, MessageOverride: body.MessageOverride, Actor: actor, IdempotencyKey: keys[0],
		})
	} else {
		command := DecideTouchPlanRecipientCommand{CampaignCode: campaignCode, PlanID: planID, CustomerID: customerID, ExpectedPlanVersion: body.ExpectedPlanVersion, ExpectedRecipientVersion: body.ExpectedRecipientVersion, Actor: actor, IdempotencyKey: keys[0]}
		if operation == "approve" {
			result, err = h.application.Approve(r.Context(), command)
		} else {
			result, err = h.application.Reject(r.Context(), command)
		}
	}
	if err != nil {
		mapInitiationError(w, err)
		return
	}
	if result.EventID < 1 || !ValidTouchPlanRecipientReview(result.Review) || result.Review.CampaignCode != campaignCode || result.Review.PlanID != planID || result.Review.CustomerID != customerID {
		writeHTTPError(w, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	writeJSON(w, stdhttp.StatusOK, recipientReviewMutationResponse{Review: recipientReviewProjection(result.Review), EventID: result.EventID, ReviewSafety: localReviewSafety()})
}

type recipientReviewMutationRequest struct {
	ExpectedPlanVersion      int64  `json:"expected_plan_version"`
	ExpectedRecipientVersion int64  `json:"expected_recipient_version"`
	MessageOverride          string `json:"message_override,omitempty"`
}

type recipientReviewResponse struct {
	CanonicalCustomerID int64                          `json:"canonical_customer_id"`
	MessageOverride     string                         `json:"message_override,omitempty"`
	Status              TouchPlanRecipientReviewStatus `json:"status"`
	Version             int64                          `json:"version"`
	UpdatedByActorID    int64                          `json:"updated_by_actor_id"`
	UpdatedAt           string                         `json:"updated_at"`
}

type recipientReviewMutationResponse struct {
	Review  recipientReviewResponse `json:"review"`
	EventID int64                   `json:"event_id,omitempty"`
	ReviewSafety
}

func recipientReviewProjection(value TouchPlanRecipientReview) recipientReviewResponse {
	return recipientReviewResponse{
		CanonicalCustomerID: value.CustomerID, MessageOverride: value.MessageOverride, Status: value.Status,
		Version: value.Version, UpdatedByActorID: value.UpdatedByActorID, UpdatedAt: value.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}
