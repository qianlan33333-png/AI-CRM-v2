// Package port defines the narrow non-shared ports required to freeze a
// Campaign-owned draft touch-plan snapshot. Concrete adapters remain pending
// the migration/API integration gate.
package port

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
)

var ErrSourceFactsUnavailable = errors.New("campaign initiation source facts unavailable")

const MaximumEligibilityTargets = campaign.MaximumDraftTouchTargets

// UnitOfWork must atomically roll back all repository/event writes when the
// callback returns an error, including an uncompleted create reservation.
type UnitOfWork interface {
	Within(context.Context, func(context.Context) error) error
}

// TargetSourceResolver locks and resolves Segment or AI Audience member facts.
// It must author their returned source watermark/version/digest from local
// facts; callers provide neither value. Customer selection is resolved inside
// Campaign from unique local OneIDs canonicalized before reservation.
// customer_filter is intentionally unavailable on current main until a saved
// normalized filter snapshot exists.
type TargetSourceResolver interface {
	ResolveCampaignTargets(context.Context, campaign.InitiationSourceRequest) (SourceResolution, error)
}

type SourceResolution struct {
	Source      campaign.InitiationSourceRef
	CustomerIDs []int64
}

// CampaignDraftReader returns a locked, current Campaign draft and its
// existing steps. Material refs are intentionally absent from this first seam:
// current main has no authoritative Campaign material fact.
type CampaignDraftReader interface {
	LockDraftCampaign(context.Context, string) (CampaignDraftFact, error)
}

type CampaignDraftFact struct {
	CampaignCode   string
	Version        int64
	ApprovalStatus campaign.ApprovalStatus
	RuntimeStatus  campaign.RuntimeStatus
	Steps          []campaign.Step
}

type EligibilityCheckpoint string

const EligibilityCheckpointPreview EligibilityCheckpoint = "preview"

// EligibilityChecker is the future Contact Touch Policy boundary. It reports
// only local OneIDs and reason codes, never phone, unionid, or provider data.
type EligibilityChecker interface {
	CheckCampaignEligibility(context.Context, EligibilityRequest) ([]EligibilityDecision, error)
}

type EligibilityRequest struct {
	Checkpoint     EligibilityCheckpoint
	MaximumTargets int
	CustomerIDs    []int64
}

type EligibilityExclusion string

const (
	EligibilityExclusionNone             EligibilityExclusion = "none"
	EligibilityExclusionInactiveCustomer EligibilityExclusion = "inactive_customer"
	EligibilityExclusionContactPolicy    EligibilityExclusion = "contact_policy"
)

type EligibilityDecision struct {
	CustomerID     int64
	CustomerActive bool
	Eligible       bool
	Exclusion      EligibilityExclusion
}

type CreateReservation struct {
	ActorID       int64
	KeyDigest     [sha256.Size]byte
	PayloadDigest [sha256.Size]byte
	PlanID        string
}

type CreateReceipt struct {
	ActorID       int64
	KeyDigest     [sha256.Size]byte
	PayloadDigest [sha256.Size]byte
	PlanID        string
	Completed     bool
}

type Repository interface {
	ReserveDraftCreate(context.Context, CreateReservation) (CreateReceipt, bool, error)
	SaveDraftTouchPlan(context.Context, campaign.DraftTouchPlan) error
	CompleteDraftCreate(context.Context, CreateReceipt) error
	// ReadDraftTouchPlan is a strict readback and must receive the UnitOfWork
	// transaction context used for ReserveDraftCreate.
	ReadDraftTouchPlan(context.Context, string) (campaign.DraftTouchPlan, error)
}

// CampaignEvent contains no raw target list or content body. A future adapter
// may persist it to event_log in the same UoW without dispatching anything.
type CampaignEvent struct {
	Type           string
	PlanID         string
	CampaignCode   string
	OwnerActorID   int64
	TargetDigest   string
	TargetCount    int32
	OccurredAt     time.Time
	IdempotencyKey string
}

type EventAppender interface {
	AppendCampaignEvent(context.Context, CampaignEvent) error
}
