package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

func TestEnsureGroupOpsPreparationReusesSufficientLeaseWithoutQueue(t *testing.T) {
	store := &preparationStoreStub{ready: true}
	effects := &preparationEffectsStub{}
	jobs := &preparationJobsStub{}
	service := newPreparationService(t, store, effects, jobs)
	now := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	result, err := service.Ensure(context.Background(), preparationSources(), now.Add(24*time.Hour), now.Add(25*time.Hour))
	if err != nil || len(result) != 0 || effects.accepts != 0 || jobs.calls != 0 || store.nextCalls != 0 {
		t.Fatalf("result=%+v err=%v store=%+v effects=%+v jobs=%+v", result, err, store, effects, jobs)
	}
}

func TestEnsureGroupOpsPreparationQueuesNewGenerationAtPreparationWindow(t *testing.T) {
	store := &preparationStoreStub{generation: 2}
	effects := &preparationEffectsStub{}
	jobs := &preparationJobsStub{}
	service := newPreparationService(t, store, effects, jobs)
	now := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	scheduledFor := now.Add(7 * 24 * time.Hour)
	result, err := service.Ensure(context.Background(), preparationSources(), scheduledFor, scheduledFor.Add(time.Hour))
	if err != nil || len(result) != 1 || result[0].Generation != 2 || effects.accepts != 1 || effects.queues != 1 || jobs.calls != 1 || !jobs.scheduledAt.Equal(scheduledFor.Add(-12*time.Hour)) {
		t.Fatalf("result=%+v err=%v store=%+v effects=%+v jobs=%+v", result, err, store, effects, jobs)
	}
}

func TestEnsureGroupOpsPreparationDoesNotReplayTerminalEffect(t *testing.T) {
	store := &preparationStoreStub{generation: 2}
	effects := &preparationEffectsStub{}
	jobs := &preparationJobsStub{}
	service := newPreparationService(t, store, effects, jobs)
	now := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	if _, err := service.Ensure(context.Background(), preparationSources(), now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	store.generation = 3 // the old generation is terminal; a refresh gets a new EER fingerprint.
	if _, err := service.Ensure(context.Background(), preparationSources(), now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if effects.accepts != 2 || effects.fingerprints[0] == effects.fingerprints[1] || effects.queues != 2 || jobs.effectIDs[0] == jobs.effectIDs[1] {
		t.Fatalf("effects=%+v jobs=%+v", effects, jobs)
	}
}

func TestGroupOpsUploadAdapterPersistsReadyBeforeExecutedAndNeverRetriesUnknown(t *testing.T) {
	source := "sha256:7777777777777777777777777777777777777777777777777777777777777777"
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerMedia, Kind: eer.KindMediaWeComUpload, SourceRefDigest: eer.Digest(source), TargetRefDigest: eer.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), PayloadDigest: eer.Digest("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), PolicyVersionHash: eer.Digest("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name                  string
		result                GroupOpsMaterialUploadResult
		err                   error
		completion            eer.Completion
		ready, unknown, final int
		wantErr               bool
	}{
		{name: "success", result: GroupOpsMaterialUploadResult{MediaID: "media-1", CreatedAt: now, ExpiresAt: now.Add(71 * time.Hour), BusinessCallDispatched: true}, completion: eer.CompletionExecuted, ready: 1},
		{name: "unknown after boundary", result: GroupOpsMaterialUploadResult{BusinessCallDispatched: true, OutcomeUnknown: true}, err: errors.New("timeout"), completion: eer.CompletionOutcomeUnknown, unknown: 1, wantErr: true},
		{name: "pre-dispatch failure", err: errors.New("invalid input"), completion: eer.CompletionFinalFailed, final: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &uploadAttemptStoreStub{input: GroupOpsMaterialUploadInput{EffectID: "eer_7", SourceDigest: source}}
			adapter := &groupOpsMaterialUploadAdapter{effectID: "eer_7", store: store, provider: uploadProviderStub{result: test.result, err: test.err}, now: func() time.Time { return now }}
			result, err := adapter.Execute(context.Background(), envelope, eer.Attempt{Number: 1})
			if (err != nil) != test.wantErr || result.Completion != test.completion || store.ready != test.ready || store.unknown != test.unknown || store.final != test.final {
				t.Fatalf("result=%+v err=%v store=%+v", result, err, store)
			}
		})
	}
}

