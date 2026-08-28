package port

import (
	"context"
	"time"
)

// Immutable source observations, never current sender authority or send work.
// Private/source digests are for migration readback only and cannot be serialized.
type HistoricalHXCRuntimeIdentity struct {
	ID                  int64    `json:"id"`
	SourceID            int64    `json:"source_id"`
	SourceKeyDigest     [32]byte `json:"-"`
	SourcePayloadDigest [32]byte `json:"-"`
	SourceFieldDigest   [32]byte `json:"-"`
	PrivateDigest       [32]byte `json:"-"`
}

type HistoricalHXCSenderConfig struct {
	HistoricalHXCRuntimeIdentity
	Priority         int64     `json:"priority"`
	OriginalIsActive bool      `json:"original_is_active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type HistoricalHXCSendRecord struct {
	HistoricalHXCRuntimeIdentity
	TaskType            string     `json:"task_type"`
	OriginalStatus      string     `json:"original_status"`
	SelectedCount       int64      `json:"selected_count"`
	EligibleCount       int64      `json:"eligible_count"`
	SentCount           int64      `json:"sent_count"`
	SkippedCount        int64      `json:"skipped_count"`
	PlannedCount        int64      `json:"planned_count"`
	QueuedCount         int64      `json:"queued_count"`
	DispatchingCount    int64      `json:"dispatching_count"`
	SucceededCount      int64      `json:"succeeded_count"`
	FailedCount         int64      `json:"failed_count"`
	BlockedCount        int64      `json:"blocked_count"`
	CancelledCount      int64      `json:"cancelled_count"`
	ImageCount          int64      `json:"image_count"`
	IncludeDoNotDisturb bool       `json:"include_do_not_disturb"`
	TargetSource        string     `json:"target_source"`
	TargetSourceID      *int64     `json:"target_source_id"`
	CreatedAt           time.Time  `json:"created_at"`
	LastStatusSyncAt    *time.Time `json:"last_status_sync_at"`
	LastRefreshedAt     *time.Time `json:"last_refreshed_at"`
}

const (
	HXCHistorySenderConfig = "sender_config"
	HXCHistorySendRecord   = "send_record"
)

// Writes and the scoped HXCHistoryJournal share the caller transaction.
type HXCRuntimeHistoryStore interface {
	CreateHistoricalHXCSenderConfig(context.Context, HistoricalHXCSenderConfig) (HistoricalHXCSenderConfig, error)
	GetHistoricalHXCSenderConfig(context.Context, int64) (HistoricalHXCSenderConfig, error)
	CreateHistoricalHXCSendRecord(context.Context, HistoricalHXCSendRecord) (HistoricalHXCSendRecord, error)
	GetHistoricalHXCSendRecord(context.Context, int64) (HistoricalHXCSendRecord, error)
}

type HXCRuntimeHistoryReader interface {
	GetHistoricalHXCSenderConfig(context.Context, int64) (HistoricalHXCSenderConfig, error)
	ListHistoricalHXCSenderConfig(context.Context, HXCHistoryQuery) ([]HistoricalHXCSenderConfig, int64, error)
	GetHistoricalHXCSendRecord(context.Context, int64) (HistoricalHXCSendRecord, error)
	ListHistoricalHXCSendRecord(context.Context, HXCHistoryQuery) ([]HistoricalHXCSendRecord, int64, error)
}
