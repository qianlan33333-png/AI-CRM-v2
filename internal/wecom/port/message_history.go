package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrMessageHistoryInvalid     = errors.New("invalid message history")
	ErrMessageHistoryConflict    = errors.New("message history conflict")
	ErrMessageHistoryUnavailable = errors.New("message history unavailable")
)

// HistoricalMessage is a separate, non-executable V1 projection. Only masked
// text is exposed; raw body, provider identities and raw payload remain in the
// encrypted archive bound by SourcePayloadDigest. CustomerID is a verified
// historical DM01 crosswalk, never a fresh Provider identity assertion.
// OriginalSendTime retains the source clock text. civil_unzoned never acquires
// an inferred timezone and therefore has no SentAt instant.
type HistoricalMessage struct {
	ID                  int64      `json:"id"`
	SourceID            int64      `json:"source_id"`
	Sequence            *int64     `json:"sequence"`
	CustomerID          *int64     `json:"customer_id"`
	ChatType            string     `json:"chat_type"`
	MessageType         string     `json:"message_type"`
	ContentMasked       *string    `json:"content_masked"`
	OriginalSendTime    string     `json:"original_send_time"`
	SendTimeBasis       string     `json:"send_time_basis"`
	SentAt              *time.Time `json:"sent_at"`
	CreatedAt           time.Time  `json:"created_at"`
	SourcePayloadDigest [32]byte   `json:"source_payload_digest"`
}

type MessageHistoryReceipt struct {
	SourceIdentifier string
	PayloadDigest    [32]byte
	TargetID         int64
	TargetDigest     [32]byte
	Replayed         bool
}

// Store and journal use the caller transaction; neither owns an event or job.
type MessageHistoryStore interface {
	CreateHistoricalMessage(context.Context, HistoricalMessage) (HistoricalMessage, error)
	GetHistoricalMessage(context.Context, int64) (HistoricalMessage, error)
}

type MessageHistoryJournal interface {
	LoadMessageHistory(context.Context, string) (MessageHistoryReceipt, bool, error)
	RecordMessageHistory(context.Context, MessageHistoryReceipt) error
}

type MessageHistoryQuery struct {
	CustomerID    *int64
	ChatType      string
	Limit, Offset int32
}

type MessageHistoryReader interface {
	GetHistoricalMessage(context.Context, int64) (HistoricalMessage, error)
	ListHistoricalMessages(context.Context, MessageHistoryQuery) ([]HistoricalMessage, int64, error)
}
