package tag

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

type testUoW struct{ active bool }

func (uow *testUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	if uow.active {
		return callback(ctx)
	}
	uow.active = true
	defer func() { uow.active = false }()
	return callback(ctx)
}

type memoryStore struct {
	byID         map[string]Effect
	reserveCalls int
	complete     AttemptCompletion
}

func newMemoryStore() *memoryStore { return &memoryStore{byID: make(map[string]Effect)} }

func (store *memoryStore) Reserve(_ context.Context, candidate Effect) (Effect, bool, error) {
	store.reserveCalls++
	for _, existing := range store.byID {
		if existing.IdempotencyDigest == candidate.IdempotencyDigest {
			return cloneEffect(existing), false, nil
		}
	}
	store.byID[candidate.EffectID] = cloneEffect(candidate)
	return cloneEffect(candidate), true, nil
}

func (store *memoryStore) GetByIdempotency(_ context.Context, actor int64, idempotencyDigest eer.Digest) (Effect, error) {
	for _, existing := range store.byID {
		if existing.Actor == actor && existing.IdempotencyDigest == idempotencyDigest {
			return cloneEffect(existing), nil
		}
	}
	return Effect{}, ErrEffectUnavailable
}

func (store *memoryStore) MarkQueued(_ context.Context, effectID string, link eer.RiverJobLink, receiptID string, at time.Time) (Effect, error) {
	record, ok := store.byID[effectID]
	if !ok || record.State != eer.StateAccepted {
		return Effect{}, ErrEffectUnavailable
	}
	record.State, record.RiverJobID, record.QueueReceiptID, record.Generation, record.UpdatedAt = eer.StateQueued, link.JobID, receiptID, link.Generation, at
	store.byID[effectID] = record
	return cloneEffect(record), nil
}

func (store *memoryStore) Get(_ context.Context, effectID string) (Effect, error) {
	record, ok := store.byID[effectID]
	if !ok {
		return Effect{}, ErrEffectUnavailable
	}
	return cloneEffect(record), nil
}

func (store *memoryStore) RecordClaim(_ context.Context, effectID string, lease eer.Lease, at time.Time) (Effect, error) {
	record, ok := store.byID[effectID]
	if !ok || record.State != eer.StateQueued {
		return Effect{}, ErrEffectUnavailable
	}
	record.Generation, record.Fence, record.LeaseExpiresAt, record.UpdatedAt = lease.Generation, lease.Fence, lease.ExpiresAt, at
	store.byID[effectID] = record
	return cloneEffect(record), nil
}

func (store *memoryStore) CompleteAttempt(_ context.Context, completion AttemptCompletion) (Effect, error) {
	record, ok := store.byID[completion.EffectID]
	if !ok || record.Generation != completion.Lease.Generation || record.Fence != completion.Lease.Fence {
		return Effect{}, ErrEffectUnavailable
	}
	store.complete = completion
	record.State, record.AttemptReceiptID = completion.State, completion.ReceiptID
	record.AttemptReceiptDigest, record.AttemptCompletedAt, record.UpdatedAt = completion.Receipt, completion.CompletedAt, completion.CompletedAt
	store.byID[completion.EffectID] = record
	return cloneEffect(record), nil
}

func (store *memoryStore) CompleteReconcile(_ context.Context, completion ReconcileCompletion) (Effect, error) {
	record, ok := store.byID[completion.EffectID]
	if !ok || record.Generation != completion.Lease.Generation || record.Fence != completion.Lease.Fence {
		return Effect{}, ErrReconcileRequired
	}
	record.State, record.ReconcileReceiptID = eer.StateReconciled, completion.ReceiptID
	record.ReconcileReceiptDigest, record.ReconcileResolution = completion.Receipt, completion.Resolution
	record.ReconcileEvidenceHash, record.ReconciledAt, record.UpdatedAt = completion.EvidenceDigest, completion.CompletedAt, completion.CompletedAt
	store.byID[completion.EffectID] = record
	return cloneEffect(record), nil
}

func cloneEffect(value Effect) Effect {
	value.ProviderTagIDs = append([]string(nil), value.ProviderTagIDs...)
	return value
}

