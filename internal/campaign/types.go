package campaign

import (
	"context"
	"time"
)

const (
	RoutePrefix                      = "/api/admin/cloud-orchestrator/campaigns"
	CapabilityAdminRead              = "admin_read"
	CapabilityOperationsRead         = "operations_read"
	CapabilityManageAutomation       = "manage_automation"
	MaximumRequestBodyBytes    int64 = 64 << 10
	MaximumCampaignCodeBytes         = 96
	MaximumCampaignNameRunes         = 160
	MaximumStepContentRunes          = 4000
	MaximumSteps                     = 100
	MaximumBatch                     = 100
)

func ValidCampaignCode(value string) bool { return validCode(value) }

type ApprovalStatus string

const (
	ApprovalDraft    ApprovalStatus = "draft"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRejected ApprovalStatus = "rejected"
)

func (value ApprovalStatus) Valid() bool {
	return value == ApprovalDraft || value == ApprovalApproved || value == ApprovalRejected
}

type RuntimeStatus string

const (
	RuntimeIdle    RuntimeStatus = "idle"
	RuntimePlanned RuntimeStatus = "planned"
	RuntimePaused  RuntimeStatus = "paused"
)

func (value RuntimeStatus) Valid() bool {
	return value == RuntimeIdle || value == RuntimePlanned || value == RuntimePaused
}

type CommandOperation string

const (
	CommandStart      CommandOperation = "start"
	CommandBatchStart CommandOperation = "batch_start"
)

type Actor struct{ ID int64 }
type AccessRequirement struct {
	Capability  string
	RequireCSRF bool
}

type Campaign struct {
	Code           string         `json:"campaign_code"`
	Name           string         `json:"name"`
	ApprovalStatus ApprovalStatus `json:"approval_status"`
	RuntimeStatus  RuntimeStatus  `json:"runtime_status"`
	Version        int64          `json:"version"`
	CreatedBy      int64          `json:"created_by"`
	UpdatedBy      int64          `json:"updated_by"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type Step struct {
	Index        int32  `json:"step_index"`
	DelayMinutes int32  `json:"delay_minutes"`
	Content      string `json:"content"`
}

// Plan and Command are CRM-local records. The two false flags are deliberate
// evidence boundaries: they cannot be promoted to a provider/runtime receipt.
type Plan struct {
	ID              int64     `json:"plan_id"`
	CampaignCode    string    `json:"campaign_code"`
	CampaignVersion int64     `json:"campaign_version"`
	StepCount       int32     `json:"step_count"`
	CreatedAt       time.Time `json:"created_at"`
}
type Command struct {
	ID              int64            `json:"command_id"`
	Operation       CommandOperation `json:"operation"`
	CampaignCode    string           `json:"campaign_code"`
	PlanID          int64            `json:"plan_id"`
	RealSend        bool             `json:"real_send"`
	RuntimeExecuted bool             `json:"runtime_executed"`
	CreatedAt       time.Time        `json:"created_at"`
}
type AuditEvent struct {
	Type           string
	CampaignCode   string
	ActorID        int64
	IdempotencyKey string
	OccurredAt     time.Time
}

type Projection struct {
	LocalProjection          bool `json:"local_projection"`
	RealExternalCallExecuted bool `json:"real_external_call_executed"`
	RealSend                 bool `json:"real_send"`
	RuntimeExecuted          bool `json:"runtime_executed"`
}

func projection() Projection {
	return Projection{LocalProjection: true, RealExternalCallExecuted: false, RealSend: false, RuntimeExecuted: false}
}

type ListInput struct {
	ApprovalStatus *ApprovalStatus
	RuntimeStatus  *RuntimeStatus
}
type ListResponse struct {
	Items []Campaign `json:"items"`
	Projection
}
type DetailResponse struct {
	Campaign Campaign `json:"campaign"`
	Steps    []Step   `json:"steps"`
	Projection
}
type MutationResponse struct {
	Campaign Campaign `json:"campaign"`
	Command  *Command `json:"command,omitempty"`
	Plan     *Plan    `json:"plan,omitempty"`
	Projection
}
type DeleteResponse struct {
	CampaignCode string `json:"campaign_code"`
	Deleted      bool   `json:"deleted"`
	Projection
}
type BatchStartItem struct {
	CampaignCode    string `json:"campaign_code"`
	ExpectedVersion int64  `json:"expected_version"`
}
type BatchStartResponse struct {
	Started []MutationResponse `json:"started"`
	Skipped []BatchStartItem   `json:"skipped"`
	Failed  []BatchStartItem   `json:"failed"`
	Projection
}

type VersionedCommand struct {
	CampaignCode    string
	ExpectedVersion int64
	Actor           Actor
	IdempotencyKey  string
}
type StepCreateCommand struct {
	CampaignCode    string
	ExpectedVersion int64
	DelayMinutes    int32
	Content         string
	Actor           Actor
	IdempotencyKey  string
}
type StepUpdateCommand struct {
	CampaignCode    string
	StepIndex       int32
	ExpectedVersion int64
	DelayMinutes    *int32
	Content         *string
	Actor           Actor
	IdempotencyKey  string
}
type StepDeleteCommand struct {
	CampaignCode    string
	StepIndex       int32
	ExpectedVersion int64
	Actor           Actor
	IdempotencyKey  string
}
type BatchStartCommand struct {
	Items          []BatchStartItem
	Actor          Actor
	IdempotencyKey string
}

type IdempotencyState string

const (
	IdempotencyReserved  IdempotencyState = "reserved"
	IdempotencyCompleted IdempotencyState = "completed"
)

type OperationResult struct {
	Mutation *MutationResponse
	Delete   *DeleteResponse
	Batch    *BatchStartResponse
}
type IdempotencyRecord struct {
	ID            int64
	ActorID       int64
	Operation     string
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	State         IdempotencyState
	Result        *OperationResult
}
type Reservation struct {
	ActorID       int64
	Operation     string
	KeyDigest     [32]byte
	PayloadDigest [32]byte
	CreatedAt     time.Time
}

type UnitOfWork interface {
	Within(context.Context, func(context.Context) error) error
}
type AuditAppender interface {
	Append(context.Context, AuditEvent) error
}
type Repository interface {
	List(context.Context, ListInput) ([]Campaign, error)
	Get(context.Context, string) (Campaign, []Step, error)
	Lock(context.Context, string) (Campaign, []Step, error)
	Save(context.Context, Campaign, []Step) error
	Delete(context.Context, string, int64) error
	CreateLocalPlanAndCommand(context.Context, Campaign, int32, CommandOperation, time.Time) (Plan, Command, error)
	ReserveIdempotency(context.Context, Reservation) (IdempotencyRecord, bool, error)
	CompleteIdempotency(context.Context, int64, OperationResult) error
}
type Application interface {
	List(context.Context, ListInput) (ListResponse, error)
	Detail(context.Context, string) (DetailResponse, error)
	AddStep(context.Context, StepCreateCommand) (MutationResponse, error)
	UpdateStep(context.Context, StepUpdateCommand) (MutationResponse, error)
	DeleteStep(context.Context, StepDeleteCommand) (MutationResponse, error)
	Approve(context.Context, VersionedCommand) (MutationResponse, error)
	Reject(context.Context, VersionedCommand) (MutationResponse, error)
	Pause(context.Context, VersionedCommand) (MutationResponse, error)
	Start(context.Context, VersionedCommand) (MutationResponse, error)
	BatchStart(context.Context, BatchStartCommand) (BatchStartResponse, error)
	Delete(context.Context, VersionedCommand) (DeleteResponse, error)
}
