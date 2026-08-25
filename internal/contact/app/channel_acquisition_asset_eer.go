package app

import (
	"context"
	"errors"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

// ChannelAcquisitionAssetExternalEffectsRuntime is the exact EER surface used
// by CH02. Crash recovery is explicit so unrelated EER consumers do not gain a
// recovery mutation through the common Runtime interface.
type ChannelAcquisitionAssetExternalEffectsRuntime interface {
	eer.Runtime
	eer.RecoveryRuntime
}

// ChannelAcquisitionAssetEERRuntime closes every CH02 command over the single
// Contact-owned family. Callers cannot choose another owner or effect kind.
type ChannelAcquisitionAssetEERRuntime struct {
	runtime  ChannelAcquisitionAssetExternalEffectsRuntime
	terminal eer.TerminalReader
}

var _ ChannelAcquisitionAssetEffectRuntime = (*ChannelAcquisitionAssetEERRuntime)(nil)

func NewChannelAcquisitionAssetEERRuntime(runtime ChannelAcquisitionAssetExternalEffectsRuntime, terminal eer.TerminalReader) (*ChannelAcquisitionAssetEERRuntime, error) {
	if channelAcquisitionAssetNil(runtime) || channelAcquisitionAssetNil(terminal) {
		return nil, ErrChannelAcquisitionAssetUnavailable
	}
	return &ChannelAcquisitionAssetEERRuntime{runtime: runtime, terminal: terminal}, nil
}

func (runtime *ChannelAcquisitionAssetEERRuntime) Terminal(ctx context.Context, effectID string) (ChannelAcquisitionAssetEffectTerminal, bool, error) {
	if !runtime.ready(ctx) || effectID == "" {
		return ChannelAcquisitionAssetEffectTerminal{}, false, ErrChannelAcquisitionAssetUnavailable
	}
	outcome, err := runtime.terminal.GetTerminalOutcome(ctx, effectID)
	if errors.Is(err, eer.ErrNotFound) {
		return ChannelAcquisitionAssetEffectTerminal{}, false, nil
	}
	if err != nil {
		return ChannelAcquisitionAssetEffectTerminal{}, false, err
	}
	if outcome.Owner != eer.OwnerContact || outcome.Kind != eer.KindContactAcquisitionAssetPublish {
		return ChannelAcquisitionAssetEffectTerminal{}, false, ErrChannelAcquisitionAssetUnavailable
	}
	terminal := ChannelAcquisitionAssetEffectTerminal{
		EffectID: outcome.EffectID, State: outcome.State,
		Receipt:               eer.OperationReceipt{ID: outcome.ReceiptID, EffectID: outcome.EffectID, CommandDigest: outcome.ReceiptDigest, State: outcome.State, CompletedAt: outcome.LeaseExpiresAt},
		Lease:                 eer.Lease{EffectID: outcome.EffectID, Generation: outcome.Generation, Fence: outcome.Fence, ExpiresAt: outcome.LeaseExpiresAt},
		ResultReferenceDigest: outcome.ResultReferenceDigest, BusinessCallDispatched: outcome.BusinessCallDispatched,
		RealExternalCallExecuted: outcome.RealExternalCallExecuted,
	}
	if !validChannelAcquisitionAssetEffectTerminal(terminal, effectID) {
		return ChannelAcquisitionAssetEffectTerminal{}, false, ErrChannelAcquisitionAssetUnavailable
	}
	return terminal, true, nil
}

func (runtime *ChannelAcquisitionAssetEERRuntime) Accept(ctx context.Context, command ChannelAcquisitionAssetEffectAcceptCommand) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error) {
	if !runtime.ready(ctx) || !validChannelAcquisitionAssetDigest(command.ReceiptKeyDigest) || command.Spec.Fingerprint() == "" {
		return ChannelAcquisitionAssetEffectProjection{}, eer.OperationReceipt{}, ErrChannelAcquisitionAssetUnavailable
	}
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{
		Owner: eer.OwnerContact, Kind: eer.KindContactAcquisitionAssetPublish,
		SourceRefDigest: command.Spec.SourceRefDigest, TargetRefDigest: command.Spec.TargetRefDigest,
		PayloadDigest: command.Spec.PayloadDigest, PolicyVersionHash: command.Spec.PolicyVersionHash,
	})
	if err != nil {
		return ChannelAcquisitionAssetEffectProjection{}, eer.OperationReceipt{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	projection, receipt, err := runtime.runtime.Accept(ctx, eer.AcceptCommand{ReceiptKeyDigest: command.ReceiptKeyDigest, Envelope: envelope})
	typed, receipt, err := runtime.projection(projection, "", eer.StateAccepted, receipt, err)
	if typed.ID != "" {
		typed.EnvelopeFingerprint = envelope.Fingerprint()
	}
	return typed, receipt, err
}

func (runtime *ChannelAcquisitionAssetEERRuntime) Queue(ctx context.Context, command ChannelAcquisitionAssetEffectQueueCommand) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error) {
	if !runtime.ready(ctx) || command.EffectID == "" || !validChannelAcquisitionAssetDigest(command.ReceiptKeyDigest) {
		return ChannelAcquisitionAssetEffectProjection{}, eer.OperationReceipt{}, ErrChannelAcquisitionAssetUnavailable
	}
	projection, receipt, err := runtime.runtime.Queue(ctx, eer.QueueCommand{
		EffectID: command.EffectID, Job: command.Job, ReceiptKeyDigest: command.ReceiptKeyDigest,
	})
	return runtime.projection(projection, command.EffectID, eer.StateQueued, receipt, err)
}

