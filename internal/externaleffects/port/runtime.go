// Package port exposes the narrow External Effects Runtime contract to owning
// business domains. It intentionally carries only digest-only EER values.
package port

import (
	"context"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
)

type (
	Digest           = eer.Digest
	EffectEnvelope   = eer.EffectEnvelope
	EnvelopeInput    = eer.EnvelopeInput
	AcceptCommand    = eer.AcceptCommand
	QueueCommand     = eer.QueueCommand
	ClaimCommand     = eer.ClaimCommand
	ReconcileCommand = eer.ReconcileCommand
	Lease            = eer.Lease
	Projection       = eer.Projection
	OperationReceipt = eer.OperationReceipt
	RiverJobLink     = eer.RiverJobLink
	Adapter          = eer.Adapter
	AdapterResult    = eer.AdapterResult
	Attempt          = eer.Attempt
	State            = eer.State
	Completion       = eer.Completion
)

const (
	OwnerOutbound         = eer.OwnerOutbound
	OwnerSurvey           = eer.OwnerSurvey
	KindOutboundMessage   = eer.KindOutboundMessage
	KindSurveyWebhook     = eer.KindSurveyWebhook
	KindOutboundMedia     = eer.KindOutboundMedia
	StateAccepted         = eer.StateAccepted
	StateQueued           = eer.StateQueued
	StateExecuted         = eer.StateExecuted
	StateOutcomeUnknown   = eer.StateOutcomeUnknown
	StateReconciled       = eer.StateReconciled
	StateRetryableFailed  = eer.StateRetryableFailed
	StateFinalFailed      = eer.StateFinalFailed
	CompletionFinalFailed = eer.CompletionFinalFailed
)

var (
	ErrAdapterFailure    = eer.ErrAdapterFailure
	ErrLeaseFence        = eer.ErrLeaseFence
	ErrPayloadMismatch   = eer.ErrPayloadMismatch
	ErrReconcileRequired = eer.ErrReconcileRequired
)

func NewEnvelope(input EnvelopeInput) (EffectEnvelope, error) { return eer.NewEnvelope(input) }

type Runtime interface {
	Accept(context.Context, AcceptCommand) (Projection, OperationReceipt, error)
	Queue(context.Context, QueueCommand) (Projection, OperationReceipt, error)
	Claim(context.Context, ClaimCommand) (Lease, Projection, error)
	RunAttempt(context.Context, Lease, Adapter) (Projection, OperationReceipt, error)
	Reconcile(context.Context, ReconcileCommand) (Projection, OperationReceipt, error)
}
