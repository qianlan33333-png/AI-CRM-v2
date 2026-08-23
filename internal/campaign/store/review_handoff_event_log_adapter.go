package store

import (
	"context"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

// ReviewHandoffEventLogAdapter writes only the established local Campaign
// fact event. Post-commit delivery never creates an Outbound send job.
type ReviewHandoffEventLogAdapter struct{ audits *campaign.EventLogAdapter }

var _ campaignport.ReviewEventAppender = (*ReviewHandoffEventLogAdapter)(nil)

func NewReviewHandoffEventLogAdapter(appender eventport.Appender) (*ReviewHandoffEventLogAdapter, error) {
	audits, err := campaign.NewEventLogAdapter(appender)
	if err != nil {
		return nil, err
	}
	return &ReviewHandoffEventLogAdapter{audits: audits}, nil
}
func (a *ReviewHandoffEventLogAdapter) AppendTouchPlanReviewEvent(ctx context.Context, event campaign.TouchPlanReviewEvent) (int64, error) {
	if a == nil || a.audits == nil {
		return 0, campaign.ErrUnavailable
	}
	id, err := a.audits.AppendTouchPlanReview(ctx, campaign.TouchPlanReviewAuditEvent{AuditType: event.AuditType, PlanID: event.PlanID, CampaignCode: event.CampaignCode, ReviewVersion: event.ReviewVersion, ActorID: event.ActorID, OccurredAt: event.OccurredAt, IdempotencyKey: event.IdempotencyKey})
	if err != nil || id < 1 {
		return 0, campaign.ErrUnavailable
	}
	return int64(id), nil
}
