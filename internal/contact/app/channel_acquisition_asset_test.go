package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

type acquisitionAssetTestUoW struct{ calls int }

func acquisitionAssetTestKey(label string) string {
	return strings.Repeat("i", 16) + ":" + label
}

func (uow *acquisitionAssetTestUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	return callback(ctx)
}

type acquisitionAssetMemoryStore struct {
	snapshot         contactport.AcquisitionAssetSnapshot
	receipts         map[string]ChannelAcquisitionAssetActorReceipt
	bindings         map[string]ChannelAcquisitionAssetBinding
	nextReceiptID    int64
	failMarkOnce     bool
	failCompleteOnce bool
	nextVersion      int64
}

func newAcquisitionAssetMemoryStore() *acquisitionAssetMemoryStore {
	return &acquisitionAssetMemoryStore{
		snapshot: contactport.AcquisitionAssetSnapshot{
			ChannelID: 41, ChannelRevision: 7, ChannelCode: "open-course", ChannelName: "公开课",
			ChannelStatus: "active", SceneValue: "scene-41", AssigneeWeComUserIDs: []string{"staff-b", "staff-a"},
		},
		receipts: make(map[string]ChannelAcquisitionAssetActorReceipt), bindings: make(map[string]ChannelAcquisitionAssetBinding),
	}
}

func (store *acquisitionAssetMemoryStore) LockSnapshot(_ context.Context, channelID int64) (contactport.AcquisitionAssetSnapshot, error) {
	if channelID != store.snapshot.ChannelID {
		return contactport.AcquisitionAssetSnapshot{}, ErrChannelAcquisitionAssetUnavailable
	}
	result := store.snapshot
	result.AssigneeWeComUserIDs = append([]string(nil), result.AssigneeWeComUserIDs...)
	return result, nil
}

func (store *acquisitionAssetMemoryStore) ReserveActorReceipt(_ context.Context, candidate ChannelAcquisitionAssetActorReceipt) (ChannelAcquisitionAssetActorReceipt, bool, error) {
	key := fmt.Sprintf("%d:%s", candidate.Actor, candidate.KeyDigest)
	if existing, ok := store.receipts[key]; ok {
		return existing, false, nil
	}
	store.nextReceiptID++
	candidate.ID = store.nextReceiptID
	store.receipts[key] = candidate
	return candidate, true, nil
}

func (store *acquisitionAssetMemoryStore) CompleteActorReceipt(_ context.Context, receiptID int64, resultEffectID, replacementEffectID string, completedAt time.Time) (ChannelAcquisitionAssetActorReceipt, error) {
	for key, receipt := range store.receipts {
		if receipt.ID == receiptID {
			receipt.State = ChannelAcquisitionAssetReceiptCompleted
			receipt.ResultEffectID = resultEffectID
			receipt.ReplacementEffectID = replacementEffectID
			receipt.CompletedAt = completedAt
			store.receipts[key] = receipt
			return receipt, nil
		}
	}
	return ChannelAcquisitionAssetActorReceipt{}, ErrChannelAcquisitionAssetUnavailable
}

func (store *acquisitionAssetMemoryStore) NextAssetVersion(_ context.Context, _, _ int64, _ contactport.AcquisitionAssetKind) (int64, error) {
	store.nextVersion++
	return store.nextVersion, nil
}

func (store *acquisitionAssetMemoryStore) InsertAccepted(_ context.Context, binding ChannelAcquisitionAssetBinding) (ChannelAcquisitionAssetBinding, error) {
	if _, exists := store.bindings[binding.EffectID]; exists {
		return ChannelAcquisitionAssetBinding{}, ErrChannelAcquisitionAssetConflict
	}
	store.bindings[binding.EffectID] = cloneChannelAcquisitionAssetBinding(binding)
	return cloneChannelAcquisitionAssetBinding(binding), nil
}

func (store *acquisitionAssetMemoryStore) MarkQueued(_ context.Context, effectID string, link eer.RiverJobLink, receipt eer.OperationReceipt, at time.Time) (ChannelAcquisitionAssetBinding, error) {
	binding, ok := store.bindings[effectID]
	if !ok || !channelAcquisitionAssetCanTransition(binding.State, eer.StateQueued) {
		return ChannelAcquisitionAssetBinding{}, ErrChannelAcquisitionAssetUnavailable
	}
	binding.State, binding.RiverJobID, binding.Generation = eer.StateQueued, link.JobID, link.Generation
	binding.QueueReceiptID, binding.QueueReceiptDigest, binding.UpdatedAt = receipt.ID, receipt.CommandDigest, at
	store.bindings[effectID] = binding
	return cloneChannelAcquisitionAssetBinding(binding), nil
}

