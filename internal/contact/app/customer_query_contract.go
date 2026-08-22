package app

import (
	"context"
	"encoding/json"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

const (
	CustomerListDefaultLimit  int32 = 50
	CustomerListMaximumLimit  int32 = 200
	CustomerListExactTotalCap int64 = 10_000
)

// CustomerRecord is the channel-neutral contact read model. External identity
// values belong to identity and must never be added here.
type CustomerRecord struct {
	ID             contactport.CustomerID
	Name           string
	AvatarURL      *string
	Gender         *int16
	StageID        *int64
	OwnerStaffID   *int64
	ChannelID      *int64
	AddedAt        *time.Time
	LastInteractAt *time.Time
	IsDeleted      bool
	Extra          json.RawMessage
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CustomerListQuery is already validated and normalized by the application
// service. Watermark stays fixed across every page; After* is the keyset only.
type CustomerListQuery struct {
	CustomerID         *contactport.CustomerID
	MatchNone          bool
	Keyword            string
	OwnerStaffID       *int64
	StageID            *int64
	ChannelID          *int64
	TagID              *int64
	IsDeleted          bool
	AddedAfter         *time.Time
	AddedBefore        *time.Time
	LastInteractAfter  *time.Time
	LastInteractBefore *time.Time
	Watermark          time.Time
	AfterUpdatedAt     *time.Time
	AfterID            *contactport.CustomerID
	Limit              int32
}

// CustomerListStoreResult returns at most Limit rows plus a bounded count.
// BoundedTotal may reach ExactTotalCap+1 only to signal an estimated 10k+ total.
type CustomerListStoreResult struct {
	Items        []CustomerRecord
	BoundedTotal int64
	HasMore      bool
}

// CustomerQueryStore is internal to contact. It does not expand the frozen
// cross-domain contact port and requires a transaction-bound context.
type CustomerQueryStore interface {
	ListCustomers(context.Context, CustomerListQuery) (CustomerListStoreResult, error)
}
