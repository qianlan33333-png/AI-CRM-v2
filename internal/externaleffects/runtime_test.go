package externaleffects

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEnvelopeClosedOwnersAndDigestOnlyPayload(t *testing.T) {
	t.Parallel()
	for _, family := range []struct {
		owner Owner
		kind  Kind
	}{
		{OwnerWeCom, KindWeComTagSync}, {OwnerWeCom, KindWeComProfileSync},
		{OwnerOutbound, KindOutboundMessage}, {OwnerOutbound, KindOutboundMedia},
		{OwnerCampaign, KindCampaignDispatch}, {OwnerCampaign, KindCampaignGroupAnnouncement},
		{OwnerSurvey, KindSurveyWebhook}, {OwnerAudience, KindAudienceWebhook},
		{OwnerOrder, KindOrderPaymentPrepay}, {OwnerOrder, KindOrderPaymentCapture}, {OwnerOrder, KindOrderRefund},
		{OwnerGroupOps, KindGroupOpsBroadcast}, {OwnerProduct, KindProductExternalPushTest},
	} {
		value, err := NewEnvelope(EnvelopeInput{Owner: family.owner, Kind: family.kind, SourceRefDigest: digest("source"), TargetRefDigest: digest("target"), PayloadDigest: digest("payload"), PolicyVersionHash: digest("policy")})
		if err != nil || value.Fingerprint() == "" {
			t.Fatalf("closed family %s/%s = %v", family.owner, family.kind, err)
		}
	}
	for _, test := range []struct {
		name string
		in   EnvelopeInput
	}{
		{name: "unknown owner", in: EnvelopeInput{Owner: "provider", Kind: KindOutboundMessage, SourceRefDigest: digest("a"), TargetRefDigest: digest("b"), PayloadDigest: digest("c"), PolicyVersionHash: digest("d")}},
		{name: "mismatched kind", in: EnvelopeInput{Owner: OwnerCampaign, Kind: KindOutboundMessage, SourceRefDigest: digest("a"), TargetRefDigest: digest("b"), PayloadDigest: digest("c"), PolicyVersionHash: digest("d")}},
		{name: "raw payload", in: EnvelopeInput{Owner: OwnerCampaign, Kind: KindCampaignDispatch, SourceRefDigest: digest("a"), TargetRefDigest: digest("b"), PayloadDigest: "mobile=13800000000", PolicyVersionHash: digest("d")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewEnvelope(test.in); !errors.Is(err, ErrInvalidCommand) {
				t.Fatalf("NewEnvelope() error = %v", err)
			}
		})
	}
}

func TestCanTransitionIsClosed(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		from State
		to   State
		want bool
	}{
		{name: "accept queues", from: StateAccepted, to: StateQueued, want: true},
		{name: "accept cancels", from: StateAccepted, to: StateCancelled, want: true},
		{name: "queue attempts", from: StateQueued, to: StateAttempted, want: true},
		{name: "queue cancels", from: StateQueued, to: StateCancelled, want: true},
		{name: "attempt outcome unknown", from: StateAttempted, to: StateOutcomeUnknown, want: true},
		{name: "attempt terminal", from: StateAttempted, to: StateExecuted, want: true},
		{name: "retryable failure requeues", from: StateRetryableFailed, to: StateQueued, want: true},
		{name: "unknown reconciles", from: StateOutcomeUnknown, to: StateReconciled, want: true},
		{name: "unknown cannot retry", from: StateOutcomeUnknown, to: StateQueued},
		{name: "terminal cannot retry", from: StateFinalFailed, to: StateQueued},
		{name: "attempt cannot cancel", from: StateAttempted, to: StateCancelled},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := CanTransition(test.from, test.to); got != test.want {
				t.Fatalf("CanTransition(%q, %q) = %v, want %v", test.from, test.to, got, test.want)
			}
		})
	}
}

