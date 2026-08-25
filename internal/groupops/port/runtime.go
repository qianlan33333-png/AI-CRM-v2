package port

import (
	"context"
	"time"
)

type RunTrigger string

const (
	RunTriggerDue       RunTrigger = "run_due"
	RunTriggerBroadcast RunTrigger = "broadcast"
	RunTriggerWebhook   RunTrigger = "webhook"
)

type ExecutionState string

const (
	ExecutionAccepted         ExecutionState = "accepted"
	ExecutionProviderAccepted ExecutionState = "provider_accepted"
	ExecutionDeliveryProven   ExecutionState = "delivery_proven"
	ExecutionOutcomeUnknown   ExecutionState = "outcome_unknown"
	ExecutionReconciled       ExecutionState = "reconciled"
	ExecutionFinalFailed      ExecutionState = "final_failed"
)

type RuntimeSafety struct {
	ProviderExecutionEligible bool `json:"provider_execution_eligible"`
	RealExternalCallExecuted  bool `json:"real_external_call_executed"`
	ProviderAccepted          bool `json:"provider_accepted"`
	DeliveryProven            bool `json:"delivery_proven"`
}

func DisabledRuntimeSafety() RuntimeSafety { return RuntimeSafety{} }

type RunDuePreview struct {
	PlanID            int64      `json:"plan_id,string"`
	PlanStatus        PlanStatus `json:"plan_status"`
	SnapshotRevision  int64      `json:"snapshot_revision"`
	EvaluatedAt       time.Time  `json:"evaluated_at"`
	DueExecutionCount int32      `json:"due_execution_count"`
	NextDueAt         *time.Time `json:"next_due_at,omitempty"`
	Blockers          []string   `json:"blockers"`
	RuntimeSafety
}

type Run struct {
	ID           int64      `json:"run_id,string"`
	PlanID       int64      `json:"plan_id,string"`
	Trigger      RunTrigger `json:"trigger"`
	PlanRevision int64      `json:"plan_revision"`
	ScheduledFor time.Time  `json:"scheduled_for"`
	AcceptedAt   time.Time  `json:"accepted_at"`
	AcceptedBy   string     `json:"accepted_by"`
}

type Execution struct {
	ID                            int64          `json:"execution_id,string"`
	RunID                         int64          `json:"run_id,string"`
	PlanID                        int64          `json:"plan_id,string"`
	PlanRevision                  int64          `json:"plan_revision"`
	NodeID                        int64          `json:"node_id,string"`
	NodePosition                  int32          `json:"node_position"`
	TargetReference               string         `json:"target_reference"`
	TargetDigest                  string         `json:"target_digest"`
	ContentDigest                 string         `json:"content_digest"`
	MaterialDigest                string         `json:"material_digest"`
	ExternalEffectID              string         `json:"external_effect_id"`
	State                         ExecutionState `json:"state"`
	ProviderAccepted              bool           `json:"provider_accepted"`
	DeliveryProven                bool           `json:"delivery_proven"`
	AttemptCount                  int32          `json:"attempt_count"`
	ProviderReceiptPresent        bool           `json:"provider_receipt_present"`
	ReconciliationEvidencePresent bool           `json:"reconciliation_evidence_present"`
	CreatedAt                     time.Time      `json:"created_at"`
	UpdatedAt                     time.Time      `json:"updated_at"`
}

type RunSummary struct {
	Run              Run         `json:"run"`
	Executions       []Execution `json:"executions"`
	Accepted         int32       `json:"accepted"`
	ProviderAccepted int32       `json:"provider_accepted_count"`
	DeliveryProven   int32       `json:"delivery_proven_count"`
	OutcomeUnknown   int32       `json:"outcome_unknown"`
	Reconciled       int32       `json:"reconciled"`
	FinalFailed      int32       `json:"final_failed"`
	RuntimeSafety
}

type ExecutionPage struct {
	Items   []Execution `json:"items"`
	Total   int64       `json:"total"`
	Limit   int32       `json:"limit"`
	Offset  int32       `json:"offset"`
	HasMore bool        `json:"has_more"`
	RuntimeSafety
}

type GroupDirectoryItem struct {
	ChatReference string    `json:"chat_reference"`
	OwnerStaffID  int64     `json:"owner_staff_id"`
	DisplayName   string    `json:"display_name"`
	MemberCount   int32     `json:"member_count"`
	RefreshedAt   time.Time `json:"refreshed_at"`
}

type GroupDirectoryPage struct {
	Items   []GroupDirectoryItem `json:"items"`
	Total   int64                `json:"total"`
	Limit   int32                `json:"limit"`
	Offset  int32                `json:"offset"`
	HasMore bool                 `json:"has_more"`
	RuntimeSafety
}

type OperationMember struct {
	SenderUserID string `json:"sender_userid"`
	DisplayName  string `json:"display_name"`
}

type OperationMemberPage struct {
	Scope    string            `json:"scope"`
	Items    []OperationMember `json:"items"`
	PageSize int32             `json:"page_size"`
	RuntimeSafety
}

type RunDueCommand struct {
	PlanID         int64
	ActorID        int64
	IdempotencyKey string
}

type AcceptPlanCommand struct {
	PlanID         int64
	Trigger        RunTrigger
	AcceptedBy     string
	IdempotencyKey string
}

type ManualReconcileCommand struct {
	ExecutionID    int64
	ActorID        int64
	IdempotencyKey string
	Generation     int64
	Fence          int64
	LeaseExpiresAt time.Time
	EvidenceDigest string
	DeliveryProven bool
}

type ExecutionOutcomeCommand struct {
	ExecutionID           int64
	State                 ExecutionState
	ProviderAccepted      bool
	DeliveryProven        bool
	ProviderReceiptDigest string
	AttemptCount          int32
}

type GroupRefreshCommand struct {
	OwnerStaffID   int64
	ActorID        int64
	Limit          int32
	IdempotencyKey string
}

type OperationMemberRefreshCommand struct {
	ActorID        int64
	PageSize       int32
	IdempotencyKey string
}

type GroupDirectorySource interface {
	ListOwnedGroups(context.Context, int64, int32) ([]GroupDirectoryItem, error)
	RefreshOperationMembers(context.Context, int32) ([]OperationMember, error)
}
