package outboundhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const MaximumCampaignHandoffRequestBytes int64 = 64 << 10

type CampaignHandoffApplication interface {
	Accept(context.Context, outboundapp.AcceptCampaignHandoffCommand) (outbound.CampaignHandoffSummary, error)
	Get(context.Context, string, string) (outbound.CampaignHandoffSummary, error)
}

type CampaignHandoffHandler struct{ application CampaignHandoffApplication }

func NewCampaignHandoffHandler(application CampaignHandoffApplication) (*CampaignHandoffHandler, error) {
	if application == nil {
		return nil, outbound.ErrCampaignHandoffUnavailable
	}
	return &CampaignHandoffHandler{application: application}, nil
}

func (handler *CampaignHandoffHandler) Accept(writer http.ResponseWriter, request *http.Request, campaignCode, planID string) {
	if handler == nil || handler.application == nil || request == nil || request.URL == nil {
		writeCampaignHandoffError(writer, request, outbound.ErrCampaignHandoffUnavailable)
		return
	}
	if request.URL.RawQuery != "" || !outbound.ValidCampaignHandoffIdentity(campaignCode, planID) {
		writeCampaignHandoffError(writer, request, outbound.ErrCampaignHandoffInvalid)
		return
	}
	actorID, err := campaignHandoffActor(request.Context(), authport.CapabilityOperationsManage)
	if err != nil {
		writeCampaignHandoffError(writer, request, err)
		return
	}
	keys := request.Header.Values("Idempotency-Key")
	if len(keys) != 1 || len(keys[0]) < 16 || len(keys[0]) > 128 || strings.TrimSpace(keys[0]) != keys[0] {
		writeCampaignHandoffError(writer, request, outbound.ErrCampaignHandoffInvalid)
		return
	}
	var body struct {
		ExpectedReviewVersion int64 `json:"expected_review_version"`
	}
	if !decodeCampaignHandoffJSON(writer, request, &body) || body.ExpectedReviewVersion < 3 {
		writeCampaignHandoffError(writer, request, outbound.ErrCampaignHandoffInvalid)
		return
	}
	result, err := handler.application.Accept(request.Context(), outboundapp.AcceptCampaignHandoffCommand{
		CampaignCode: campaignCode, PlanID: planID, ExpectedReviewVersion: body.ExpectedReviewVersion,
		ActorID: actorID, IdempotencyKey: keys[0],
	})
	if err != nil {
		writeCampaignHandoffError(writer, request, err)
		return
	}
	response, valid := campaignHandoffReconciliationResponseOf(result, campaignCode, planID)
	if !valid {
		writeCampaignHandoffError(writer, request, outbound.ErrCampaignHandoffUnavailable)
		return
	}
	writeCampaignHandoffJSON(writer, http.StatusOK, response)
}

func (handler *CampaignHandoffHandler) Summary(writer http.ResponseWriter, request *http.Request, campaignCode, planID string) {
	result, ok := handler.read(writer, request, campaignCode, planID)
	if !ok {
		return
	}
	response, valid := campaignHandoffSummaryResponseOf(result, campaignCode, planID)
	if !valid {
		writeCampaignHandoffError(writer, request, outbound.ErrCampaignHandoffUnavailable)
		return
	}
	writeCampaignHandoffJSON(writer, http.StatusOK, response)
}

func (handler *CampaignHandoffHandler) Reconciliation(writer http.ResponseWriter, request *http.Request, campaignCode, planID string) {
	result, ok := handler.read(writer, request, campaignCode, planID)
	if !ok {
		return
	}
	response, valid := campaignHandoffReconciliationResponseOf(result, campaignCode, planID)
	if !valid {
		writeCampaignHandoffError(writer, request, outbound.ErrCampaignHandoffUnavailable)
		return
	}
	writeCampaignHandoffJSON(writer, http.StatusOK, response)
}

func (handler *CampaignHandoffHandler) read(writer http.ResponseWriter, request *http.Request, campaignCode, planID string) (outbound.CampaignHandoffSummary, bool) {
	if handler == nil || handler.application == nil || request == nil || request.URL == nil {
		writeCampaignHandoffError(writer, request, outbound.ErrCampaignHandoffUnavailable)
		return outbound.CampaignHandoffSummary{}, false
	}
	if request.URL.RawQuery != "" || request.Body != nil && request.Body != http.NoBody || !outbound.ValidCampaignHandoffIdentity(campaignCode, planID) {
		writeCampaignHandoffError(writer, request, outbound.ErrCampaignHandoffInvalid)
		return outbound.CampaignHandoffSummary{}, false
	}
	if _, err := campaignHandoffActor(request.Context(), authport.CapabilityOperationsRead); err != nil {
		writeCampaignHandoffError(writer, request, err)
		return outbound.CampaignHandoffSummary{}, false
	}
	result, err := handler.application.Get(request.Context(), campaignCode, planID)
	if err != nil {
		writeCampaignHandoffError(writer, request, err)
		return outbound.CampaignHandoffSummary{}, false
	}
	return result, true
}

