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

type CampaignDispatchApplication interface {
	Dispatch(context.Context, outboundapp.CampaignDispatchCommand) (outbound.CampaignDispatchSummary, error)
	Reconciliation(context.Context, string, string) (outbound.CampaignDispatchSummary, error)
	ManualReconcile(context.Context, outboundapp.CampaignDispatchReconcileCommand) (outbound.CampaignDispatchSummary, error)
}

func (handler *CampaignDispatchHandler) Reconciliation(writer http.ResponseWriter, request *http.Request, campaignCode, planID string) {
	if handler == nil || handler.application == nil || request == nil || request.URL == nil || request.URL.RawQuery != "" || !outbound.ValidCampaignHandoffIdentity(campaignCode, planID) {
		writeCampaignDispatchError(writer, request, outbound.ErrCampaignDispatchInvalid)
		return
	}
	if _, err := campaignHandoffActor(request.Context(), authport.CapabilityOperationsRead); err != nil {
		writeCampaignDispatchError(writer, request, err)
		return
	}
	summary, err := handler.application.Reconciliation(request.Context(), campaignCode, planID)
	if err != nil {
		writeCampaignDispatchError(writer, request, err)
		return
	}
	if !outbound.ValidCampaignDispatchSummary(summary) {
		writeCampaignDispatchError(writer, request, outbound.ErrCampaignDispatchUnavailable)
		return
	}
	writeCampaignHandoffJSON(writer, http.StatusOK, campaignDispatchResponseOf(summary))
}

func (handler *CampaignDispatchHandler) ManualReconcile(writer http.ResponseWriter, request *http.Request, campaignCode, planID, effectID string) {
	if handler == nil || handler.application == nil || request == nil || request.URL == nil || request.URL.RawQuery != "" || !outbound.ValidCampaignHandoffIdentity(campaignCode, planID) || effectID == "" {
		writeCampaignDispatchError(writer, request, outbound.ErrCampaignDispatchInvalid)
		return
	}
	actorID, err := campaignHandoffActor(request.Context(), authport.CapabilityOperationsManage)
	if err != nil {
		writeCampaignDispatchError(writer, request, err)
		return
	}
	keys := request.Header.Values("Idempotency-Key")
	if len(keys) != 1 || len(keys[0]) < 16 || len(keys[0]) > 128 || strings.TrimSpace(keys[0]) != keys[0] {
		writeCampaignDispatchError(writer, request, outbound.ErrCampaignDispatchInvalid)
		return
	}
	var body struct {
		Generation     int64  `json:"generation"`
		Fence          int64  `json:"fence"`
		LeaseExpiresAt string `json:"lease_expires_at"`
		EvidenceDigest string `json:"evidence_digest"`
	}
	request.Body = http.MaxBytesReader(writer, request.Body, MaximumCampaignHandoffRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeCampaignDispatchError(writer, request, outbound.ErrCampaignDispatchInvalid)
		return
	}
	expiresAt, parseErr := time.Parse(time.RFC3339Nano, body.LeaseExpiresAt)
	if parseErr != nil {
		writeCampaignDispatchError(writer, request, outbound.ErrCampaignDispatchInvalid)
		return
	}
	summary, err := handler.application.ManualReconcile(request.Context(), outboundapp.CampaignDispatchReconcileCommand{CampaignCode: campaignCode, PlanID: planID, EffectID: effectID, ActorID: actorID, IdempotencyKey: keys[0], Generation: body.Generation, Fence: body.Fence, LeaseExpiresAt: expiresAt, EvidenceDigest: body.EvidenceDigest})
	if err != nil {
		writeCampaignDispatchError(writer, request, err)
		return
	}
	if !outbound.ValidCampaignDispatchSummary(summary) {
		writeCampaignDispatchError(writer, request, outbound.ErrCampaignDispatchUnavailable)
		return
	}
	writeCampaignHandoffJSON(writer, http.StatusOK, campaignDispatchResponseOf(summary))
}

