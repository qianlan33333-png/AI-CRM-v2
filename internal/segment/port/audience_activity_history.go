package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrAudienceActivityHistoryInvalid     = errors.New("invalid audience activity history")
	ErrAudienceActivityHistoryConflict    = errors.New("audience activity history conflict")
	ErrAudienceActivityHistoryUnavailable = errors.New("audience activity history unavailable")
)

// These rows are immutable V1 history only. They never become a current
// Segment, audience member, job, event, or Provider request.
type HistoricalAudienceActivityRun struct {
	ID                  int64      `json:"id"`
	SourceKeyDigest     [32]byte   `json:"-"`
	SourcePayloadDigest [32]byte   `json:"-"`
	SourceFieldDigest   [32]byte   `json:"-"`
	SourceID            int64      `json:"-"`
	PackageHistoryID    int64      `json:"package_history_id"`
	VersionHistoryID    *int64     `json:"version_history_id"`
	RunType             string     `json:"run_type"`
	OriginalStatus      string     `json:"original_status"`
	RefreshStartedAt    time.Time  `json:"refresh_started_at"`
	RefreshFinishedAt   *time.Time `json:"refresh_finished_at"`
	LastWatermarkAt     *time.Time `json:"last_watermark_at"`
	NextWatermarkAt     *time.Time `json:"next_watermark_at"`
	ReturnedCount       int32      `json:"returned_count"`
	EnteredCount        int32      `json:"entered_count"`
	UpdatedCount        int32      `json:"updated_count"`
	ExitedCount         int32      `json:"exited_count"`
	MemberEventCount    int32      `json:"member_event_count"`
	DurationMS          int32      `json:"duration_ms"`
	CreatedAt           time.Time  `json:"created_at"`
	PrivateDigest       [32]byte   `json:"-"`
}

type HistoricalAudienceActivityMemberEvent struct {
	ID                  int64     `json:"id"`
	SourceKeyDigest     [32]byte  `json:"-"`
	SourcePayloadDigest [32]byte  `json:"-"`
	SourceFieldDigest   [32]byte  `json:"-"`
	SourceID            int64     `json:"-"`
	PackageHistoryID    int64     `json:"package_history_id"`
	RunHistoryID        *int64    `json:"run_history_id"`
	MemberHistoryID     *int64    `json:"member_history_id"`
	EventType           string    `json:"event_type"`
	IdentityKind        string    `json:"-"`
	OccurredAt          time.Time `json:"occurred_at"`
	CreatedAt           time.Time `json:"created_at"`
	PrivateDigest       [32]byte  `json:"-"`
}

// These views are the only models intended for a future public read API.
// They deliberately omit source IDs, private digests, identity material, and
// V1 error text.
type AudienceActivityRunView struct {
	ID                int64      `json:"id"`
	PackageHistoryID  int64      `json:"package_history_id"`
	VersionHistoryID  *int64     `json:"version_history_id"`
	RunType           string     `json:"run_type"`
	OriginalStatus    string     `json:"original_status"`
	RefreshStartedAt  time.Time  `json:"refresh_started_at"`
	RefreshFinishedAt *time.Time `json:"refresh_finished_at"`
	LastWatermarkAt   *time.Time `json:"last_watermark_at"`
	NextWatermarkAt   *time.Time `json:"next_watermark_at"`
	ReturnedCount     int32      `json:"returned_count"`
	EnteredCount      int32      `json:"entered_count"`
	UpdatedCount      int32      `json:"updated_count"`
	ExitedCount       int32      `json:"exited_count"`
	MemberEventCount  int32      `json:"member_event_count"`
	DurationMS        int32      `json:"duration_ms"`
	CreatedAt         time.Time  `json:"created_at"`
}

type AudienceActivityMemberEventView struct {
	ID               int64     `json:"id"`
	PackageHistoryID int64     `json:"package_history_id"`
	RunHistoryID     *int64    `json:"run_history_id"`
	MemberHistoryID  *int64    `json:"member_history_id"`
	EventType        string    `json:"event_type"`
	OccurredAt       time.Time `json:"occurred_at"`
	CreatedAt        time.Time `json:"created_at"`
}

type AudienceActivityHistoryReceipt struct {
	SourceIdentifier string
	PayloadDigest    [32]byte
	TargetID         int64
	TargetDigest     [32]byte
	Replayed         bool
}

// Stores and journals use the same transaction supplied by the caller.
type AudienceActivityHistoryStore interface {
	CreateHistoricalAudienceActivityRun(context.Context, HistoricalAudienceActivityRun) (HistoricalAudienceActivityRun, error)
	GetHistoricalAudienceActivityRun(context.Context, int64) (HistoricalAudienceActivityRun, error)
	CreateHistoricalAudienceActivityMemberEvent(context.Context, HistoricalAudienceActivityMemberEvent) (HistoricalAudienceActivityMemberEvent, error)
	GetHistoricalAudienceActivityMemberEvent(context.Context, int64) (HistoricalAudienceActivityMemberEvent, error)
	GetHistoricalAudienceActivityPackage(context.Context, int64) (AudienceActivityPackageReference, error)
	GetHistoricalAudienceActivityVersion(context.Context, int64) (AudienceActivityVersionReference, error)
	GetHistoricalAudienceActivityMember(context.Context, int64) (AudienceActivityMemberReference, error)
}

type AudienceActivityPackageReference struct{ ID int64 }
type AudienceActivityVersionReference struct {
	ID               int64
	PackageHistoryID int64
}
type AudienceActivityMemberReference struct {
	ID               int64
	PackageHistoryID int64
}

type AudienceActivityHistoryJournal interface {
	LoadAudienceActivityHistory(context.Context, string, string) (AudienceActivityHistoryReceipt, bool, error)
	RecordAudienceActivityHistory(context.Context, string, AudienceActivityHistoryReceipt) error
}

type AudienceActivityHistoryReader interface {
	ListAudienceActivityRuns(context.Context, int64, int32, int32) ([]AudienceActivityRunView, int64, error)
	ListAudienceActivityMemberEvents(context.Context, int64, int32, int32) ([]AudienceActivityMemberEventView, int64, error)
}

// AudienceActivityHistoryReferences resolves only already-imported immutable
// Audience history by its V1 source ID. It never reads current Audience rows.
type AudienceActivityHistoryReferences interface {
	ResolveAudienceActivityPackage(context.Context, int64) (AudienceActivityPackageReference, error)
	ResolveAudienceActivityVersion(context.Context, int64) (AudienceActivityVersionReference, error)
	ResolveAudienceActivityMember(context.Context, int64) (AudienceActivityMemberReference, error)
	ResolveAudienceActivityRun(context.Context, int64) (HistoricalAudienceActivityRun, error)
}
