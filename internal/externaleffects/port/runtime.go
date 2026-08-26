// Package port exposes the narrow External Effects Runtime contract to owning
// business domains. It intentionally carries only digest-only EER values.
package port

import (
	"context"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
)

type (
	Digest                         = eer.Digest
	EffectEnvelope                 = eer.EffectEnvelope
	EnvelopeInput                  = eer.EnvelopeInput
	AcceptCommand                  = eer.AcceptCommand
	QueueCommand                   = eer.QueueCommand
	ClaimCommand                   = eer.ClaimCommand
	ReconcileCommand               = eer.ReconcileCommand
	RecoverAttemptedCommand        = eer.RecoverAttemptedCommand
	CompleteRecordedAttemptCommand = eer.CompleteRecordedAttemptCommand
	Lease                          = eer.Lease
	Projection                     = eer.Projection
	OperationReceipt               = eer.OperationReceipt
	RiverJobLink                   = eer.RiverJobLink
	Adapter                        = eer.Adapter
	AdapterResult                  = eer.AdapterResult
	Attempt                        = eer.Attempt
	State                          = eer.State
	Completion                     = eer.Completion
	TerminalOutcome                = eer.TerminalOutcome
)

const (
	OwnerOutbound                      = eer.OwnerOutbound
	OwnerContact                       = eer.OwnerContact
	OwnerProduct                       = eer.OwnerProduct
	OwnerSurvey                        = eer.OwnerSurvey
	OwnerGroupOps                      = eer.OwnerGroupOps
	OwnerWeCom                         = eer.OwnerWeCom
	OwnerMedia                         = eer.OwnerMedia
	KindOutboundMessage                = eer.KindOutboundMessage
	KindContactAcquisitionAssetPublish = eer.KindContactAcquisitionAssetPublish
	KindProductExternalPushTest        = eer.KindProductExternalPushTest
	KindSurveyWebhook                  = eer.KindSurveyWebhook
	KindOutboundMedia                  = eer.KindOutboundMedia
	KindGroupOpsBroadcast              = eer.KindGroupOpsBroadcast
	KindWeComTagSync                   = eer.KindWeComTagSync
	KindMediaWeComUpload               = eer.KindMediaWeComUpload
	StateAccepted                      = eer.StateAccepted
	StateQueued                        = eer.StateQueued
	StateAttempted                     = eer.StateAttempted
	StateExecuted                      = eer.StateExecuted
	StateOutcomeUnknown                = eer.StateOutcomeUnknown
	StateReconciled                    = eer.StateReconciled
	StateRetryableFailed               = eer.StateRetryableFailed
	StateFinalFailed                   = eer.StateFinalFailed
	CompletionFinalFailed              = eer.CompletionFinalFailed
	CompletionExecuted                 = eer.CompletionExecuted
	CompletionOutcomeUnknown           = eer.CompletionOutcomeUnknown
)

var (
	ErrAdapterFailure    = eer.ErrAdapterFailure
	ErrLeaseFence        = eer.ErrLeaseFence
	ErrPayloadMismatch   = eer.ErrPayloadMismatch
	ErrReconcileRequired = eer.ErrReconcileRequired
	ErrNotFound          = eer.ErrNotFound
)

func NewEnvelope(input EnvelopeInput) (EffectEnvelope, error) { return eer.NewEnvelope(input) }

type Runtime interface {
	Accept(context.Context, AcceptCommand) (Projection, OperationReceipt, error)
	Queue(context.Context, QueueCommand) (Projection, OperationReceipt, error)
	Claim(context.Context, ClaimCommand) (Lease, Projection, error)
	RunAttempt(context.Context, Lease, Adapter) (Projection, OperationReceipt, error)
	Reconcile(context.Context, ReconcileCommand) (Projection, OperationReceipt, error)
}

// RecoveryRuntime is deliberately separate from Runtime so existing domains
// cannot gain a crash-recovery mutation merely by depending on the common
// execution surface. Owner-domain adapters opt in explicitly.
type RecoveryRuntime interface {
	RecoverAttemptedToUnknown(context.Context, RecoverAttemptedCommand) (Projection, OperationReceipt, error)
	CompleteRecordedAttempt(context.Context, CompleteRecordedAttemptCommand) (Projection, OperationReceipt, error)
}

type TerminalReader interface {
	GetTerminalOutcome(context.Context, string) (TerminalOutcome, error)
}
