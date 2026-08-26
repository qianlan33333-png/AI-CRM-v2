package port

import (
	"context"
	"crypto/sha256"
	"time"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
)

// RecipientReviewRepository is an app/domain seam pending the central
// recipient-review sidecar migration and sqlc adapter. It intentionally does
// not extend ReviewRepository, so current production storage cannot expose a
// partially persisted recipient decision.
type RecipientReviewRepository interface {
	ReserveTouchPlanRecipientReviewReceipt(context.Context, RecipientReviewReceiptReservation) (campaign.TouchPlanRecipientReviewReceipt, bool, error)
	LockTouchPlanReview(context.Context, string, string) (campaign.TouchPlanReview, error)
	GetTouchPlanRecipient(context.Context, string, string, int64) (campaign.TouchPlanRecipient, error)
	LockTouchPlanRecipientReview(context.Context, string, string, int64) (campaign.TouchPlanRecipientReview, bool, error)
	SaveTouchPlanRecipientReview(context.Context, campaign.TouchPlanRecipientReview, int64) error
	ReadTouchPlanRecipientReview(context.Context, string, string, int64) (campaign.TouchPlanRecipientReview, error)
	CompleteTouchPlanRecipientReviewReceipt(context.Context, int64, campaign.TouchPlanRecipientReviewResult, time.Time) error
}

type RecipientReviewReceiptReservation struct {
	ActorID       int64
	Operation     string
	KeyDigest     [sha256.Size]byte
	PayloadDigest [sha256.Size]byte
	PlanID        string
	CampaignCode  string
	CustomerID    int64
	CreatedAt     time.Time
}

// RecipientReviewEventAppender records only a local Campaign fact in the
// same UnitOfWork. It must not create an Outbound task or call a Provider.
type RecipientReviewEventAppender interface {
	AppendTouchPlanRecipientReviewEvent(context.Context, campaign.TouchPlanRecipientReviewEvent) (int64, error)
}