func (runtime *ChannelAcquisitionAssetEERRuntime) Claim(ctx context.Context, command ChannelAcquisitionAssetEffectClaimCommand) (eer.Lease, ChannelAcquisitionAssetEffectProjection, error) {
	if !runtime.ready(ctx) || command.EffectID == "" || !validChannelAcquisitionAssetDigest(command.WorkerDigest) {
		return eer.Lease{}, ChannelAcquisitionAssetEffectProjection{}, ErrChannelAcquisitionAssetUnavailable
	}
	lease, projection, err := runtime.runtime.Claim(ctx, eer.ClaimCommand{EffectID: command.EffectID, WorkerDigest: command.WorkerDigest})
	typed, _, projectionErr := runtime.projection(projection, command.EffectID, eer.StateQueued, eer.OperationReceipt{}, err)
	if projectionErr != nil || lease.EffectID != command.EffectID || lease.Generation != typed.Generation || lease.Fence < 1 || lease.ExpiresAt.IsZero() {
		return eer.Lease{}, ChannelAcquisitionAssetEffectProjection{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, projectionErr)
	}
	return lease, typed, nil
}

func (runtime *ChannelAcquisitionAssetEERRuntime) RunAttempt(ctx context.Context, lease eer.Lease, execute func(context.Context) (eer.AdapterResult, error)) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error) {
	if !runtime.ready(ctx) || lease.EffectID == "" || lease.Generation < 1 || lease.Fence < 1 || lease.ExpiresAt.IsZero() || execute == nil {
		return ChannelAcquisitionAssetEffectProjection{}, eer.OperationReceipt{}, ErrChannelAcquisitionAssetUnavailable
	}
	projection, receipt, err := runtime.runtime.RunAttempt(ctx, lease, channelAcquisitionAssetAdapter{lease: lease, execute: execute})
	if !channelAcquisitionAssetAttemptTerminal(projection.State) {
		return ChannelAcquisitionAssetEffectProjection{}, eer.OperationReceipt{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	return runtime.projection(projection, lease.EffectID, projection.State, receipt, err)
}

func (runtime *ChannelAcquisitionAssetEERRuntime) Reconcile(ctx context.Context, command ChannelAcquisitionAssetEffectReconcileCommand) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error) {
	if !runtime.ready(ctx) || !validChannelAcquisitionAssetDigest(command.ReceiptKeyDigest) || !validChannelAcquisitionAssetDigest(command.EvidenceDigest) {
		return ChannelAcquisitionAssetEffectProjection{}, eer.OperationReceipt{}, ErrChannelAcquisitionAssetUnavailable
	}
	projection, receipt, err := runtime.runtime.Reconcile(ctx, eer.ReconcileCommand{
		Lease: command.Lease, ReceiptKeyDigest: command.ReceiptKeyDigest, EvidenceDigest: command.EvidenceDigest,
	})
	return runtime.projection(projection, command.Lease.EffectID, eer.StateReconciled, receipt, err)
}

func (runtime *ChannelAcquisitionAssetEERRuntime) RecoverAttempted(ctx context.Context, lease eer.Lease) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error) {
	if !runtime.ready(ctx) || lease.EffectID == "" || lease.Generation < 1 || lease.Fence < 1 || lease.ExpiresAt.IsZero() {
		return ChannelAcquisitionAssetEffectProjection{}, eer.OperationReceipt{}, ErrChannelAcquisitionAssetUnavailable
	}
	projection, receipt, err := runtime.runtime.RecoverAttemptedToUnknown(ctx, eer.RecoverAttemptedCommand{Lease: lease})
	return runtime.projection(projection, lease.EffectID, eer.StateOutcomeUnknown, receipt, err)
}

