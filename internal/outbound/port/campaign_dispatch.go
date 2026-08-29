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
	// ContentSnapshot freezes the exact approved content before the effect is
	// accepted. The worker must never reconstruct it from a mutable review.
	ContentSnapshot string
	State           outbound.CampaignDispatchState
	BlockReason     string
	// SenderUserIDSnapshot and ExternalUserIDSnapshot are private runtime
	// facts. They are populated together only for audience-package dispatches
	// and must never be exposed from a read API.
	SenderUserIDSnapshot   string
	ExternalUserIDSnapshot string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type CampaignDispatchReceipt struct {
	ID            int64
	ActorID       int64
	HandoffID     int64
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	Result        outbound.CampaignDispatchSummary
}

// CampaignDispatchProviderRequest is the minimal private request material
// resolved by the Outbound-owned worker after EER has durably fenced an
// attempt. It must never be projected through a read API or into EER.
type CampaignDispatchProviderRequest struct {
	DispatchID             int64
	HandoffID              int64
	CustomerID             int64
	StepIndex              int32
	Content                string
	PayloadDigest          string
	AudiencePackageID      int64
	SenderUserIDSnapshot   string
	ExternalUserIDSnapshot string
}

type CampaignDispatchProviderAttemptReceipt struct {
	Completion                   string
	ReceiptDigest                eer.Digest
	BusinessCallDispatched       bool
	RealExternalCallExecuted     bool
	ProviderMessageID            string
	ProviderCode                 string
	ProviderResultReceived       bool
	DeliveryProven               bool
	ReconciliationEvidenceDigest eer.Digest
}

// AudienceDispatchTargetQualification is the exact relationship-owner target
// selected during the Dispatch UoW. Eligible=false is a business exclusion;
// an adapter error remains an unavailable transaction and must not create a
// permanent blocked dispatch.
type AudienceDispatchTargetQualification struct {
	CustomerID     int64
	Eligible       bool
	SenderUserID   string
	ExternalUserID string
	Exclusion      string
}

type AudienceDispatchTargetQualifier interface {
	QualifyAudienceDispatchTargets(context.Context, int64, []int64) ([]AudienceDispatchTargetQualification, error)
}

type AudienceCampaignDispatchSourceReader interface {
	AudiencePackageForCampaignHandoff(context.Context, int64) (int64, bool, error)
}

type CampaignDispatchRepository interface {
	LockCampaignHandoffForDispatch(context.Context, string, string) (int64, error)
	ReadCampaignHandoffForDispatch(context.Context, string, string) (int64, error)
	ListCampaignDispatchCandidates(context.Context, int64) ([]CampaignDispatchCandidate, error)
	LoadCampaignDispatchReceipt(context.Context, int64, [32]byte) (CampaignDispatchReceipt, bool, error)
	ReserveCampaignDispatchReceipt(context.Context, int64, int64, [32]byte, [32]byte, outbound.CampaignDispatchSummary) (CampaignDispatchReceipt, error)
	InsertCampaignDispatchBinding(context.Context, CampaignDispatchBinding) (CampaignDispatchBinding, error)
	LoadCampaignDispatchByEffect(context.Context, string) (CampaignDispatchBinding, error)
	LoadCampaignDispatchProviderRequest(context.Context, string) (CampaignDispatchProviderRequest, error)
	UpdateCampaignDispatchState(context.Context, string, outbound.CampaignDispatchState) error
	ReadCampaignDispatchSummary(context.Context, int64) (outbound.CampaignDispatchSummary, error)
	RecordCampaignProviderAttemptReceipt(context.Context, string, int32, CampaignDispatchProviderAttemptReceipt) error
}

// CampaignDispatchRecipientApprovalReader is the additional narrow fact
// required by the legacy single-recipient approval action. A local approval is
// only eligibility for a controlled dispatch; it is never a send receipt.
type CampaignDispatchRecipientApproval struct {
	Approved        bool
	MessageOverride string
}

type CampaignDispatchRecipientApprovalReader interface {
	ReadCampaignDispatchRecipientApproval(context.Context, int64, int64) (CampaignDispatchRecipientApproval, error)
}

// CampaignDispatchReconciliationEvidence contains only the private facts a
// protocol verifier needs. A caller-supplied digest is never evidence.
type CampaignDispatchReconciliationEvidence struct {
	ExternalEffectID         string
	ProviderMessageID        string
	SenderUserID             string
	ExternalUserID           string
	ProviderReceiptDigest    eer.Digest
	BusinessCallDispatched   bool
	RealExternalCallExecuted bool
}

type CampaignDispatchReconciliationEvidenceReader interface {
	LoadAudienceCampaignDispatchReconciliationEvidence(context.Context, string) (CampaignDispatchReconciliationEvidence, bool, error)
}

type CampaignDispatchReconciliationEvidenceVerifier interface {
	VerifyAudienceCampaignDispatch(context.Context, CampaignDispatchReconciliationEvidence) (deliveryProven bool, evidenceDigest eer.Digest, error error)
}

type CampaignDispatchEnqueuer interface {
	EnqueueCampaignDispatch(context.Context, string) (eer.RiverJobLink, error)
}
