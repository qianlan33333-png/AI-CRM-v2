package port

import (
	"context"
	"time"
)

// These are historical source facts, never current channel attribution or
// staff permissions. Source references must not be used as current V2 IDs.
type HistoricalChannelContact struct {
	ID              int64     `json:"id"`
	ChannelID       int64     `json:"channel_id"`
	SourceContactID int64     `json:"source_contact_id"`
	CustomerID      *int64    `json:"customer_id"`
	OwnerReference  string    `json:"owner_reference"`
	FirstEnteredAt  time.Time `json:"first_entered_at"`
	LastEnteredAt   time.Time `json:"last_entered_at"`
	EnterCount      int32     `json:"enter_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type HistoricalChannelAssignee struct {
	ID                  int64  `json:"id"`
	ChannelID           int64  `json:"channel_id"`
	SourceAssigneeID    int64  `json:"source_assignee_id"`
	StaffReference      string `json:"staff_reference"`
	DisplayNameSnapshot string `json:"display_name_snapshot"`
	Priority            int32  `json:"priority"`
	RatioPercent        *int32 `json:"ratio_percent"`
	MaxScans24h         *int32 `json:"max_scans_24h"`
	Status              string `json:"status"`
	// V1 stored civil timestamps without a timezone. Preserve that distinction.
	SourceCreatedAt string `json:"source_created_at"`
	SourceUpdatedAt string `json:"source_updated_at"`
}

type HistoricalChannelContactDefinition struct {
	SourceIdentifier string
	PayloadDigest    [32]byte
	Contact          HistoricalChannelContact
}

type HistoricalChannelAssigneeDefinition struct {
	SourceIdentifier string
	PayloadDigest    [32]byte
	Assignee         HistoricalChannelAssignee
}

type HistoricalChannelRelationsStore interface {
	CreateHistoricalChannelContact(context.Context, HistoricalChannelContact) (HistoricalChannelContact, error)
	GetHistoricalChannelContact(context.Context, int64) (HistoricalChannelContact, error)
	CreateHistoricalChannelAssignee(context.Context, HistoricalChannelAssignee) (HistoricalChannelAssignee, error)
	GetHistoricalChannelAssignee(context.Context, int64) (HistoricalChannelAssignee, error)
}

// Kind is exactly contacts or assignees. The adapter chooses one scoped
// migration-owned journal and uses the same transaction as the Contact store.
type HistoricalChannelRelationsJournal interface {
	LoadHistoricalChannelRelation(context.Context, string, string) (HistoricalChannelReceipt, bool, error)
	RecordHistoricalChannelRelation(context.Context, string, HistoricalChannelReceipt) error
}

type HistoricalChannelHistoryReader interface {
	ListHistoricalChannelContacts(context.Context, int64, int32, int32) ([]HistoricalChannelContact, int64, error)
	ListHistoricalChannelAssignees(context.Context, int64) ([]HistoricalChannelAssignee, error)
}
