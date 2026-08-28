package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrContactHistoryInvalid     = errors.New("invalid contact history")
	ErrContactHistoryConflict    = errors.New("contact history conflict")
	ErrContactHistoryUnavailable = errors.New("contact history unavailable")
)

// These projections are historical facts, never current Sidebar profiles or
// executable owner migrations. Raw identities and row payloads stay archived.
type HistoricalSidebarProfile struct {
	ID                    int64     `json:"id"`
	SourceKeyDigest       [32]byte  `json:"source_key_digest"`
	CustomerID            *int64    `json:"customer_id"`
	Source                string    `json:"source"`
	Industry              string    `json:"industry"`
	IndustryDescription   string    `json:"industry_description"`
	NeedsBlockersFollowup string    `json:"needs_blockers_followup"`
	UpdatedAt             time.Time `json:"updated_at"`
	SourcePayloadDigest   [32]byte  `json:"source_payload_digest"`
}

type HistoricalOwnerMigrationResult struct {
	ID                     int64     `json:"id"`
	SourceKeyDigest        [32]byte  `json:"source_key_digest"`
	ScopeType              string    `json:"scope_type"`
	FileHash               string    `json:"file_hash"`
	PreviewHash            string    `json:"preview_hash"`
	TotalRows              int64     `json:"total_rows"`
	EligibleCount          int64     `json:"eligible_count"`
	WeComSuccess           int64     `json:"wecom_success"`
	WeComFailed            int64     `json:"wecom_failed"`
	CRMUpdated             int64     `json:"crm_updated"`
	IncludeWeComTransfer   bool      `json:"include_wecom_transfer"`
	TransferWelcomeMessage string    `json:"transfer_welcome_message"`
	SessionRelation        string    `json:"session_relation"`
	PreviewRelation        string    `json:"preview_relation"`
	CreatedAt              time.Time `json:"created_at"`
	ExecutedAt             time.Time `json:"executed_at"`
	SourcePayloadDigest    [32]byte  `json:"source_payload_digest"`
}

const (
	ContactHistorySidebar     = "sidebar_profile"
	ContactHistoryOwnerResult = "owner_migration_result"
)

type ContactHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

// Store and journal share the caller transaction, with no current-domain event.
type ContactHistoryStore interface {
	CreateHistoricalSidebarProfile(context.Context, HistoricalSidebarProfile) (HistoricalSidebarProfile, error)
	GetHistoricalSidebarProfile(context.Context, int64) (HistoricalSidebarProfile, error)
	CreateHistoricalOwnerMigrationResult(context.Context, HistoricalOwnerMigrationResult) (HistoricalOwnerMigrationResult, error)
	GetHistoricalOwnerMigrationResult(context.Context, int64) (HistoricalOwnerMigrationResult, error)
}

type ContactHistoryJournal interface {
	LoadContactHistory(context.Context, string, string) (ContactHistoryReceipt, bool, error)
	RecordContactHistory(context.Context, ContactHistoryReceipt) error
}

type ContactHistoryQuery struct {
	CustomerID    *int64
	Limit, Offset int32
}

type ContactHistoryReader interface {
	GetHistoricalSidebarProfile(context.Context, int64) (HistoricalSidebarProfile, error)
	ListHistoricalSidebarProfiles(context.Context, ContactHistoryQuery) ([]HistoricalSidebarProfile, int64, error)
	GetHistoricalOwnerMigrationResult(context.Context, int64) (HistoricalOwnerMigrationResult, error)
	ListHistoricalOwnerMigrationResults(context.Context, ContactHistoryQuery) ([]HistoricalOwnerMigrationResult, int64, error)
}