type memoryJobs struct {
	calls int
	args  JobArgs
}

func (jobs *memoryJobs) Insert(_ context.Context, args JobArgs, generation int64, at time.Time) (eer.RiverJobLink, error) {
	jobs.calls++
	jobs.args = args
	return eer.RiverJobLink{JobID: int64(jobs.calls), Generation: generation, Queue: "sync", ArgsDigest: digest("test-job", args.EffectID), ScheduledAt: at}, nil
}

type memoryEER struct {
	envelope    eer.EffectEnvelope
	fingerprint eer.Digest
	receiptKey  eer.Digest
	projection  eer.Projection
	lease       eer.Lease
	claimCalls  int
	runCalls    int
	now         time.Time
}

func newMemoryEER(now time.Time) *memoryEER { return &memoryEER{now: now} }

func (runtime *memoryEER) Accept(_ context.Context, command eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
	fingerprint := command.Envelope.Fingerprint()
	if runtime.projection.ID != "" {
		if runtime.receiptKey != command.ReceiptKeyDigest || runtime.fingerprint != fingerprint {
			return eer.Projection{}, eer.OperationReceipt{}, eer.ErrPayloadMismatch
		}
		return runtime.projection, operationReceipt("accept", runtime.projection, command.CommandDigest(), runtime.now), nil
	}
	runtime.envelope, runtime.fingerprint, runtime.receiptKey = command.Envelope, fingerprint, command.ReceiptKeyDigest
	runtime.projection = eer.Projection{ID: "eer_41", Owner: ownerWeCom, Kind: kindTagSync, State: eer.StateAccepted, Generation: 1, UpdatedAt: runtime.now}
	return runtime.projection, operationReceipt("accept", runtime.projection, command.CommandDigest(), runtime.now), nil
}

func (runtime *memoryEER) Queue(_ context.Context, command eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error) {
	if command.EffectID != runtime.projection.ID || runtime.projection.State != eer.StateAccepted {
		return eer.Projection{}, eer.OperationReceipt{}, ErrEffectUnavailable
	}
	runtime.projection.State, runtime.projection.Generation, runtime.projection.UpdatedAt = eer.StateQueued, command.Job.Generation, runtime.now.Add(time.Second)
	return runtime.projection, operationReceipt("queue", runtime.projection, command.CommandDigest(), runtime.projection.UpdatedAt), nil
}

func (runtime *memoryEER) Claim(_ context.Context, command eer.ClaimCommand) (eer.Lease, eer.Projection, error) {
	runtime.claimCalls++
	if command.EffectID != runtime.projection.ID || runtime.projection.State != eer.StateQueued {
		return eer.Lease{}, eer.Projection{}, ErrEffectUnavailable
	}
	runtime.lease = eer.Lease{EffectID: command.EffectID, Generation: runtime.projection.Generation, Fence: int64(runtime.claimCalls), ExpiresAt: runtime.now.Add(time.Minute)}
	return runtime.lease, runtime.projection, nil
}

func (runtime *memoryEER) RunAttempt(ctx context.Context, lease eer.Lease, adapter eer.Adapter) (eer.Projection, eer.OperationReceipt, error) {
	runtime.runCalls++
	result, adapterErr := adapter.Execute(ctx, runtime.envelope, eer.Attempt{Number: int32(runtime.runCalls), Generation: lease.Generation, Fence: lease.Fence, StartedAt: runtime.now})
	if adapterErr != nil {
		result = eer.AdapterResult{Completion: "outcome_unknown", ReceiptDigest: digest("test-unknown", lease.EffectID)}
	}
	runtime.projection.State, runtime.projection.UpdatedAt = eer.State(result.Completion), runtime.now.Add(2*time.Second)
	receipt := operationReceipt("attempt", runtime.projection, result.ReceiptDigest, runtime.projection.UpdatedAt)
	if adapterErr != nil {
		return runtime.projection, receipt, eer.ErrAdapterFailure
	}
	return runtime.projection, receipt, nil
}

