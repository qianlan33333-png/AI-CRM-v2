package port

import "time"

// CompletionOperations stores only local opaque references. It intentionally
// cannot carry a redirect URL or execute a post-submission action.
type CompletionOperations struct {
	NavigationTargetID string `json:"navigation_target_id,omitempty"`
	ChannelID          int64  `json:"channel_id,omitempty"`
}

// ExternalPushOperations stores a local enablement decision and an opaque
// configuration reference. Provider URLs, credentials, payloads, and retry
// policy are deliberately outside this contract.
type ExternalPushOperations struct {
	Enabled                bool   `json:"enabled"`
	ConfigurationReference string `json:"configuration_reference,omitempty"`
}

type OperationsProjection struct {
	QuestionnaireID ID                     `json:"questionnaire_id"`
	Completion      CompletionOperations   `json:"completion"`
	ExternalPush    ExternalPushOperations `json:"external_push"`
	LocalOnly       bool                   `json:"local_only"`
}

type SaveCompletionOperationsCommand struct {
	QuestionnaireID ID
	Actor           int64
	IdempotencyKey  string
	Completion      CompletionOperations
}

type SaveExternalPushOperationsCommand struct {
	QuestionnaireID ID
	Actor           int64
	IdempotencyKey  string
	ExternalPush    ExternalPushOperations
}

type QueueExternalPushTestCommand struct {
	QuestionnaireID ID
	Actor           int64
	IdempotencyKey  string
}

// ExternalPushTest is a durable local queue record. Its fixed false values
// make the API incapable of representing a provider call or result.
type ExternalPushTest struct {
	TestRunID              int64     `json:"test_run_id"`
	QuestionnaireID        ID        `json:"questionnaire_id"`
	Status                 string    `json:"status"`
	AttemptCount           int32     `json:"attempt_count"`
	SideEffectExecuted     bool      `json:"side_effect_executed"`
	ProviderResultReceived bool      `json:"provider_result_received"`
	UnknownAfterDispatch   bool      `json:"unknown_after_dispatch"`
	AutoRetryAllowed       bool      `json:"auto_retry_allowed"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type ExternalPushLogPage struct {
	Items     []ExternalPushTest `json:"items"`
	Total     int64              `json:"total"`
	Limit     int32              `json:"limit"`
	Offset    int32              `json:"offset"`
	HasMore   bool               `json:"has_more"`
	LocalOnly bool               `json:"local_only"`
}
