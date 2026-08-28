package port

import (
	"context"
	"time"
)

// These observations preserve non-current V1 definitions, not executable
// Campaigns. Source IDs and statuses are not V2 command or lifecycle inputs.
// Digest fields remain private to migration/reconciliation, not HTTP DTOs.
type HistoricalCampaignDefinition struct {
	ID                  int64
	SourceID            int64
	Code                string
	DisplayName         string
	Intent              string
	AnchorMode          string
	AnchorDate          string
	ReviewStatus        string
	RunStatus           string
	ApprovedAt          *time.Time
	StartedAt           *time.Time
	FinishedAt          *time.Time
	PausedAt            *time.Time
	PausedReason        string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	OriginalDisposition string
	OriginalReason      string
	PrivateDigest       [32]byte
	SourceKeyDigest     [32]byte
	SourcePayloadDigest [32]byte
	SourceFieldDigest   [32]byte
}

// A step may reference a verified history header or an existing current
// definition. Missing source parents remain unlinked. Neither link permits
// executing the historical schedule or content.
type HistoricalCampaignDefinitionStep struct {
	ID                  int64
	SourceID            int64
	CampaignSourceID    int64
	SegmentSourceID     int64
	HistoryDefinitionID *int64
	CurrentCampaignID   *int64
	SourceParentState   string
	StepIndex           int32
	DayOffset           int32
	SendTime            string
	Timezone            string
	ContentMasked       string
	StopOnReply         bool
	SkipRecentDays      int32
	CreatedAt           time.Time
	UpdatedAt           time.Time
	OriginalDisposition string
	OriginalReason      string
	ContentDigest       [32]byte
	PrivateDigest       [32]byte
	SourceKeyDigest     [32]byte
	SourcePayloadDigest [32]byte
	SourceFieldDigest   [32]byte
}

// Store methods use the caller-bound transaction together with the generic
// migration journal. They never write current Campaigns, events or commands.
type CampaignDefinitionHistoryStore interface {
	CreateHistoricalCampaignDefinition(context.Context, HistoricalCampaignDefinition) (HistoricalCampaignDefinition, error)
	GetHistoricalCampaignDefinition(context.Context, int64) (HistoricalCampaignDefinition, error)
	CreateHistoricalCampaignDefinitionStep(context.Context, HistoricalCampaignDefinitionStep) (HistoricalCampaignDefinitionStep, error)
	GetHistoricalCampaignDefinitionStep(context.Context, int64) (HistoricalCampaignDefinitionStep, error)
}

type CampaignDefinitionHistoryReader interface {
	GetHistoricalCampaignDefinition(context.Context, int64) (HistoricalCampaignDefinition, error)
	ListHistoricalCampaignDefinitions(context.Context, int32, int32) ([]HistoricalCampaignDefinition, int64, error)
	// The optional filter is a V1 source campaign ID, never a current V2 ID.
	// An unfiltered list also exposes steps with current or missing parents.
	ListHistoricalCampaignDefinitionSteps(context.Context, *int64, int32, int32) ([]HistoricalCampaignDefinitionStep, int64, error)
}

type CampaignDefinitionHistoryJournal interface {
	LoadCampaignDefinitionHistory(context.Context, string, string) (CampaignHistoryReceipt, bool, error)
	RecordCampaignDefinitionHistory(context.Context, string, CampaignHistoryReceipt) error
}
