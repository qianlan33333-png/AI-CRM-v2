package groupopsworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

func TestDispatchWorkerProjectsControlledProviderTerminalStates(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider providerStub
		want     groupopsport.ExecutionState
		attempt  bool
		real     bool
		manual   bool
	}{
		{name: "pre dispatch", provider: providerStub{result: groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchPreDispatchFailure}}, want: groupopsport.ExecutionFinalFailed},
		{name: "accepted", provider: providerStub{result: groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchProviderAccepted, ReceiptDigest: string(digest("accepted")), BusinessCallDispatched: true, RealExternalCallExecuted: true}}, want: groupopsport.ExecutionProviderAccepted, attempt: true, real: true},
		{name: "unknown", provider: providerStub{result: groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchOutcomeUnknown, BusinessCallDispatched: true, RealExternalCallExecuted: true}}, want: groupopsport.ExecutionOutcomeUnknown, attempt: true, real: true, manual: true},
		{name: "boundary error", provider: providerStub{result: groupopsport.DispatchProviderResult{BusinessCallDispatched: true, RealExternalCallExecuted: true}, err: errors.New("controlled transport interruption")}, want: groupopsport.ExecutionOutcomeUnknown, attempt: true, real: true, manual: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &readerStub{execution: dispatchExecution(groupopsport.ExecutionAccepted)}
			projector := &projectorStub{}
			runtime := &runtimeStub{}
			worker, err := NewDispatchWorker(reader, projector, runtime, &test.provider)
			if err != nil {
				t.Fatal(err)
			}
			result, err := worker.Dispatch(context.Background(), "eer_41", digest("worker"))
			if err != nil || result.State != test.want || result.ProviderCallAttempted != test.attempt || result.RealExternalCallExecuted != test.real || result.ManualReconcileRequired != test.manual || test.provider.calls != 1 || runtime.claims != 1 || runtime.runs != 1 {
				t.Fatalf("result=%+v err=%v calls=%d claims=%d runs=%d", result, err, test.provider.calls, runtime.claims, runtime.runs)
			}
			if projector.command.State != test.want || projector.command.DeliveryProven || projector.command.AttemptCount != 1 {
				t.Fatalf("projected=%+v", projector.command)
			}
			if test.want == groupopsport.ExecutionProviderAccepted && projector.command.ProviderReceiptDigest == "" {
				t.Fatal("provider acceptance missing receipt")
			}
			if test.want == groupopsport.ExecutionOutcomeUnknown && projector.command.ProviderReceiptDigest != "" {
				t.Fatalf("unknown claimed provider receipt: %+v", projector.command)
			}
		})
	}
}

func TestDispatchWorkerNeverReplaysOutcomeUnknown(t *testing.T) {
	provider := &providerStub{result: groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchProviderAccepted, ReceiptDigest: string(digest("accepted")), BusinessCallDispatched: true, RealExternalCallExecuted: true}}
	runtime := &runtimeStub{}
	worker, err := NewDispatchWorker(&readerStub{execution: dispatchExecution(groupopsport.ExecutionOutcomeUnknown)}, &projectorStub{}, runtime, provider)
	if err != nil {
		t.Fatal(err)
	}
	result, err := worker.Dispatch(context.Background(), "eer_41", digest("worker"))
	if err != nil || result.State != groupopsport.ExecutionOutcomeUnknown || !result.ManualReconcileRequired || provider.calls != 0 || runtime.claims != 0 || runtime.runs != 0 {
		t.Fatalf("result=%+v err=%v calls=%d claims=%d runs=%d", result, err, provider.calls, runtime.claims, runtime.runs)
	}
}

func TestDispatchWorkerRecoversTerminalEERBeforeCallingProvider(t *testing.T) {
	provider := &providerStub{result: groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchProviderAccepted, ReceiptDigest: string(digest("unexpected")), BusinessCallDispatched: true, RealExternalCallExecuted: true}}
	runtime := &runtimeStub{terminal: eer.TerminalOutcome{EffectID: "eer_41", Owner: eer.OwnerGroupOps, Kind: eer.KindGroupOpsBroadcast, State: eer.StateExecuted, ReceiptDigest: digest("accepted")}}
	projector := &projectorStub{}
	worker, err := NewDispatchWorker(&readerStub{execution: dispatchExecution(groupopsport.ExecutionAccepted)}, projector, runtime, provider)
	if err != nil {
		t.Fatal(err)
	}
	result, err := worker.Dispatch(context.Background(), "eer_41", digest("worker"))
	if err != nil || result.State != groupopsport.ExecutionProviderAccepted || !result.ProviderAccepted || result.ProviderCallAttempted || result.RealExternalCallExecuted || provider.calls != 0 || runtime.claims != 0 || runtime.runs != 0 || projector.command.ProviderReceiptDigest != string(digest("accepted")) {
		t.Fatalf("result=%+v err=%v calls=%d claims=%d runs=%d command=%+v", result, err, provider.calls, runtime.claims, runtime.runs, projector.command)
	}
}

