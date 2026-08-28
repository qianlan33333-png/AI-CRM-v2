package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrAutomationHistoryInvalid     = errors.New("invalid automation history")
	ErrAutomationHistoryConflict    = errors.New("automation history conflict")
	ErrAutomationHistoryUnavailable = errors.New("automation history unavailable")
)

// HistoricalAutomationIdentity belongs only to frozen V1 history. SourceID is
// never an ID for a current V2 automation, agent, workflow, or permission.
type HistoricalAutomationIdentity struct {
	ID                  int64    `json:"id"`
	SourceID            int64    `json:"source_id"`
	SourceKeyDigest     [32]byte `json:"source_key_digest"`
	SourcePayloadDigest [32]byte `json:"source_payload_digest"`
}

type HistoricalAutomationSOP struct {
	HistoricalAutomationIdentity
	PoolKey         string    `json:"pool_key"`
	DayIndex        int32     `json:"day_index"`
	ContentMasked   string    `json:"content_masked"`
	ImagesDigest    [32]byte  `json:"images_digest"`
	OriginalEnabled bool      `json:"original_enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type HistoricalAutomationConfig struct {
	HistoricalAutomationIdentity
	AgentCode           string    `json:"agent_code"`
	DisplayName         string    `json:"display_name"`
	ScenarioCode        string    `json:"scenario_code"`
	OriginalEnabled     bool      `json:"original_enabled"`
	DraftVersion        int32     `json:"draft_version"`
	PublishedVersion    int32     `json:"published_version"`
	PublishedAt         string    `json:"published_at"`
	LastModifiedAt      string    `json:"last_modified_at"`
	LastModifiedSource  string    `json:"last_modified_source"`
	SubmittedForPublish bool      `json:"submitted_for_publish"`
	SubmittedAt         string    `json:"submitted_at"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	ActorsDigest        [32]byte  `json:"actors_digest"`
	ConfigDigest        [32]byte  `json:"config_digest"`
}

type HistoricalAutomationPrompt struct {
	HistoricalAutomationIdentity
	AgentCode       string    `json:"agent_code"`
	DisplayName     string    `json:"display_name"`
	OriginalEnabled bool      `json:"original_enabled"`
	Version         int32     `json:"version"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	PromptDigest    [32]byte  `json:"prompt_digest"`
}

type HistoricalAutomationAgent struct {
	HistoricalAutomationIdentity
	ProgramSourceID     int64     `json:"program_source_id"`
	WorkflowSourceID    int64     `json:"workflow_source_id"`
	NodeSourceID        int64     `json:"node_source_id"`
	TaskSourceID        int64     `json:"task_source_id"`
	AgentCode           string    `json:"agent_code"`
	AgentName           string    `json:"agent_name"`
	OriginalType        string    `json:"original_type"`
	OriginalStatus      string    `json:"original_status"`
	SortOrder           int32     `json:"sort_order"`
	OriginalEnabled     bool      `json:"original_enabled"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	ArchivedAt          string    `json:"archived_at"`
	ActorsDigest        [32]byte  `json:"actors_digest"`
	ConfigurationDigest [32]byte  `json:"configuration_digest"`
}

const (
	AutomationHistorySOP    = "sop"
	AutomationHistoryConfig = "config"
	AutomationHistoryPrompt = "prompt"
	AutomationHistoryAgent  = "agent"
)

type AutomationHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

// Store and Journal must share the caller transaction. There is no publish,
// execution, scheduler, LLM, event, or external-effect operation in this port.
type AutomationHistoryStore interface {
	CreateHistoricalAutomationSOP(context.Context, HistoricalAutomationSOP) (HistoricalAutomationSOP, error)
	GetHistoricalAutomationSOP(context.Context, int64) (HistoricalAutomationSOP, error)
	CreateHistoricalAutomationConfig(context.Context, HistoricalAutomationConfig) (HistoricalAutomationConfig, error)
	GetHistoricalAutomationConfig(context.Context, int64) (HistoricalAutomationConfig, error)
	CreateHistoricalAutomationPrompt(context.Context, HistoricalAutomationPrompt) (HistoricalAutomationPrompt, error)
	GetHistoricalAutomationPrompt(context.Context, int64) (HistoricalAutomationPrompt, error)
	CreateHistoricalAutomationAgent(context.Context, HistoricalAutomationAgent) (HistoricalAutomationAgent, error)
	GetHistoricalAutomationAgent(context.Context, int64) (HistoricalAutomationAgent, error)
}

type AutomationHistoryJournal interface {
	LoadAutomationHistory(context.Context, string, string) (AutomationHistoryReceipt, bool, error)
	RecordAutomationHistory(context.Context, AutomationHistoryReceipt) error
}

type AutomationHistoryQuery struct{ Limit, Offset int32 }

type AutomationHistoryReader interface {
	GetHistoricalAutomationSOP(context.Context, int64) (HistoricalAutomationSOP, error)
	ListHistoricalAutomationSOPs(context.Context, AutomationHistoryQuery) ([]HistoricalAutomationSOP, int64, error)
	GetHistoricalAutomationConfig(context.Context, int64) (HistoricalAutomationConfig, error)
	ListHistoricalAutomationConfigs(context.Context, AutomationHistoryQuery) ([]HistoricalAutomationConfig, int64, error)
	GetHistoricalAutomationPrompt(context.Context, int64) (HistoricalAutomationPrompt, error)
	ListHistoricalAutomationPrompts(context.Context, AutomationHistoryQuery) ([]HistoricalAutomationPrompt, int64, error)
	GetHistoricalAutomationAgent(context.Context, int64) (HistoricalAutomationAgent, error)
	ListHistoricalAutomationAgents(context.Context, AutomationHistoryQuery) ([]HistoricalAutomationAgent, int64, error)
}
