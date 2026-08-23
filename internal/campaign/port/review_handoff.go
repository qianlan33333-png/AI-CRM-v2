package port

import (
	"context"
	"crypto/sha256"
	"time"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
)

type ReviewReceiptReservation struct {
	ActorID       int64
	Operation     string
	KeyDigest     [sha256.Size]byte
	PayloadDigest [sha256.Size]byte
	PlanID        string
	CampaignCode  string
	CreatedAt     time.Time
}

// ReviewRepository is Campaign-owned. The implementation must lock receipt,
// immutable plan, review sidecar, then handoff, in that order.
type ReviewRepository interface {
	ReserveReviewReceipt(context.Context, ReviewReceiptReservation) (campaign.TouchPlanReviewReceipt, bool, error)
	LockTouchPlanReview(context.Context, string, string) (campaign.TouchPlanReview, error)
	SaveTouchPlanReview(context.Context, campaign.TouchPlanReview, int64) error
	ReadTouchPlanReview(context.Context, string, string) (campaign.TouchPlanReview, error)
	CreateTouchPlanHandoff(context.Context, campaign.TouchPlanHandoff) error
	ReadTouchPlanHandoff(context.Context, string, string) (campaign.TouchPlanHandoff, error)
	CompleteReviewReceipt(context.Context, int64, campaign.TouchPlanReviewResult, time.Time) error
	ListTouchPlanRecipients(context.Context, string, string, int64, int32) ([]campaign.TouchPlanRecipient, error)
	GetTouchPlanRecipient(context.Context, string, string, int64) (campaign.TouchPlanRecipient, error)
}

// ReviewEventAppender writes only the existing EvCloudCampaignFact binding.
// Its post-commit consumer completes an internal Events receipt; it does not
// create an Outbound task or invoke a Provider.
type ReviewEventAppender interface {
	AppendTouchPlanReviewEvent(context.Context, campaign.TouchPlanReviewEvent) (int64, error)
}
