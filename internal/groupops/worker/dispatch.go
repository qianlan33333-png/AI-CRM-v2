// Package groupopsworker runs one already-queued Group Ops EER effect through
// the narrow provider boundary. River registration and concrete Provider
// protocol wiring remain integration concerns.
package groupopsworker

import (
	"context"
	"encoding/hex"
	"errors"
	"reflect"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	groupopsprovider "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/provider"
)

var (
	ErrInvalid     = errors.New("group ops dispatch invalid")
	ErrUnavailable = errors.New("group ops dispatch unavailable")
)

type Runtime interface {
	Claim(context.Context, eer.ClaimCommand) (eer.Lease, eer.Projection, error)
	RunAttempt(context.Context, eer.Lease, eer.Adapter) (eer.Projection, eer.OperationReceipt, error)
	RecoverAttemptedToUnknown(context.Context, eer.RecoverAttemptedCommand) (eer.Projection, eer.OperationReceipt, error)
	GetTerminalOutcome(context.Context, string) (eer.TerminalOutcome, error)
}

type DispatchWorker struct {
	reader    groupopsport.DispatchExecutionReader
	projector groupopsport.ExecutionOutcomeProjector
	runtime   Runtime
	provider  groupopsport.DispatchProvider
}

func NewDispatchWorker(reader groupopsport.DispatchExecutionReader, projector groupopsport.ExecutionOutcomeProjector, runtime Runtime, provider groupopsport.DispatchProvider) (*DispatchWorker, error) {
	if nilDependency(reader) || nilDependency(projector) || nilDependency(runtime) || nilDependency(provider) {
		return nil, ErrInvalid
	}
	return &DispatchWorker{reader: reader, projector: projector, runtime: runtime, provider: provider}, nil
}