func TestDispatchWorkerRecoversExpiredAttemptToUnknownWithoutProviderReplay(t *testing.T) {
	provider := &providerStub{result: groupopsport.DispatchProviderResult{Outcome: groupopsport.DispatchProviderAccepted, ReceiptDigest: string(digest("unexpected")), BusinessCallDispatched: true, RealExternalCallExecuted: true}}
	runtime := &runtimeStub{}
	execution := dispatchExecution(groupopsport.ExecutionAccepted)
	execution.AttemptRecovery = &groupopsport.AttemptRecoveryLease{Generation: 2, Fence: 3, ExpiresAt: time.Now().UTC().Add(-time.Minute)}
	projector := &projectorStub{}
	worker, err := NewDispatchWorker(&readerStub{execution: execution}, projector, runtime, provider)
	if err != nil {
		t.Fatal(err)
	}
	result, err := worker.Dispatch(context.Background(), "eer_41", digest("worker"))
	if err != nil || result.State != groupopsport.ExecutionOutcomeUnknown || !result.ManualReconcileRequired || result.ProviderCallAttempted || result.RealExternalCallExecuted || provider.calls != 0 || runtime.recovers != 1 || runtime.claims != 0 || runtime.runs != 0 || projector.command.State != groupopsport.ExecutionOutcomeUnknown {
		t.Fatalf("result=%+v err=%v calls=%d recovers=%d claims=%d runs=%d command=%+v", result, err, provider.calls, runtime.recovers, runtime.claims, runtime.runs, projector.command)
	}
}

type readerStub struct {
	execution groupopsport.DispatchExecution
}

func (stub *readerStub) LoadDispatchExecution(_ context.Context, effectID string) (groupopsport.DispatchExecution, error) {
	if effectID != stub.execution.ExternalEffectID {
		return groupopsport.DispatchExecution{}, errors.New("not found")
	}
	return stub.execution, nil
}

type projectorStub struct {
	command groupopsport.ExecutionOutcomeCommand
}

func (stub *projectorStub) ProjectExecutionOutcome(_ context.Context, command groupopsport.ExecutionOutcomeCommand) (groupopsport.Execution, error) {
	stub.command = command
	return groupopsport.Execution{ID: command.ExecutionID, State: command.State, ProviderAccepted: command.ProviderAccepted, DeliveryProven: command.DeliveryProven, AttemptCount: command.AttemptCount, ProviderReceiptPresent: command.ProviderReceiptDigest != ""}, nil
}

type runtimeStub struct {
	claims   int
	runs     int
	recovers int
	terminal eer.TerminalOutcome
}

func (stub *runtimeStub) Claim(_ context.Context, command eer.ClaimCommand) (eer.Lease, eer.Projection, error) {
	stub.claims++
	now := time.Now().UTC()
	return eer.Lease{EffectID: command.EffectID, Generation: 1, Fence: 1, ExpiresAt: now.Add(time.Minute)}, eer.Projection{ID: command.EffectID, Owner: eer.OwnerGroupOps, Kind: eer.KindGroupOpsBroadcast, State: eer.StateQueued, Generation: 1, UpdatedAt: now}, nil
}

func (stub *runtimeStub) RunAttempt(ctx context.Context, lease eer.Lease, adapter eer.Adapter) (eer.Projection, eer.OperationReceipt, error) {
	stub.runs++
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerGroupOps, Kind: eer.KindGroupOpsBroadcast, SourceRefDigest: digest("source"), TargetRefDigest: digest("target"), PayloadDigest: digest("payload"), PolicyVersionHash: digest("policy")})
	if err != nil {
		return eer.Projection{}, eer.OperationReceipt{}, err
	}
	result, adapterErr := adapter.Execute(ctx, envelope, eer.Attempt{Number: 1, Generation: lease.Generation, Fence: lease.Fence, StartedAt: time.Now().UTC()})
	state := eer.StateOutcomeUnknown
	switch result.Completion {
	case eer.CompletionExecuted:
		state = eer.StateExecuted
	case eer.CompletionFinalFailed:
		state = eer.StateFinalFailed
	case eer.CompletionOutcomeUnknown:
		state = eer.StateOutcomeUnknown
	}
	now := time.Now().UTC()
	projection := eer.Projection{ID: lease.EffectID, Owner: eer.OwnerGroupOps, Kind: eer.KindGroupOpsBroadcast, State: state, AttemptCount: 1, Generation: lease.Generation, UpdatedAt: now}
	return projection, eer.OperationReceipt{ID: "attempt", EffectID: lease.EffectID, CommandDigest: result.ReceiptDigest, State: state, CompletedAt: now}, adapterErr
}

func (stub *runtimeStub) RecoverAttemptedToUnknown(_ context.Context, command eer.RecoverAttemptedCommand) (eer.Projection, eer.OperationReceipt, error) {
	stub.recovers++
	now := time.Now().UTC()
	return eer.Projection{ID: command.Lease.EffectID, Owner: eer.OwnerGroupOps, Kind: eer.KindGroupOpsBroadcast, State: eer.StateOutcomeUnknown, AttemptCount: 1, Generation: command.Lease.Generation, UpdatedAt: now}, eer.OperationReceipt{ID: "recover", EffectID: command.Lease.EffectID, CommandDigest: digest("recover"), State: eer.StateOutcomeUnknown, CompletedAt: now}, nil
}

func (stub *runtimeStub) GetTerminalOutcome(_ context.Context, effectID string) (eer.TerminalOutcome, error) {
	if stub.terminal.EffectID != effectID {
		return eer.TerminalOutcome{}, eer.ErrNotFound
	}
	return stub.terminal, nil
}

type providerStub struct {
	result groupopsport.DispatchProviderResult
	err    error
	calls  int
}

func (stub *providerStub) Dispatch(_ context.Context, _ groupopsport.DispatchRequest) (groupopsport.DispatchProviderResult, error) {
	stub.calls++
	return stub.result, stub.err
}

func dispatchExecution(state groupopsport.ExecutionState) groupopsport.DispatchExecution {
	return groupopsport.DispatchExecution{ExecutionID: 11, ExternalEffectID: "eer_41", State: state, TargetReference: "chat:controlled", SenderUserID: "staff-1", ContentSnapshot: []byte(`{"schema_version":1}`), ContentDigest: string(digest("content")), MaterialSnapshot: []byte(`{"schema_version":1}`), MaterialDigest: string(digest("material"))}
}

func digest(value string) eer.Digest {
	sum := sha256.Sum256([]byte(value))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
