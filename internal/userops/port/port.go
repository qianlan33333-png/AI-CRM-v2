// Package port freezes the User Ops local-only boundary. Composition adapters
// may bridge Contact, Customer 360, Identity, the platform UoW and event log,
// but User Ops itself never imports or writes those domains.
package port

import (
	"context"
	"errors"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/userops/domain"
)

const (
	DefaultPageLimit int32 = 50
	MaximumPageLimit int32 = 200
	MaximumBatchSize int   = 1_000
)

var (
	ErrInvalid      = errors.New("invalid user ops request")
	ErrNotFound     = errors.New("user ops resource not found")
	ErrConflict     = errors.New("user ops request conflict")
	ErrPreviewStale = errors.New("user ops batch preview is stale")
	ErrUnavailable  = errors.New("user ops local storage unavailable")
)

// Safety is deliberately part of every successful response. This package
// creates local facts only, never an outbound job or provider call.
type Safety struct {
	ProviderExecutionEligible bool `json:"provider_execution_eligible"`
	RealExternalCallExecuted  bool `json:"real_external_call_executed"`
	DeliveryProven            bool `json:"delivery_proven"`
}

func LocalSafety() Safety {
	return Safety{
		ProviderExecutionEligible: false,
		RealExternalCallExecuted:  false,
		DeliveryProven:            false,
	}
}

// DirectoryQuery contains only local CRM filters. PhoneExact is opaque input
// for a future Identity composition adapter; User Ops neither normalizes it
// nor returns it in any projection.
type DirectoryQuery struct {
	Keyword      string
	OwnerStaffID *int64
	StageID      *int64
	ChannelID    *int64
	TagID        *int64
	PhoneExact   string
	Cursor       string
	Limit        int32
}

// CustomerSummary mirrors only existing safe Contact list fields. It omits
// avatar URLs, Extra JSON, identity values, phone numbers and provider state.
type CustomerSummary struct {
	CustomerID     domain.CustomerID `json:"customer_id"`
	Name           string            `json:"name"`
	OwnerStaffID   *int64            `json:"owner_staff_id,omitempty"`
	StageID        *int64            `json:"stage_id,omitempty"`
	ChannelID      *int64            `json:"channel_id,omitempty"`
	AddedAt        *time.Time        `json:"added_at,omitempty"`
	LastInteractAt *time.Time        `json:"last_interact_at,omitempty"`
}

type DirectoryPageRead struct {
	Items           []CustomerSummary
	NextCursor      *string
	Total           int64
	TotalIsEstimate bool
}

type DirectoryPage struct {
	Items           []CustomerSummary `json:"items"`
	NextCursor      *string           `json:"next_cursor,omitempty"`
	Total           int64             `json:"total"`
	TotalIsEstimate bool              `json:"total_is_estimate"`
	Safety
}

type DirectoryOverviewRead struct {
	CustomerCount           int64
	CustomerCountIsEstimate bool
}

type LocalOverviewRead struct {
	ActiveDNDCount         int64
	DraftPlanCount         int64
	PendingReviewPlanCount int64
}

type Overview struct {
	CustomerCount           int64 `json:"customer_count"`
	CustomerCountIsEstimate bool  `json:"customer_count_is_estimate"`
	ActiveDNDCount          int64 `json:"active_dnd_count"`
	DraftPlanCount          int64 `json:"draft_plan_count"`
	PendingReviewPlanCount  int64 `json:"pending_review_plan_count"`
	Safety
}

type CustomerTag struct {
	ID        int64   `json:"id"`
	GroupID   *int64  `json:"group_id,omitempty"`
	GroupName *string `json:"group_name,omitempty"`
	Name      string  `json:"name"`
}

// CustomerDetail is a local safe projection. Timeline has no payload or
// actor, and the projection carries no raw message content or identities.
type CustomerDetail struct {
	Customer CustomerSummary `json:"customer"`
	Tags     []CustomerTag   `json:"tags"`
	Timeline []TimelineEntry `json:"timeline"`
}

type TimelineEntry struct {
	EventType  string    `json:"event_type"`
	OccurredAt time.Time `json:"occurred_at"`
}

