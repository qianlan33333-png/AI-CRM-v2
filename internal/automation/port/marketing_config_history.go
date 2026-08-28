package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrMarketingConfigHistoryInvalid     = errors.New("invalid automation history")
	ErrMarketingConfigHistoryConflict    = errors.New("automation history conflict")
	ErrMarketingConfigHistoryUnavailable = errors.New("automation history unavailable")
)

// Immutable source history. Source IDs are not current V2 IDs; private digests
// bind all original fields without exposing identities or replaying execution.
type HistoricalMarketingAutomationConfig struct {
	ID                  int64     `json:"id"`
	SourceKeyDigest     [32]byte  `json:"-"`
	SourcePayloadDigest [32]byte  `json:"-"`
	SourceFieldDigest   [32]byte  `json:"-"`
	SourceID            int64     `json:"source_id"`
	AutomationKey       string    `json:"automation_key"`
	AutomationName      string    `json:"automation_name"`
	TargetEvent         string    `json:"target_event"`
	ChannelType         string    `json:"channel_type"`
	OriginalStatus      string    `json:"original_status"`
	DoNotStartAfterHour int32     `json:"do_not_start_after_hour"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	ConfigPayloadDigest [32]byte  `json:"-"`
}

type HistoricalMarketingAutomationRule struct {
	ID                     int64     `json:"id"`
	SourceKeyDigest        [32]byte  `json:"-"`
	SourcePayloadDigest    [32]byte  `json:"-"`
	SourceFieldDigest      [32]byte  `json:"-"`
	SourceID               int64     `json:"source_id"`
	ConfigID               int64     `json:"config_id"`
	ConfigSourceID         int64     `json:"config_source_id"`
	QuestionnaireSourceID  *int64    `json:"questionnaire_source_id"`
	QuestionSourceID       *int64    `json:"question_source_id"`
	RuleCode               string    `json:"rule_code"`
	RuleName               string    `json:"rule_name"`
	AnswerMatchType        string    `json:"answer_match_type"`
	ScoreDelta             int32     `json:"score_delta"`
	SegmentHint            string    `json:"segment_hint"`
	StageHint              string    `json:"stage_hint"`
	OriginalActive         bool      `json:"original_active"`
	SortOrder              int32     `json:"sort_order"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
	AnswerMatchValueDigest [32]byte  `json:"-"`
	RulePayloadDigest      [32]byte  `json:"-"`
}

type MarketingConfigHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}
type MarketingConfigHistoryQuery struct{ Limit, Offset int32 }
type MarketingConfigHistoryStore interface {
	CreateHistoricalMarketingAutomationConfig(context.Context, HistoricalMarketingAutomationConfig) (HistoricalMarketingAutomationConfig, error)
	GetHistoricalMarketingAutomationConfig(context.Context, int64) (HistoricalMarketingAutomationConfig, error)
	CreateHistoricalMarketingAutomationRule(context.Context, HistoricalMarketingAutomationRule) (HistoricalMarketingAutomationRule, error)
	GetHistoricalMarketingAutomationRule(context.Context, int64) (HistoricalMarketingAutomationRule, error)
}
type MarketingConfigHistoryJournal interface {
	LoadMarketingConfigHistory(context.Context, string, string) (MarketingConfigHistoryReceipt, bool, error)
	RecordMarketingConfigHistory(context.Context, MarketingConfigHistoryReceipt) error
}
type MarketingConfigHistoryReader interface {
	GetHistoricalMarketingAutomationConfig(context.Context, int64) (HistoricalMarketingAutomationConfig, error)
	ListHistoricalMarketingAutomationConfig(context.Context, MarketingConfigHistoryQuery) ([]HistoricalMarketingAutomationConfig, int64, error)
	GetHistoricalMarketingAutomationRule(context.Context, int64) (HistoricalMarketingAutomationRule, error)
	ListHistoricalMarketingAutomationRule(context.Context, MarketingConfigHistoryQuery) ([]HistoricalMarketingAutomationRule, int64, error)
}
