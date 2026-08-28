package port

import (
	"context"
	"errors"
	"time"
)

// Immutable V1 facts only. Source references are not current V2 identifiers.
var (
	ErrStaticMediaHistoryInvalid     = errors.New("invalid media static history")
	ErrStaticMediaHistoryConflict    = errors.New("media static history conflict")
	ErrStaticMediaHistoryUnavailable = errors.New("media static history unavailable")
)

type HistoricalGroupInvite struct {
	ID                   int64     `json:"id"`
	SourceID             int64     `json:"source_id"`
	SourceKeyDigest      [32]byte  `json:"source_key_digest"`
	SourcePayloadDigest  [32]byte  `json:"source_payload_digest"`
	Name                 string    `json:"name"`
	Title                string    `json:"title"`
	Description          string    `json:"description"`
	OriginalState        string    `json:"original_state"`
	OriginalAutoCreate   bool      `json:"original_auto_create"`
	RoomBaseName         string    `json:"room_base_name"`
	RoomBaseSourceID     *int64    `json:"room_base_source_id"`
	OriginalEnabled      bool      `json:"original_enabled"`
	OriginalBindingState string    `json:"original_binding_state"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type StaticMediaHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}
type StaticMediaHistoryQuery struct {
	Limit, Offset int32
}

// Writes and journal receipts must share the caller transaction.
type StaticMediaHistoryStore interface {
	CreateHistoricalGroupInvite(context.Context, HistoricalGroupInvite) (HistoricalGroupInvite, error)
	GetHistoricalGroupInvite(context.Context, int64) (HistoricalGroupInvite, error)
}
type StaticMediaHistoryJournal interface {
	LoadStaticMediaHistory(context.Context, string, string) (StaticMediaHistoryReceipt, bool, error)
	RecordStaticMediaHistory(context.Context, StaticMediaHistoryReceipt) error
}
type StaticMediaHistoryReader interface {
	GetHistoricalGroupInvite(context.Context, int64) (HistoricalGroupInvite, error)
	ListHistoricalGroupInvite(context.Context, StaticMediaHistoryQuery) ([]HistoricalGroupInvite, int64, error)
}
