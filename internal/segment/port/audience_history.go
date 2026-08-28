package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrAudienceHistoryInvalid     = errors.New("invalid audience history")
	ErrAudienceHistoryConflict    = errors.New("audience history conflict")
	ErrAudienceHistoryUnavailable = errors.New("audience history unavailable")
)

// Historical facts never activate a Segment, evaluate a query, or grant access.
// Source IDs and original status/timestamps are not current V2 runtime state.
// CustomerID/StaffID only express a verified historical DM01 crosswalk, not
// a fresh Provider identity assertion. Raw identities and executable definitions
// remain in the existing encrypted source archive, outside these public models.

type HistoricalAudienceGroup struct {
	ID        int64     `json:"id"`
	SourceID  int64     `json:"source_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type HistoricalAudiencePackage struct {
	ID                        int64      `json:"id"`
	SourceID                  int64      `json:"source_id"`
	GroupHistoryID            *int64     `json:"group_history_id"`
	CurrentVersionSourceID    *int64     `json:"current_version_source_id"`
	PackageKey                string     `json:"package_key"`
	Name                      string     `json:"name"`
	NaturalLanguageDefinition string     `json:"natural_language_definition"`
	OriginalStatus            string     `json:"original_status"`
	QueryMode                 string     `json:"query_mode"`
	IdentityPolicy            string     `json:"identity_policy"`
	IncrementalEnabled        bool       `json:"incremental_enabled"`
	DailyEnabled              bool       `json:"daily_enabled"`
	IncrementalIntervalSecs   int64      `json:"incremental_interval_seconds"`
	DailyRefreshTime          string     `json:"daily_refresh_time"`
	Timezone                  string     `json:"timezone"`
	LookbackSecs              int64      `json:"lookback_seconds"`
	LastIncrementalAt         *time.Time `json:"last_incremental_at"`
	LastDailyRefreshedAt      *time.Time `json:"last_daily_refreshed_at"`
	NextIncrementalAt         *time.Time `json:"next_incremental_at"`
	NextDailyAt               *time.Time `json:"next_daily_at"`
	PausedReason              string     `json:"paused_reason"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
	RuntimeDigest             [32]byte   `json:"runtime_digest"`
}

type HistoricalAudienceVersion struct {
	ID                         int64      `json:"id"`
	SourceID                   int64      `json:"source_id"`
	PackageHistoryID           int64      `json:"package_history_id"`
	VersionNumber              int64      `json:"version_number"`
	OriginalStatus             string     `json:"original_status"`
	AIPrompt                   string     `json:"ai_prompt"`
	AIRationale                string     `json:"ai_rationale"`
	NaturalLanguageExplanation string     `json:"natural_language_explanation"`
	CreatedAt                  time.Time  `json:"created_at"`
	PublishedAt                *time.Time `json:"published_at"`
	TemplateKey                string     `json:"template_key"`
	TemplateVersion            *int64     `json:"template_version"`
	TemplateFingerprint        string     `json:"template_fingerprint"`
	DefinitionDigest           [32]byte   `json:"definition_digest"`
}

type HistoricalAudienceSender struct {
	ID               int64     `json:"id"`
	SourceID         int64     `json:"source_id"`
	PackageHistoryID int64     `json:"package_history_id"`
	StaffID          *int64    `json:"staff_id"`
	DisplayName      string    `json:"display_name"`
	Priority         int64     `json:"priority"`
	OriginalStatus   string    `json:"original_status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type HistoricalAudienceRule struct {
	ID             int64     `json:"id"`
	SourceID       int64     `json:"source_id"`
	RuleKey        string    `json:"rule_key"`
	DisplayName    string    `json:"display_name"`
	Description    string    `json:"description"`
	RuleType       string    `json:"rule_type"`
	OwnerStaffID   *int64    `json:"owner_staff_id"`
	OriginalStatus string    `json:"original_status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type HistoricalAudienceRuleVersion struct {
	ID               int64      `json:"id"`
	SourceID         int64      `json:"source_id"`
	RuleHistoryID    int64      `json:"rule_history_id"`
	Version          int64      `json:"version"`
	ExecutorType     string     `json:"executor_type"`
	OriginalStatus   string     `json:"original_status"`
	PublishedAt      *time.Time `json:"published_at"`
	CreatedAt        time.Time  `json:"created_at"`
	DefinitionDigest [32]byte   `json:"definition_digest"`
}