func (worker *DispatchWorker) Dispatch(ctx context.Context, effectID string, workerDigest eer.Digest) (groupopsport.DispatchResult, error) {
	if worker == nil || ctx == nil || effectID == "" || !validDigest(workerDigest) {
		return groupopsport.DispatchResult{}, ErrInvalid
	}
	execution, err := worker.reader.LoadDispatchExecution(ctx, effectID)
	if err != nil {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	if execution.ExecutionID < 1 || execution.ExternalEffectID != effectID {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	if execution.State != groupopsport.ExecutionAccepted {
		return terminalResult(execution), nil
	}
	if terminal, terminalErr := worker.runtime.GetTerminalOutcome(ctx, effectID); terminalErr == nil {
		return worker.projectTerminalOutcome(ctx, execution, terminal)
	} else if !errors.Is(terminalErr, eer.ErrNotFound) {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	if execution.AttemptRecovery != nil {
		return worker.recoverAttempted(ctx, execution)
	}
	adapter, err := groupopsprovider.NewDispatchAdapter(worker.provider, execution)
	if err != nil {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	lease, queued, err := worker.runtime.Claim(ctx, eer.ClaimCommand{EffectID: effectID, WorkerDigest: workerDigest})
	if err != nil {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	if lease.EffectID != effectID || queued.ID != effectID || queued.Owner != eer.OwnerGroupOps || queued.Kind != eer.KindGroupOpsBroadcast || queued.State != eer.StateQueued {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	captured := &recordingAdapter{adapter: adapter}
	projection, _, runErr := worker.runtime.RunAttempt(ctx, lease, captured)
	if projection.ID != effectID || projection.Owner != eer.OwnerGroupOps || projection.Kind != eer.KindGroupOpsBroadcast || !terminalState(projection.State) {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	// A terminal projection is authoritative here. RunAttempt returns an
	// adapter error after it has persisted outcome_unknown; that outcome must
	// still be projected to Group Ops rather than silently left at accepted.
	_ = runErr
	command, result, ok := projectCommand(execution, projection, captured)
	if !ok {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	updated, err := worker.projector.ProjectExecutionOutcome(ctx, command)
	if err != nil || updated.ID != execution.ExecutionID || updated.State != command.State || updated.DeliveryProven {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	result.State = updated.State
	result.ProviderAccepted = updated.ProviderAccepted
	return result, nil
}

func (worker *DispatchWorker) recoverAttempted(ctx context.Context, execution groupopsport.DispatchExecution) (groupopsport.DispatchResult, error) {
	recovery := execution.AttemptRecovery
	if recovery == nil || recovery.Generation < 1 || recovery.Fence < 1 || recovery.ExpiresAt.IsZero() {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	projection, _, err := worker.runtime.RecoverAttemptedToUnknown(ctx, eer.RecoverAttemptedCommand{Lease: eer.Lease{EffectID: execution.ExternalEffectID, Generation: recovery.Generation, Fence: recovery.Fence, ExpiresAt: recovery.ExpiresAt}})
	if err != nil || projection.ID != execution.ExternalEffectID || projection.Owner != eer.OwnerGroupOps || projection.Kind != eer.KindGroupOpsBroadcast || projection.State != eer.StateOutcomeUnknown || projection.AttemptCount < 1 {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	updated, err := worker.projector.ProjectExecutionOutcome(ctx, groupopsport.ExecutionOutcomeCommand{ExecutionID: execution.ExecutionID, State: groupopsport.ExecutionOutcomeUnknown, AttemptCount: projection.AttemptCount})
	if err != nil || updated.ID != execution.ExecutionID || updated.State != groupopsport.ExecutionOutcomeUnknown || updated.DeliveryProven {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	return groupopsport.DispatchResult{ExecutionID: execution.ExecutionID, EffectID: execution.ExternalEffectID, State: updated.State, ProviderAccepted: updated.ProviderAccepted, ManualReconcileRequired: true}, nil
}

// projectTerminalOutcome closes the crash window between EER completion and
// the owner-domain projection. It reads only the safe terminal receipt; an
// unknown outcome still gets no Provider receipt or automatic retry.
func (worker *DispatchWorker) projectTerminalOutcome(ctx context.Context, execution groupopsport.DispatchExecution, terminal eer.TerminalOutcome) (groupopsport.DispatchResult, error) {
	if terminal.EffectID != execution.ExternalEffectID || terminal.Owner != eer.OwnerGroupOps || terminal.Kind != eer.KindGroupOpsBroadcast || !validDigest(terminal.ReceiptDigest) {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	command := groupopsport.ExecutionOutcomeCommand{ExecutionID: execution.ExecutionID, AttemptCount: 1}
	result := groupopsport.DispatchResult{ExecutionID: execution.ExecutionID, EffectID: execution.ExternalEffectID}
	switch terminal.State {
	case eer.StateExecuted:
		command.State, command.ProviderAccepted, command.ProviderReceiptDigest = groupopsport.ExecutionProviderAccepted, true, string(terminal.ReceiptDigest)
	case eer.StateOutcomeUnknown:
		command.State, result.ManualReconcileRequired = groupopsport.ExecutionOutcomeUnknown, true
	case eer.StateFinalFailed:
		command.State = groupopsport.ExecutionFinalFailed
	default:
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	updated, err := worker.projector.ProjectExecutionOutcome(ctx, command)
	if err != nil || updated.ID != execution.ExecutionID || updated.State != command.State || updated.DeliveryProven {
		return groupopsport.DispatchResult{}, ErrUnavailable
	}
	result.State, result.ProviderAccepted = updated.State, updated.ProviderAccepted
	return result, nil
}

type recordingAdapter struct {
	adapter   *groupopsprovider.DispatchAdapter
	result    eer.AdapterResult
	completed bool
}

func (adapter *recordingAdapter) Execute(ctx context.Context, envelope eer.EffectEnvelope, attempt eer.Attempt) (eer.AdapterResult, error) {
	result, err := adapter.adapter.Execute(ctx, envelope, attempt)
	adapter.result = result
	adapter.completed = err == nil
	return result, err
}

func projectCommand(execution groupopsport.DispatchExecution, projection eer.Projection, captured *recordingAdapter) (groupopsport.ExecutionOutcomeCommand, groupopsport.DispatchResult, bool) {
	result := groupopsport.DispatchResult{ExecutionID: execution.ExecutionID, EffectID: execution.ExternalEffectID, DeliveryProven: false}
	if captured != nil {
		result.ProviderCallAttempted = captured.result.BusinessCallDispatched
		result.RealExternalCallExecuted = captured.result.RealExternalCallExecuted
	}
	command := groupopsport.ExecutionOutcomeCommand{ExecutionID: execution.ExecutionID, AttemptCount: projection.AttemptCount}
	if command.AttemptCount < 1 {
		return groupopsport.ExecutionOutcomeCommand{}, groupopsport.DispatchResult{}, false
	}
	switch projection.State {
	case eer.StateExecuted:
		if captured == nil || !captured.completed || !captured.result.BusinessCallDispatched || !captured.result.RealExternalCallExecuted {
			return groupopsport.ExecutionOutcomeCommand{}, groupopsport.DispatchResult{}, false
		}
		command.State, command.ProviderAccepted, command.ProviderReceiptDigest = groupopsport.ExecutionProviderAccepted, true, string(captured.result.ReceiptDigest)
	case eer.StateOutcomeUnknown:
		command.State = groupopsport.ExecutionOutcomeUnknown
		result.ManualReconcileRequired = true
	case eer.StateFinalFailed:
		command.State = groupopsport.ExecutionFinalFailed
		if captured != nil && captured.completed && captured.result.BusinessCallDispatched {
			command.ProviderReceiptDigest = string(captured.result.ReceiptDigest)
		}
	default:
		return groupopsport.ExecutionOutcomeCommand{}, groupopsport.DispatchResult{}, false
	}
	return command, result, true
}

func terminalResult(execution groupopsport.DispatchExecution) groupopsport.DispatchResult {
	return groupopsport.DispatchResult{ExecutionID: execution.ExecutionID, EffectID: execution.ExternalEffectID, State: execution.State, ProviderAccepted: execution.State == groupopsport.ExecutionProviderAccepted || execution.State == groupopsport.ExecutionDeliveryProven || execution.DeliveryProven, DeliveryProven: execution.DeliveryProven, ManualReconcileRequired: execution.State == groupopsport.ExecutionOutcomeUnknown}
}

func terminalState(state eer.State) bool {
	return state == eer.StateExecuted || state == eer.StateOutcomeUnknown || state == eer.StateFinalFailed
}

func validDigest(value eer.Digest) bool {
	if len(value) != len("sha256:")+64 || string(value[:7]) != "sha256:" {
		return false
	}
	_, err := hex.DecodeString(string(value[7:]))
	return err == nil
}

func nilDependency(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	return (ref.Kind() == reflect.Interface || ref.Kind() == reflect.Ptr) && ref.IsNil()
}
