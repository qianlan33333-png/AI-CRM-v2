// Package port freezes the cross-domain contact contract. It contains no store
// or HTTP implementation and never carries external identity values.
package port

import (
	"context"
	"encoding/json"
	"time"
)

type CustomerID int64
type EventID int64
type Actor string

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
