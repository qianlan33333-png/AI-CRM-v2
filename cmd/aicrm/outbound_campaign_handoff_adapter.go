package main

import (
	"context"
	"errors"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

// outboundCampaignHandoffSourceAdapter is the only Campaign-to-Outbound DTO
// mapping point. The Campaign repository uses the Outbound service's existing
// transaction context; this adapter never opens a nested UoW.
type outboundCampaignHandoffSourceAdapter struct {
	reader campaignport.ApprovedTouchPlanHandoffReader
}

func newOutboundCampaignHandoffSourceAdapter(reader campaignport.ApprovedTouchPlanHandoffReader) (*outboundCampaignHandoffSourceAdapter, error) {
	if reader == nil {
		return nil, outbound.ErrCampaignHandoffUnavailable
	}
	return &outboundCampaignHandoffSourceAdapter{reader: reader}, nil
}

func (adapter *outboundCampaignHandoffSourceAdapter) LockApprovedCampaignHandoff(ctx context.Context, campaignCode, planID string) (outboundport.ApprovedCampaignHandoffSnapshot, error) {
	if adapter == nil || adapter.reader == nil {
		return outboundport.ApprovedCampaignHandoffSnapshot{}, outbound.ErrCampaignHandoffUnavailable
	}
	value, err := adapter.reader.LockApprovedTouchPlanHandoff(ctx, campaignCode, planID)
	if err != nil {
		switch {
		case errors.Is(err, campaign.ErrNotFound):
			return outboundport.ApprovedCampaignHandoffSnapshot{}, outbound.ErrCampaignHandoffNotFound
		case errors.Is(err, campaign.ErrInvalidArgument):
			return outboundport.ApprovedCampaignHandoffSnapshot{}, outbound.ErrCampaignHandoffInvalid
		default:
			return outboundport.ApprovedCampaignHandoffSnapshot{}, outbound.ErrCampaignHandoffUnavailable
		}
	}
	steps := make([]outbound.CampaignHandoffStep, len(value.Steps))
	for index, step := range value.Steps {
		steps[index] = outbound.CampaignHandoffStep{Index: step.Index, DelayMinutes: step.DelayMinutes, Content: step.Content}
	}
	return outboundport.ApprovedCampaignHandoffSnapshot{
		CampaignCode: value.CampaignCode, PlanID: value.PlanID, ReviewVersion: value.ReviewVersion,
		SourceDigest: value.SourceDigest, TargetDigest: value.TargetDigest, ContentDigest: value.ContentDigest,
		CustomerIDs: append([]int64(nil), value.CustomerIDs...), Steps: steps, ApprovedAt: value.ApprovedAt,
	}, nil
}
