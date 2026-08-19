// Package port owns the safe, local customer-context boundary. It contains no
// HTTP, storage, AI generation, provider, or external-identity implementation.
package port

import (
	"context"
	"errors"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

var (
	ErrInvalidCustomerContext     = errors.New("invalid customer context query")
	ErrCustomerContextUnavailable = errors.New("customer context unavailable")
)

type CustomerContextQuery struct {
	CustomerID     contactport.CustomerID
	OwnerStaffID   *int64
	TimelineCursor string
	TimelineLimit  int32
}

// Customer is a minimal local CRM projection. It never includes avatar URLs,
// raw Extra JSON, identity hints, or channel/provider identity values.
type Customer struct {
	ID             contactport.CustomerID
	Name           string
	StageID        *int64
	OwnerStaffID   *int64
	ChannelID      *int64
	AddedAt        *time.Time
	LastInteractAt *time.Time
}

type Tag struct {
	ID             int64
	GroupID        *int64
	GroupName      *string
	GroupSortOrder int32
	Name           string
	SortOrder      int32
}

// TimelineEntry contains no actor or event payload.
type TimelineEntry struct {
	ID         int64
	EventType  string
	OccurredAt time.Time
}

// ChatSummary contains no message content, external identity, media URL, or
// provider delivery/receipt state. LocalArchiveAvailable only means this local
// read projection could be loaded; it never asserts an external-system state.
type ChatSummary struct {
	LocalArchiveAvailable bool
	Items                 []ChatEntry
	Total                 int64
}

type ChatEntry struct {
	ChatType    string
	MessageType string
	SentAt      time.Time
}

// CustomerContext is not atomic: contact and local archive data are read from
// independent local projections and each consumer must treat their timestamps
// accordingly.
type CustomerContext struct {
	Customer           Customer
	Tags               []Tag
	Timeline           []TimelineEntry
	TimelineNextCursor *string
	Chat               ChatSummary
}

type Reader interface {
	ReadCustomerContext(context.Context, CustomerContextQuery) (CustomerContext, error)
}
