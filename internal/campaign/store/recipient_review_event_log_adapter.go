package store

import (
	"context"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

// RecipientReviewEventLogAdapter persists only the local Campaign audit fact.
// The existing Campaign fact consumer does not enqueue Provider delivery.
type RecipientReviewEventLogAdapter struct{ audits *campaign.EventLogAdapter }

var _ campaignport.RecipientReviewEventAppender = (*RecipientReviewEventLogAdapter)(nil)

func NewRecipientReviewEventLogAdapter(appender eventport.Appender) (*RecipientReviewEventLogAdapter, error) {
	audits, err := campaign.NewEventLogAdapter(appender)
	if err != nil {
		return nil, err
	}
	return &RecipientReviewEventLogAdapter{audits: audits}, nil
}

func (a *RecipientReviewEventLogAdapter) AppendTouchPlanRecipientReviewEvent(ctx context.Context, event campaign.TouchPlanRecipientReviewEvent) (int64, error) {
	if a == nil || a.audits == nil {
		return 0, campaign.ErrUnavailable
	}
	id, err := a.audits.AppendTouchPlanRecipientReview(ctx, campaign.TouchPlanRecipientReviewAuditEvent{
		AuditType: event.AuditType, PlanID: event.PlanID, CampaignCode: event.CampaignCode, CustomerID: event.CustomerID,
		RecipientVersion: event.RecipientVersion, ActorID: event.ActorID, OccurredAt: event.OccurredAt, IdempotencyKey: event.IdempotencyKey,
	})
	if err != nil || id < 1 {
		return 0, campaign.ErrUnavailable
	}
	return int64(id), nil
}
