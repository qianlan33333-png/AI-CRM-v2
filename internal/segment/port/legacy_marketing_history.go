package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrLegacyMarketingHistoryInvalid     = errors.New("invalid legacy marketing history")
	ErrLegacyMarketingHistoryConflict    = errors.New("legacy marketing history conflict")
	ErrLegacyMarketingHistoryUnavailable = errors.New("legacy marketing history unavailable")
)

// Immutable observations of V1 marketing_state_current and marketing_value_segment_current.
// Source references never activate V2 segments, automation, or customer bindings.
// Private identity and JSON digests remain bound to the complete encrypted archive.
type HistoricalLegacyMarketingState struct {
	ID                   int64      `json:"id"`
	SourceKeyDigest      [32]byte   `json:"-"`
	SourcePayloadDigest  [32]byte   `json:"-"`
	SourceFieldDigest    [32]byte   `json:"-"`
	SourceID             int64      `json:"source_id"`
	ExternalUserIDDigest [32]byte   `json:"-"`
	ScenarioKey          string     `json:"scenario_key"`
	MarketingPhase       string     `json:"marketing_phase"`
	PhaseLabel           string     `json:"phase_label"`
	PhaseReason          string     `json:"phase_reason"`
	LifecycleStatus      string     `json:"lifecycle_status"`
	LastBatchSourceID    *int64     `json:"-"`
	LastBatchStatus      string     `json:"last_batch_status"`
	LastBatchWindowStart string     `json:"last_batch_window_start"`
	LastBatchWindowEnd   string     `json:"last_batch_window_end"`
	LastTriggerMessageAt string     `json:"last_trigger_message_at"`
	EnteredAt            *time.Time `json:"entered_at"`
	ExitedAt             *time.Time `json:"exited_at"`
	ExitReason           string     `json:"exit_reason"`
	StatePayloadDigest   [32]byte   `json:"-"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type HistoricalLegacyMarketingValue struct {
	ID                   int64     `json:"id"`
	SourceKeyDigest      [32]byte  `json:"-"`
	SourcePayloadDigest  [32]byte  `json:"-"`
	SourceFieldDigest    [32]byte  `json:"-"`
	SourceID             int64     `json:"source_id"`
	ExternalUserIDDigest [32]byte  `json:"-"`
	ScenarioKey          string    `json:"scenario_key"`
	ValueSegment         string    `json:"value_segment"`
	SegmentLabel         string    `json:"segment_label"`
	Score                int64     `json:"score"`
	ScoreBreakdownDigest [32]byte  `json:"-"`
	StatePayloadDigest   [32]byte  `json:"-"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type LegacyMarketingHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

type LegacyMarketingHistoryStore interface {
	CreateHistoricalLegacyMarketingState(context.Context, HistoricalLegacyMarketingState) (HistoricalLegacyMarketingState, error)
	GetHistoricalLegacyMarketingState(context.Context, int64) (HistoricalLegacyMarketingState, error)
	CreateHistoricalLegacyMarketingValue(context.Context, HistoricalLegacyMarketingValue) (HistoricalLegacyMarketingValue, error)
	GetHistoricalLegacyMarketingValue(context.Context, int64) (HistoricalLegacyMarketingValue, error)
}

type LegacyMarketingHistoryJournal interface {
	LoadLegacyMarketingHistory(context.Context, string, string) (LegacyMarketingHistoryReceipt, bool, error)
	RecordLegacyMarketingHistory(context.Context, LegacyMarketingHistoryReceipt) error
}

type LegacyMarketingHistoryQuery struct{ Limit, Offset int32 }

type LegacyMarketingHistoryReader interface {
	GetHistoricalLegacyMarketingState(context.Context, int64) (HistoricalLegacyMarketingState, error)
	ListHistoricalLegacyMarketingState(context.Context, LegacyMarketingHistoryQuery) ([]HistoricalLegacyMarketingState, int64, error)
	GetHistoricalLegacyMarketingValue(context.Context, int64) (HistoricalLegacyMarketingValue, error)
	ListHistoricalLegacyMarketingValue(context.Context, LegacyMarketingHistoryQuery) ([]HistoricalLegacyMarketingValue, int64, error)
}