func (store *acquisitionAssetMemoryStore) LockBinding(_ context.Context, effectID string) (ChannelAcquisitionAssetBinding, error) {
	binding, ok := store.bindings[effectID]
	if !ok {
		return ChannelAcquisitionAssetBinding{}, ErrChannelAcquisitionAssetUnavailable
	}
	return cloneChannelAcquisitionAssetBinding(binding), nil
}

func (store *acquisitionAssetMemoryStore) MarkAttempted(_ context.Context, effectID string, lease eer.Lease, at time.Time) (ChannelAcquisitionAssetBinding, error) {
	if store.failMarkOnce {
		store.failMarkOnce = false
		return ChannelAcquisitionAssetBinding{}, ErrChannelAcquisitionAssetUnavailable
	}
	binding, ok := store.bindings[effectID]
	if !ok || !channelAcquisitionAssetCanTransition(binding.State, channelAcquisitionAssetStateAttempted) {
		return ChannelAcquisitionAssetBinding{}, ErrChannelAcquisitionAssetUnavailable
	}
	binding.State, binding.Generation, binding.Fence, binding.LeaseExpiresAt, binding.UpdatedAt = channelAcquisitionAssetStateAttempted, lease.Generation, lease.Fence, lease.ExpiresAt, at
	store.bindings[effectID] = binding
	return cloneChannelAcquisitionAssetBinding(binding), nil
}

func (store *acquisitionAssetMemoryStore) CompleteAttempt(_ context.Context, completion ChannelAcquisitionAssetAttemptCompletion) (ChannelAcquisitionAssetBinding, error) {
	if store.failCompleteOnce {
		store.failCompleteOnce = false
		return ChannelAcquisitionAssetBinding{}, ErrChannelAcquisitionAssetUnavailable
	}
	binding, ok := store.bindings[completion.EffectID]
	if !ok || !channelAcquisitionAssetCanTransition(binding.State, completion.State) || binding.Generation != completion.Lease.Generation || binding.Fence != completion.Lease.Fence {
		return ChannelAcquisitionAssetBinding{}, ErrChannelAcquisitionAssetUnavailable
	}
	binding.State, binding.AttemptReceiptID, binding.AttemptReceiptDigest = completion.State, completion.Receipt.ID, completion.Receipt.CommandDigest
	binding.ProviderAssetReferenceDigest = completion.ProviderAssetReferenceDigest
	binding.ProviderCallAttempted, binding.RealExternalCallExecuted = completion.ProviderCallAttempted, completion.RealExternalCallExecuted
	binding.UpdatedAt = completion.CompletedAt
	store.bindings[completion.EffectID] = binding
	return cloneChannelAcquisitionAssetBinding(binding), nil
}

func TestCH02TerminalEERConvergesContactWithoutSecondProviderCall(t *testing.T) {
	t.Run("attempted executed", func(t *testing.T) {
		provider := &acquisitionAssetProvider{result: contactport.AcquisitionAssetProviderResult{
			Outcome: contactport.AcquisitionAssetProviderExecuted, ReceiptDigest: sha256.Sum256([]byte("terminal-receipt")),
			AssetReferenceDigest: sha256.Sum256([]byte("terminal-reference")), BusinessEndpointDispatched: true, RealExternalCallExecuted: true,
		}}
		service, store, _, _ := newAcquisitionAssetServiceFixture(t, provider)
		accepted, err := service.Publish(context.Background(), PublishChannelAcquisitionAssetCommand{ChannelID: 41, Actor: 7, IdempotencyKey: acquisitionAssetTestKey("terminal-attempted"), Kind: contactport.AcquisitionAssetQRCode})
		if err != nil {
			t.Fatal(err)
		}
		store.failCompleteOnce = true
		if _, err = service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "first")); err == nil {
			t.Fatal("expected injected Contact completion failure")
		}
		if store.bindings[accepted.EffectID].State != channelAcquisitionAssetStateAttempted || provider.calls != 1 {
			t.Fatalf("binding=%+v calls=%d", store.bindings[accepted.EffectID], provider.calls)
		}
		result, err := service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "redelivery"))
		if err != nil || result.State != eer.StateExecuted || provider.calls != 1 {
			t.Fatalf("result=%+v calls=%d err=%v", result, provider.calls, err)
		}
	})

	t.Run("queued unknown before provider IO", func(t *testing.T) {
		provider := &acquisitionAssetProvider{}
		service, store, _, _ := newAcquisitionAssetServiceFixture(t, provider)
		accepted, err := service.Publish(context.Background(), PublishChannelAcquisitionAssetCommand{ChannelID: 41, Actor: 7, IdempotencyKey: acquisitionAssetTestKey("terminal-queued"), Kind: contactport.AcquisitionAssetQRCode})
		if err != nil {
			t.Fatal(err)
		}
		store.failMarkOnce = true
		if _, err = service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "first")); err == nil {
			t.Fatal("expected injected mark failure")
		}
		if store.bindings[accepted.EffectID].State != eer.StateQueued || provider.calls != 0 {
			t.Fatalf("binding=%+v calls=%d", store.bindings[accepted.EffectID], provider.calls)
		}
		result, err := service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "redelivery"))
		if err != nil || result.State != eer.StateOutcomeUnknown || result.ProviderCallAttempted || result.RealExternalCallExecuted || provider.calls != 0 {
			t.Fatalf("result=%+v calls=%d err=%v", result, provider.calls, err)
		}
	})
}

