package port

import (
	"context"
	"encoding/json"
)

// DispatchExecution is the immutable execution snapshot a Group Ops worker
// may pass to its Provider boundary. It deliberately contains no Provider
// credentials, protocol fields, or response body.
type DispatchExecution struct {
	ExecutionID      int64
	ExternalEffectID string
	State            ExecutionState
	DeliveryProven   bool
	TargetReference  string
	ContentSnapshot  json.RawMessage
	ContentDigest    string
	MaterialSnapshot json.RawMessage
	MaterialDigest   string
}

// DispatchExecutionReader must load the immutable 00085 snapshot for one EER
// effect under the owner-domain transaction/fence used by the integration.
// The Group Ops core intentionally does not guess a Provider request schema.
type DispatchExecutionReader interface {
	LoadDispatchExecution(context.Context, string) (DispatchExecution, error)
}

// ExecutionOutcomeProjector persists the matching Group Ops terminal fact
// after EER has completed an attempt. RuntimeService implements this boundary.
type ExecutionOutcomeProjector interface {
	ProjectExecutionOutcome(context.Context, ExecutionOutcomeCommand) (Execution, error)
}

type DispatchOutcome string

const (
	// DispatchPreDispatchFailure means validation, snapshot resolution, or
	// Provider configuration failed before its business boundary was crossed.
	DispatchPreDispatchFailure DispatchOutcome = "pre_dispatch_failure"
	// DispatchProviderAccepted means the Provider explicitly accepted the
	// request. It is not a delivery claim.
	DispatchProviderAccepted DispatchOutcome = "provider_accepted"
	// DispatchOutcomeUnknown means the Provider boundary was crossed but no
	// terminal answer can be safely classified. It never auto-retries.
	DispatchOutcomeUnknown DispatchOutcome = "outcome_unknown"
	// DispatchProviderRejected means an explicit Provider result rejected the
	// request after the business boundary was crossed.
	DispatchProviderRejected DispatchOutcome = "provider_rejected"
)

// DispatchRequest is protocol-neutral. An integration may translate these
// immutable snapshots only after it has an approved Provider contract.
type DispatchRequest struct {
	ExecutionID      int64
	ExternalEffectID string
	TargetReference  string
	ContentSnapshot  json.RawMessage
	ContentDigest    string
	MaterialSnapshot json.RawMessage
	MaterialDigest   string
}

// DispatchProvider is the sole Group Ops outbound boundary. Implementations
// must return a classified result for every pre-dispatch failure; returning an
// error means the call crossed this boundary and is therefore outcome_unknown.
type DispatchProvider interface {
	Dispatch(context.Context, DispatchRequest) (DispatchProviderResult, error)
}

// DispatchProviderResult exposes only a safe digest, never Provider request,
// response, credential, or delivery evidence.
type DispatchProviderResult struct {
	Outcome       DispatchOutcome
	ReceiptDigest string
}

// DispatchResult is the owner-safe worker projection. Provider acceptance is
// intentionally distinct from delivery, and unknown results require manual
// reconciliation.
type DispatchResult struct {
	ExecutionID              int64
	EffectID                 string
	State                    ExecutionState
	ProviderCallAttempted    bool
	RealExternalCallExecuted bool
	ProviderAccepted         bool
	DeliveryProven           bool
	ManualReconcileRequired  bool
}
