package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrHXCHistoryInvalid     = errors.New("invalid HXC history")
	ErrHXCHistoryConflict    = errors.New("HXC history conflict")
	ErrHXCHistoryUnavailable = errors.New("HXC history unavailable")
)

// SourceID is a signed V1 reference, never a current V2 task or customer ID.
type HistoricalHXCIdentity struct {
	ID                  int64    `json:"id"`
	SourceID            int64    `json:"source_id"`
	SourceKeyDigest     [32]byte `json:"source_key_digest"`
	SourcePayloadDigest [32]byte `json:"source_payload_digest"`
}

// Facts are immutable observations. Nullable DATE strings preserve calendar
// meaning, and batch references never authorize or replay an import.
type HistoricalHXCMeta struct {
	HistoricalHXCIdentity
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	Status        string     `json:"status"`
	RowCount      int64      `json:"row_count"`
	MemberHit     int64      `json:"member_hit"`
	UserHit       int64      `json:"user_hit"`
	OnlyMember    int64      `json:"only_member"`
	TriggerSource string     `json:"trigger_source"`
}

type HistoricalHXCSnapshot struct {
	HistoricalHXCIdentity
	CustomerID              *int64     `json:"customer_id"`
	Observation             string     `json:"observation"`
	ObservedAt              time.Time  `json:"observed_at"`
	InLeadPool              bool       `json:"in_lead_pool"`
	InPeople                bool       `json:"in_people"`
	InQuestionnaire         bool       `json:"in_questionnaire"`
	ClassTermNo             *int64     `json:"class_term_no"`
	ClassTermLabel          string     `json:"class_term_label"`
	CRMHXCState             string     `json:"crm_hxc_state"`
	CRMCreatedAt            *string    `json:"crm_created_at"`
	LastQuestionnaireAt     *string    `json:"last_questionnaire_at"`
	HXCMemberHit            bool       `json:"hxc_member_hit"`
	HXCUserHit              bool       `json:"hxc_user_hit"`
	FunnelState             string     `json:"funnel_state"`
	HXCMemberStatus         string     `json:"hxc_member_status"`
	HXCRegisteredAt         *time.Time `json:"hxc_registered_at"`
	HXCLastLoginAt          *time.Time `json:"hxc_last_login_at"`
	MembershipType          string     `json:"membership_type"`
	MembershipStatus        string     `json:"membership_status"`
	MembershipEndAt         *time.Time `json:"membership_end_at"`
	MembershipDaysLeft      *int64     `json:"membership_days_left"`
	ConsultationUsed        *int64     `json:"consultation_used"`
	ConsultationLimit       *int64     `json:"consultation_limit"`
	ConversationChat        int64      `json:"conversation_chat"`
	ConversationConsult     int64      `json:"conversation_consult"`
	ConversationLesson      int64      `json:"conversation_lesson"`
	MessagesUser            int64      `json:"messages_user"`
	MessagesAI              int64      `json:"messages_ai"`
	ConsultCompleted        int64      `json:"consult_completed"`
	LastMessageAt           *time.Time `json:"last_message_at"`
	SubscriptionTier        string     `json:"subscription_tier"`
	SubscriptionExpires     *time.Time `json:"subscription_expires"`
	SubscriptionQuota       *int64     `json:"subscription_quota"`
	SubscriptionUsed        *int64     `json:"subscription_used"`
	SubscriptionPeriodStart *string    `json:"subscription_period_start"`
}

type HistoricalHXCActivation struct {
	HistoricalHXCIdentity
	SourceTable          string    `json:"source_table"`
	OriginalState        string    `json:"original_state"`
	IsActive             bool      `json:"is_active"`
	LegacyImportBatchRef *string   `json:"legacy_import_batch_ref"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type HistoricalHXCLead struct {
	HistoricalHXCIdentity
	OriginalType         string    `json:"original_type"`
	IsActive             bool      `json:"is_active"`
	LegacyImportBatchRef *string   `json:"legacy_import_batch_ref"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type HistoricalHXCBatch struct {
	HistoricalHXCIdentity
	ImportType  string    `json:"import_type"`
	TotalRows   int64     `json:"total_rows"`
	SuccessRows int64     `json:"success_rows"`
	FailedRows  int64     `json:"failed_rows"`
	CreatedAt   time.Time `json:"created_at"`
}

const (
	HXCHistoryMeta                   = "meta"
	HXCHistorySnapshot               = "snapshot"
	HXCHistoryActivationStatus       = "activation_status"
	HXCHistoryHuangxiaocanActivation = "huangxiaocan_activation"
	HXCHistoryLead                   = "lead"
	HXCHistoryBatch                  = "batch"
)

type HXCHistoryReceipt struct {
	Kind, SourceIdentifier      string
	PayloadDigest, TargetDigest [32]byte
	TargetID                    int64
	Replayed                    bool
}

// Migration-only writes and their journal use the caller transaction. No
// current HXC, sender, entitlement, event, queue, or Provider operation exists.
type HXCHistoryStore interface {
	CreateHistoricalHXCMeta(context.Context, HistoricalHXCMeta) (HistoricalHXCMeta, error)
	GetHistoricalHXCMeta(context.Context, int64) (HistoricalHXCMeta, error)
	CreateHistoricalHXCSnapshot(context.Context, HistoricalHXCSnapshot) (HistoricalHXCSnapshot, error)
	GetHistoricalHXCSnapshot(context.Context, int64) (HistoricalHXCSnapshot, error)
	CreateHistoricalHXCActivation(context.Context, HistoricalHXCActivation) (HistoricalHXCActivation, error)
	GetHistoricalHXCActivation(context.Context, int64) (HistoricalHXCActivation, error)
	CreateHistoricalHXCLead(context.Context, HistoricalHXCLead) (HistoricalHXCLead, error)
	GetHistoricalHXCLead(context.Context, int64) (HistoricalHXCLead, error)
	CreateHistoricalHXCBatch(context.Context, HistoricalHXCBatch) (HistoricalHXCBatch, error)
	GetHistoricalHXCBatch(context.Context, int64) (HistoricalHXCBatch, error)
}

type HXCHistoryJournal interface {
	LoadHXCHistory(context.Context, string, string) (HXCHistoryReceipt, bool, error)
	RecordHXCHistory(context.Context, HXCHistoryReceipt) error
}

type HXCHistoryQuery struct {
	CustomerID    *int64
	SourceTable   string
	Limit, Offset int32
}

type HXCHistoryReader interface {
	GetHistoricalHXCMeta(context.Context, int64) (HistoricalHXCMeta, error)
	ListHistoricalHXCMeta(context.Context, HXCHistoryQuery) ([]HistoricalHXCMeta, int64, error)
	GetHistoricalHXCSnapshot(context.Context, int64) (HistoricalHXCSnapshot, error)
	ListHistoricalHXCSnapshot(context.Context, HXCHistoryQuery) ([]HistoricalHXCSnapshot, int64, error)
	GetHistoricalHXCActivation(context.Context, int64) (HistoricalHXCActivation, error)
	ListHistoricalHXCActivation(context.Context, HXCHistoryQuery) ([]HistoricalHXCActivation, int64, error)
	GetHistoricalHXCLead(context.Context, int64) (HistoricalHXCLead, error)
	ListHistoricalHXCLead(context.Context, HXCHistoryQuery) ([]HistoricalHXCLead, int64, error)
	GetHistoricalHXCBatch(context.Context, int64) (HistoricalHXCBatch, error)
	ListHistoricalHXCBatch(context.Context, HXCHistoryQuery) ([]HistoricalHXCBatch, int64, error)
}