func (store *acquisitionAssetMemoryStore) CompleteReconcile(_ context.Context, completion ChannelAcquisitionAssetReconcileCompletion) (ChannelAcquisitionAssetBinding, error) {
	binding, ok := store.bindings[completion.EffectID]
	if !ok || !channelAcquisitionAssetCanTransition(binding.State, eer.StateReconciled) || binding.Generation != completion.Lease.Generation || binding.Fence != completion.Lease.Fence {
		return ChannelAcquisitionAssetBinding{}, ErrChannelAcquisitionAssetReconcileRequired
	}
	binding.State, binding.ReconcileReceiptID, binding.ReconcileReceiptDigest = eer.StateReconciled, completion.Receipt.ID, completion.Receipt.CommandDigest
	binding.ReconcileEvidenceDigest, binding.ReconcileResolution, binding.ReconciledAt, binding.UpdatedAt = completion.EvidenceDigest, completion.Resolution, completion.CompletedAt, completion.CompletedAt
	store.bindings[completion.EffectID] = binding
	return cloneChannelAcquisitionAssetBinding(binding), nil
}

type acquisitionAssetRuntime struct {
	now            time.Time
	nextEffectID   int
	acceptCalls    int
	queueCalls     int
	claimCalls     int
	runCalls       int
	reconcileCalls int
	recoverCalls   int
	states         map[string]eer.State
	terminals      map[string]ChannelAcquisitionAssetEffectTerminal
}

func newAcquisitionAssetRuntime(now time.Time) *acquisitionAssetRuntime {
	return &acquisitionAssetRuntime{now: now, states: make(map[string]eer.State), terminals: make(map[string]ChannelAcquisitionAssetEffectTerminal)}
}

func (runtime *acquisitionAssetRuntime) Accept(_ context.Context, command ChannelAcquisitionAssetEffectAcceptCommand) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error) {
	runtime.acceptCalls++
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{
		Owner: eer.OwnerContact, Kind: eer.KindContactAcquisitionAssetPublish,
		SourceRefDigest: command.Spec.SourceRefDigest, TargetRefDigest: command.Spec.TargetRefDigest,
		PayloadDigest: command.Spec.PayloadDigest, PolicyVersionHash: command.Spec.PolicyVersionHash,
	})
	if err != nil {
		return ChannelAcquisitionAssetEffectProjection{}, eer.OperationReceipt{}, err
	}
	runtime.nextEffectID++
	effectID := fmt.Sprintf("eer_ch02_%d", runtime.nextEffectID)
	runtime.states[effectID] = eer.StateAccepted
	projection := ChannelAcquisitionAssetEffectProjection{ID: effectID, State: eer.StateAccepted, Generation: 1, UpdatedAt: runtime.now, EnvelopeFingerprint: envelope.Fingerprint()}
	return projection, acquisitionAssetEffectReceipt("accept", effectID, eer.StateAccepted, runtime.now), nil
}

func (runtime *acquisitionAssetRuntime) Queue(_ context.Context, command ChannelAcquisitionAssetEffectQueueCommand) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error) {
	runtime.queueCalls++
	if runtime.states[command.EffectID] != eer.StateAccepted {
		return ChannelAcquisitionAssetEffectProjection{}, eer.OperationReceipt{}, ErrChannelAcquisitionAssetUnavailable
	}
	runtime.states[command.EffectID] = eer.StateQueued
	projection := ChannelAcquisitionAssetEffectProjection{ID: command.EffectID, State: eer.StateQueued, Generation: command.Job.Generation, UpdatedAt: runtime.now}
	return projection, acquisitionAssetEffectReceipt("queue", command.EffectID, eer.StateQueued, runtime.now), nil
}

func (runtime *acquisitionAssetRuntime) Claim(_ context.Context, command ChannelAcquisitionAssetEffectClaimCommand) (eer.Lease, ChannelAcquisitionAssetEffectProjection, error) {
	runtime.claimCalls++
	if runtime.states[command.EffectID] != eer.StateQueued {
		return eer.Lease{}, ChannelAcquisitionAssetEffectProjection{}, ErrChannelAcquisitionAssetUnavailable
	}
	lease := eer.Lease{EffectID: command.EffectID, Generation: 2, Fence: 1, ExpiresAt: runtime.now.Add(time.Minute)}
	return lease, ChannelAcquisitionAssetEffectProjection{ID: command.EffectID, State: eer.StateQueued, Generation: 2, UpdatedAt: runtime.now}, nil
}

