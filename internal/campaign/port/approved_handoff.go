package port

import (
	"context"
	"time"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
)

// ApprovedTouchPlanHandoffSnapshot is the only Campaign-owned snapshot that
// an Outbound root adapter may consume. It contains no mutable Campaign
// control-plane state and must be loaded with the caller's transaction-bound
// context while holding locks on the approved review, handoff, and plan.
type ApprovedTouchPlanHandoffSnapshot struct {
	CampaignCode     string
	PlanID           string
	ReviewVersion    int64
	SourceDigest     string
	TargetDigest     string
	ContentDigest    string
	CustomerIDs      []int64
	Steps            []campaign.Step
	ApprovedAt       time.Time
	HandoffCreatedAt time.Time
}

type ApprovedTouchPlanHandoffReader interface {
	LockApprovedTouchPlanHandoff(context.Context, string, string) (ApprovedTouchPlanHandoffSnapshot, error)
}
