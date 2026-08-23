package store

import (
	"context"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

// InitiationEventLogAdapter adapts the immutable initiation command to the
// existing bound Campaign EventLog fact. After commit Events may create one
// local audit delivery job, and Campaign's FactDeliveryConsumer only completes
// it; this path does not create Outbound work or invoke a provider.
type InitiationEventLogAdapter struct{ audits *campaign.EventLogAdapter }

var _ campaignport.EventAppender = (*InitiationEventLogAdapter)(nil)

func NewInitiationEventLogAdapter(appender eventport.Appender) (*InitiationEventLogAdapter, error) {
	if appender == nil {
		return nil, campaign.ErrUnavailable
	}
	audits, err := campaign.NewEventLogAdapter(appender)
	if err != nil {
		return nil, err
	}
	return &InitiationEventLogAdapter{audits: audits}, nil
}

func (adapter *InitiationEventLogAdapter) AppendCampaignEvent(ctx context.Context, event campaignport.CampaignEvent) (int64, error) {
	if adapter == nil || adapter.audits == nil || event.Type != "campaign.draft_touch_plan.created" ||
		!campaign.ValidDraftTouchPlanID(event.PlanID) || !campaign.ValidCampaignCode(event.CampaignCode) || event.OwnerActorID < 1 ||
		event.TargetCount < 1 || event.OccurredAt.IsZero() {
		return 0, campaign.ErrUnavailable
	}
	eventID, err := adapter.audits.AppendTouchPlanCreated(ctx, campaign.TouchPlanCreatedAuditEvent{
		PlanID: event.PlanID, CampaignCode: event.CampaignCode, OwnerActorID: event.OwnerActorID,
		TargetDigest: event.TargetDigest, TargetCount: event.TargetCount, ContentDigest: event.ContentDigest,
		OccurredAt: event.OccurredAt, IdempotencyKey: event.IdempotencyKey,
	})
	if err != nil || eventID < 1 {
		return 0, campaign.ErrUnavailable
	}
	return int64(eventID), nil
}