func (runtime *acquisitionAssetRuntime) RunAttempt(ctx context.Context, lease eer.Lease, attempt func(context.Context) (eer.AdapterResult, error)) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error) {
	runtime.runCalls++
	result, err := attempt(ctx)
	state := eer.StateOutcomeUnknown
	if err == nil {
		switch result.Completion {
		case "executed":
			state = eer.StateExecuted
		case "final_failed":
			state = eer.StateFinalFailed
		case "outcome_unknown":
			state = eer.StateOutcomeUnknown
		default:
			err = eer.ErrAdapterFailure
		}
	}
	runtime.states[lease.EffectID] = state
	receipt := acquisitionAssetEffectReceipt("attempt", lease.EffectID, state, runtime.now)
	runtime.terminals[lease.EffectID] = ChannelAcquisitionAssetEffectTerminal{EffectID: lease.EffectID, State: state, Receipt: receipt, Lease: lease, ResultReferenceDigest: result.ResultReferenceDigest, BusinessCallDispatched: result.BusinessCallDispatched, RealExternalCallExecuted: result.RealExternalCallExecuted}
	return ChannelAcquisitionAssetEffectProjection{ID: lease.EffectID, State: state, Generation: lease.Generation, UpdatedAt: runtime.now}, receipt, err
}

func (runtime *acquisitionAssetRuntime) Reconcile(_ context.Context, command ChannelAcquisitionAssetEffectReconcileCommand) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error) {
	runtime.reconcileCalls++
	if runtime.states[command.Lease.EffectID] != eer.StateOutcomeUnknown {
		return ChannelAcquisitionAssetEffectProjection{}, eer.OperationReceipt{}, ErrChannelAcquisitionAssetReconcileRequired
	}
	runtime.states[command.Lease.EffectID] = eer.StateReconciled
	return ChannelAcquisitionAssetEffectProjection{ID: command.Lease.EffectID, State: eer.StateReconciled, Generation: command.Lease.Generation, UpdatedAt: runtime.now}, acquisitionAssetEffectReceipt("reconcile", command.Lease.EffectID, eer.StateReconciled, runtime.now), nil
}

func (runtime *acquisitionAssetRuntime) RecoverAttempted(_ context.Context, lease eer.Lease) (ChannelAcquisitionAssetEffectProjection, eer.OperationReceipt, error) {
	runtime.recoverCalls++
	if runtime.states[lease.EffectID] != channelAcquisitionAssetStateAttempted || lease.ExpiresAt.After(runtime.now) {
		return ChannelAcquisitionAssetEffectProjection{}, eer.OperationReceipt{}, ErrChannelAcquisitionAssetUnavailable
	}
	runtime.states[lease.EffectID] = eer.StateOutcomeUnknown
	receipt := acquisitionAssetEffectReceipt("recover", lease.EffectID, eer.StateOutcomeUnknown, runtime.now)
	runtime.terminals[lease.EffectID] = ChannelAcquisitionAssetEffectTerminal{EffectID: lease.EffectID, State: eer.StateOutcomeUnknown, Receipt: receipt, Lease: lease}
	return ChannelAcquisitionAssetEffectProjection{ID: lease.EffectID, State: eer.StateOutcomeUnknown, Generation: lease.Generation, UpdatedAt: runtime.now}, receipt, nil
}

func (runtime *acquisitionAssetRuntime) Terminal(_ context.Context, effectID string) (ChannelAcquisitionAssetEffectTerminal, bool, error) {
	terminal, ok := runtime.terminals[effectID]
	return terminal, ok, nil
}

func acquisitionAssetEffectReceipt(label, effectID string, state eer.State, at time.Time) eer.OperationReceipt {
	return eer.OperationReceipt{ID: label + "-receipt-" + effectID, EffectID: effectID, CommandDigest: channelAcquisitionAssetDigest(label, effectID), State: state, CompletedAt: at}
}

type acquisitionAssetJobs struct {
	calls int
	ids   []string
	now   time.Time
}

func (jobs *acquisitionAssetJobs) Insert(_ context.Context, args ChannelAcquisitionAssetJobArgs, generation int64, scheduledAt time.Time) (eer.RiverJobLink, error) {
	jobs.calls++
	jobs.ids = append(jobs.ids, args.EffectID)
	return eer.RiverJobLink{JobID: int64(jobs.calls), Generation: generation, Queue: "external_effects", ArgsDigest: channelAcquisitionAssetDigest("job", args.EffectID), ScheduledAt: scheduledAt}, nil
}

type acquisitionAssetProvider struct {
	calls         int
	result        contactport.AcquisitionAssetProviderResult
	err           error
	notDispatched bool
}

func (provider *acquisitionAssetProvider) PublishAcquisitionAsset(_ context.Context, _ contactport.AcquisitionAssetPublishRequest) (contactport.AcquisitionAssetProviderResult, error) {
	provider.calls++
	if provider.err != nil && !provider.notDispatched && !provider.result.BusinessEndpointDispatched {
		provider.result.BusinessEndpointDispatched = true
	}
	return provider.result, provider.err
}