type HistoricalAudienceDefinition struct {
	ID               int64      `json:"id"`
	SourceID         int64      `json:"source_id"`
	Code             string     `json:"code"`
	DisplayName      string     `json:"display_name"`
	Description      string     `json:"description"`
	SourceType       string     `json:"source_type"`
	SQLDialect       string     `json:"sql_dialect"`
	OriginalStatus   string     `json:"original_status"`
	Version          int64      `json:"version"`
	CachedHeadcount  int64      `json:"cached_headcount"`
	LastRefreshedAt  *time.Time `json:"last_refreshed_at"`
	UsageCount       int64      `json:"usage_count"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DefinitionDigest [32]byte   `json:"definition_digest"`
}

type HistoricalAudienceMember struct {
	ID               int64      `json:"id"`
	SourceID         int64      `json:"source_id"`
	PackageHistoryID int64      `json:"package_history_id"`
	CustomerID       *int64     `json:"customer_id"`
	IdentityKind     string     `json:"identity_kind"`
	OriginalStatus   string     `json:"original_status"`
	FirstEnteredAt   time.Time  `json:"first_entered_at"`
	LastSeenAt       time.Time  `json:"last_seen_at"`
	LastUpdatedAt    time.Time  `json:"last_updated_at"`
	ExitedAt         *time.Time `json:"exited_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	PayloadDigest    [32]byte   `json:"payload_digest"`
}

type AudienceHistoryReceipt struct {
	SourceIdentifier string
	PayloadDigest    [32]byte
	TargetID         int64
	TargetDigest     [32]byte
	Replayed         bool
}

// Stores and the scoped journal must share the caller transaction.
type AudienceHistoryStore interface {
	CreateHistoricalAudienceGroup(context.Context, HistoricalAudienceGroup) (HistoricalAudienceGroup, error)
	GetHistoricalAudienceGroup(context.Context, int64) (HistoricalAudienceGroup, error)
	CreateHistoricalAudiencePackage(context.Context, HistoricalAudiencePackage) (HistoricalAudiencePackage, error)
	GetHistoricalAudiencePackage(context.Context, int64) (HistoricalAudiencePackage, error)
	CreateHistoricalAudienceVersion(context.Context, HistoricalAudienceVersion) (HistoricalAudienceVersion, error)
	GetHistoricalAudienceVersion(context.Context, int64) (HistoricalAudienceVersion, error)
	CreateHistoricalAudienceSender(context.Context, HistoricalAudienceSender) (HistoricalAudienceSender, error)
	GetHistoricalAudienceSender(context.Context, int64) (HistoricalAudienceSender, error)
	CreateHistoricalAudienceRule(context.Context, HistoricalAudienceRule) (HistoricalAudienceRule, error)
	GetHistoricalAudienceRule(context.Context, int64) (HistoricalAudienceRule, error)
	CreateHistoricalAudienceRuleVersion(context.Context, HistoricalAudienceRuleVersion) (HistoricalAudienceRuleVersion, error)
	GetHistoricalAudienceRuleVersion(context.Context, int64) (HistoricalAudienceRuleVersion, error)
	CreateHistoricalAudienceDefinition(context.Context, HistoricalAudienceDefinition) (HistoricalAudienceDefinition, error)
	GetHistoricalAudienceDefinition(context.Context, int64) (HistoricalAudienceDefinition, error)
	CreateHistoricalAudienceMember(context.Context, HistoricalAudienceMember) (HistoricalAudienceMember, error)
	GetHistoricalAudienceMember(context.Context, int64) (HistoricalAudienceMember, error)
}

// Kind is exactly groups, packages, versions, senders, rules, rule_versions,
// definitions, or members; each kind has its own existing migration journal.
type AudienceHistoryJournal interface {
	LoadAudienceHistory(context.Context, string, string) (AudienceHistoryReceipt, bool, error)
	RecordAudienceHistory(context.Context, string, AudienceHistoryReceipt) error
}

// Child-list methods take the actual V2 historical parent ID, then limit/offset.
type AudienceHistoryReader interface {
	GetHistoricalAudiencePackage(context.Context, int64) (HistoricalAudiencePackage, error)
	GetHistoricalAudienceDefinition(context.Context, int64) (HistoricalAudienceDefinition, error)
	ListHistoricalAudienceGroups(context.Context, int32, int32) ([]HistoricalAudienceGroup, int64, error)
	ListHistoricalAudiencePackages(context.Context, int32, int32) ([]HistoricalAudiencePackage, int64, error)
	ListHistoricalAudienceVersions(context.Context, int64, int32, int32) ([]HistoricalAudienceVersion, int64, error)
	ListHistoricalAudienceSenders(context.Context, int64, int32, int32) ([]HistoricalAudienceSender, int64, error)
	ListHistoricalAudienceRules(context.Context, int32, int32) ([]HistoricalAudienceRule, int64, error)
	ListHistoricalAudienceRuleVersions(context.Context, int64, int32, int32) ([]HistoricalAudienceRuleVersion, int64, error)
	ListHistoricalAudienceDefinitions(context.Context, int32, int32) ([]HistoricalAudienceDefinition, int64, error)
	ListHistoricalAudienceMembers(context.Context, int64, int32, int32) ([]HistoricalAudienceMember, int64, error)
}