func TestCommandDigestsCoverEveryControlInput(t *testing.T) {
	t.Parallel()
	baseJob := RiverJobLink{JobID: 7, Generation: 3, Queue: "external_effects", ArgsDigest: digest("args"), ScheduledAt: time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)}
	queue := QueueCommand{EffectID: "eer_1", Job: baseJob, ReceiptKeyDigest: digest("queue key")}
	retry := RetryCommand{EffectID: "eer_1", Job: baseJob, ReceiptKeyDigest: digest("retry key")}
	for _, mutate := range []func(*RiverJobLink){
		func(job *RiverJobLink) { job.JobID++ },
		func(job *RiverJobLink) { job.Generation++ },
		func(job *RiverJobLink) { job.Queue = "other_queue" },
		func(job *RiverJobLink) { job.ArgsDigest = digest("other args") },
		func(job *RiverJobLink) { job.ScheduledAt = job.ScheduledAt.Add(time.Second) },
	} {
		changedQueue, changedRetry := queue, retry
		mutate(&changedQueue.Job)
		changedRetry.Job = changedQueue.Job
		if queue.CommandDigest() == changedQueue.CommandDigest() || retry.CommandDigest() == changedRetry.CommandDigest() {
			t.Fatal("River job change was omitted from command digest")
		}
	}
	if queue.CommandDigest() == queue.ReceiptKeyDigest || retry.CommandDigest() == retry.ReceiptKeyDigest {
		t.Fatal("receipt key digest was used as complete command digest")
	}
	lease := Lease{EffectID: "eer_1", Generation: 3, Fence: 4, ExpiresAt: baseJob.ScheduledAt}
	cancel := CancelCommand{EffectID: "eer_1", ReceiptKeyDigest: digest("cancel key")}
	reconcile := ReconcileCommand{Lease: lease, ReceiptKeyDigest: digest("reconcile key"), EvidenceDigest: digest("evidence")}
	changedCancel := cancel
	changedCancel.EffectID = "eer_2"
	changedReconcile := reconcile
	changedReconcile.EvidenceDigest = digest("other evidence")
	if cancel.CommandDigest() == changedCancel.CommandDigest() || reconcile.CommandDigest() == changedReconcile.CommandDigest() {
		t.Fatal("control input was omitted from command digest")
	}
}

