package store

import (
	"context"
	"encoding/json"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

type CampaignHandoffEventLogAdapter struct{ appender eventport.Appender }

var _ outboundport.CampaignHandoffEventAppender = (*CampaignHandoffEventLogAdapter)(nil)

func NewCampaignHandoffEventLogAdapter(appender eventport.Appender) (*CampaignHandoffEventLogAdapter, error) {
	if appender == nil {
		return nil, outbound.ErrCampaignHandoffUnavailable
	}
	return &CampaignHandoffEventLogAdapter{appender: appender}, nil
}

func (adapter *CampaignHandoffEventLogAdapter) AppendCampaignHandoffFact(ctx context.Context, event outboundport.CampaignHandoffEvent) (int64, error) {
	if adapter == nil || adapter.appender == nil || event.HandoffID < 1 || !outbound.ValidCampaignHandoffIdentity(event.CampaignCode, event.PlanID) ||
		event.ReviewVersion < 3 || event.TargetCount < 1 || event.StepCount < 1 || event.ActorID < 1 || event.OccurredAt.IsZero() || event.IdempotencyKey == "" {
		return 0, outbound.ErrCampaignHandoffUnavailable
	}
	payload, err := json.Marshal(struct {
		AuditType            string `json:"audit_type"`
		HandoffID            int64  `json:"handoff_id"`
		CampaignCode         string `json:"campaign_code"`
		PlanID               string `json:"plan_id"`
		ReviewVersion        int64  `json:"review_version"`
		TargetDigest         string `json:"target_digest"`
		ContentDigest        string `json:"content_digest"`
		TargetCount          int32  `json:"target_count"`
		StepCount            int32  `json:"step_count"`
		ActorID              int64  `json:"actor_id"`
		LocalOnly            bool   `json:"local_only"`
		ProviderEligible     bool   `json:"provider_execution_eligible"`
		ExternalCallExecuted bool   `json:"real_external_call_executed"`
		DeliveryProven       bool   `json:"delivery_proven"`
	}{
		AuditType: "accepted", HandoffID: event.HandoffID, CampaignCode: event.CampaignCode, PlanID: event.PlanID,
		ReviewVersion: event.ReviewVersion, TargetDigest: event.TargetDigest, ContentDigest: event.ContentDigest,
		TargetCount: event.TargetCount, StepCount: event.StepCount, ActorID: event.ActorID, LocalOnly: true,
	})
	if err != nil {
		return 0, outbound.ErrCampaignHandoffUnavailable
	}
	id, err := adapter.appender.Append(ctx, eventport.Event{Type: eventport.EvOutboundCampaignHandoffFact, Payload: payload, OccurredAt: event.OccurredAt, IdempotencyKey: event.IdempotencyKey})
	if err != nil || id < 1 {
		return 0, outbound.ErrCampaignHandoffUnavailable
	}
	return int64(id), nil
}
