package externaleffects_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	eerstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var externalEffectsDatabaseURL = flag.String("external-effects-database-url", "", "isolated EER PostgreSQL 16 database")

type adapterFunc func(context.Context, eer.EffectEnvelope, eer.Attempt) (eer.AdapterResult, error)

func (f adapterFunc) Execute(ctx context.Context, envelope eer.EffectEnvelope, attempt eer.Attempt) (eer.AdapterResult, error) {
	return f(ctx, envelope, attempt)
}

func TestExternalEffectsPG16CASLeaseFenceReceiptsAndReconcile(t *testing.T) {
	if *externalEffectsDatabaseURL == "" {
		t.Skip("-external-effects-database-url is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *externalEffectsDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := eerstore.NewRepository(pool, platformstore.NewUnitOfWork(pool))
	service, err := eer.NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	accepted, _, err := service.Accept(ctx, eer.AcceptCommand{ReceiptKeyDigest: digest("accept"), Envelope: envelope(t)})
	if err != nil || accepted.State != eer.StateAccepted {
		t.Fatalf("accept=%+v %v", accepted, err)
	}
	firstJob := job()
	queued, firstReceipt, err := service.Queue(ctx, eer.QueueCommand{EffectID: accepted.ID, ReceiptKeyDigest: digest("queue"), Job: firstJob})
	if err != nil || queued.State != eer.StateQueued {
		t.Fatalf("queue=%+v %v", queued, err)
	}
	_, replay, err := service.Queue(ctx, eer.QueueCommand{EffectID: accepted.ID, ReceiptKeyDigest: digest("queue"), Job: firstJob})
	if err != nil || replay.ID != firstReceipt.ID {
		t.Fatalf("queue replay=%+v %v", replay, err)
	}
	lease, _, err := service.Claim(ctx, eer.ClaimCommand{EffectID: accepted.ID, WorkerDigest: digest("worker")})
	if err != nil {
		t.Fatal(err)
	}
	completed, _, err := service.RunAttempt(ctx, lease, adapterFunc(func(context.Context, eer.EffectEnvelope, eer.Attempt) (eer.AdapterResult, error) {
		return eer.AdapterResult{Completion: eer.CompletionExecuted, ReceiptDigest: digest("executed")}, nil
	}))
	if err != nil || completed.State != eer.StateExecuted {
		t.Fatalf("run=%+v %v", completed, err)
	}
	if _, _, err = service.RunAttempt(ctx, lease, adapterFunc(func(context.Context, eer.EffectEnvelope, eer.Attempt) (eer.AdapterResult, error) {
		t.Fatal("stale lease invoked adapter")
		return eer.AdapterResult{}, nil
	})); !errors.Is(err, eer.ErrLeaseFence) {
		t.Fatalf("stale lease=%v", err)
	}
	unknown, _, err := service.Accept(ctx, eer.AcceptCommand{ReceiptKeyDigest: digest("accept unknown"), Envelope: envelopeFor(t, "unknown")})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.Queue(ctx, eer.QueueCommand{EffectID: unknown.ID, ReceiptKeyDigest: digest("queue unknown"), Job: eer.RiverJobLink{JobID: 2, Generation: 1, Queue: "external_effects", ArgsDigest: digest("args2"), ScheduledAt: time.Now().UTC()}})
	if err != nil {
		t.Fatal(err)
	}
	unknownLease, _, err := service.Claim(ctx, eer.ClaimCommand{EffectID: unknown.ID, WorkerDigest: digest("worker unknown")})
	if err != nil {
		t.Fatal(err)
	}
	state, _, err := service.RunAttempt(ctx, unknownLease, adapterFunc(func(context.Context, eer.EffectEnvelope, eer.Attempt) (eer.AdapterResult, error) {
		return eer.AdapterResult{}, errors.New("transport unknown")
	}))
	if !errors.Is(err, eer.ErrAdapterFailure) || state.State != eer.StateOutcomeUnknown {
		t.Fatalf("unknown=%+v %v", state, err)
	}
	reconciled, _, err := service.Reconcile(ctx, eer.ReconcileCommand{Lease: unknownLease, ReceiptKeyDigest: digest("reconcile"), EvidenceDigest: digest("evidence")})
	if err != nil || reconciled.State != eer.StateReconciled {
		t.Fatalf("reconcile=%+v %v", reconciled, err)
	}
	recovering, _, err := service.Accept(ctx, eer.AcceptCommand{ReceiptKeyDigest: digest("accept recovery"), Envelope: envelopeFor(t, "recovery")})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.Queue(ctx, eer.QueueCommand{EffectID: recovering.ID, ReceiptKeyDigest: digest("queue recovery"), Job: eer.RiverJobLink{JobID: 3, Generation: 1, Queue: "external_effects", ArgsDigest: digest("args3"), ScheduledAt: time.Now().UTC()}})
	if err != nil {
		t.Fatal(err)
	}
	recoveryLease, _, err := service.Claim(ctx, eer.ClaimCommand{EffectID: recovering.ID, WorkerDigest: digest("worker recovery")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = store.PersistAttempt(ctx, recoveryLease); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE external_effects SET lease_expires_at=now()-interval '1 second' WHERE id=3`); err != nil {
		t.Fatal(err)
	}
	recoveryLease.ExpiresAt = time.Now().Add(-time.Second)
	recovered, _, err := service.RecoverAttemptedToUnknown(ctx, eer.RecoverAttemptedCommand{Lease: recoveryLease})
	if err != nil || recovered.State != eer.StateOutcomeUnknown {
		t.Fatalf("recovery=%+v %v", recovered, err)
	}
}
func envelope(t *testing.T) eer.EffectEnvelope {
	return envelopeFor(t, "")
}
func envelopeFor(t *testing.T, suffix string) eer.EffectEnvelope {
	t.Helper()
	value, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, SourceRefDigest: digest("source" + suffix), TargetRefDigest: digest("target" + suffix), PayloadDigest: digest("payload" + suffix), PolicyVersionHash: digest("policy")})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func job() eer.RiverJobLink {
	return eer.RiverJobLink{JobID: 1, Generation: 1, Queue: "external_effects", ArgsDigest: digest("args"), ScheduledAt: time.Now().UTC()}
}
func digest(value string) eer.Digest {
	sum := sha256.Sum256([]byte(value))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
