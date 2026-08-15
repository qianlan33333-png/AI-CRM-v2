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

type Stage struct {
	ID        StageID
	Name      string
	SortOrder int32
	Config    json.RawMessage
}

type CreateStageCommand struct {
	Name      string
	SortOrder int32
	Config    json.RawMessage
	Actor     Actor
}

type RenameStageCommand struct {
	ID    StageID
	Name  string
	Actor Actor
}

// StageService is the only public stage mutation boundary. Implementations
// must commit each mutation with its domain event in one UnitOfWork.
type StageService interface {
	ListStages(context.Context) ([]Stage, error)
	CreateStage(context.Context, CreateStageCommand) (Stage, error)
	RenameStage(context.Context, RenameStageCommand) (Stage, error)
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
