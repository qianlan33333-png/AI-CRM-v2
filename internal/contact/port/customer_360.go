package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidCustomer360Read     = errors.New("invalid customer 360 read")
	ErrCustomer360ReadUnavailable = errors.New("customer 360 read unavailable")
)

// Customer360ReadInput is the contact-owned, local CRM read scope. It carries
// only the local OneID and optional local owner scope; it never accepts an
// external identity or identity hint.
type Customer360ReadInput struct {
	CustomerID     CustomerID
	OwnerStaffID   *int64
	TimelineCursor string
	TimelineLimit  int32
}

// Customer360Customer deliberately excludes contact Extra, avatar URLs,
// gender, and every external identity snapshot.
type Customer360Customer struct {
	ID             CustomerID
	Name           string
	StageID        *int64
	OwnerStaffID   *int64
	ChannelID      *int64
	AddedAt        *time.Time
	LastInteractAt *time.Time
}

// Customer360Tag is the local CRM tag projection. It contains no provider tag
// identifier and its order is the contact-owned catalog order.
type Customer360Tag struct {
	ID             int64
	GroupID        *int64
	GroupName      *string
	GroupSortOrder int32
	Name           string
	SortOrder      int32
}

// Customer360TimelineEntry deliberately omits the raw event payload and actor.
type Customer360TimelineEntry struct {
	ID         int64
	EventType  string
	OccurredAt time.Time
}

type Customer360Read struct {
	Customer           Customer360Customer
	Tags               []Customer360Tag
	Timeline           []Customer360TimelineEntry
	TimelineNextCursor *string
}

// Customer360Reader is a read-only local CRM boundary for customer-facing and
// AI context consumers. Results are not an atomic snapshot across detail and
// timeline reads.
type Customer360Reader interface {
	ReadCustomer360(context.Context, Customer360ReadInput) (Customer360Read, error)
}