func newAcquisitionAssetServiceFixture(t *testing.T, provider *acquisitionAssetProvider) (*ChannelAcquisitionAssetService, *acquisitionAssetMemoryStore, *acquisitionAssetRuntime, *acquisitionAssetJobs) {
	t.Helper()
	now := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	store := newAcquisitionAssetMemoryStore()
	runtime := newAcquisitionAssetRuntime(now)
	jobs := &acquisitionAssetJobs{now: now}
	service, err := NewChannelAcquisitionAssetService(&acquisitionAssetTestUoW{}, store, runtime, jobs, provider, "corp-test")
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	return service, store, runtime, jobs
}

func TestCH02PublishQueuesTypedVersionAndReplaysActorIdempotencyReceipt(t *testing.T) {
	service, store, runtime, jobs := newAcquisitionAssetServiceFixture(t, &acquisitionAssetProvider{})
	command := PublishChannelAcquisitionAssetCommand{ChannelID: 41, Actor: 7, IdempotencyKey: acquisitionAssetTestKey("publish"), Kind: contactport.AcquisitionAssetQRCode}
	first, err := service.Publish(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Publish(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.State != eer.StateQueued || first.AssetVersion != 1 || first.EntrantReady || first.RealExternalCallExecuted {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	if runtime.acceptCalls != 1 || runtime.queueCalls != 1 || jobs.calls != 1 || len(store.bindings) != 1 {
		t.Fatalf("accept=%d queue=%d jobs=%d bindings=%d", runtime.acceptCalls, runtime.queueCalls, jobs.calls, len(store.bindings))
	}
	binding := store.bindings[first.EffectID]
	if binding.AssetVersion != 1 || binding.SupersedesVersion != 0 || binding.EntrantReady || binding.Snapshot.AssigneeWeComUserIDs[0] != "staff-a" || binding.Snapshot.AssigneeWeComUserIDs[1] != "staff-b" {
		t.Fatalf("binding=%+v", binding)
	}
	spec := channelAcquisitionAssetEffectSpec(binding.Snapshot, binding.SnapshotDigest, binding.Kind, binding.AssetVersion, binding.SupersedesVersion)
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{
		Owner: eer.OwnerContact, Kind: eer.KindContactAcquisitionAssetPublish,
		SourceRefDigest: spec.SourceRefDigest, TargetRefDigest: spec.TargetRefDigest,
		PayloadDigest: spec.PayloadDigest, PolicyVersionHash: spec.PolicyVersionHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if binding.EnvelopeFingerprint != envelope.Fingerprint() || binding.EnvelopeFingerprint == spec.Fingerprint() {
		t.Fatalf("stored envelope fingerprint=%q actual=%q spec=%q", binding.EnvelopeFingerprint, envelope.Fingerprint(), spec.Fingerprint())
	}
	_, err = service.Publish(context.Background(), PublishChannelAcquisitionAssetCommand{ChannelID: 41, Actor: 7, IdempotencyKey: command.IdempotencyKey, Kind: contactport.AcquisitionAssetLink})
	if !errors.Is(err, ErrChannelAcquisitionAssetConflict) || jobs.calls != 1 {
		t.Fatalf("conflict err=%v jobs=%d", err, jobs.calls)
	}
}

func TestCH02OutcomeUnknownNeverAutomaticallyCallsProviderAgain(t *testing.T) {
	provider := &acquisitionAssetProvider{err: errors.New("ambiguous transport result")}
	service, store, runtime, _ := newAcquisitionAssetServiceFixture(t, provider)
	accepted, err := service.Publish(context.Background(), PublishChannelAcquisitionAssetCommand{ChannelID: 41, Actor: 7, IdempotencyKey: acquisitionAssetTestKey("unknown"), Kind: contactport.AcquisitionAssetQRCode})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "one"))
	if err != nil || first.State != eer.StateOutcomeUnknown || !first.ProviderCallAttempted || first.RealExternalCallExecuted || !first.ManualReconcileRequired || first.EntrantReady {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "two"))
	if err != nil || second.State != eer.StateOutcomeUnknown || provider.calls != 1 || runtime.runCalls != 1 || runtime.claimCalls != 1 {
		t.Fatalf("second=%+v err=%v provider=%d run=%d claim=%d", second, err, provider.calls, runtime.runCalls, runtime.claimCalls)
	}
	if store.bindings[accepted.EffectID].EntrantReady {
		t.Fatal("provider ambiguity must never make entrants ready")
	}
}

func TestCH02TokenGrantFailureDoesNotClaimBusinessDispatch(t *testing.T) {
	provider := &acquisitionAssetProvider{err: errors.New("token grant failed"), notDispatched: true}
	service, _, _, _ := newAcquisitionAssetServiceFixture(t, provider)
	accepted, err := service.Publish(context.Background(), PublishChannelAcquisitionAssetCommand{ChannelID: 41, Actor: 7, IdempotencyKey: acquisitionAssetTestKey("token-failure"), Kind: contactport.AcquisitionAssetQRCode})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "token-failure"))
	if err != nil || result.State != eer.StateOutcomeUnknown || result.ProviderCallAttempted || result.RealExternalCallExecuted || provider.calls != 1 {
		t.Fatalf("result=%+v calls=%d err=%v", result, provider.calls, err)
	}
}

