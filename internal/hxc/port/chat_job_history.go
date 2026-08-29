package port

import (
	"context"
	"encoding/json"
	"time"
)

// HistoricalHXCChatJob preserves a sealed observation, never executable work.
// Source queue/member/send-record IDs do not identify current V2 objects.
type HistoricalHXCChatJob struct {
	ID                  int64           `json:"id"`
	SourceID            int64           `json:"source_id"`
	SourceKeyDigest     [32]byte        `json:"-"`
	SourcePayloadDigest [32]byte        `json:"-"`
	SourceFieldDigest   [32]byte        `json:"-"`
	QueueSourceID       *int64          `json:"queue_source_id"`
	MemberSourceID      *int64          `json:"member_source_id"`
	ExternalContactID   string          `json:"-"`
	Phone               string          `json:"-"`
	ExternalMessageID   string          `json:"-"`
	ExternalSessionID   string          `json:"-"`
	LaohuangTaskID      string          `json:"-"`
	RequestPayloadJSON  json.RawMessage `json:"-"`
	AcceptedPayloadJSON json.RawMessage `json:"-"`
	CallbackPayloadJSON json.RawMessage `json:"-"`
	OriginalStatus      string          `json:"original_status"`
	ReplyText           string          `json:"-"`
	ErrorCode           string          `json:"-"`
	ErrorMessage        string          `json:"-"`
	SendChannel         string          `json:"send_channel"`
	SendRecordSourceID  *int64          `json:"send_record_source_id"`
	SendResultJSON      json.RawMessage `json:"-"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	FinishedAtSource    string          `json:"finished_at_source"`
}

const HXCHistoryChatJob = "chat_job"

type HXCChatJobHistoryQuery struct{ Limit, Offset int32 }

// Writes share the caller transaction with the existing HXCHistoryJournal.
type HXCChatJobHistoryStore interface {
	CreateHistoricalHXCChatJob(context.Context, HistoricalHXCChatJob) (HistoricalHXCChatJob, error)
	GetHistoricalHXCChatJob(context.Context, int64) (HistoricalHXCChatJob, error)
}

type HXCChatJobHistoryReader interface {
	GetHistoricalHXCChatJob(context.Context, int64) (HistoricalHXCChatJob, error)
	ListHistoricalHXCChatJob(context.Context, HXCChatJobHistoryQuery) ([]HistoricalHXCChatJob, int64, error)
}