type CustomerDetailResult struct {
	Detail CustomerDetail       `json:"detail"`
	DND    *domain.DoNotDisturb `json:"dnd,omitempty"`
	Safety
}

type SafeExportField string

const (
	SafeExportCustomerID     SafeExportField = "customer_id"
	SafeExportName           SafeExportField = "name"
	SafeExportOwnerStaffID   SafeExportField = "owner_staff_id"
	SafeExportStageID        SafeExportField = "stage_id"
	SafeExportChannelID      SafeExportField = "channel_id"
	SafeExportAddedAt        SafeExportField = "added_at"
	SafeExportLastInteractAt SafeExportField = "last_interact_at"
)

func (field SafeExportField) Valid() bool {
	switch field {
	case SafeExportCustomerID, SafeExportName, SafeExportOwnerStaffID, SafeExportStageID,
		SafeExportChannelID, SafeExportAddedAt, SafeExportLastInteractAt:
		return true
	default:
		return false
	}
}

type SafeExportRequest struct {
	Query  DirectoryQuery
	Fields []SafeExportField
}

// SafeExport represents the whitelist-projected rows that a caller may turn
// into a local download. It cannot carry an arbitrary customer column.
type SafeExport struct {
	Fields          []SafeExportField `json:"fields"`
	Rows            [][]string        `json:"rows"`
	NextCursor      *string           `json:"next_cursor,omitempty"`
	Total           int64             `json:"total"`
	TotalIsEstimate bool              `json:"total_is_estimate"`
	Safety
}

type BatchPreviewInput struct {
	CustomerIDs []domain.CustomerID
	Content     domain.ContentInput
}

// BatchPreview computes only current local targets. It says nothing about a
// message, provider acceptance, dispatch, or delivery.
type BatchPreview struct {
	TargetCustomerIDs []domain.CustomerID    `json:"target_customer_ids"`
	ExcludedDNDCount  int32                  `json:"excluded_dnd_count"`
	TargetDigest      string                 `json:"target_digest"`
	Content           domain.ContentSnapshot `json:"content"`
	Safety
}

type CreateLocalPlanInput struct {
	CustomerIDs           []domain.CustomerID
	ExpectedTargetDigest  string
	Content               domain.ContentInput
	ExpectedContentDigest string
	State                 domain.LocalPlanState
	ActorID               int64
	IdempotencyKey        string
}

type LocalPlanResult struct {
	Plan domain.LocalPlan `json:"plan"`
	Safety
}

type UpsertDNDInput struct {
	CustomerID      domain.CustomerID
	Reason          string
	ExpectedVersion *int64
	ActorID         int64
	IdempotencyKey  string
}

type ClearDNDInput struct {
	CustomerID      domain.CustomerID
	ExpectedVersion int64
	ActorID         int64
	IdempotencyKey  string
}

type DNDMutationResult struct {
	DND     *domain.DoNotDisturb `json:"dnd,omitempty"`
	Cleared bool                 `json:"cleared"`
	Safety
}

type SendRecordQuery struct {
	PlanID domain.PlanID
	Cursor string
	Limit  int32
}

type SendRecordPageRead struct {
	Items      []domain.SendRecord
	NextCursor *string
	Total      int64
}

type SendRecordPage struct {
	Items      []domain.SendRecord `json:"items"`
	NextCursor *string             `json:"next_cursor,omitempty"`
	Total      int64               `json:"total"`
	Safety
}

// CustomerDirectoryReader is implemented only by the composition layer. Its
// data may originate from Contact and Identity but never reveals a raw phone
// or other external identity to User Ops.
type CustomerDirectoryReader interface {
	ReadOverview(context.Context, DirectoryQuery) (DirectoryOverviewRead, error)
	ListCustomers(context.Context, DirectoryQuery) (DirectoryPageRead, error)
	ResolveCustomers(context.Context, []domain.CustomerID) ([]CustomerSummary, error)
}

// CustomerDetailReader is a narrow safe Customer 360 bridge. It must accept
// only canonical local OneID values.
type CustomerDetailReader interface {
	ReadCustomerDetail(context.Context, domain.CustomerID) (CustomerDetail, error)
}