func TestCH02InvalidProviderResponseBecomesUnknownAndNeverRetriesProvider(t *testing.T) {
	provider := &acquisitionAssetProvider{result: contactport.AcquisitionAssetProviderResult{
		Outcome: contactport.AcquisitionAssetProviderExecuted,
		// Missing receipt, asset reference, and real-call proof is deliberately invalid.
	}}
	service, _, runtime, _ := newAcquisitionAssetServiceFixture(t, provider)
	accepted, err := service.Publish(context.Background(), PublishChannelAcquisitionAssetCommand{ChannelID: 41, Actor: 7, IdempotencyKey: acquisitionAssetTestKey("invalid-provider"), Kind: contactport.AcquisitionAssetQRCode})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "invalid-provider"))
	if err != nil || first.State != eer.StateOutcomeUnknown || !first.ManualReconcileRequired || first.RealExternalCallExecuted {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "invalid-provider-replay"))
	if err != nil || second.State != eer.StateOutcomeUnknown || provider.calls != 1 || runtime.claimCalls != 1 || runtime.runCalls != 1 {
		t.Fatalf("second=%+v err=%v provider=%d claim=%d run=%d", second, err, provider.calls, runtime.claimCalls, runtime.runCalls)
	}
}

func TestCH02ExecutePersistsClosedProviderTerminalStates(t *testing.T) {
	for _, test := range []struct {
		name      string
		outcome   contactport.AcquisitionAssetProviderOutcome
		asset     [32]byte
		wantState eer.State
	}{
		{name: "executed", outcome: contactport.AcquisitionAssetProviderExecuted, asset: sha256.Sum256([]byte("safe-provider-asset-reference")), wantState: eer.StateExecuted},
		{name: "final_failed", outcome: contactport.AcquisitionAssetProviderFinalFailed, wantState: eer.StateFinalFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &acquisitionAssetProvider{result: contactport.AcquisitionAssetProviderResult{
				Outcome: test.outcome, ReceiptDigest: sha256.Sum256([]byte("provider-receipt-" + test.name)),
				AssetReferenceDigest: test.asset, BusinessEndpointDispatched: true, RealExternalCallExecuted: true,
			}}
			service, store, runtime, _ := newAcquisitionAssetServiceFixture(t, provider)
			accepted, err := service.Publish(context.Background(), PublishChannelAcquisitionAssetCommand{ChannelID: 41, Actor: 7, IdempotencyKey: acquisitionAssetTestKey("terminal-" + test.name), Kind: contactport.AcquisitionAssetQRCode})
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", test.name))
			if err != nil || result.State != test.wantState || !result.ProviderCallAttempted || !result.RealExternalCallExecuted || result.EntrantReady || result.ManualReconcileRequired {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if _, err = service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker-replay", test.name)); err != nil || provider.calls != 1 || runtime.runCalls != 1 {
				t.Fatalf("replay err=%v provider=%d run=%d", err, provider.calls, runtime.runCalls)
			}
			binding := store.bindings[accepted.EffectID]
			if binding.State != test.wantState || binding.ProviderAssetReferenceDigest != test.asset || binding.EntrantReady {
				t.Fatalf("binding=%+v", binding)
			}
		})
	}
}

func TestCH02AttemptedCrashFenceDoesNotCallProvider(t *testing.T) {
	provider := &acquisitionAssetProvider{}
	service, store, runtime, _ := newAcquisitionAssetServiceFixture(t, provider)
	accepted, err := service.Publish(context.Background(), PublishChannelAcquisitionAssetCommand{ChannelID: 41, Actor: 7, IdempotencyKey: acquisitionAssetTestKey("attempted-crash"), Kind: contactport.AcquisitionAssetQRCode})
	if err != nil {
		t.Fatal(err)
	}
	binding := store.bindings[accepted.EffectID]
	binding.State = channelAcquisitionAssetStateAttempted
	binding.Fence = 7
	binding.LeaseExpiresAt = time.Date(2026, 8, 26, 8, 1, 0, 0, time.UTC)
	store.bindings[accepted.EffectID] = binding
	result, err := service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "after-crash"))
	if err != nil || result.State != channelAcquisitionAssetStateAttempted || !result.ManualReconcileRequired || result.ProviderCallAttempted || result.EntrantReady || provider.calls != 0 || runtime.claimCalls != 0 || runtime.runCalls != 0 {
		t.Fatalf("result=%+v err=%v provider=%d claim=%d run=%d", result, err, provider.calls, runtime.claimCalls, runtime.runCalls)
	}
}

