package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrCampaignHistoryInvalid     = errors.New("invalid campaign history")
	ErrCampaignHistoryConflict    = errors.New("campaign history conflict")
	ErrCampaignHistoryUnavailable = errors.New("campaign history unavailable")
)

// These are non-executable V1 facts, separate from current Campaign/Outbound.
// Source IDs are not current V2 IDs. CustomerID is only a verified DM01 crosswalk.
// A segment with source_parent_state=missing_campaign preserves a valid orphan
// source row, without claiming a current Campaign/Segment association.
// No historical status, sent_at, or send_time proves a new Provider effect.
// Secrets, raw identities and runtime configuration remain in the sealed archive.

type HistoricalCampaignSegment struct {
	ID                  int64     `json:"id"`
	SourceID            int64     `json:"source_id"`
	CampaignSourceID    int64     `json:"campaign_source_id"`
	SegmentSourceID     int64     `json:"segment_source_id"`
	SourceParentState   string    `json:"source_parent_state"`
	Code                string    `json:"code"`
	Priority            int32     `json:"priority"`
	Label               string    `json:"label"`
	CreatedAt           time.Time `json:"created_at"`
	SourcePayloadDigest [32]byte  `json:"source_payload_digest"`
}

type HistoricalCampaignMember struct {
	ID                      int64      `json:"id"`
	SourceID                int64      `json:"source_id"`
	CampaignSourceID        int64      `json:"campaign_source_id"`
	CampaignSegmentSourceID int64      `json:"campaign_segment_source_id"`
	SegmentSourceID         int64      `json:"segment_source_id"`
	MemberSourceID          int64      `json:"member_source_id"`
	SegmentHistoryID        int64      `json:"segment_history_id"`
	CustomerID              *int64     `json:"customer_id"`
	JoinedAt                time.Time  `json:"joined_at"`
	AnchorDate              string     `json:"anchor_date"`
	CurrentStepIndex        int32      `json:"current_step_index"`
	NextDueAt               *time.Time `json:"next_due_at"`
	OriginalStatus          string     `json:"original_status"`
	StopReason              string     `json:"stop_reason"`
	LastStepSentAt          *time.Time `json:"last_step_sent_at"`
	RetryCount              int32      `json:"retry_count"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
	SourcePayloadDigest     [32]byte   `json:"source_payload_digest"`
}

type HistoricalBroadcastPlan struct {
	ID                    int64      `json:"id"`
	SourceID              int64      `json:"source_id"`
	SourcePlanID          string     `json:"source_plan_id"`
	CampaignSourceID      *int64     `json:"campaign_source_id"`
	SegmentSourceID       *int64     `json:"segment_source_id"`
	DisplayName           string     `json:"display_name"`
	Intent                string     `json:"intent"`
	ContentStrategy       string     `json:"content_strategy"`
	ContentTemplateMasked string     `json:"content_template_masked"`
	MaxRecipients         int64      `json:"max_recipients"`
	CandidateCount        int64      `json:"candidate_count"`
	SkippedCount          int64      `json:"skipped_count"`
	RequiresManualCopy    bool       `json:"requires_manual_copy"`
	OriginalStatus        string     `json:"original_status"`
	OriginalReviewStatus  string     `json:"original_review_status"`
	OriginalRunStatus     string     `json:"original_run_status"`
	CommittedAt           *time.Time `json:"committed_at"`
	ExpiresAt             *time.Time `json:"expires_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	RuntimeDigest         [32]byte   `json:"runtime_digest"`
	SourcePayloadDigest   [32]byte   `json:"source_payload_digest"`
}