// MaterialReader validates only local material references in the caller's
// UnitOfWork. Implementations must lock and fail closed; they must never fetch
// a URL, invoke a provider, or expose material metadata.
type MaterialReader interface {
	ImageEligible(context.Context, int64) (bool, error)
	MiniProgramEligible(context.Context, int64) (bool, error)
	AttachmentEligible(context.Context, int64) (bool, error)
}

// Mutation reports whether the Repository replayed an existing idempotent
// write. A replay must not append a duplicate event.
type Mutation struct {
	Replayed bool
}

// DNDMutation carries the durable idempotency snapshot on replay. Fresh set
// mutations leave DND unset, while a fresh clear sets Cleared=true; the
// application performs strict readback in both cases.
type DNDMutation struct {
	Mutation
	DND     *domain.DoNotDisturb
	Cleared bool
}

// PlanMutation carries the durable idempotency snapshot on replay. Fresh
// mutations return only PlanID and require a strict readback by the app.
type PlanMutation struct {
	Mutation
	PlanID domain.PlanID
	Plan   *domain.LocalPlan
}

// Repository owns only User Ops local facts. Every method requires the
// transaction-bound context supplied by UnitOfWork. Write methods must enforce
// idempotency and CAS atomically before returning. CreateLocalPlan's
// idempotency payload binds the normalized target IDs, target digest, state,
// and complete ContentSnapshot.
type Repository interface {
	ReadLocalOverview(context.Context) (LocalOverviewRead, error)
	ReadDND(context.Context, domain.CustomerID) (*domain.DoNotDisturb, error)
	ListActiveDND(context.Context, []domain.CustomerID) ([]domain.DoNotDisturb, error)
	LockActiveDND(context.Context, []domain.CustomerID) ([]domain.DoNotDisturb, error)
	UpsertDND(context.Context, UpsertDNDInput) (DNDMutation, error)
	ClearDND(context.Context, ClearDNDInput) (DNDMutation, error)
	// ReplayLocalPlan checks only the actor-scoped receipt. It intentionally
	// precedes target, DND and material validation so a completed request is
	// stable even after those independent local facts later change.
	ReplayLocalPlan(context.Context, CreateLocalPlanInput, domain.ContentSnapshot) (PlanMutation, error)
	CreateLocalPlan(context.Context, CreateLocalPlanInput, []domain.CustomerID, string, domain.ContentSnapshot) (PlanMutation, error)
	ReadLocalPlan(context.Context, domain.PlanID) (domain.LocalPlan, error)
	ListSendRecords(context.Context, SendRecordQuery) (SendRecordPageRead, error)
}

// UnitOfWork supplies the same transaction context to User Ops facts and its
// event append. It must roll both back when a callback returns an error.
type UnitOfWork interface {
	Within(context.Context, func(context.Context) error) error
}

// LocalEvent omits raw reason text, target lists, identities, provider details
// and delivery state. The composition adapter persists it alongside the local
// fact; it must not dispatch an external effect.
type LocalEvent struct {
	Type           string
	CustomerID     domain.CustomerID
	PlanID         domain.PlanID
	Version        int64
	TargetCount    int32
	OccurredAt     time.Time
	IdempotencyKey string
}

type EventAppender interface {
	Append(context.Context, LocalEvent) error
}

// Application is the closed local use-case boundary consumed by the HTTP
// leaf. A future composition root supplies all adapters.
type Application interface {
	Overview(context.Context, DirectoryQuery) (Overview, error)
	ListCustomers(context.Context, DirectoryQuery) (DirectoryPage, error)
	GetCustomerDetail(context.Context, domain.CustomerID) (CustomerDetailResult, error)
	SafeExport(context.Context, SafeExportRequest) (SafeExport, error)
	PreviewBatch(context.Context, BatchPreviewInput) (BatchPreview, error)
	CreateLocalPlan(context.Context, CreateLocalPlanInput) (LocalPlanResult, error)
	SetDND(context.Context, UpsertDNDInput) (DNDMutationResult, error)
	ClearDND(context.Context, ClearDNDInput) (DNDMutationResult, error)
	ListSendRecords(context.Context, SendRecordQuery) (SendRecordPage, error)
}