func TestCH02ExpiredAttemptedCrashRecoversUnknownWithoutProviderIO(t *testing.T) {
	provider := &acquisitionAssetProvider{}
	service, store, runtime, _ := newAcquisitionAssetServiceFixture(t, provider)
	accepted, err := service.Publish(context.Background(), PublishChannelAcquisitionAssetCommand{ChannelID: 41, Actor: 7, IdempotencyKey: acquisitionAssetTestKey("attempted-recovery"), Kind: contactport.AcquisitionAssetQRCode})
	if err != nil {
		t.Fatal(err)
	}
	binding := store.bindings[accepted.EffectID]
	binding.State = channelAcquisitionAssetStateAttempted
	binding.Fence = 7
	binding.LeaseExpiresAt = runtime.now.Add(-time.Minute)
	store.bindings[accepted.EffectID] = binding
	runtime.states[accepted.EffectID] = channelAcquisitionAssetStateAttempted

	result, err := service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "recovery"))
	if err != nil || result.State != eer.StateOutcomeUnknown || !result.ManualReconcileRequired || result.ProviderCallAttempted ||
		result.RealExternalCallExecuted || result.EntrantReady || provider.calls != 0 || runtime.recoverCalls != 1 || runtime.claimCalls != 0 || runtime.runCalls != 0 {
		t.Fatalf("result=%+v err=%v provider=%d recover=%d claim=%d run=%d", result, err, provider.calls, runtime.recoverCalls, runtime.claimCalls, runtime.runCalls)
	}
	recovered := store.bindings[accepted.EffectID]
	if recovered.State != eer.StateOutcomeUnknown || recovered.ProviderCallAttempted || recovered.RealExternalCallExecuted || recovered.AttemptReceiptID == "" {
		t.Fatalf("recovered binding=%+v", recovered)
	}
}

func TestCH02ProviderNotAppliedReconcileCreatesNewVersionWithoutRevivingOld(t *testing.T) {
	provider := &acquisitionAssetProvider{err: errors.New("ambiguous transport result")}
	service, store, runtime, jobs := newAcquisitionAssetServiceFixture(t, provider)
	accepted, err := service.Publish(context.Background(), PublishChannelAcquisitionAssetCommand{ChannelID: 41, Actor: 7, IdempotencyKey: acquisitionAssetTestKey("replace-publish"), Kind: contactport.AcquisitionAssetQRCode})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "replace")); err != nil {
		t.Fatal(err)
	}
	unknown := store.bindings[accepted.EffectID]
	command := ReconcileChannelAcquisitionAssetCommand{
		EffectID: accepted.EffectID, Actor: 9, IdempotencyKey: acquisitionAssetTestKey("reconcile"), Generation: unknown.Generation,
		Fence: unknown.Fence, LeaseExpiresAt: unknown.LeaseExpiresAt, EvidenceDigest: channelAcquisitionAssetDigest("provider-query", accepted.EffectID),
		Resolution: ChannelAcquisitionAssetProviderNotApplied,
	}
	first, err := service.Reconcile(context.Background(), command)
	if err != nil || first.State != eer.StateReconciled || first.Resolution != ChannelAcquisitionAssetProviderNotApplied || first.Replacement == nil || first.Replacement.AssetVersion != 2 || first.Replacement.State != eer.StateQueued || first.Replacement.EntrantReady || first.EntrantReady {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	old := store.bindings[accepted.EffectID]
	replacement := store.bindings[first.Replacement.EffectID]
	if old.State != eer.StateReconciled || old.AssetVersion != 1 || replacement.EffectID == old.EffectID || replacement.AssetVersion != 2 || replacement.SupersedesVersion != 1 || replacement.State != eer.StateQueued {
		t.Fatalf("old=%+v replacement=%+v", old, replacement)
	}
	replayed, err := service.Reconcile(context.Background(), command)
	if err != nil || replayed.Replacement == nil || replayed.Replacement.EffectID != first.Replacement.EffectID || runtime.reconcileCalls != 1 || runtime.acceptCalls != 2 || jobs.calls != 2 || provider.calls != 1 {
		t.Fatalf("replayed=%+v err=%v reconcile=%d accept=%d jobs=%d provider=%d", replayed, err, runtime.reconcileCalls, runtime.acceptCalls, jobs.calls, provider.calls)
	}
}

