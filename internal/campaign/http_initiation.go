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

// DraftTouchPlanApplication is deliberately restricted to immutable draft
// creation and recipient-safe reads. Review mutations and recipient paging
// are reserved for the later contract.
type DraftTouchPlanApplication interface {
	CreateDraftTouchPlan(context.Context, CreateDraftTouchPlanCommand) (DraftTouchPlan, error)
	ListDraftTouchPlans(context.Context, string, string, int32) (DraftTouchPlanPage, error)
	GetDraftTouchPlan(context.Context, string, string) (DraftTouchPlan, error)
}

// InitiationRouteFragment is deliberately unregistered. The composition root
// owns the canonical cloud-orchestrator route registration and CSRF middleware.
type InitiationRouteFragment struct {
	application DraftTouchPlanApplication
	authorizer  Authorizer
}

func NewInitiationRouteFragment(application DraftTouchPlanApplication, authorizer Authorizer) (*InitiationRouteFragment, error) {
	if nilish(application) || nilish(authorizer) {
		return nil, ErrUnavailable
	}
	return &InitiationRouteFragment{application: application, authorizer: authorizer}, nil
}

func (handler *InitiationRouteFragment) Routes() []Route {
	return []Route{
		{Method: stdhttp.MethodGet, Pattern: RoutePrefix + "/{campaign_code}/touch-plans", Capability: CapabilityOperationsRead, RequiresCSRF: false},
		{Method: stdhttp.MethodPost, Pattern: RoutePrefix + "/{campaign_code}/touch-plans", Capability: CapabilityManageAutomation, RequiresCSRF: true},
		{Method: stdhttp.MethodGet, Pattern: RoutePrefix + "/{campaign_code}/touch-plans/{plan_id}", Capability: CapabilityOperationsRead, RequiresCSRF: false},
	}
}