func (runtime *memoryEER) Reconcile(_ context.Context, command eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	if runtime.projection.State != eer.StateOutcomeUnknown && runtime.projection.State != eer.StateReconciled {
		return eer.Projection{}, eer.OperationReceipt{}, eer.ErrReconcileRequired
	}
	if command.Lease != runtime.lease {
		return eer.Projection{}, eer.OperationReceipt{}, eer.ErrLeaseFence
	}
	runtime.projection.State, runtime.projection.UpdatedAt = eer.StateReconciled, runtime.now.Add(3*time.Second)
	return runtime.projection, operationReceipt("reconcile", runtime.projection, command.CommandDigest(), runtime.projection.UpdatedAt), nil
}

func operationReceipt(id string, projection eer.Projection, command eer.Digest, at time.Time) eer.OperationReceipt {
	return eer.OperationReceipt{ID: "eerop_" + id, EffectID: projection.ID, CommandDigest: command, State: projection.State, CompletedAt: at}
}

type providerStub struct {
	calls  int
	result ProviderResult
	err    error
	seen   ProviderCommand
}

func (provider *providerStub) Execute(_ context.Context, command ProviderCommand, _ eer.Attempt) (ProviderResult, error) {
	provider.calls++
	provider.seen = command
	return provider.result, provider.err
}