func newPreparationService(t *testing.T, store *preparationStoreStub, effects *preparationEffectsStub, jobs *preparationJobsStub) *GroupOpsMaterialPreparationService {
	t.Helper()
	scope := eer.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	service, err := NewGroupOpsMaterialPreparationService(store, effects, jobs, scope)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func preparationSources() mediaport.GroupOpsMaterialSourceSnapshot {
	return mediaport.GroupOpsMaterialSourceSnapshot{SchemaVersion: 1, References: []mediaport.GroupOpsMaterialSourceReference{{Reference: mediaport.GroupOpsMaterialReference{Kind: "image", ID: 7}, SourceDigest: "sha256:7777777777777777777777777777777777777777777777777777777777777777"}}}
}

type preparationStoreStub struct {
	ready, bound bool
	generation   int64
	nextCalls    int
}

func (stub *preparationStoreStub) HasSufficientGroupOpsUploadLease(context.Context, string, int64, string, string, string, time.Time) (bool, error) {
	return stub.ready, nil
}
func (stub *preparationStoreStub) NextGroupOpsUploadPreparationGeneration(context.Context, string, int64, string, string, string) (int64, error) {
	stub.nextCalls++
	return stub.generation, nil
}
func (stub *preparationStoreStub) BindGroupOpsUploadPreparation(_ context.Context, value GroupOpsMaterialPreparation) (bool, error) {
	stub.bound = true
	return true, nil
}

type preparationEffectsStub struct {
	accepts, queues int
	fingerprints    []eer.Digest
}

func (stub *preparationEffectsStub) Accept(_ context.Context, command eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
	stub.accepts++
	stub.fingerprints = append(stub.fingerprints, command.Envelope.Fingerprint())
	return eer.Projection{ID: fmt.Sprintf("eer_%d", stub.accepts), Owner: eer.OwnerMedia, Kind: eer.KindMediaWeComUpload, State: eer.StateAccepted, Generation: 1, UpdatedAt: time.Now()}, eer.OperationReceipt{}, nil
}
func (stub *preparationEffectsStub) Queue(_ context.Context, command eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error) {
	stub.queues++
	return eer.Projection{ID: command.EffectID, Owner: eer.OwnerMedia, Kind: eer.KindMediaWeComUpload, State: eer.StateQueued, Generation: 2, UpdatedAt: time.Now()}, eer.OperationReceipt{}, nil
}
func (*preparationEffectsStub) Claim(context.Context, eer.ClaimCommand) (eer.Lease, eer.Projection, error) {
	return eer.Lease{}, eer.Projection{}, fmt.Errorf("not used")
}
func (*preparationEffectsStub) RunAttempt(context.Context, eer.Lease, eer.Adapter) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, fmt.Errorf("not used")
}

type preparationJobsStub struct {
	calls       int
	effectIDs   []string
	scheduledAt time.Time
}

func (stub *preparationJobsStub) Insert(_ context.Context, args GroupOpsMaterialPreparationJobArgs, scheduledAt time.Time) (eer.RiverJobLink, error) {
	stub.calls++
	stub.effectIDs = append(stub.effectIDs, args.EffectID)
	stub.scheduledAt = scheduledAt
	return eer.RiverJobLink{JobID: int64(stub.calls), Generation: 1, Queue: "outbound", ArgsDigest: preparationDigest("job", args.EffectID), ScheduledAt: scheduledAt}, nil
}

type uploadAttemptStoreStub struct {
	input                 GroupOpsMaterialUploadInput
	ready, unknown, final int
}

func (stub *uploadAttemptStoreStub) LoadGroupOpsMaterialUpload(context.Context, string) (GroupOpsMaterialUploadInput, error) {
	return stub.input, nil
}
func (stub *uploadAttemptStoreStub) RecordGroupOpsMaterialUploadReady(context.Context, string, GroupOpsMaterialUploadResult, eer.Digest) error {
	stub.ready++
	return nil
}
func (stub *uploadAttemptStoreStub) MarkGroupOpsMaterialUploadOutcomeUnknown(context.Context, string, time.Time) error {
	stub.unknown++
	return nil
}
func (stub *uploadAttemptStoreStub) MarkGroupOpsMaterialUploadFinalFailed(context.Context, string, time.Time) error {
	stub.final++
	return nil
}

type uploadProviderStub struct {
	result GroupOpsMaterialUploadResult
	err    error
}

func (stub uploadProviderStub) Upload(context.Context, GroupOpsMaterialUploadInput) (GroupOpsMaterialUploadResult, error) {
	return stub.result, stub.err
}