func (handler *InitiationRouteFragment) ServeHTTP(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	if handler == nil || nilish(handler.application) || nilish(handler.authorizer) || request == nil || request.URL == nil {
		writeHTTPError(writer, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	if request.URL.EscapedPath() != request.URL.Path || strings.Contains(request.URL.Path, "\\") {
		writeHTTPError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	tail := strings.TrimPrefix(request.URL.Path, RoutePrefix+"/")
	parts := strings.Split(tail, "/")
	if len(parts) < 2 || len(parts) > 3 || !validCode(parts[0]) || parts[1] != "touch-plans" {
		writeHTTPError(writer, stdhttp.StatusNotFound, "NOT_FOUND")
		return
	}
	if len(parts) == 2 {
		switch request.Method {
		case stdhttp.MethodGet:
			handler.list(writer, request, parts[0])
		case stdhttp.MethodPost:
			handler.create(writer, request, parts[0])
		default:
			methodNotAllowed(writer, "GET, POST")
		}
		return
	}
	if request.Method != stdhttp.MethodGet {
		methodNotAllowed(writer, "GET")
		return
	}
	if !ValidDraftTouchPlanID(parts[2]) {
		writeHTTPError(writer, stdhttp.StatusNotFound, "NOT_FOUND")
		return
	}
	handler.detail(writer, request, parts[0], parts[2])
}

func (handler *InitiationRouteFragment) list(writer stdhttp.ResponseWriter, request *stdhttp.Request, campaignCode string) {
	if !emptyBody(request) {
		writeHTTPError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	if _, ok := handler.authorize(writer, request, CapabilityOperationsRead, false); !ok {
		return
	}
	cursor, limit, valid := parseDraftTouchPlanListInput(request.URL.RawQuery)
	if !valid {
		writeHTTPError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	page, err := handler.application.ListDraftTouchPlans(request.Context(), campaignCode, cursor, limit)
	if err != nil {
		mapInitiationError(writer, err)
		return
	}
	if len(page.Items) > MaximumDraftTouchPlanPageLimit || page.NextCursor != "" && len(page.NextCursor) > 512 {
		writeHTTPError(writer, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	items := make([]draftTouchPlanSummaryResponse, len(page.Items))
	for index, plan := range page.Items {
		if !ValidDraftTouchPlanSummary(plan) || plan.CampaignCode != campaignCode {
			writeHTTPError(writer, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
			return
		}
		items[index] = draftTouchPlanSummaryProjection(plan)
	}
	writeJSON(writer, stdhttp.StatusOK, draftTouchPlanListResponse{Items: items, NextCursor: optionalCursor(page.NextCursor), InitiationSafety: LocalInitiationSafety()})
}

func (handler *InitiationRouteFragment) create(writer stdhttp.ResponseWriter, request *stdhttp.Request, campaignCode string) {
	if request.URL.RawQuery != "" {
		writeHTTPError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	actor, key, ok := handler.writeHeader(writer, request)
	if !ok {
		return
	}
	var body draftTouchPlanCreateRequest
	if !decodeJSON(request, &body) || body.ExpectedCampaignVersion < 1 {
		writeHTTPError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	plan, err := handler.application.CreateDraftTouchPlan(request.Context(), CreateDraftTouchPlanCommand{
		CampaignCode: campaignCode, ExpectedCampaignVersion: body.ExpectedCampaignVersion, Source: body.Source,
		Owner: actor, IdempotencyKey: key,
	})
	if err != nil {
		mapInitiationError(writer, err)
		return
	}
	if !ValidDraftTouchPlan(plan) || plan.CampaignCode != campaignCode {
		writeHTTPError(writer, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	writeJSON(writer, stdhttp.StatusCreated, draftTouchPlanDetailProjection(plan))
}

func (handler *InitiationRouteFragment) detail(writer stdhttp.ResponseWriter, request *stdhttp.Request, campaignCode, planID string) {
	if !emptyBody(request) || request.URL.RawQuery != "" {
		writeHTTPError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	if _, ok := handler.authorize(writer, request, CapabilityOperationsRead, false); !ok {
		return
	}
	plan, err := handler.application.GetDraftTouchPlan(request.Context(), campaignCode, planID)
	if err != nil {
		mapInitiationError(writer, err)
		return
	}
	if !ValidDraftTouchPlan(plan) || plan.CampaignCode != campaignCode || plan.ID != planID {
		writeHTTPError(writer, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	writeJSON(writer, stdhttp.StatusOK, draftTouchPlanDetailProjection(plan))
}

func (handler *InitiationRouteFragment) writeHeader(writer stdhttp.ResponseWriter, request *stdhttp.Request) (Actor, string, bool) {
	actor, ok := handler.authorize(writer, request, CapabilityManageAutomation, true)
	if !ok {
		return Actor{}, "", false
	}
	keys := request.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !validKey(keys[0]) {
		writeHTTPError(writer, stdhttp.StatusBadRequest, "MALFORMED_REQUEST")
		return Actor{}, "", false
	}
	return actor, keys[0], true
}

func (handler *InitiationRouteFragment) authorize(writer stdhttp.ResponseWriter, request *stdhttp.Request, capability string, csrf bool) (Actor, bool) {
	actor, err := handler.authorizer.Authorize(request, AccessRequirement{Capability: capability, RequireCSRF: csrf})
	if errors.Is(err, ErrUnauthenticated) {
		writeHTTPError(writer, stdhttp.StatusUnauthorized, "UNAUTHENTICATED")
		return Actor{}, false
	}
	if err != nil || actor.ID < 1 {
		writeHTTPError(writer, stdhttp.StatusForbidden, "FORBIDDEN")
		return Actor{}, false
	}
	return actor, true
}

type draftTouchPlanCreateRequest struct {
	ExpectedCampaignVersion int64                   `json:"expected_campaign_version"`
	Source                  InitiationSourceRequest `json:"source"`
}

type draftTouchPlanListResponse struct {
	Items      []draftTouchPlanSummaryResponse `json:"items"`
	NextCursor *string                         `json:"next_cursor,omitempty"`
	InitiationSafety
}

type draftTouchPlanSummaryResponse struct {
	ID               string                  `json:"id"`
	CampaignCode     string                  `json:"campaign_code"`
	CampaignVersion  int64                   `json:"campaign_version"`
	Source           InitiationSourceRef     `json:"source"`
	TargetCount      int32                   `json:"target_count"`
	TargetDigest     string                  `json:"target_digest"`
	ContentStepCount int32                   `json:"content_step_count"`
	ContentDigest    string                  `json:"content_digest"`
	OwnerActorID     int64                   `json:"owner_actor_id"`
	Exclusions       PreviewExclusionSummary `json:"preview_exclusion_summary"`
	CreatedAt        string                  `json:"created_at"`
	InitiationSafety
}

type draftTouchPlanDetailResponse struct {
	ID              string                  `json:"id"`
	CampaignCode    string                  `json:"campaign_code"`
	CampaignVersion int64                   `json:"campaign_version"`
	Source          InitiationSourceRef     `json:"source"`
	TargetCount     int32                   `json:"target_count"`
	TargetDigest    string                  `json:"target_digest"`
	Content         ContentSnapshot         `json:"content"`
	OwnerActorID    int64                   `json:"owner_actor_id"`
	Exclusions      PreviewExclusionSummary `json:"preview_exclusion_summary"`
	CreatedAt       string                  `json:"created_at"`
	InitiationSafety
}

func draftTouchPlanSummaryProjection(plan DraftTouchPlanSummary) draftTouchPlanSummaryResponse {
	return draftTouchPlanSummaryResponse{
		ID: plan.ID, CampaignCode: plan.CampaignCode, CampaignVersion: plan.CampaignVersion,
		Source: CanonicalInitiationSourceRef(plan.Source), TargetCount: plan.TargetCount, TargetDigest: plan.TargetDigest,
		ContentStepCount: plan.ContentStepCount, ContentDigest: plan.ContentDigest, OwnerActorID: plan.OwnerActorID,
		Exclusions: plan.Exclusions, CreatedAt: plan.CreatedAt.UTC().Format(time.RFC3339Nano),
		InitiationSafety: plan.Safety,
	}
}

func draftTouchPlanDetailProjection(plan DraftTouchPlan) draftTouchPlanDetailResponse {
	return draftTouchPlanDetailResponse{
		ID: plan.ID, CampaignCode: plan.CampaignCode, CampaignVersion: plan.CampaignVersion,
		Source: CanonicalInitiationSourceRef(plan.Source), TargetCount: int32(len(plan.Targets.CustomerIDs)), TargetDigest: plan.Targets.Digest,
		Content: CanonicalContentSnapshot(plan.Content.Steps), OwnerActorID: plan.OwnerActorID, Exclusions: plan.Exclusions,
		CreatedAt: plan.CreatedAt.UTC().Format(time.RFC3339Nano), InitiationSafety: plan.Safety,
	}
}

func parseDraftTouchPlanListInput(raw string) (string, int32, bool) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return "", 0, false
	}
	var cursor string
	var limit int32
	for key, items := range values {
		if len(items) != 1 {
			return "", 0, false
		}
		switch key {
		case "cursor":
			if items[0] == "" || len(items[0]) > 512 {
				return "", 0, false
			}
			cursor = items[0]
		case "limit":
			parsed, parseErr := strconv.ParseInt(items[0], 10, 32)
			if parseErr != nil || parsed < 1 || parsed > 100 {
				return "", 0, false
			}
			limit = int32(parsed)
		default:
			return "", 0, false
		}
	}
	return cursor, limit, true
}

func mapInitiationError(writer stdhttp.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidArgument):
		writeHTTPError(writer, stdhttp.StatusBadRequest, "INVALID_ARGUMENT")
	case errors.Is(err, ErrNotFound):
		writeHTTPError(writer, stdhttp.StatusNotFound, "NOT_FOUND")
	case errors.Is(err, ErrBlockedRedline):
		writeHTTPError(writer, stdhttp.StatusConflict, "BLOCKED_REDLINE")
	case errors.Is(err, ErrConflict), errors.Is(err, ErrStateConflict), errors.Is(err, ErrIdempotencyConflict):
		writeHTTPError(writer, stdhttp.StatusConflict, "CONFLICT")
	default:
		writeHTTPError(writer, stdhttp.StatusServiceUnavailable, "UNAVAILABLE")
	}
}

func optionalCursor(value string) *string {
	if value == "" {
		return nil
	}
	result := value
	return &result
}
