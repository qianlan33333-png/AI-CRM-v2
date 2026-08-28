package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrMarketingStateHistoryInvalid     = errors.New("invalid marketing state history")
	ErrMarketingStateHistoryConflict    = errors.New("marketing state history conflict")
	ErrMarketingStateHistoryUnavailable = errors.New("marketing state history unavailable")
)

// Immutable V1 observations, never current segments, scores, customer bindings or triggers.
// Source IDs are signed historical values. Private fields stay out of API JSON,
// but every field must be included in target digests used by import/reconcile.

type HistoricalMarketingStateSnapshot struct {
	ID                     int64      `json:"id"`
	SourceKeyDigest        [32]byte   `json:"-"`
	SourcePayloadDigest    [32]byte   `json:"-"`
	SourceFieldDigest      [32]byte   `json:"-"`
	SourceID               int64      `json:"source_id"`
	PersonSourceID         *int64     `json:"-"`
	ExternalUserIDDigest   [32]byte   `json:"-"`
	AutomationKey          string     `json:"automation_key"`
	MainStage              string     `json:"main_stage"`
	SubStage               string     `json:"sub_stage"`
	Activated              bool       `json:"activated"`
	Converted              bool       `json:"converted"`
	EligibleForConversion  bool       `json:"eligible_for_conversion"`
	LifecycleStatus        string     `json:"lifecycle_status"`
	LastActivationAt       string     `json:"last_activation_at"`
	LastConversionMarkedAt string     `json:"last_conversion_marked_at"`
	LastMessageAt          string     `json:"last_message_at"`
	LastBatchSourceID      *int64     `json:"-"`
	LastBatchStatus        string     `json:"last_batch_status"`
	LastBatchWindowStart   string     `json:"last_batch_window_start"`
	LastBatchWindowEnd     string     `json:"last_batch_window_end"`
	LastTriggerMessageAt   string     `json:"last_trigger_message_at"`
	EnteredAt              *time.Time `json:"entered_at"`
	ExitedAt               *time.Time `json:"exited_at"`
	ExitReason             string     `json:"exit_reason"`
	StatePayloadDigest     [32]byte   `json:"-"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type HistoricalMarketingStateChange struct {
	ID                     int64     `json:"id"`
	SourceKeyDigest        [32]byte  `json:"-"`
	SourcePayloadDigest    [32]byte  `json:"-"`
	SourceFieldDigest      [32]byte  `json:"-"`
	SourceID               int64     `json:"source_id"`
	PersonSourceID         *int64    `json:"-"`
	BatchSourceID          *int64    `json:"-"`
	ExternalUserIDDigest   [32]byte  `json:"-"`
	AutomationKey          string    `json:"automation_key"`
	MainStage              string    `json:"main_stage"`
	SubStage               string    `json:"sub_stage"`
	Activated              bool      `json:"activated"`
	Converted              bool      `json:"converted"`
	EligibleForConversion  bool      `json:"eligible_for_conversion"`
	LifecycleStatus        string    `json:"lifecycle_status"`
	LastActivationAt       string    `json:"last_activation_at"`
	LastConversionMarkedAt string    `json:"last_conversion_marked_at"`
	LastMessageAt          string    `json:"last_message_at"`
	ExitReason             string    `json:"exit_reason"`
	ChangeReason           string    `json:"change_reason"`
	StatePayloadDigest     [32]byte  `json:"-"`
	RecordedAt             time.Time `json:"recorded_at"`
	CreatedAt              time.Time `json:"created_at"`
}

type HistoricalValueSegmentSnapshot struct {
	ID                       int64     `json:"id"`
	SourceKeyDigest          [32]byte  `json:"-"`
	SourcePayloadDigest      [32]byte  `json:"-"`
	SourceFieldDigest        [32]byte  `json:"-"`
	SourceID                 int64     `json:"source_id"`
	ExternalUserIDDigest     [32]byte  `json:"-"`
	Segment                  string    `json:"segment"`
	SegmentRank              int32     `json:"segment_rank"`
	Score                    int32     `json:"score"`
	ScoringVersion           string    `json:"scoring_version"`
	SubmissionSourceID       *int64    `json:"-"`
	MatchedQuestionIDsDigest [32]byte  `json:"-"`
	StatePayloadDigest       [32]byte  `json:"-"`
	ComputedReason           string    `json:"computed_reason"`
	EvaluatedAt              time.Time `json:"evaluated_at"`
	ComputedAt               time.Time `json:"computed_at"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type HistoricalValueSegmentChange struct {
	ID                       int64     `json:"id"`
	SourceKeyDigest          [32]byte  `json:"-"`
	SourcePayloadDigest      [32]byte  `json:"-"`
	SourceFieldDigest        [32]byte  `json:"-"`
	SourceID                 int64     `json:"source_id"`
	ExternalUserIDDigest     [32]byte  `json:"-"`
	Segment                  string    `json:"segment"`
	SegmentRank              int32     `json:"segment_rank"`
	Score                    int32     `json:"score"`
	ScoringVersion           string    `json:"scoring_version"`
	SubmissionSourceID       *int64    `json:"-"`
	MatchedQuestionIDsDigest [32]byte  `json:"-"`
	StatePayloadDigest       [32]byte  `json:"-"`
	ChangeReason             string    `json:"change_reason"`
	EvaluatedAt              time.Time `json:"evaluated_at"`
	RecordedAt               time.Time `json:"recorded_at"`
	CreatedAt                time.Time `json:"created_at"`
}

type MarketingStateHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

type MarketingStateHistoryStore interface {
	CreateHistoricalMarketingStateSnapshot(context.Context, HistoricalMarketingStateSnapshot) (HistoricalMarketingStateSnapshot, error)
	GetHistoricalMarketingStateSnapshot(context.Context, int64) (HistoricalMarketingStateSnapshot, error)
	CreateHistoricalMarketingStateChange(context.Context, HistoricalMarketingStateChange) (HistoricalMarketingStateChange, error)
	GetHistoricalMarketingStateChange(context.Context, int64) (HistoricalMarketingStateChange, error)
	CreateHistoricalValueSegmentSnapshot(context.Context, HistoricalValueSegmentSnapshot) (HistoricalValueSegmentSnapshot, error)
	GetHistoricalValueSegmentSnapshot(context.Context, int64) (HistoricalValueSegmentSnapshot, error)
	CreateHistoricalValueSegmentChange(context.Context, HistoricalValueSegmentChange) (HistoricalValueSegmentChange, error)
	GetHistoricalValueSegmentChange(context.Context, int64) (HistoricalValueSegmentChange, error)
}

type MarketingStateHistoryJournal interface {
	LoadMarketingStateHistory(context.Context, string, string) (MarketingStateHistoryReceipt, bool, error)
	RecordMarketingStateHistory(context.Context, MarketingStateHistoryReceipt) error
}

type MarketingStateHistoryQuery struct{ Limit, Offset int32 }

type MarketingStateHistoryReader interface {
	GetHistoricalMarketingStateSnapshot(context.Context, int64) (HistoricalMarketingStateSnapshot, error)
	ListHistoricalMarketingStateSnapshot(context.Context, MarketingStateHistoryQuery) ([]HistoricalMarketingStateSnapshot, int64, error)
	GetHistoricalMarketingStateChange(context.Context, int64) (HistoricalMarketingStateChange, error)
	ListHistoricalMarketingStateChange(context.Context, MarketingStateHistoryQuery) ([]HistoricalMarketingStateChange, int64, error)
	GetHistoricalValueSegmentSnapshot(context.Context, int64) (HistoricalValueSegmentSnapshot, error)
	ListHistoricalValueSegmentSnapshot(context.Context, MarketingStateHistoryQuery) ([]HistoricalValueSegmentSnapshot, int64, error)
	GetHistoricalValueSegmentChange(context.Context, int64) (HistoricalValueSegmentChange, error)
	ListHistoricalValueSegmentChange(context.Context, MarketingStateHistoryQuery) ([]HistoricalValueSegmentChange, int64, error)
}