func campaignHandoffActor(ctx context.Context, expected authport.Capability) (int64, error) {
	principal, principalOK := authport.PrincipalFromContext(ctx)
	authorization, authorizationOK := authport.AuthorizationFromContext(ctx)
	if !principalOK || principal.AdminUserID < 1 {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	if !authorizationOK || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) ||
		authorization.Capability != expected || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return 0, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	return principal.AdminUserID, nil
}

func decodeCampaignHandoffJSON(writer http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, MaximumCampaignHandoffRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return false
	}
	return decoder.Decode(&struct{}{}) == io.EOF
}

type campaignHandoffSafetyResponse struct {
	LocalOnly                 bool `json:"local_only"`
	ProviderExecutionEligible bool `json:"provider_execution_eligible"`
	RealExternalCallExecuted  bool `json:"real_external_call_executed"`
	DeliveryProven            bool `json:"delivery_proven"`
}

type campaignHandoffSummaryResponse struct {
	ID            int64                         `json:"id"`
	CampaignCode  string                        `json:"campaign_code"`
	PlanID        string                        `json:"plan_id"`
	ReviewVersion int64                         `json:"review_version"`
	Status        string                        `json:"status"`
	TargetCount   int32                         `json:"target_count"`
	StepCount     int32                         `json:"step_count"`
	AcceptedAt    string                        `json:"accepted_at"`
	Safety        campaignHandoffSafetyResponse `json:"safety"`
}

type campaignHandoffReconciliationResponse struct {
	campaignHandoffSummaryResponse
	HeldCount          int32 `json:"held_count"`
	BlockedCount       int32 `json:"blocked_count"`
	PendingCount       int32 `json:"pending_count"`
	NotEvaluatedCount  int32 `json:"not_evaluated_count"`
	EligibleCount      int32 `json:"eligible_count"`
	InactiveCount      int32 `json:"inactive_count"`
	ContactPolicyCount int32 `json:"contact_policy_count"`
}

func campaignHandoffSummaryResponseOf(value outbound.CampaignHandoffSummary, campaignCode, planID string) (campaignHandoffSummaryResponse, bool) {
	if !outbound.ValidCampaignHandoffSummary(value) || value.CampaignCode != campaignCode || value.PlanID != planID {
		return campaignHandoffSummaryResponse{}, false
	}
	return campaignHandoffSummaryResponse{
		ID: value.ID, CampaignCode: value.CampaignCode, PlanID: value.PlanID, ReviewVersion: value.ReviewVersion,
		Status: value.Status, TargetCount: value.TargetCount, StepCount: value.StepCount,
		AcceptedAt: value.AcceptedAt.UTC().Format(time.RFC3339Nano),
		Safety:     campaignHandoffSafetyResponse{LocalOnly: true},
	}, true
}

func campaignHandoffReconciliationResponseOf(value outbound.CampaignHandoffSummary, campaignCode, planID string) (campaignHandoffReconciliationResponse, bool) {
	header, valid := campaignHandoffSummaryResponseOf(value, campaignCode, planID)
	if !valid {
		return campaignHandoffReconciliationResponse{}, false
	}
	return campaignHandoffReconciliationResponse{
		campaignHandoffSummaryResponse: header,
		HeldCount:                      value.HeldCount, BlockedCount: value.BlockedCount, PendingCount: value.PendingCount,
		NotEvaluatedCount: value.NotEvaluatedCount, EligibleCount: value.EligibleCount,
		InactiveCount: value.InactiveCount, ContactPolicyCount: value.ContactPolicyCount,
	}, true
}

func writeCampaignHandoffJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeCampaignHandoffError(writer http.ResponseWriter, request *http.Request, err error) {
	var platformError *platformhttp.HTTPError
	if errors.As(err, &platformError) {
		platformhttp.WriteError(writer, request, platformError)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, outbound.ErrCampaignHandoffInvalid):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, outbound.ErrCampaignHandoffNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, outbound.ErrCampaignHandoffConflict), errors.Is(err, outbound.ErrCampaignHandoffIdempotencyConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}
