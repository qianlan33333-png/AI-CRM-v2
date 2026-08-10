// Package port freezes the cross-domain transactional event append contract.
package port

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type CustomerID int64
type EventID int64

var (
	ErrInvalidEvent        = errors.New("invalid event")
	ErrIdempotencyConflict = errors.New("event idempotency conflict")
)

type Event struct {
	Type           string
	CustomerID     CustomerID
	Payload        json.RawMessage
	OccurredAt     time.Time
	IdempotencyKey string
}

// Record is the immutable event fact loaded by a River delivery job.
type Record struct {
	ID EventID
	Event
}

// Subscriber consumes committed event facts. Implementations must persistently
// deduplicate by Record.ID (or a domain-specific stable key) because delivery is
// at least once. Multiple subscribers may handle the same event type.
type Subscriber interface {
	EventTypes() []string
	Consume(context.Context, Record) error
}

// Appender only persists the event in the transaction supplied by UnitOfWork.
// It does not dispatch work or call an external service.
type Appender interface {
	Append(context.Context, Event) (EventID, error)
}
