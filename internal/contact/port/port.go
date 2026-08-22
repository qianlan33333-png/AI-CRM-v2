// Package port freezes the cross-domain contact contract. It contains no store
// or HTTP implementation and never carries external identity values.
package port

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type CustomerID int64
type EventID int64
type Actor string
type StageID int64

var (
	ErrInvalidStage          = errors.New("invalid stage")
	ErrStageNotFound         = errors.New("stage not found")
	ErrStageReferenced       = errors.New("stage is still referenced by customers")
	ErrStageConflict         = errors.New("stage command conflict")
	ErrInvalidMergeCommand   = errors.New("invalid contact merge command")
	ErrMergeCustomerNotFound = errors.New("contact merge customer not found")
	ErrMergeConflict         = errors.New("contact merge conflict")
	ErrMergeStoreFailed      = errors.New("contact merge store failed")
	ErrExternalEventConflict = errors.New("external customer event conflict")
)

var (
	ErrCustomerReadNotFound    = errors.New("customer read projection not found")
	ErrCustomerReadUnavailable = errors.New("customer read projection unavailable")
)

// CustomerProjection is deliberately channel neutral. Phone numbers and
// external identifiers remain snapshots owned by the consuming business fact.
type CustomerProjection struct {
	ID   CustomerID
	Name string
}

type CustomerReader interface {
	ReadCustomer(context.Context, CustomerID) (CustomerProjection, error)
}

// ImageReferenceReader is the Contact-owned read-only answer to whether a
// channel welcome projection references one Media image. It returns only local
// channel IDs in ascending order.
type ImageReferenceReader interface {
	ListImageReferenceChannelIDs(context.Context, int64) ([]int64, error)
}

// AttachmentReferenceReader is the Contact-owned read-only answer to whether
// a channel welcome projection references one private attachment.
type AttachmentReferenceReader interface {
	ListAttachmentReferenceChannelIDs(context.Context, int64) ([]int64, error)
}

// StaffDirectoryReader exposes the narrowly-scoped local staff projection to
// approved read-only consumers. It contains only the approved staff identity
// fields and no provider payload or broader contact PII.
type StaffDirectoryReader interface {
	ListEligibleStaff(context.Context) ([]StaffDirectoryEntry, error)
}

// ActiveStaffReader answers only whether one local numeric staff fact remains
// active. The supplied context must carry the caller's UnitOfWork transaction.
// It intentionally exposes neither directory fields nor external identities.
type ActiveStaffReader interface {
	IsActiveStaff(context.Context, int64) (bool, error)
}

type StaffDirectoryEntry struct {
	WeComUserID string
	DisplayName string
	UpdatedAt   time.Time
}

type Stage struct {
	ID        StageID
	Name      string
	SortOrder int32
	Config    json.RawMessage
}

type CreateStageCommand struct {
	Name           string
	SortOrder      int32
	Config         json.RawMessage
	Actor          Actor
	IdempotencyKey string
}

type RenameStageCommand struct {
	ID             StageID
	Name           string
	Actor          Actor
	IdempotencyKey string
}

// ReorderStagesCommand is an exact active-stage ordering. Callers must supply
// every active stage exactly once; this prevents a stale client from silently
// dropping a stage from the local lifecycle.
type ReorderStagesCommand struct {
	IDs            []StageID
	Actor          Actor
	IdempotencyKey string
}

type ArchiveStageCommand struct {
	ID             StageID
	Actor          Actor
	IdempotencyKey string
}

// StageService is the only public stage mutation boundary. Implementations
// must commit each mutation with its domain event in one UnitOfWork.
type StageService interface {
	ListStages(context.Context) ([]Stage, error)
	CreateStage(context.Context, CreateStageCommand) (Stage, error)
	RenameStage(context.Context, RenameStageCommand) (Stage, error)
	ReorderStages(context.Context, ReorderStagesCommand) ([]Stage, error)
	ArchiveStage(context.Context, ArchiveStageCommand) (Stage, error)
}

type CreateForIdentityCommand struct {
	Name         string
	OwnerStaffID *int64
	ChannelID    *int64
	Actor        Actor
}

type MergeCustomersCommand struct {
	PrimaryID CustomerID
	MergedID  CustomerID
	Actor     Actor
	Reason    string
}

type ExternalEventCommand struct {
	CustomerID     CustomerID
	EventType      string
	Payload        json.RawMessage
	Actor          Actor
	OccurredAt     time.Time
	IdempotencyKey string
}

// MergePort methods require the transaction context supplied by UnitOfWork.
type MergePort interface {
	CreateForIdentity(context.Context, CreateForIdentityCommand) (CustomerID, error)
	MergeCustomers(context.Context, MergeCustomersCommand) error
	AppendExternalEvent(context.Context, ExternalEventCommand) (EventID, error)
}
