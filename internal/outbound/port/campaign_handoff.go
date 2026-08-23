package port

import (
	"context"
	"crypto/sha256"
	"time"

	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
)

type ApprovedCampaignHandoffSnapshot struct {
	CampaignCode  string
	PlanID        string
	ReviewVersion int64
	SourceDigest  string
	TargetDigest  string
	ContentDigest string
	CustomerIDs   []int64
	Steps         []outbound.CampaignHandoffStep
	ApprovedAt    time.Time
}

// ApprovedCampaignHandoffSource must use the caller's transaction-bound
// context. Implementations must not open a nested UoW.
type ApprovedCampaignHandoffSource interface {
	LockApprovedCampaignHandoff(context.Context, string, string) (ApprovedCampaignHandoffSnapshot, error)
}

type CampaignHandoffReservation struct {
	ActorID       int64
	KeyDigest     [sha256.Size]byte
	PayloadDigest [sha256.Size]byte
	CampaignCode  string
	PlanID        string
	CreatedAt     time.Time
}

type CampaignHandoffReceipt struct {
	ID            int64
	ActorID       int64
	KeyDigest     [sha256.Size]byte
	PayloadDigest [sha256.Size]byte
	CampaignCode  string
	PlanID        string
	State         string
	Result        *outbound.CampaignHandoffSummary
}

type CampaignHandoffRepository interface {
	ReserveCampaignHandoff(context.Context, CampaignHandoffReservation) (CampaignHandoffReceipt, bool, error)
	CreateAcceptedCampaignHandoff(context.Context, ApprovedCampaignHandoffSnapshot, int64, time.Time) (int64, error)
	ReadAcceptedCampaignHandoff(context.Context, string, string) (outbound.AcceptedCampaignHandoff, error)
	ReadCampaignHandoffSummary(context.Context, string, string) (outbound.CampaignHandoffSummary, error)
	CompleteCampaignHandoffReceipt(context.Context, int64, int64, outbound.CampaignHandoffSummary, time.Time) error
}

type CampaignHandoffEvent struct {
	HandoffID      int64
	CampaignCode   string
	PlanID         string
	ReviewVersion  int64
	TargetDigest   string
	ContentDigest  string
	TargetCount    int32
	StepCount      int32
	ActorID        int64
	OccurredAt     time.Time
	IdempotencyKey string
}

type CampaignHandoffEventAppender interface {
	AppendCampaignHandoffFact(context.Context, CampaignHandoffEvent) (int64, error)
}