func TestQueueIsTypedAtomicAndIdempotent(t *testing.T) {
	now := time.Date(2026, time.August, 25, 4, 0, 0, 0, time.UTC)
	uow, store, runtime, jobs := &testUoW{}, newMemoryStore(), newMemoryEER(now), &memoryJobs{}
	service, err := NewService(uow, store, runtime, jobs, "corp-a")
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	command := QueueCommand{LegacyReceiptID: 38, Actor: 7, IdempotencyKey: "wecom-tag-idem-0001", Operation: OperationMark, ExternalUserID: "external-1", ProviderTagIDs: []string{"tag-b", "tag-a"}}
	first, err := service.Queue(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Queue(context.Background(), command)
	if err != nil || first != second || jobs.calls != 1 || jobs.args != (JobArgs{EffectID: "eer_41"}) {
		t.Fatalf("queue replay = %#v / %#v, jobs=%d/%#v, err=%v", first, second, jobs.calls, jobs.args, err)
	}
	stored, _ := store.Get(context.Background(), first.EffectID)
	if !reflect.DeepEqual(stored.ProviderTagIDs, []string{"tag-a", "tag-b"}) || stored.State != eer.StateQueued || first.RealExternalCallExecuted {
		t.Fatalf("typed stored effect/receipt = %#v / %#v", stored, first)
	}
	conflict := command
	conflict.Operation = OperationUnmark
	if _, err = service.Queue(context.Background(), conflict); !errors.Is(err, ErrEffectConflict) {
		t.Fatalf("same-key changed command error = %v", err)
	}
}

func TestTransactionEntryPointsRejectMissingPlatformTransactionBeforeWrites(t *testing.T) {
	now := time.Date(2026, time.August, 25, 4, 30, 0, 0, time.UTC)
	store, runtime, jobs := newMemoryStore(), newMemoryEER(now), &memoryJobs{}
	service, err := NewService(&testUoW{}, store, runtime, jobs, "corp-a")
	if err != nil {
		t.Fatal(err)
	}
	command := QueueCommand{LegacyReceiptID: 38, Actor: 7, IdempotencyKey: "wecom-tag-idem-txless", Operation: OperationMark, ExternalUserID: "external-1", ProviderTagIDs: []string{"tag-1"}}
	if _, err = service.QueueInTransaction(context.Background(), command); !errors.Is(err, ErrEffectUnavailable) {
		t.Fatalf("QueueInTransaction() error = %v, want unavailable", err)
	}
	if _, err = service.ReplayInTransaction(context.Background(), command); !errors.Is(err, ErrEffectUnavailable) {
		t.Fatalf("ReplayInTransaction() error = %v, want unavailable", err)
	}
	if runtime.projection.ID != "" || store.reserveCalls != 0 || jobs.calls != 0 {
		t.Fatalf("txless writes: projection=%q reserve=%d jobs=%d", runtime.projection.ID, store.reserveCalls, jobs.calls)
	}
}

func TestOutcomeUnknownReplayNeverCallsProviderAgainAndManualReconcileIsTyped(t *testing.T) {
	service, store, runtime := queuedTestService(t, OperationMark)
	provider := &providerStub{err: errors.New("transport interrupted")}
	first, err := service.Execute(context.Background(), "eer_41", digest("worker-test", "1"), provider)
	if err != nil || first.State != eer.StateOutcomeUnknown || !first.ManualReconcileRequired || !first.ProviderCallAttempted || provider.calls != 1 {
		t.Fatalf("first unknown execution = %#v calls=%d err=%v", first, provider.calls, err)
	}
	second, err := service.Execute(context.Background(), "eer_41", digest("worker-test", "1"), provider)
	if err != nil || second.State != eer.StateOutcomeUnknown || second.ProviderCallAttempted || provider.calls != 1 || runtime.claimCalls != 1 {
		t.Fatalf("unknown replay = %#v provider=%d claims=%d err=%v", second, provider.calls, runtime.claimCalls, err)
	}
	record, _ := store.Get(context.Background(), "eer_41")
	evidence := digest("manual-evidence", "provider-query-1")
	reconciled, err := service.Reconcile(context.Background(), ReconcileCommand{
		EffectID: "eer_41", Actor: 9, IdempotencyKey: "wecom-reconcile-0001", Generation: record.Generation,
		Fence: record.Fence, LeaseExpiresAt: record.LeaseExpiresAt, EvidenceDigest: evidence, Resolution: ResolutionProviderNotApplied,
	})
	if err != nil || reconciled.State != eer.StateReconciled || reconciled.Resolution != ResolutionProviderNotApplied || reconciled.ProviderCallAttempted || reconciled.ProviderSuccessClaimed {
		t.Fatalf("reconcile = %#v, %v", reconciled, err)
	}
}

func TestCatalogSyncPersistsOnlyObservedTypedSnapshot(t *testing.T) {
	service, store, _ := queuedTestService(t, OperationCatalogSync)
	provider := &providerStub{result: ProviderResult{Completion: "executed", ReceiptDigest: digest("provider-receipt", "catalog"), Catalog: CatalogSnapshot{
		Observed: true, Groups: []CatalogGroup{{ProviderGroupID: "group-1", Name: "Lifecycle"}},
		Tags: []CatalogTag{{ProviderTagID: "tag-1", ProviderGroupID: "group-1", Name: "Warm"}},
	}}}
	got, err := service.Execute(context.Background(), "eer_41", digest("worker-test", "catalog"), provider)
	if err != nil || got.State != eer.StateExecuted || provider.calls != 1 || !store.complete.Catalog.Observed || len(store.complete.Catalog.Tags) != 1 {
		t.Fatalf("catalog execution = %#v provider=%d completion=%#v err=%v", got, provider.calls, store.complete, err)
	}
}

func TestDisabledProviderIsTypedFinalFailureWithoutExternalSuccessClaim(t *testing.T) {
	service, _, _ := queuedTestService(t, OperationUnmark)
	got, err := service.Execute(context.Background(), "eer_41", digest("worker-test", "disabled"), DisabledProvider{})
	if err != nil || got.State != eer.StateFinalFailed || !got.ProviderCallAttempted || got.RealExternalCallExecuted || got.ManualReconcileRequired {
		t.Fatalf("disabled execution = %#v, %v", got, err)
	}
}

func queuedTestService(t *testing.T, operation Operation) (*Service, *memoryStore, *memoryEER) {
	t.Helper()
	now := time.Date(2026, time.August, 25, 5, 0, 0, 0, time.UTC)
	store, runtime := newMemoryStore(), newMemoryEER(now)
	service, err := NewService(&testUoW{}, store, runtime, &memoryJobs{}, "corp-a")
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	command := QueueCommand{LegacyReceiptID: 38, Actor: 7, IdempotencyKey: "wecom-tag-idem-0002", Operation: operation}
	if operation == OperationCatalogSync {
		command.SyncTrigger = SyncTriggerManual
	} else {
		command.ExternalUserID, command.ProviderTagIDs = "external-1", []string{"tag-1"}
	}
	if _, err = service.Queue(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	return service, store, runtime
}
