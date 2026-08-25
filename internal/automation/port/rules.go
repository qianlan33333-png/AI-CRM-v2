package port

import (
	"context"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

type RuleID int64

type RuleStatus string

const (
	RuleStatusActive   RuleStatus = "active"
	RuleStatusPaused   RuleStatus = "paused"
	RuleStatusArchived RuleStatus = "archived"
)

// TagAppliedCondition is the frozen A01 condition form.  A generic DSL is
// intentionally out of scope; unsupported condition forms are rejected.
type TagAppliedCondition struct {
	TagID int64 `json:"tag_id"`
}

// Action is closed to a local receipt and an Outbound/EER-owned message handoff.
// V2 stores only the existing Outbound template reference; the EER envelope is
// digest-only and never carries a recipient, body, provider, or credential.
type Action struct {
	Type        string `json:"type"`
	TemplateKey string `json:"template_key,omitempty"`
}

type Rule struct {
	ID        RuleID              `json:"id"`
	Code      string              `json:"code"`
	Name      string              `json:"name"`
	Status    RuleStatus          `json:"status"`
	Version   int64               `json:"version"`
	Condition TagAppliedCondition `json:"condition"`
	Action    Action              `json:"action"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

type CreateRuleCommand struct {
	Code           string
	Name           string
	Status         RuleStatus
	Condition      TagAppliedCondition
	Action         Action
	Actor          int64
	IdempotencyKey string
}

type UpdateRuleCommand struct {
	ID             RuleID
	Name           string
	Status         RuleStatus
	Condition      TagAppliedCondition
	Action         Action
	Actor          int64
	IdempotencyKey string
}

type RuleService interface {
	CreateRule(context.Context, CreateRuleCommand) (Rule, error)
	UpdateRule(context.Context, UpdateRuleCommand) (Rule, error)
	SetRuleStatus(context.Context, RuleID, RuleStatus, int64, string) (Rule, error)
	GetRule(context.Context, RuleID) (Rule, error)
	ListRules(context.Context) ([]Rule, error)
}

// RuntimeExecution is the safe read model for action records. JSON payloads
// and external provider bodies are deliberately not exposed.
type RuntimeExecution struct {
	ActionID         int64      `json:"action_id"`
	EnrollmentID     int64      `json:"enrollment_id"`
	AutomationID     RuleID     `json:"automation_id"`
	Version          int64      `json:"version"`
	SourceEventID    int64      `json:"source_event_id"`
	CustomerID       int64      `json:"customer_id"`
	ActionType       string     `json:"action_type"`
	State            string     `json:"state"`
	ExternalEffectID *string    `json:"external_effect_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

type RuntimeReader interface {
	ListRuleExecutions(context.Context, int32, int32) ([]RuntimeExecution, error)
}

// ReconcileOutboundMessageCommand is an operator-only local closure of a
// previously outcome-unknown EER binding. Its evidence is a digest, never a
// provider body or delivery assertion.
type ReconcileOutboundMessageCommand struct {
	ActionID       int64
	Actor          int64
	IdempotencyKey string
	Generation     int64
	Fence          int64
	LeaseExpiresAt time.Time
	EvidenceDigest eer.Digest
}

type RuntimeReconciler interface {
	ReconcileOutboundMessage(context.Context, ReconcileOutboundMessageCommand) (RuntimeExecution, error)
}

type RuleOperationReceipt struct {
	ID            int64
	PayloadDigest [32]byte
	Result        []byte
}
