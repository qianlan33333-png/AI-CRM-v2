package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrCustomerTimelineHistoryInvalid     = errors.New("invalid customer timeline history")
	ErrCustomerTimelineHistoryConflict    = errors.New("customer timeline history conflict")
	ErrCustomerTimelineHistoryUnavailable = errors.New("customer timeline history unavailable")
)

// HistoricalCustomerTimelineEvent is immutable V1 observation evidence. It
// never creates a current customer event; CustomerID is only an optional,
// separately verified historical reference.
type HistoricalCustomerTimelineEvent struct {
	ID                  int64     `json:"id"`
	SourceKeyDigest     [32]byte  `json:"-"`
	SourcePayloadDigest [32]byte  `json:"-"`
	SourceFieldDigest   [32]byte  `json:"-"`
	SourceID            int64     `json:"source_id"`
	EventID             string    `json:"event_id"`
	EventType           string    `json:"event_type"`
	EventTime           time.Time `json:"event_time"`
	Title               string    `json:"-"`
	Summary             string    `json:"-"`
	SourceTable         string    `json:"source_table"`
	SourceValue         string    `json:"source_value"`
	MetadataJSON        []byte    `json:"-"`
	CreatedAt           time.Time `json:"created_at"`
	UnionID             string    `json:"-"`
	CustomerID          *int64    `json:"customer_id,omitempty"`
}

// CustomerTimelineHistoryRead is the complete safe read model. It excludes
// unionid, title, summary and metadata_json even when the caller is an admin.
type CustomerTimelineHistoryRead struct {
	ID          int64     `json:"id"`
	SourceID    int64     `json:"source_id"`
	EventID     string    `json:"event_id"`
	EventType   string    `json:"event_type"`
	EventTime   time.Time `json:"event_time"`
	SourceTable string    `json:"source_table"`
	SourceValue string    `json:"source_value"`
	CreatedAt   time.Time `json:"created_at"`
	CustomerID  *int64    `json:"customer_id"`
}

type CustomerTimelineHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

type CustomerTimelineHistoryStore interface {
	CreateHistoricalCustomerTimelineEvent(context.Context, HistoricalCustomerTimelineEvent) (HistoricalCustomerTimelineEvent, error)
	GetHistoricalCustomerTimelineEvent(context.Context, int64) (HistoricalCustomerTimelineEvent, error)
}

type CustomerTimelineHistoryJournal interface {
	LoadCustomerTimelineHistory(context.Context, string, string) (CustomerTimelineHistoryReceipt, bool, error)
	RecordCustomerTimelineHistory(context.Context, CustomerTimelineHistoryReceipt) error
}

type CustomerTimelineHistoryQuery struct {
	Limit, Offset int32
}

type CustomerTimelineHistoryReader interface {
	GetHistoricalCustomerTimelineEvent(context.Context, int64) (CustomerTimelineHistoryRead, error)
	ListHistoricalCustomerTimelineEvents(context.Context, CustomerTimelineHistoryQuery) ([]CustomerTimelineHistoryRead, int64, error)
}