type HistoricalBroadcastRecipient struct {
	ID                     int64      `json:"id"`
	SourceID               int64      `json:"source_id"`
	PlanHistoryID          int64      `json:"plan_history_id"`
	CustomerID             *int64     `json:"customer_id"`
	DisplayName            string     `json:"display_name"`
	PlannedMessageCount    int64      `json:"planned_message_count"`
	OriginalApprovalStatus string     `json:"original_approval_status"`
	OriginalSendStatus     string     `json:"original_send_status"`
	ApprovedAt             *time.Time `json:"approved_at"`
	RejectedAt             *time.Time `json:"rejected_at"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	SourcePayloadDigest    [32]byte   `json:"source_payload_digest"`
}

type HistoricalBroadcastMessage struct {
	ID                   int64      `json:"id"`
	SourceID             int64      `json:"source_id"`
	PlanHistoryID        int64      `json:"plan_history_id"`
	RecipientHistoryID   int64      `json:"recipient_history_id"`
	CustomerID           *int64     `json:"customer_id"`
	SequenceIndex        int64      `json:"sequence_index"`
	DayOffset            int64      `json:"day_offset"`
	OriginalSendTime     string     `json:"original_send_time"`
	ContentMasked        string     `json:"content_masked"`
	OriginalStatus       string     `json:"original_status"`
	SentAt               *time.Time `json:"sent_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	ContentPayloadDigest [32]byte   `json:"content_payload_digest"`
	AttachmentsDigest    [32]byte   `json:"attachments_digest"`
	SourcePayloadDigest  [32]byte   `json:"source_payload_digest"`
}

type CampaignHistoryReceipt struct {
	SourceIdentifier string
	PayloadDigest    [32]byte
	TargetID         int64
	TargetDigest     [32]byte
	Replayed         bool
}

// Store and journal participate in the caller's transaction, with no event/job.
type CampaignHistoryStore interface {
	CreateHistoricalCampaignSegment(context.Context, HistoricalCampaignSegment) (HistoricalCampaignSegment, error)
	GetHistoricalCampaignSegment(context.Context, int64) (HistoricalCampaignSegment, error)
	CreateHistoricalCampaignMember(context.Context, HistoricalCampaignMember) (HistoricalCampaignMember, error)
	GetHistoricalCampaignMember(context.Context, int64) (HistoricalCampaignMember, error)
	CreateHistoricalBroadcastPlan(context.Context, HistoricalBroadcastPlan) (HistoricalBroadcastPlan, error)
	GetHistoricalBroadcastPlan(context.Context, int64) (HistoricalBroadcastPlan, error)
	CreateHistoricalBroadcastRecipient(context.Context, HistoricalBroadcastRecipient) (HistoricalBroadcastRecipient, error)
	GetHistoricalBroadcastRecipient(context.Context, int64) (HistoricalBroadcastRecipient, error)
	CreateHistoricalBroadcastMessage(context.Context, HistoricalBroadcastMessage) (HistoricalBroadcastMessage, error)
	GetHistoricalBroadcastMessage(context.Context, int64) (HistoricalBroadcastMessage, error)
}

// Kind is segments, members, plans, recipients or messages.
type CampaignHistoryJournal interface {
	LoadCampaignHistory(context.Context, string, string) (CampaignHistoryReceipt, bool, error)
	RecordCampaignHistory(context.Context, string, CampaignHistoryReceipt) error
}

type CampaignHistoryReader interface {
	GetHistoricalCampaignSegment(context.Context, int64) (HistoricalCampaignSegment, error)
	GetHistoricalCampaignMember(context.Context, int64) (HistoricalCampaignMember, error)
	GetHistoricalBroadcastPlan(context.Context, int64) (HistoricalBroadcastPlan, error)
	GetHistoricalBroadcastRecipient(context.Context, int64) (HistoricalBroadcastRecipient, error)
	GetHistoricalBroadcastMessage(context.Context, int64) (HistoricalBroadcastMessage, error)
	// Segment filter is a source campaign ID, not a current V2 Campaign.
	ListHistoricalCampaignSegments(context.Context, *int64, int32, int32) ([]HistoricalCampaignSegment, int64, error)
	// Member filters are actual history segment ID and verified V2 customer ID.
	ListHistoricalCampaignMembers(context.Context, *int64, *int64, int32, int32) ([]HistoricalCampaignMember, int64, error)
	ListHistoricalBroadcastPlans(context.Context, int32, int32) ([]HistoricalBroadcastPlan, int64, error)
	ListHistoricalBroadcastRecipients(context.Context, int64, int32, int32) ([]HistoricalBroadcastRecipient, int64, error)
	ListHistoricalBroadcastMessages(context.Context, int64, int32, int32) ([]HistoricalBroadcastMessage, int64, error)
}
