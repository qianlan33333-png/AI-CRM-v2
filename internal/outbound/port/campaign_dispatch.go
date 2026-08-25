package port

import (
	"context"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
)

type CampaignDispatchCandidate struct {
	CustomerID int64
	StepIndex  int32
	Content    string
}

type CampaignDispatchBinding struct {
	ID               int64
	HandoffID        int64
	CustomerID       int64
	StepIndex        int32
	ExternalEffectID string
	RecipientDigest  string
	PayloadDigest    string
	State            outbound.CampaignDispatchState
	BlockReason      string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CampaignDispatchReceipt struct {
	ID            int64
	ActorID       int64
	HandoffID     int64
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	Result        outbound.CampaignDispatchSummary
}

type CampaignDispatchRepository interface {
	LockCampaignHandoffForDispatch(context.Context, string, string) (int64, error)
	ReadCampaignHandoffForDispatch(context.Context, string, string) (int64, error)
	ListCampaignDispatchCandidates(context.Context, int64) ([]CampaignDispatchCandidate, error)
	ReserveCampaignDispatchReceipt(context.Context, int64, int64, [32]byte, [32]byte, outbound.CampaignDispatchSummary) (CampaignDispatchReceipt, error)
	InsertCampaignDispatchBinding(context.Context, CampaignDispatchBinding) (CampaignDispatchBinding, error)
	LoadCampaignDispatchByEffect(context.Context, string) (CampaignDispatchBinding, error)
	UpdateCampaignDispatchState(context.Context, string, outbound.CampaignDispatchState) error
	ReadCampaignDispatchSummary(context.Context, int64) (outbound.CampaignDispatchSummary, error)
	RecordCampaignProviderAttemptReceipt(context.Context, string, int32, string, eer.Digest) error
}

type CampaignDispatchEnqueuer interface {
	EnqueueCampaignDispatch(context.Context, string) (eer.RiverJobLink, error)
}