func TestCH02ProviderAppliedReconcileClosesWithoutReplacementOrEntrantReadiness(t *testing.T) {
	provider := &acquisitionAssetProvider{err: errors.New("ambiguous transport result")}
	service, store, runtime, jobs := newAcquisitionAssetServiceFixture(t, provider)
	accepted, err := service.Publish(context.Background(), PublishChannelAcquisitionAssetCommand{ChannelID: 41, Actor: 7, IdempotencyKey: acquisitionAssetTestKey("applied-publish"), Kind: contactport.AcquisitionAssetQRCode})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "applied")); err != nil {
		t.Fatal(err)
	}
	unknown := store.bindings[accepted.EffectID]
	result, err := service.Reconcile(context.Background(), ReconcileChannelAcquisitionAssetCommand{
		EffectID: accepted.EffectID, Actor: 9, IdempotencyKey: acquisitionAssetTestKey("applied-reconcile"), Generation: unknown.Generation,
		Fence: unknown.Fence, LeaseExpiresAt: unknown.LeaseExpiresAt, EvidenceDigest: channelAcquisitionAssetDigest("provider-query", "applied"),
		Resolution: ChannelAcquisitionAssetProviderApplied,
	})
	if err != nil || result.State != eer.StateReconciled || result.Replacement != nil || !result.ProviderSuccessClaimed || result.EntrantReady || len(store.bindings) != 1 || runtime.acceptCalls != 1 || jobs.calls != 1 {
		t.Fatalf("result=%+v err=%v bindings=%d accept=%d jobs=%d", result, err, len(store.bindings), runtime.acceptCalls, jobs.calls)
	}
}

func TestCH02APICommandServiceDoesNotRequireProviderAndDerivesReconcileLease(t *testing.T) {
	provider := &acquisitionAssetProvider{err: errors.New("ambiguous transport result")}
	service, store, runtime, jobs := newAcquisitionAssetServiceFixture(t, provider)
	accepted, err := service.Publish(context.Background(), PublishChannelAcquisitionAssetCommand{ChannelID: 41, Actor: 7, IdempotencyKey: acquisitionAssetTestKey("api-publish"), Kind: contactport.AcquisitionAssetQRCode})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "api")); err != nil {
		t.Fatal(err)
	}
	service.provider = nil
	result, err := service.ReconcileCurrent(context.Background(), ReconcileCurrentChannelAcquisitionAssetCommand{
		EffectID: accepted.EffectID, ChannelID: 41, Actor: 9, IdempotencyKey: acquisitionAssetTestKey("api-reconcile"),
		EvidenceDigest: channelAcquisitionAssetDigest("provider-query", "api"), Resolution: ChannelAcquisitionAssetProviderApplied,
	})
	if err != nil || result.State != eer.StateReconciled || runtime.reconcileCalls != 1 || jobs.calls != 1 || provider.calls != 1 {
		t.Fatalf("result=%+v err=%v reconcile=%d jobs=%d provider=%d", result, err, runtime.reconcileCalls, jobs.calls, provider.calls)
	}
	if _, err = service.ReconcileCurrent(context.Background(), ReconcileCurrentChannelAcquisitionAssetCommand{
		EffectID: accepted.EffectID, ChannelID: 99, Actor: 9, IdempotencyKey: acquisitionAssetTestKey("api-reconcile-masked"),
		EvidenceDigest: channelAcquisitionAssetDigest("provider-query", "masked"), Resolution: ChannelAcquisitionAssetProviderApplied,
	}); !errors.Is(err, ErrChannelAcquisitionAssetNotFound) {
		t.Fatalf("cross-channel err=%v binding=%+v", err, store.bindings[accepted.EffectID])
	}
	if _, err = service.Execute(context.Background(), accepted.EffectID, channelAcquisitionAssetDigest("worker", "closed")); !errors.Is(err, ErrInvalidChannelAcquisitionAsset) {
		t.Fatalf("execute without provider err=%v", err)
	}
}

func TestCH02StateMachineRejectsRetriesAndRevival(t *testing.T) {
	allowed := map[[2]eer.State]bool{
		{eer.StateAccepted, eer.StateQueued}:                             true,
		{eer.StateQueued, channelAcquisitionAssetStateAttempted}:         true,
		{channelAcquisitionAssetStateAttempted, eer.StateExecuted}:       true,
		{channelAcquisitionAssetStateAttempted, eer.StateFinalFailed}:    true,
		{channelAcquisitionAssetStateAttempted, eer.StateOutcomeUnknown}: true,
		{eer.StateOutcomeUnknown, eer.StateReconciled}:                   true,
	}
	states := []eer.State{eer.StateAccepted, eer.StateQueued, channelAcquisitionAssetStateAttempted, eer.StateExecuted, eer.StateFinalFailed, eer.StateOutcomeUnknown, eer.StateReconciled, eer.StateRetryableFailed}
	for _, from := range states {
		for _, to := range states {
			if got := channelAcquisitionAssetCanTransition(from, to); got != allowed[[2]eer.State{from, to}] {
				t.Fatalf("transition %s -> %s = %v", from, to, got)
			}
		}
	}
}
