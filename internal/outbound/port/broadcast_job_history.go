package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrBroadcastJobHistoryInvalid     = errors.New("invalid broadcast job history")
	ErrBroadcastJobHistoryConflict    = errors.New("broadcast job history conflict")
	ErrBroadcastJobHistoryUnavailable = errors.New("broadcast job history unavailable")
)

// HistoricalBroadcastJob is an immutable V1 observation, never a runnable
// outbound task. Original status and provider flags cannot trigger V2 effects.
// Private digests preserve the sealed archive, including prior redactions.
type HistoricalBroadcastJob struct {
	ID                        int64      `json:"id"`
	SourceID                  int64      `json:"source_id"`
	OriginalSourceType        string     `json:"original_source_type"`
	SourceReferenceDigest     [32]byte   `json:"-"`
	SourceTable               string     `json:"source_table"`
	ScheduledFor              time.Time  `json:"scheduled_for"`
	Priority                  int32      `json:"priority"`
	BatchKeyDigest            [32]byte   `json:"-"`
	OriginalStatus            string     `json:"original_status"`
	RequiresApproval          bool       `json:"requires_approval"`
	ApprovedByDigest          [32]byte   `json:"-"`
	ApprovedAt                *time.Time `json:"approved_at"`
	CancelledByDigest         [32]byte   `json:"-"`
	CancelledAt               *time.Time `json:"cancelled_at"`
	CancelReasonDigest        [32]byte   `json:"-"`
	TargetCount               int32      `json:"target_count"`
	TargetSummaryDigest       [32]byte   `json:"-"`
	ContentType               string     `json:"content_type"`
	ContentPayloadDigest      [32]byte   `json:"-"`
	ContentSummaryDigest      [32]byte   `json:"-"`
	AttemptCount              int32      `json:"attempt_count"`
	LastErrorDigest           [32]byte   `json:"-"`
	LegacyOutboundTaskID      *int64     `json:"-"`
	SentCount                 int32      `json:"sent_count"`
	FailedCount               int32      `json:"failed_count"`
	TraceIDDigest             [32]byte   `json:"-"`
	CreatedByDigest           [32]byte   `json:"-"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
	ClaimedAt                 *time.Time `json:"claimed_at"`
	SentAt                    *time.Time `json:"sent_at"`
	ClaimTokenDigest          [32]byte   `json:"-"`
	LeaseExpiresAt            *time.Time `json:"lease_expires_at"`
	BusinessDomain            *string    `json:"business_domain"`
	IdempotencyKeyDigest      *[32]byte  `json:"-"`
	Channel                   *string    `json:"channel"`
	TargetKind                *string    `json:"target_kind"`
	FailureType               *string    `json:"failure_type"`
	RetryPolicyDigest         [32]byte   `json:"-"`
	MetadataDigest            [32]byte   `json:"-"`
	TargetUnionIDsDigest      [32]byte   `json:"-"`
	MaxAttempts               int32      `json:"max_attempts"`
	NextRetryAt               *time.Time `json:"next_retry_at"`
	DispatchStartedAt         *time.Time `json:"dispatch_started_at"`
	SideEffectExecuted        bool       `json:"original_side_effect_executed"`
	ProviderResultReceived    bool       `json:"original_provider_result_received"`
	ResultSummaryDigest       [32]byte   `json:"-"`
	ReconciliationRequired    bool       `json:"original_reconciliation_required"`
	CompletedAt               *time.Time `json:"completed_at"`
	HoldReasonDigest          [32]byte   `json:"-"`
	HoldAt                    *time.Time `json:"hold_at"`
	LegacyExternalEffectJobID *int64     `json:"-"`
	ExecutionIDDigest         [32]byte   `json:"-"`
	ExecutionOwnerDigest      [32]byte   `json:"-"`
	SourceKeyDigest           [32]byte   `json:"-"`
	SourcePayloadDigest       [32]byte   `json:"-"`
	SourceFieldDigest         [32]byte   `json:"-"`
	RedactedRoots             []string   `json:"-"`
}

type BroadcastJobHistoryReceipt struct {
	SourceIdentifier            string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

type BroadcastJobHistoryStore interface {
	CreateHistoricalBroadcastJob(context.Context, HistoricalBroadcastJob) (HistoricalBroadcastJob, error)
	GetHistoricalBroadcastJob(context.Context, int64) (HistoricalBroadcastJob, error)
}

type BroadcastJobHistoryJournal interface {
	LoadBroadcastJobHistory(context.Context, string) (BroadcastJobHistoryReceipt, bool, error)
	RecordBroadcastJobHistory(context.Context, BroadcastJobHistoryReceipt) error
}

type BroadcastJobHistoryQuery struct{ Limit, Offset int32 }

type BroadcastJobHistoryReader interface {
	GetHistoricalBroadcastJob(context.Context, int64) (HistoricalBroadcastJob, error)
	ListHistoricalBroadcastJobs(context.Context, BroadcastJobHistoryQuery) ([]HistoricalBroadcastJob, int64, error)
}
