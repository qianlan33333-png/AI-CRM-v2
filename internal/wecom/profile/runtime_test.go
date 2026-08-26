package profile

import (
	"context"
	"testing"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

type profileTestUOW struct{}

func (profileTestUOW) Within(ctx context.Context, f func(context.Context) error) error { return f(ctx) }

type profileStoreFake struct {
	item       Effect
	exists     bool
	complete   AttemptCompletion
	reconciled ReconcileCompletion
}

func (s *profileStoreFake) Reserve(_ context.Context, e Effect) (Effect, bool, error) {
	if s.exists {
		return s.item, false, nil
	}
	s.item = e
	s.exists = true
	return e, true, nil
}
func (s *profileStoreFake) GetByIdempotency(_ context.Context, _ int64, _ eer.Digest) (Effect, error) {
	if !s.exists {
		return Effect{}, ErrEffectUnavailable
	}
	return s.item, nil
}
func (s *profileStoreFake) MarkQueued(_ context.Context, _ string, l eer.RiverJobLink, r string, at time.Time) (Effect, error) {
	s.item.State = eer.StateQueued
	s.item.RiverJobID = l.JobID
	s.item.QueueReceiptID = r
	s.item.Generation = l.Generation
	s.item.UpdatedAt = at
	return s.item, nil
}
func (s *profileStoreFake) Get(_ context.Context, _ string) (Effect, error) {
	if !s.exists {
		return Effect{}, ErrEffectUnavailable
	}
	return s.item, nil
}
func (s *profileStoreFake) RecordClaim(_ context.Context, _ string, l eer.Lease, at time.Time) (Effect, error) {
	s.item.Fence = l.Fence
	s.item.Generation = l.Generation
	s.item.LeaseExpiresAt = l.ExpiresAt
	s.item.UpdatedAt = at
	return s.item, nil
}
func (s *profileStoreFake) CompleteAttempt(_ context.Context, c AttemptCompletion) (Effect, error) {
	s.complete = c
	s.item.State = c.State
	s.item.AttemptReceiptID = c.ReceiptID
	s.item.AttemptReceiptDigest = c.Receipt
	s.item.AttemptCompletedAt = c.CompletedAt
	s.item.ProviderCallAttempted = c.ProviderCallAttempted
	s.item.RealExternalCallExecuted = c.RealExternalCallExecuted
	s.item.UpdatedAt = c.CompletedAt
	return s.item, nil
}
func (s *profileStoreFake) CompleteReconcile(_ context.Context, c ReconcileCompletion) (Effect, error) {
	s.reconciled = c
	s.item.State = eer.StateReconciled
	s.item.ReconcileReceiptID = c.ReceiptID
	s.item.ReconcileResolution = c.Resolution
	s.item.UpdatedAt = c.CompletedAt
	return s.item, nil
}

type profileJobsFake struct{ calls int }

func (j *profileJobsFake) Insert(_ context.Context, a JobArgs, g int64, at time.Time) (eer.RiverJobLink, error) {
	j.calls++
	return eer.RiverJobLink{JobID: int64(j.calls), Generation: g, Queue: "sync", ArgsDigest: digest("job", a.EffectID), ScheduledAt: at}, nil
}

type profileRuntimeFake struct {
	now         time.Time
	p           eer.Projection
	envelope    eer.EffectEnvelope
	lease       eer.Lease
	run, claims int
}

func (f *profileRuntimeFake) receipt(id string, d eer.Digest) eer.OperationReceipt {
	return eer.OperationReceipt{ID: "eerop_" + id, EffectID: f.p.ID, CommandDigest: d, State: f.p.State, CompletedAt: f.p.UpdatedAt}
}
func (f *profileRuntimeFake) Accept(_ context.Context, c eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
	if f.p.ID != "" {
		if f.envelope.Fingerprint() != c.Envelope.Fingerprint() {
			return eer.Projection{}, eer.OperationReceipt{}, eer.ErrPayloadMismatch
		}
		return f.p, f.receipt("accept", c.CommandDigest()), nil
	}
	f.envelope = c.Envelope
	f.p = eer.Projection{ID: "eer_41", Owner: eer.OwnerWeCom, Kind: "wecom_profile_sync", State: eer.StateAccepted, Generation: 1, UpdatedAt: f.now}
	return f.p, f.receipt("accept", c.CommandDigest()), nil
}
func (f *profileRuntimeFake) Queue(_ context.Context, c eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error) {
	f.p.State = eer.StateQueued
	f.p.Generation = c.Job.Generation
	f.p.UpdatedAt = f.now.Add(time.Second)
	return f.p, f.receipt("queue", c.CommandDigest()), nil
}
func (f *profileRuntimeFake) Claim(_ context.Context, c eer.ClaimCommand) (eer.Lease, eer.Projection, error) {
	f.claims++
	f.lease = eer.Lease{EffectID: c.EffectID, Generation: f.p.Generation, Fence: int64(f.claims), ExpiresAt: f.now.Add(time.Minute)}
	return f.lease, f.p, nil
}
func (f *profileRuntimeFake) RunAttempt(ctx context.Context, l eer.Lease, a eer.Adapter) (eer.Projection, eer.OperationReceipt, error) {
	f.run++
	r, err := a.Execute(ctx, f.envelope, eer.Attempt{Number: int32(f.run), Generation: l.Generation, Fence: l.Fence, StartedAt: f.now})
	if err != nil {
		r = eer.AdapterResult{Completion: eer.CompletionOutcomeUnknown, ReceiptDigest: digest("unknown", l.EffectID), BusinessCallDispatched: true, RealExternalCallExecuted: true}
	}
	f.p.State = eer.State(r.Completion)
	f.p.UpdatedAt = f.now.Add(2 * time.Second)
	return f.p, f.receipt("attempt", r.ReceiptDigest), nil
}
func (f *profileRuntimeFake) Reconcile(_ context.Context, c eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	if c.Lease != f.lease {
		return eer.Projection{}, eer.OperationReceipt{}, eer.ErrLeaseFence
	}
	f.p.State = eer.StateReconciled
	f.p.UpdatedAt = f.now.Add(3 * time.Second)
	return f.p, f.receipt("reconcile", c.CommandDigest()), nil
}

type profileWriterFake struct {
	calls  int
	result eer.AdapterResult
	err    error
}

func (w *profileWriterFake) WriteContactProfile(_ context.Context, _ wecomport.ContactProfileWriteRequest) (eer.AdapterResult, error) {
	w.calls++
	return w.result, w.err
}

func TestProfileQueueReplayAndUnknownReconcile(t *testing.T) {
	now := time.Date(2026, 8, 26, 1, 0, 0, 0, time.UTC)
	store := &profileStoreFake{}
	runtime := &profileRuntimeFake{now: now}
	jobs := &profileJobsFake{}
	service, err := NewService(profileTestUOW{}, store, runtime, jobs, "corp-1")
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	command := QueueCommand{LegacyReceiptID: 12, Actor: 7, IdempotencyKey: "wecom-profile-idem-0001", StaffUserID: "staff-1", ExternalUserID: "external-1", Remark: "VIP", Description: "follow up"}
	first, err := service.Queue(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Queue(context.Background(), command)
	if err != nil || first != second || jobs.calls != 1 {
		t.Fatalf("replay first=%+v second=%+v jobs=%d err=%v", first, second, jobs.calls, err)
	}
	writer := &profileWriterFake{result: eer.AdapterResult{Completion: eer.CompletionOutcomeUnknown, ReceiptDigest: digest("transport", "1"), BusinessCallDispatched: true, RealExternalCallExecuted: true}}
	one, err := service.Execute(context.Background(), first.EffectID, digest("worker", "1"), writer)
	if err != nil || one.State != eer.StateOutcomeUnknown || !one.ManualReconcileRequired || !one.ProviderCallAttempted || !one.RealExternalCallExecuted || writer.calls != 1 {
		t.Fatalf("unknown=%+v calls=%d err=%v", one, writer.calls, err)
	}
	two, err := service.Execute(context.Background(), first.EffectID, digest("worker", "1"), writer)
	if err != nil || two.ProviderCallAttempted || two.RealExternalCallExecuted || writer.calls != 1 || runtime.claims != 1 {
		t.Fatalf("replay=%+v calls=%d claims=%d err=%v", two, writer.calls, runtime.claims, err)
	}
	reconciled, err := service.Reconcile(context.Background(), ReconcileCommand{EffectID: first.EffectID, Actor: 7, IdempotencyKey: "wecom-profile-reconcile-0001", Generation: store.item.Generation, Fence: store.item.Fence, LeaseExpiresAt: store.item.LeaseExpiresAt, EvidenceDigest: digest("evidence", "1"), Resolution: ResolutionProviderApplied})
	if err != nil || reconciled.State != eer.StateReconciled || !reconciled.RealExternalCallExecuted {
		t.Fatalf("reconcile=%+v err=%v", reconciled, err)
	}
}
