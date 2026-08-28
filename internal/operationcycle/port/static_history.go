package port

import (
	"context"
	"errors"
	"time"
)

// Immutable V1 facts only. Source references are not current V2 identifiers.
var (
	ErrStaticCycleHistoryInvalid     = errors.New("invalid operationcycle static history")
	ErrStaticCycleHistoryConflict    = errors.New("operationcycle static history conflict")
	ErrStaticCycleHistoryUnavailable = errors.New("operationcycle static history unavailable")
)

type HistoricalCycleStrategy struct {
	ID                  int64     `json:"id"`
	SourceID            int64     `json:"source_id"`
	SourceKeyDigest     [32]byte  `json:"source_key_digest"`
	SourcePayloadDigest [32]byte  `json:"source_payload_digest"`
	StrategyKey         string    `json:"strategy_key"`
	Title               string    `json:"title"`
	Description         string    `json:"description"`
	Cadence             string    `json:"cadence"`
	Timezone            string    `json:"timezone"`
	OriginalStatus      string    `json:"original_status"`
	CurrentVersion      int64     `json:"current_version"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type HistoricalCycleVersion struct {
	ID                  int64      `json:"id"`
	SourceID            int64      `json:"source_id"`
	SourceKeyDigest     [32]byte   `json:"source_key_digest"`
	SourcePayloadDigest [32]byte   `json:"source_payload_digest"`
	StrategySourceID    int64      `json:"strategy_source_id"`
	StrategyHistoryID   int64      `json:"strategy_history_id"`
	Version             int64      `json:"version"`
	Label               string     `json:"label"`
	Objective           string     `json:"objective"`
	VersionHash         string     `json:"version_hash"`
	EffectiveFrom       *time.Time `json:"effective_from"`
	OriginalGovernance  string     `json:"original_governance"`
	ConfirmedAt         *time.Time `json:"confirmed_at"`
	OperationSkillHash  string     `json:"operation_skill_hash"`
	CreatedAt           time.Time  `json:"created_at"`
}

type HistoricalCycleDocument struct {
	ID                          int64      `json:"id"`
	SourceID                    int64      `json:"source_id"`
	SourceKeyDigest             [32]byte   `json:"source_key_digest"`
	SourcePayloadDigest         [32]byte   `json:"source_payload_digest"`
	StrategyVersionSourceID     int64      `json:"strategy_version_source_id"`
	VersionHistoryID            int64      `json:"version_history_id"`
	SchemaVersion               string     `json:"schema_version"`
	ExecutionGuideSHA256        string     `json:"execution_guide_sha256"`
	ExecutionGuideGeneratedAt   *time.Time `json:"execution_guide_generated_at"`
	CopyGuideSHA256             string     `json:"copy_guide_sha256"`
	CopyGuideGeneratedAt        *time.Time `json:"copy_guide_generated_at"`
	MeasurementGuideSHA256      string     `json:"measurement_guide_sha256"`
	MeasurementGuideGeneratedAt *time.Time `json:"measurement_guide_generated_at"`
	DocumentPackHash            string     `json:"document_pack_hash"`
	CreatedAt                   time.Time  `json:"created_at"`
}

type StaticCycleHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}
type StaticCycleHistoryQuery struct {
	StrategyHistoryID, VersionHistoryID *int64
	Limit, Offset                       int32
}

// Writes and journal receipts must share the caller transaction.
type StaticCycleHistoryStore interface {
	CreateHistoricalCycleStrategy(context.Context, HistoricalCycleStrategy) (HistoricalCycleStrategy, error)
	GetHistoricalCycleStrategy(context.Context, int64) (HistoricalCycleStrategy, error)
	CreateHistoricalCycleVersion(context.Context, HistoricalCycleVersion) (HistoricalCycleVersion, error)
	GetHistoricalCycleVersion(context.Context, int64) (HistoricalCycleVersion, error)
	CreateHistoricalCycleDocument(context.Context, HistoricalCycleDocument) (HistoricalCycleDocument, error)
	GetHistoricalCycleDocument(context.Context, int64) (HistoricalCycleDocument, error)
}
type StaticCycleHistoryJournal interface {
	LoadStaticCycleHistory(context.Context, string, string) (StaticCycleHistoryReceipt, bool, error)
	RecordStaticCycleHistory(context.Context, StaticCycleHistoryReceipt) error
}
type StaticCycleHistoryReader interface {
	GetHistoricalCycleStrategy(context.Context, int64) (HistoricalCycleStrategy, error)
	ListHistoricalCycleStrategy(context.Context, StaticCycleHistoryQuery) ([]HistoricalCycleStrategy, int64, error)
	GetHistoricalCycleVersion(context.Context, int64) (HistoricalCycleVersion, error)
	ListHistoricalCycleVersion(context.Context, StaticCycleHistoryQuery) ([]HistoricalCycleVersion, int64, error)
	GetHistoricalCycleDocument(context.Context, int64) (HistoricalCycleDocument, error)
	ListHistoricalCycleDocument(context.Context, StaticCycleHistoryQuery) ([]HistoricalCycleDocument, int64, error)
}