func TestAcceptExactReplayHasNoSecondEffectAndMismatchFailsClosed(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(t)
	service := mustService(t, store)
	command := AcceptCommand{ReceiptKeyDigest: digest("receipt"), Envelope: envelope(t)}

	first, firstReceipt, err := service.Accept(context.Background(), command)
	if err != nil || first.State != StateAccepted {
		t.Fatalf("first Accept() = %+v %+v %v", first, firstReceipt, err)
	}
	second, secondReceipt, err := service.Accept(context.Background(), command)
	if err != nil || second.ID != first.ID || secondReceipt.ID != firstReceipt.ID || store.effectCount() != 1 {
		t.Fatalf("replay = %+v %+v %v effects=%d", second, secondReceipt, err, store.effectCount())
	}
	mismatch := command
	mismatch.Envelope, err = NewEnvelope(EnvelopeInput{Owner: OwnerCampaign, Kind: KindCampaignDispatch, SourceRefDigest: digest("other"), TargetRefDigest: digest("target"), PayloadDigest: digest("payload"), PolicyVersionHash: digest("policy")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Accept(context.Background(), mismatch); !errors.Is(err, ErrPayloadMismatch) || store.effectCount() != 1 {
		t.Fatalf("mismatch error=%v effects=%d", err, store.effectCount())
	}
}

func TestQueueAndRetryReceiptsReplayOnlyExactCommand(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(t)
	service := mustService(t, store)
	accepted, _, err := service.Accept(context.Background(), AcceptCommand{ReceiptKeyDigest: digest("queue accept"), Envelope: envelope(t)})
	if err != nil {
		t.Fatal(err)
	}
	queue := QueueCommand{EffectID: accepted.ID, Job: jobLink(), ReceiptKeyDigest: digest("queue receipt")}
	queued, firstReceipt, err := service.Queue(context.Background(), queue)
	if err != nil || firstReceipt.CommandDigest == queue.ReceiptKeyDigest {
		t.Fatalf("queue = %+v %+v %v", queued, firstReceipt, err)
	}
	_, secondReceipt, err := service.Queue(context.Background(), queue)
	if err != nil || secondReceipt.ID != firstReceipt.ID {
		t.Fatalf("queue replay = %+v %v", secondReceipt, err)
	}
	mismatchedQueue := queue
	mismatchedQueue.Job.JobID++
	if _, _, err := service.Queue(context.Background(), mismatchedQueue); !errors.Is(err, ErrPayloadMismatch) {
		t.Fatalf("queue mismatch = %v", err)
	}

	retryable := acceptQueued(t, service, digest("retry receipt accept"))
	lease, _, err := service.Claim(context.Background(), ClaimCommand{EffectID: retryable.ID, WorkerDigest: digest("retry receipt worker")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.RunAttempt(context.Background(), lease, adapterFunc(func(context.Context, EffectEnvelope, Attempt) (AdapterResult, error) {
		return AdapterResult{Completion: CompletionRetryableFailed, ReceiptDigest: digest("retry receipt failure")}, nil
	})); err != nil {
		t.Fatal(err)
	}
	retry := RetryCommand{EffectID: retryable.ID, Job: retryJobLink(), ReceiptKeyDigest: digest("retry receipt")}
	_, firstRetryReceipt, err := service.Retry(context.Background(), retry)
	if err != nil || firstRetryReceipt.CommandDigest == retry.ReceiptKeyDigest {
		t.Fatalf("retry = %+v %v", firstRetryReceipt, err)
	}
	_, secondRetryReceipt, err := service.Retry(context.Background(), retry)
	if err != nil || secondRetryReceipt.ID != firstRetryReceipt.ID {
		t.Fatalf("retry replay = %+v %v", secondRetryReceipt, err)
	}
	mismatchedRetry := retry
	mismatchedRetry.Job.ArgsDigest = digest("other retry args")
	if _, _, err := service.Retry(context.Background(), mismatchedRetry); !errors.Is(err, ErrPayloadMismatch) {
		t.Fatalf("retry mismatch = %v", err)
	}
}

func TestStateGuardsRetryUnknownCancelAndReconcile(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(t)
	service := mustService(t, store)
	projection, _, err := service.Accept(context.Background(), AcceptCommand{ReceiptKeyDigest: digest("accept"), Envelope: envelope(t)})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Retry(context.Background(), RetryCommand{EffectID: projection.ID, Job: retryJobLink(), ReceiptKeyDigest: digest("retry")}); !errors.Is(err, ErrRetryForbidden) {
		t.Fatalf("retry accepted = %v", err)
	}
	if _, _, err := service.Queue(context.Background(), QueueCommand{EffectID: projection.ID, Job: jobLink(), ReceiptKeyDigest: digest("queue")}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Cancel(context.Background(), CancelCommand{EffectID: projection.ID, ReceiptKeyDigest: digest("cancel")}); err != nil {
		t.Fatalf("pre-attempt cancel = %v", err)
	}
	if _, _, err := service.Retry(context.Background(), RetryCommand{EffectID: projection.ID, Job: retryJobLink(), ReceiptKeyDigest: digest("retry")}); !errors.Is(err, ErrRetryForbidden) {
		t.Fatalf("retry cancelled = %v", err)
	}

	retryable := acceptQueued(t, service, digest("retryable"))
	retryLease, _, err := service.Claim(context.Background(), ClaimCommand{EffectID: retryable.ID, WorkerDigest: digest("retry worker")})
	if err != nil {
		t.Fatal(err)
	}
	failed, _, err := service.RunAttempt(context.Background(), retryLease, adapterFunc(func(context.Context, EffectEnvelope, Attempt) (AdapterResult, error) {
		return AdapterResult{Completion: CompletionRetryableFailed, ReceiptDigest: digest("retryable failure")}, nil
	}))
	if err != nil || failed.State != StateRetryableFailed {
		t.Fatalf("retryable failure = %+v %v", failed, err)
	}
	retried, _, err := service.Retry(context.Background(), RetryCommand{EffectID: retryable.ID, Job: retryJobLink(), ReceiptKeyDigest: digest("retry now")})
	if err != nil || retried.State != StateQueued || retried.Generation != failed.Generation+1 {
		t.Fatalf("retry = %+v %v", retried, err)
	}

	unknown := acceptQueued(t, service, digest("unknown"))
	lease, _, err := service.Claim(context.Background(), ClaimCommand{EffectID: unknown.ID, WorkerDigest: digest("worker")})
	if err != nil {
		t.Fatal(err)
	}
	result, _, err := service.RunAttempt(context.Background(), lease, adapterFunc(func(context.Context, EffectEnvelope, Attempt) (AdapterResult, error) {
		return AdapterResult{}, errors.New("transport lost")
	}))
	if !errors.Is(err, ErrAdapterFailure) || result.State != StateOutcomeUnknown {
		t.Fatalf("unknown result=%+v err=%v", result, err)
	}
	if _, _, err := service.Retry(context.Background(), RetryCommand{EffectID: unknown.ID, Job: retryJobLink(), ReceiptKeyDigest: digest("retry unknown")}); !errors.Is(err, ErrRetryForbidden) {
		t.Fatalf("retry unknown = %v", err)
	}
	if _, _, err := service.Cancel(context.Background(), CancelCommand{EffectID: unknown.ID, ReceiptKeyDigest: digest("cancel unknown")}); !errors.Is(err, ErrCancelForbidden) {
		t.Fatalf("cancel unknown = %v", err)
	}
	store.expireLease(unknown.ID)
	reconcile := ReconcileCommand{Lease: store.leaseFor(unknown.ID), ReceiptKeyDigest: digest("reconcile"), EvidenceDigest: digest("evidence")}
	wrongFence := reconcile
	wrongFence.Lease.Fence++
	wrongFence.ReceiptKeyDigest = digest("wrong fence receipt")
	if _, _, err := service.Reconcile(context.Background(), wrongFence); !errors.Is(err, ErrReconcileRequired) {
		t.Fatalf("reconcile wrong fence = %v", err)
	}
	reconciled, firstReconcileReceipt, err := service.Reconcile(context.Background(), reconcile)
	if err != nil || reconciled.State != StateReconciled {
		t.Fatalf("reconcile = %+v %v", reconciled, err)
	}
	_, secondReconcileReceipt, err := service.Reconcile(context.Background(), reconcile)
	if err != nil || secondReconcileReceipt.ID != firstReconcileReceipt.ID {
		t.Fatalf("expired reconcile replay = %+v %v", secondReconcileReceipt, err)
	}
	mismatchedEvidence := reconcile
	mismatchedEvidence.EvidenceDigest = digest("other evidence")
	if _, _, err := service.Reconcile(context.Background(), mismatchedEvidence); !errors.Is(err, ErrPayloadMismatch) {
		t.Fatalf("reconcile evidence mismatch = %v", err)
	}
}

func TestRecoverAttemptedToUnknownAfterTerminalWriteFailureDoesNotCallAdapterAgain(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(t)
	service := mustService(t, store)
	queued := acceptQueued(t, service, digest("recover accept"))
	lease, _, err := service.Claim(context.Background(), ClaimCommand{EffectID: queued.ID, WorkerDigest: digest("recover worker")})
	if err != nil {
		t.Fatal(err)
	}
	store.completeErr = errors.New("terminal write unavailable")
	calls := 0
	adapter := adapterFunc(func(context.Context, EffectEnvelope, Attempt) (AdapterResult, error) {
		calls++
		return AdapterResult{Completion: CompletionExecuted, ReceiptDigest: digest("provider completed")}, nil
	})
	if _, _, err := service.RunAttempt(context.Background(), lease, adapter); err == nil || calls != 1 {
		t.Fatalf("terminal write failure = %v calls=%d", err, calls)
	}
	if _, _, err := service.RecoverAttemptedToUnknown(context.Background(), RecoverAttemptedCommand{Lease: lease}); !errors.Is(err, ErrRecoveryForbidden) {
		t.Fatalf("unexpired attempted recovery = %v", err)
	}
	store.expireLease(queued.ID)
	expiredLease := store.leaseFor(queued.ID)
	recovered, receipt, err := service.RecoverAttemptedToUnknown(context.Background(), RecoverAttemptedCommand{Lease: expiredLease})
	if err != nil || recovered.State != StateOutcomeUnknown || receipt.CommandDigest == "" || calls != 1 {
		t.Fatalf("recover = %+v %+v %v calls=%d", recovered, receipt, err, calls)
	}
	if _, _, err := service.RunAttempt(context.Background(), lease, adapter); !errors.Is(err, ErrLeaseFence) || calls != 1 {
		t.Fatalf("recovered effect ran adapter again: err=%v calls=%d", err, calls)
	}
}

func TestRunAttemptPersistsFenceBeforeAdapterAndCompletesAfter(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(t)
	service := mustService(t, store)
	projection := acceptQueued(t, service, digest("attempt"))
	lease, _, err := service.Claim(context.Background(), ClaimCommand{EffectID: projection.ID, WorkerDigest: digest("worker")})
	if err != nil {
		t.Fatal(err)
	}
	adapter := adapterFunc(func(_ context.Context, _ EffectEnvelope, attempt Attempt) (AdapterResult, error) {
		if !store.isAttempted(projection.ID, attempt) {
			t.Fatal("adapter ran before attempted/fence was persisted")
		}
		return AdapterResult{Completion: CompletionExecuted, ReceiptDigest: digest("provider receipt")}, nil
	})
	completed, receipt, err := service.RunAttempt(context.Background(), lease, adapter)
	if err != nil || completed.State != StateExecuted || receipt.State != StateExecuted || store.callOrder(projection.ID) != "persist,complete" {
		t.Fatalf("RunAttempt() = %+v %+v %v order=%s", completed, receipt, err, store.callOrder(projection.ID))
	}
}

func TestRunAttemptRejectsStaleFenceExpiredLeaseAndConcurrentClaim(t *testing.T) {
	t.Parallel()
	store := newMemoryStore(t)
	service := mustService(t, store)
	projection := acceptQueued(t, service, digest("concurrent"))
	lease, _, err := service.Claim(context.Background(), ClaimCommand{EffectID: projection.ID, WorkerDigest: digest("worker")})
	if err != nil {
		t.Fatal(err)
	}
	stale := lease
	stale.Fence++
	if _, _, err := service.RunAttempt(context.Background(), stale, adapterFunc(successAdapter)); !errors.Is(err, ErrLeaseFence) {
		t.Fatalf("stale fence = %v", err)
	}
	expired := lease
	expired.ExpiresAt = time.Now().Add(-time.Second)
	if _, _, err := service.RunAttempt(context.Background(), expired, adapterFunc(successAdapter)); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("expired input = %v", err)
	}

	entered := make(chan struct{})
	start := make(chan struct{})
	var enteredOnce sync.Once
	adapter := adapterFunc(func(context.Context, EffectEnvelope, Attempt) (AdapterResult, error) {
		enteredOnce.Do(func() { close(entered) })
		<-start
		return AdapterResult{Completion: CompletionExecuted, ReceiptDigest: digest("concurrent receipt")}, nil
	})
	type response struct{ err error }
	responses := make(chan response, 2)
	go func() {
		_, _, err := service.RunAttempt(context.Background(), lease, adapter)
		responses <- response{err}
	}()
	<-entered
	go func() {
		_, _, err := service.RunAttempt(context.Background(), lease, adapter)
		responses <- response{err}
	}()
	close(start)
	first, second := (<-responses).err, (<-responses).err
	if !((first == nil && errors.Is(second, ErrLeaseFence)) || (second == nil && errors.Is(first, ErrLeaseFence))) {
		t.Fatalf("concurrent errors = %v / %v", first, second)
	}
}

type adapterFunc func(context.Context, EffectEnvelope, Attempt) (AdapterResult, error)

func (fn adapterFunc) Execute(ctx context.Context, envelope EffectEnvelope, attempt Attempt) (AdapterResult, error) {
	return fn(ctx, envelope, attempt)
}

func successAdapter(context.Context, EffectEnvelope, Attempt) (AdapterResult, error) {
	return AdapterResult{Completion: CompletionExecuted, ReceiptDigest: digest("success")}, nil
}

type memoryEffect struct {
	projection Projection
	envelope   EffectEnvelope
	receipt    OperationReceipt
	lease      Lease
	attempt    Attempt
	order      []string
}

type memoryStore struct {
	t           *testing.T
	mu          sync.Mutex
	effects     map[string]*memoryEffect
	keys        map[Digest]Digest
	receipts    map[Digest]OperationReceipt
	completeErr error
	next        int
}

func newMemoryStore(t *testing.T) *memoryStore {
	return &memoryStore{t: t, effects: map[string]*memoryEffect{}, keys: map[Digest]Digest{}, receipts: map[Digest]OperationReceipt{}}
}

func (store *memoryStore) replayLocked(key, command Digest) (OperationReceipt, bool, error) {
	if existing, ok := store.keys[key]; ok {
		if existing != command {
			return OperationReceipt{}, false, ErrPayloadMismatch
		}
		receipt, ok := store.receipts[command]
		if !ok {
			store.t.Fatal("receipt index has no receipt")
		}
		return receipt, true, nil
	}
	return OperationReceipt{}, false, nil
}

func (store *memoryStore) recordLocked(key, command Digest, receipt OperationReceipt) {
	store.keys[key], store.receipts[command] = command, receipt
}

func (store *memoryStore) Accept(_ context.Context, command AcceptCommand) (Projection, OperationReceipt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	commandDigest := command.digest()
	if receipt, replay, err := store.replayLocked(command.ReceiptKeyDigest, commandDigest); err != nil {
		return Projection{}, OperationReceipt{}, err
	} else if replay {
		return store.effects[receipt.EffectID].projection, receipt, nil
	}
	store.next++
	id := fmt.Sprintf("eer_%d", store.next)
	now := time.Now().UTC()
	effect := &memoryEffect{
		projection: Projection{ID: id, Owner: command.Envelope.Owner(), Kind: command.Envelope.Kind(), State: StateAccepted, Generation: 1, UpdatedAt: now},
		envelope:   command.Envelope,
		receipt:    OperationReceipt{ID: "receipt_" + id, EffectID: id, CommandDigest: commandDigest, State: StateAccepted, CompletedAt: now},
		lease:      Lease{EffectID: id, Generation: 1, Fence: 1, ExpiresAt: now.Add(time.Minute)},
	}
	store.effects[id] = effect
	store.recordLocked(command.ReceiptKeyDigest, commandDigest, effect.receipt)
	return effect.projection, effect.receipt, nil
}

func (store *memoryStore) Queue(_ context.Context, command QueueCommand) (Projection, OperationReceipt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	commandDigest := command.digest()
	if receipt, replay, err := store.replayLocked(command.ReceiptKeyDigest, commandDigest); err != nil {
		return Projection{}, OperationReceipt{}, err
	} else if replay {
		return store.effects[receipt.EffectID].projection, receipt, nil
	}
	effect := store.effects[command.EffectID]
	if effect == nil || effect.projection.State != StateAccepted {
		return Projection{}, OperationReceipt{}, ErrInvalidTransition
	}
	effect.projection.State, effect.projection.UpdatedAt = StateQueued, time.Now().UTC()
	operationReceipt := receipt(command.EffectID, commandDigest, StateQueued)
	store.recordLocked(command.ReceiptKeyDigest, commandDigest, operationReceipt)
	return effect.projection, operationReceipt, nil
}

func (store *memoryStore) Claim(_ context.Context, command ClaimCommand) (Lease, Projection, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	effect := store.effects[command.EffectID]
	if effect == nil || effect.projection.State != StateQueued {
		return Lease{}, Projection{}, ErrLeaseFence
	}
	return effect.lease, effect.projection, nil
}

func (store *memoryStore) Retry(_ context.Context, command RetryCommand) (Projection, OperationReceipt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	commandDigest := command.digest()
	if receipt, replay, err := store.replayLocked(command.ReceiptKeyDigest, commandDigest); err != nil {
		return Projection{}, OperationReceipt{}, err
	} else if replay {
		return store.effects[receipt.EffectID].projection, receipt, nil
	}
	effect := store.effects[command.EffectID]
	if effect == nil || effect.projection.State != StateRetryableFailed {
		return Projection{}, OperationReceipt{}, ErrRetryForbidden
	}
	effect.projection.State, effect.projection.Generation, effect.projection.UpdatedAt = StateQueued, effect.projection.Generation+1, time.Now().UTC()
	effect.lease = Lease{EffectID: command.EffectID, Generation: effect.projection.Generation, Fence: effect.lease.Fence + 1, ExpiresAt: time.Now().Add(time.Minute)}
	operationReceipt := receipt(command.EffectID, commandDigest, StateQueued)
	store.recordLocked(command.ReceiptKeyDigest, commandDigest, operationReceipt)
	return effect.projection, operationReceipt, nil
}

func (store *memoryStore) Cancel(_ context.Context, command CancelCommand) (Projection, OperationReceipt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	commandDigest := command.digest()
	if receipt, replay, err := store.replayLocked(command.ReceiptKeyDigest, commandDigest); err != nil {
		return Projection{}, OperationReceipt{}, err
	} else if replay {
		return store.effects[receipt.EffectID].projection, receipt, nil
	}
	effect := store.effects[command.EffectID]
	if effect == nil || (effect.projection.State != StateAccepted && effect.projection.State != StateQueued) {
		return Projection{}, OperationReceipt{}, ErrCancelForbidden
	}
	effect.projection.State, effect.projection.UpdatedAt = StateCancelled, time.Now().UTC()
	operationReceipt := receipt(command.EffectID, commandDigest, StateCancelled)
	store.recordLocked(command.ReceiptKeyDigest, commandDigest, operationReceipt)
	return effect.projection, operationReceipt, nil
}

func (store *memoryStore) PersistAttempt(_ context.Context, lease Lease) (EffectEnvelope, Attempt, Projection, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	effect := store.effects[lease.EffectID]
	if effect == nil || effect.projection.State != StateQueued || !sameFence(lease, effect.lease) || !effect.lease.ExpiresAt.After(time.Now()) {
		return EffectEnvelope{}, Attempt{}, Projection{}, ErrLeaseFence
	}
	effect.attempt = Attempt{Number: effect.projection.AttemptCount + 1, Generation: lease.Generation, Fence: lease.Fence, StartedAt: time.Now().UTC()}
	effect.projection.State, effect.projection.AttemptCount, effect.projection.UpdatedAt = StateAttempted, effect.attempt.Number, time.Now().UTC()
	effect.order = append(effect.order, "persist")
	return effect.envelope, effect.attempt, effect.projection, nil
}

func (store *memoryStore) CompleteAttempt(_ context.Context, lease Lease, attempt Attempt, result AdapterResult) (Projection, OperationReceipt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	effect := store.effects[lease.EffectID]
	if store.completeErr != nil {
		err := store.completeErr
		store.completeErr = nil
		return Projection{}, OperationReceipt{}, err
	}
	if effect == nil || effect.projection.State != StateAttempted || !sameFence(lease, effect.lease) || attempt != effect.attempt {
		return Projection{}, OperationReceipt{}, ErrLeaseFence
	}
	state, _ := result.Completion.state()
	effect.projection.State, effect.projection.UpdatedAt = state, time.Now().UTC()
	effect.order = append(effect.order, "complete")
	return effect.projection, receipt(lease.EffectID, result.ReceiptDigest, state), nil
}

func (store *memoryStore) Reconcile(_ context.Context, command ReconcileCommand) (Projection, OperationReceipt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	commandDigest := command.digest()
	if receipt, replay, err := store.replayLocked(command.ReceiptKeyDigest, commandDigest); err != nil {
		return Projection{}, OperationReceipt{}, err
	} else if replay {
		return store.effects[receipt.EffectID].projection, receipt, nil
	}
	effect := store.effects[command.Lease.EffectID]
	if effect == nil || effect.projection.State != StateOutcomeUnknown || !sameFence(command.Lease, effect.lease) {
		return Projection{}, OperationReceipt{}, ErrReconcileRequired
	}
	effect.projection.State, effect.projection.UpdatedAt = StateReconciled, time.Now().UTC()
	operationReceipt := receipt(command.Lease.EffectID, commandDigest, StateReconciled)
	store.recordLocked(command.ReceiptKeyDigest, commandDigest, operationReceipt)
	return effect.projection, operationReceipt, nil
}

func (store *memoryStore) RecoverAttemptedToUnknown(_ context.Context, command RecoverAttemptedCommand) (Projection, OperationReceipt, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	commandDigest := command.digest()
	if receipt, ok := store.receipts[commandDigest]; ok {
		return store.effects[receipt.EffectID].projection, receipt, nil
	}
	effect := store.effects[command.Lease.EffectID]
	if effect == nil || effect.projection.State != StateAttempted || !sameFence(command.Lease, effect.lease) || effect.lease.ExpiresAt.After(time.Now()) {
		return Projection{}, OperationReceipt{}, ErrRecoveryForbidden
	}
	effect.projection.State, effect.projection.UpdatedAt = StateOutcomeUnknown, time.Now().UTC()
	operationReceipt := receipt(command.Lease.EffectID, commandDigest, StateOutcomeUnknown)
	store.receipts[commandDigest] = operationReceipt
	return effect.projection, operationReceipt, nil
}

func (store *memoryStore) effectCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.effects)
}
func (store *memoryStore) leaseFor(id string) Lease {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.effects[id].lease
}
func (store *memoryStore) callOrder(id string) string {
	store.mu.Lock()
	defer store.mu.Unlock()
	return strings.Join(store.effects[id].order, ",")
}
func (store *memoryStore) isAttempted(id string, attempt Attempt) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	effect := store.effects[id]
	return effect != nil && effect.projection.State == StateAttempted && effect.attempt == attempt
}

func (store *memoryStore) expireLease(id string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.effects[id].lease.ExpiresAt = time.Now().Add(-time.Second)
}

func sameFence(left, right Lease) bool {
	return left.EffectID == right.EffectID && left.Generation == right.Generation && left.Fence == right.Fence
}

func receipt(effectID string, commandDigest Digest, state State) OperationReceipt {
	return OperationReceipt{ID: "receipt_" + effectID + "_" + string(state), EffectID: effectID, CommandDigest: commandDigest, State: state, CompletedAt: time.Now().UTC()}
}

func acceptQueued(t *testing.T, service *Service, key Digest) Projection {
	t.Helper()
	projection, _, err := service.Accept(context.Background(), AcceptCommand{ReceiptKeyDigest: key, Envelope: envelope(t)})
	if err != nil {
		t.Fatal(err)
	}
	queued, _, err := service.Queue(context.Background(), QueueCommand{EffectID: projection.ID, Job: jobLink(), ReceiptKeyDigest: digest("queue " + string(key))})
	if err != nil {
		t.Fatal(err)
	}
	return queued
}

func mustService(t *testing.T, store Store) *Service {
	t.Helper()
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	return service
}
func envelope(t *testing.T) EffectEnvelope {
	t.Helper()
	value, err := NewEnvelope(EnvelopeInput{Owner: OwnerCampaign, Kind: KindCampaignDispatch, SourceRefDigest: digest("source"), TargetRefDigest: digest("target"), PayloadDigest: digest("payload"), PolicyVersionHash: digest("policy")})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func jobLink() RiverJobLink {
	return RiverJobLink{JobID: 1, Generation: 1, Queue: "external_effects", ArgsDigest: digest("river"), ScheduledAt: time.Now().UTC()}
}
func retryJobLink() RiverJobLink {
	return RiverJobLink{JobID: 2, Generation: 2, Queue: "external_effects", ArgsDigest: digest("retry river"), ScheduledAt: time.Now().UTC()}
}
func digest(input string) Digest {
	sum := sha256.Sum256([]byte(input))
	return Digest("sha256:" + hex.EncodeToString(sum[:]))
}
