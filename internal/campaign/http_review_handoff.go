package campaign

import (
	"context"
	"errors"
	stdhttp "net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ReviewHandoffApplication exposes the closed 00067 review seam. It never
// enriches recipient OneIDs with Identity, phone, or provider data.
type ReviewHandoffApplication interface {
	Submit(context.Context, SubmitTouchPlanReviewCommand) (TouchPlanReview, error)
	Approve(context.Context, DecideTouchPlanReviewCommand) (TouchPlanReviewResult, error)
	Reject(context.Context, DecideTouchPlanReviewCommand) (TouchPlanReviewResult, error)
	ListRecipients(context.Context, string, string, *TouchPlanRecipientKeyset, int32) (TouchPlanRecipientPage, error)
	GetRecipient(context.Context, string, string, int64) (TouchPlanRecipient, error)
	GetReview(context.Context, string, string) (TouchPlanReviewResult, error)
}
type ReviewHandoffRouteFragment struct {
	application ReviewHandoffApplication
	authorizer  Authorizer
}

func NewReviewHandoffRouteFragment(application ReviewHandoffApplication, authorizer Authorizer) (*ReviewHandoffRouteFragment, error) {
	if nilish(application) || nilish(authorizer) {
		return nil, ErrUnavailable
	}
	return &ReviewHandoffRouteFragment{application: application, authorizer: authorizer}, nil
}
func (h *ReviewHandoffRouteFragment) Routes() []Route {
	return []Route{
		{Method: stdhttp.MethodGet, Pattern: RoutePrefix + "/{campaign_code}/touch-plans/{plan_id}/review", Capability: CapabilityOperationsRead},
		{Method: stdhttp.MethodGet, Pattern: RoutePrefix + "/{campaign_code}/touch-plans/{plan_id}/recipients", Capability: CapabilityOperationsRead},
		{Method: stdhttp.MethodGet, Pattern: RoutePrefix + "/{campaign_code}/touch-plans/{plan_id}/recipients/{customer_id}", Capability: CapabilityOperationsRead},
		{Method: stdhttp.MethodPost, Pattern: RoutePrefix + "/{campaign_code}/touch-plans/{plan_id}/review/submit", Capability: CapabilityManageAutomation, RequiresCSRF: true},
		{Method: stdhttp.MethodPost, Pattern: RoutePrefix + "/{campaign_code}/touch-plans/{plan_id}/review/approve", Capability: CapabilityManageAutomation, RequiresCSRF: true},
		{Method: stdhttp.MethodPost, Pattern: RoutePrefix + "/{campaign_code}/touch-plans/{plan_id}/review/reject", Capability: CapabilityManageAutomation, RequiresCSRF: true},
	}
}
func (h *ReviewHandoffRouteFragment) ServeHTTP(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if h == nil || nilish(h.application) || nilish(h.authorizer) || r == nil || r.URL == nil {
		writeHTTPError(w, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	if r.URL.EscapedPath() != r.URL.Path || strings.Contains(r.URL.Path, "\\") {
		writeHTTPError(w, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, RoutePrefix+"/"), "/")
	if len(parts) < 4 || !validCode(parts[0]) || parts[1] != "touch-plans" || !ValidTouchPlanReviewID(parts[2]) {
		writeHTTPError(w, stdhttp.StatusNotFound, "NOT_FOUND")
		return
	}
	switch {
	case len(parts) == 4 && parts[3] == "review" && r.Method == stdhttp.MethodGet:
		h.getReview(w, r, parts[0], parts[2])
	case len(parts) == 4 && parts[3] == "recipients" && r.Method == stdhttp.MethodGet:
		h.listRecipients(w, r, parts[0], parts[2])
	case len(parts) == 5 && parts[3] == "recipients" && r.Method == stdhttp.MethodGet:
		h.getRecipient(w, r, parts[0], parts[2], parts[4])
	case len(parts) == 5 && parts[3] == "review" && r.Method == stdhttp.MethodPost && (parts[4] == "submit" || parts[4] == "approve" || parts[4] == "reject"):
		h.review(w, r, parts[0], parts[2], parts[4])
	default:
		writeHTTPError(w, stdhttp.StatusNotFound, "NOT_FOUND")
	}
}
func (h *ReviewHandoffRouteFragment) authorize(w stdhttp.ResponseWriter, r *stdhttp.Request, capability string) (Actor, bool) {
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
func (h *ReviewHandoffRouteFragment) listRecipients(w stdhttp.ResponseWriter, r *stdhttp.Request, campaignCode, planID string) {
	if !emptyBody(r) {
		writeHTTPError(w, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	if _, ok := h.authorize(w, r, CapabilityOperationsRead); !ok {
		return
	}
	cursor, limit, valid := parseRecipientQuery(r.URL.RawQuery, planID)
	if !valid {
		writeHTTPError(w, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	page, err := h.application.ListRecipients(r.Context(), campaignCode, planID, cursor, limit)
	if err != nil {
		mapInitiationError(w, err)
		return
	}
	items := make([]reviewRecipientResponse, len(page.Items))
	for i, item := range page.Items {
		if item.PlanID != planID || item.CustomerID < 1 {
			writeHTTPError(w, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
			return
		}
		items[i] = reviewRecipientResponse{CanonicalCustomerID: item.CustomerID}
	}
	var next *string
	if page.Next != nil {
		if page.Next.PlanID != planID || page.Next.CustomerID < 1 {
			writeHTTPError(w, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
			return
		}
		value := strconv.FormatInt(page.Next.CustomerID, 10)
		next = &value
	}
	writeJSON(w, stdhttp.StatusOK, reviewRecipientListResponse{Items: items, NextCursor: next, ReviewSafety: localReviewSafety()})
}
func (h *ReviewHandoffRouteFragment) getRecipient(w stdhttp.ResponseWriter, r *stdhttp.Request, campaignCode, planID, rawCustomerID string) {
	if !emptyBody(r) || r.URL.RawQuery != "" {
		writeHTTPError(w, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	if _, ok := h.authorize(w, r, CapabilityOperationsRead); !ok {
		return
	}
	customerID, err := strconv.ParseInt(rawCustomerID, 10, 64)
	if err != nil || customerID < 1 {
		writeHTTPError(w, stdhttp.StatusNotFound, "NOT_FOUND")
		return
	}
	item, err := h.application.GetRecipient(r.Context(), campaignCode, planID, customerID)
	if err != nil {
		mapInitiationError(w, err)
		return
	}
	if item.PlanID != planID || item.CustomerID != customerID {
		writeHTTPError(w, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	writeJSON(w, stdhttp.StatusOK, reviewRecipientDetailResponse{CanonicalCustomerID: item.CustomerID, ReviewSafety: localReviewSafety()})
}
func (h *ReviewHandoffRouteFragment) getReview(w stdhttp.ResponseWriter, r *stdhttp.Request, campaignCode, planID string) {
	if !emptyBody(r) || r.URL.RawQuery != "" {
		writeHTTPError(w, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	if _, ok := h.authorize(w, r, CapabilityOperationsRead); !ok {
		return
	}
	result, err := h.application.GetReview(r.Context(), campaignCode, planID)
	if err != nil {
		mapInitiationError(w, err)
		return
	}
	if !ValidTouchPlanReview(result.Review) || result.Review.PlanID != planID || result.Review.CampaignCode != campaignCode {
		writeHTTPError(w, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	response := reviewMutationResponse{Review: reviewProjection(result.Review), ReviewSafety: localReviewSafety()}
	if result.Handoff != nil {
		if !ValidTouchPlanHandoff(*result.Handoff) || result.Handoff.PlanID != planID || result.Handoff.CampaignCode != campaignCode {
			writeHTTPError(w, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
			return
		}
		response.Handoff = &reviewHandoffResponse{Status: result.Handoff.Status, ReviewVersion: result.Handoff.ReviewVersion, CreatedAt: result.Handoff.CreatedAt.UTC().Format(time.RFC3339Nano)}
	}
	writeJSON(w, stdhttp.StatusOK, response)
}
func (h *ReviewHandoffRouteFragment) review(w stdhttp.ResponseWriter, r *stdhttp.Request, campaignCode, planID, operation string) {
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
	var body reviewMutationRequest
	if !decodeJSON(r, &body) || body.ExpectedVersion < 1 {
		writeHTTPError(w, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	if operation == "submit" {
		if body.Confirmation != "" {
			writeHTTPError(w, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
			return
		}
		review, err := h.application.Submit(r.Context(), SubmitTouchPlanReviewCommand{CampaignCode: campaignCode, PlanID: planID, ExpectedVersion: body.ExpectedVersion, Actor: actor, IdempotencyKey: keys[0]})
		if err != nil {
			mapInitiationError(w, err)
			return
		}
		if !ValidTouchPlanReview(review) || review.PlanID != planID || review.CampaignCode != campaignCode {
			writeHTTPError(w, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
			return
		}
		writeJSON(w, stdhttp.StatusOK, reviewMutationResponse{Review: reviewProjection(review), ReviewSafety: localReviewSafety()})
		return
	}
	if body.Confirmation != ReviewConfirmation(operation, planID) {
		writeHTTPError(w, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	command := DecideTouchPlanReviewCommand{CampaignCode: campaignCode, PlanID: planID, ExpectedVersion: body.ExpectedVersion, Actor: actor, IdempotencyKey: keys[0], Confirmation: body.Confirmation}
	var result TouchPlanReviewResult
	var err error
	if operation == "approve" {
		result, err = h.application.Approve(r.Context(), command)
	} else {
		result, err = h.application.Reject(r.Context(), command)
	}
	if err != nil {
		mapInitiationError(w, err)
		return
	}
	if !ValidTouchPlanReview(result.Review) || result.Review.PlanID != planID || result.Review.CampaignCode != campaignCode {
		writeHTTPError(w, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	response := reviewMutationResponse{Review: reviewProjection(result.Review), ReviewSafety: localReviewSafety()}
	if result.Handoff != nil {
		if !ValidTouchPlanHandoff(*result.Handoff) || result.Handoff.PlanID != planID || result.Handoff.CampaignCode != campaignCode {
			writeHTTPError(w, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
			return
		}
		response.Handoff = &reviewHandoffResponse{Status: result.Handoff.Status, ReviewVersion: result.Handoff.ReviewVersion, CreatedAt: result.Handoff.CreatedAt.UTC().Format(time.RFC3339Nano)}
	}
	writeJSON(w, stdhttp.StatusOK, response)
}
func parseRecipientQuery(raw, planID string) (*TouchPlanRecipientKeyset, int32, bool) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, 0, false
	}
	limit := int32(50)
	var cursor *TouchPlanRecipientKeyset
	for key, items := range values {
		if len(items) != 1 {
			return nil, 0, false
		}
		switch key {
		case "cursor":
			value, err := strconv.ParseInt(items[0], 10, 64)
			if err != nil || value < 1 {
				return nil, 0, false
			}
			cursor = &TouchPlanRecipientKeyset{PlanID: planID, CustomerID: value}
		case "limit":
			value, err := strconv.ParseInt(items[0], 10, 32)
			if err != nil || value < 1 || value > MaximumReviewRecipientPage {
				return nil, 0, false
			}
			limit = int32(value)
		default:
			return nil, 0, false
		}
	}
	return cursor, limit, true
}

type ReviewSafety struct {
	LocalOnly                 bool `json:"local_only"`
	ProviderExecutionEligible bool `json:"provider_execution_eligible"`
	RealExternalCallExecuted  bool `json:"real_external_call_executed"`
	DeliveryProven            bool `json:"delivery_proven"`
}

func localReviewSafety() ReviewSafety { return ReviewSafety{LocalOnly: true} }

type reviewRecipientResponse struct {
	CanonicalCustomerID int64 `json:"canonical_customer_id"`
}
type reviewRecipientListResponse struct {
	Items      []reviewRecipientResponse `json:"items"`
	NextCursor *string                   `json:"next_cursor,omitempty"`
	ReviewSafety
}
type reviewRecipientDetailResponse struct {
	CanonicalCustomerID int64 `json:"canonical_customer_id"`
	ReviewSafety
}
type reviewMutationRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	Confirmation    string `json:"confirmation,omitempty"`
}
type reviewResponse struct {
	Status             TouchPlanReviewStatus `json:"status"`
	Version            int64                 `json:"version"`
	SubmittedByActorID *int64                `json:"submitted_by_actor_id,omitempty"`
	SubmittedAt        *string               `json:"submitted_at,omitempty"`
	ReviewedByActorID  *int64                `json:"reviewed_by_actor_id,omitempty"`
	ReviewedAt         *string               `json:"reviewed_at,omitempty"`
}

func reviewProjection(value TouchPlanReview) reviewResponse {
	result := reviewResponse{Status: value.Status, Version: value.Version}
	if value.SubmittedByActorID > 0 {
		actor := value.SubmittedByActorID
		at := value.SubmittedAt.UTC().Format(time.RFC3339Nano)
		result.SubmittedByActorID = &actor
		result.SubmittedAt = &at
	}
	if value.ReviewedByActorID > 0 {
		actor := value.ReviewedByActorID
		at := value.ReviewedAt.UTC().Format(time.RFC3339Nano)
		result.ReviewedByActorID = &actor
		result.ReviewedAt = &at
	}
	return result
}

type reviewHandoffResponse struct {
	Status        string `json:"status"`
	ReviewVersion int64  `json:"review_version"`
	CreatedAt     string `json:"created_at"`
}
type reviewMutationResponse struct {
	Review  reviewResponse         `json:"review"`
	Handoff *reviewHandoffResponse `json:"handoff,omitempty"`
	ReviewSafety
}