type CampaignDispatchHandler struct{ application CampaignDispatchApplication }

func NewCampaignDispatchHandler(application CampaignDispatchApplication) (*CampaignDispatchHandler, error) {
	if application == nil {
		return nil, outbound.ErrCampaignDispatchUnavailable
	}
	return &CampaignDispatchHandler{application: application}, nil
}

func (handler *CampaignDispatchHandler) Dispatch(writer http.ResponseWriter, request *http.Request, campaignCode, planID string) {
	if handler == nil || handler.application == nil || request == nil || request.URL == nil || request.URL.RawQuery != "" || !outbound.ValidCampaignHandoffIdentity(campaignCode, planID) {
		writeCampaignDispatchError(writer, request, outbound.ErrCampaignDispatchInvalid)
		return
	}
	actorID, err := campaignHandoffActor(request.Context(), authport.CapabilityOperationsManage)
	if err != nil {
		writeCampaignDispatchError(writer, request, err)
		return
	}
	keys := request.Header.Values("Idempotency-Key")
	if len(keys) != 1 || len(keys[0]) < 16 || len(keys[0]) > 128 || strings.TrimSpace(keys[0]) != keys[0] {
		writeCampaignDispatchError(writer, request, outbound.ErrCampaignDispatchInvalid)
		return
	}
	var body struct {
		ExternalGate bool `json:"external_gate"`
	}
	request.Body = http.MaxBytesReader(writer, request.Body, MaximumCampaignHandoffRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeCampaignDispatchError(writer, request, outbound.ErrCampaignDispatchInvalid)
		return
	}
	summary, err := handler.application.Dispatch(request.Context(), outboundapp.CampaignDispatchCommand{CampaignCode: campaignCode, PlanID: planID, ActorID: actorID, IdempotencyKey: keys[0], ExternalGate: body.ExternalGate})
	if err != nil {
		writeCampaignDispatchError(writer, request, err)
		return
	}
	if !outbound.ValidCampaignDispatchSummary(summary) {
		writeCampaignDispatchError(writer, request, outbound.ErrCampaignDispatchUnavailable)
		return
	}
	writeCampaignHandoffJSON(writer, http.StatusOK, campaignDispatchResponseOf(summary))
}

type campaignDispatchResponse struct {
	HandoffID                 int64 `json:"handoff_id"`
	Blocked                   int64 `json:"blocked"`
	Accepted                  int64 `json:"accepted"`
	Queued                    int64 `json:"queued"`
	Attempted                 int64 `json:"attempted"`
	Executed                  int64 `json:"executed"`
	OutcomeUnknown            int64 `json:"outcome_unknown"`
	Reconciled                int64 `json:"reconciled"`
	RetryableFailed           int64 `json:"retryable_failed"`
	FinalFailed               int64 `json:"final_failed"`
	ProviderExecutionEligible bool  `json:"provider_execution_eligible"`
	RealExternalCallExecuted  bool  `json:"real_external_call_executed"`
	DeliveryProven            bool  `json:"delivery_proven"`
}

func campaignDispatchResponseOf(value outbound.CampaignDispatchSummary) campaignDispatchResponse {
	return campaignDispatchResponse{HandoffID: value.HandoffID, Blocked: value.Blocked, Accepted: value.Accepted, Queued: value.Queued, Attempted: value.Attempted, Executed: value.Executed, OutcomeUnknown: value.OutcomeUnknown, Reconciled: value.Reconciled, RetryableFailed: value.RetryableFailed, FinalFailed: value.FinalFailed}
}

func writeCampaignDispatchError(writer http.ResponseWriter, request *http.Request, err error) {
	var platformError *platformhttp.HTTPError
	if errors.As(err, &platformError) {
		platformhttp.WriteError(writer, request, platformError)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, outbound.ErrCampaignDispatchInvalid):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, outbound.ErrCampaignHandoffNotFound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, outbound.ErrCampaignDispatchConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}
