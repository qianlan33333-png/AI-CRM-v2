package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrOutboundTaskHistoryInvalid     = errors.New("invalid outbound task history")
	ErrOutboundTaskHistoryConflict    = errors.New("outbound task history conflict")
	ErrOutboundTaskHistoryUnavailable = errors.New("outbound task history unavailable")
)

// HistoricalOutboundTask is an immutable V1 observation, never a runnable V2
// task. Digests cover archived bytes, including any prior archive redactions.
type HistoricalOutboundTask struct {
	ID                    int64     `json:"id"`
	SourceID              int64     `json:"source_id"`
	TaskType              string    `json:"task_type"`
	Status                string    `json:"status"`
	CreatedAt             time.Time `json:"created_at"`
	BroadcastJobHistoryID *int64    `json:"broadcast_job_history_id"`
	RequestPayloadDigest  [32]byte  `json:"-"`
	ResponsePayloadDigest [32]byte  `json:"-"`
	WeComTaskIDDigest     *[32]byte `json:"-"`
	TraceIDDigest         [32]byte  `json:"-"`
	LegacyBroadcastJobID  *int64    `json:"-"`
	SourceKeyDigest       [32]byte  `json:"-"`
	SourcePayloadDigest   [32]byte  `json:"-"`
	SourceFieldDigest     [32]byte  `json:"-"`
	RedactedRoots         []string  `json:"-"`
}

// OutboundTaskHistoryParent contains only the historical pointers needed to
// prove a reciprocal relation. Neither source ID implies a current V2 task.
type OutboundTaskHistoryParent struct {
	ID                   int64
	SourceID             int64
	LegacyOutboundTaskID *int64
}

type OutboundTaskHistoryReceipt struct {
	SourceIdentifier            string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

type OutboundTaskHistoryStore interface {
	CreateHistoricalOutboundTask(context.Context, HistoricalOutboundTask) (HistoricalOutboundTask, error)
	GetHistoricalOutboundTask(context.Context, int64) (HistoricalOutboundTask, error)
	LookupOutboundTaskHistoryParents(context.Context, int64) ([]OutboundTaskHistoryParent, error)
}

type OutboundTaskHistoryJournal interface {
	LoadOutboundTaskHistory(context.Context, string) (OutboundTaskHistoryReceipt, bool, error)
	RecordOutboundTaskHistory(context.Context, OutboundTaskHistoryReceipt) error
}

type OutboundTaskHistoryQuery struct{ Limit, Offset int32 }

type OutboundTaskHistoryReader interface {
	GetHistoricalOutboundTask(context.Context, int64) (HistoricalOutboundTask, error)
	ListHistoricalOutboundTasks(context.Context, OutboundTaskHistoryQuery) ([]HistoricalOutboundTask, int64, error)
}