func (runtime *ChannelAcquisitionAssetEERRuntime) ready(ctx context.Context) bool {
	return runtime != nil && ctx != nil && ctx.Err() == nil && !channelAcquisitionAssetNil(runtime.runtime) && !channelAcquisitionAssetNil(runtime.terminal)
}

func (*ChannelAcquisitionAssetEERRuntime) projection(projection eer.Projection, effectID string, state eer.State, receipt eer.OperationReceipt, err error) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error) {
	if err != nil && projection.ID == "" {
		return ChannelAcquisitionAssetEffectProjection{}, eer.OperationReceipt{}, err
	}
	if projection.ID == "" || effectID != "" && projection.ID != effectID || projection.Owner != eer.OwnerContact ||
		projection.Kind != eer.KindContactAcquisitionAssetPublish || projection.State != state || projection.Generation < 1 || projection.UpdatedAt.IsZero() {
		return ChannelAcquisitionAssetEffectProjection{}, eer.OperationReceipt{}, errors.Join(ErrChannelAcquisitionAssetUnavailable, err)
	}
	return ChannelAcquisitionAssetEffectProjection{
		ID: projection.ID, State: projection.State, Generation: projection.Generation, UpdatedAt: projection.UpdatedAt,
	}, receipt, err
}

type channelAcquisitionAssetAdapter struct {
	lease   eer.Lease
	execute func(context.Context) (eer.AdapterResult, error)
}

func (adapter channelAcquisitionAssetAdapter) Execute(ctx context.Context, envelope eer.EffectEnvelope, attempt eer.Attempt) (eer.AdapterResult, error) {
	if ctx == nil || ctx.Err() != nil || adapter.execute == nil || envelope.Owner() != eer.OwnerContact ||
		envelope.Kind() != eer.KindContactAcquisitionAssetPublish || attempt.Number < 1 ||
		attempt.Generation != adapter.lease.Generation || attempt.Fence != adapter.lease.Fence || attempt.StartedAt.IsZero() {
		return eer.AdapterResult{}, ErrChannelAcquisitionAssetUnavailable
	}
	return adapter.execute(ctx)
}

func channelAcquisitionAssetAttemptTerminal(state eer.State) bool {
	return state == eer.StateExecuted || state == eer.StateFinalFailed || state == eer.StateOutcomeUnknown
}
