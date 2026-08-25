package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	eerstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var ch02StoreDatabaseURL = flag.String("ch02-store-database-url", "", "isolated PostgreSQL database migrated through 00090")

func TestCH02TerminalEvidencePG16(t *testing.T) {
	if *ch02StoreDatabaseURL == "" {
		t.Skip("-ch02-store-database-url is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *ch02StoreDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := eerstore.NewRepository(pool, platformstore.NewUnitOfWork(pool))
	service, err := eer.NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerContact, Kind: eer.KindContactAcquisitionAssetPublish,
		SourceRefDigest: ch02Digest("source"), TargetRefDigest: ch02Digest("target"), PayloadDigest: ch02Digest("payload"), PolicyVersionHash: ch02Digest("policy")})
	if err != nil {
		t.Fatal(err)
	}
	accepted, acceptReceipt, err := service.Accept(ctx, eer.AcceptCommand{ReceiptKeyDigest: ch02Digest("accept"), Envelope: envelope})
	if err != nil {
		t.Fatal(err)
	}
	queued, queueReceipt, err := service.Queue(ctx, eer.QueueCommand{EffectID: accepted.ID, ReceiptKeyDigest: ch02Digest("queue"), Job: eer.RiverJobLink{JobID: 90, Generation: 2, Queue: "external_effects", ArgsDigest: ch02Digest("args"), ScheduledAt: time.Now().UTC()}})
	if err != nil {
		t.Fatal(err)
	}
	ch02SeedBinding(t, ctx, pool, "normal", accepted.ID, acceptReceipt.ID, queueReceipt.ID, queued.Generation, envelope.Fingerprint(), ch02Digest("accept"), ch02Digest("queue"))
	lease, _, err := service.Claim(ctx, eer.ClaimCommand{EffectID: accepted.ID, WorkerDigest: ch02Digest("worker")})
	if err != nil {
		t.Fatal(err)
	}
	var effectID int64
	if _, err := fmt.Sscanf(accepted.ID, "eer_%d", &effectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE channel_acquisition_asset_bindings SET state='attempted', fence=$2, lease_expires_at=$3, updated_at=now() WHERE effect_id=$1`, effectID, lease.Fence, lease.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	reference := ch02Digest("provider-reference")
	projection, _, err := service.RunAttempt(ctx, lease, ch02Adapter(func(context.Context, eer.EffectEnvelope, eer.Attempt) (eer.AdapterResult, error) {
		return eer.AdapterResult{Completion: eer.CompletionExecuted, ReceiptDigest: ch02Digest("completed"), ResultReferenceDigest: reference, BusinessCallDispatched: true, RealExternalCallExecuted: true}, nil
	}))
	if err != nil || projection.State != eer.StateExecuted {
		t.Fatalf("completion=%+v err=%v", projection, err)
	}
	terminal, err := store.GetTerminalOutcome(ctx, accepted.ID)
	if err != nil || terminal.ResultReferenceDigest != reference || !terminal.BusinessCallDispatched || !terminal.RealExternalCallExecuted {
		t.Fatalf("terminal=%+v err=%v", terminal, err)
	}
	recoveryEnvelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerContact, Kind: eer.KindContactAcquisitionAssetPublish,
		SourceRefDigest: ch02Digest("recovery source"), TargetRefDigest: ch02Digest("recovery target"), PayloadDigest: ch02Digest("recovery payload"), PolicyVersionHash: ch02Digest("recovery policy")})
	if err != nil {
		t.Fatal(err)
	}
	recoveryAccepted, recoveryAcceptReceipt, err := service.Accept(ctx, eer.AcceptCommand{ReceiptKeyDigest: ch02Digest("recovery accept"), Envelope: recoveryEnvelope})
	if err != nil {
		t.Fatal(err)
	}
	recoveryQueued, recoveryQueueReceipt, err := service.Queue(ctx, eer.QueueCommand{EffectID: recoveryAccepted.ID, ReceiptKeyDigest: ch02Digest("recovery queue"), Job: eer.RiverJobLink{JobID: 91, Generation: 2, Queue: "external_effects", ArgsDigest: ch02Digest("recovery args"), ScheduledAt: time.Now().UTC()}})
	if err != nil {
		t.Fatal(err)
	}
	ch02SeedBinding(t, ctx, pool, "recovery", recoveryAccepted.ID, recoveryAcceptReceipt.ID, recoveryQueueReceipt.ID, recoveryQueued.Generation, recoveryEnvelope.Fingerprint(), ch02Digest("recovery accept"), ch02Digest("recovery queue"))
	recoveryLease, _, err := service.Claim(ctx, eer.ClaimCommand{EffectID: recoveryAccepted.ID, WorkerDigest: ch02Digest("recovery worker")})
	if err != nil {
		t.Fatal(err)
	}
	var recoveryEffectID int64
	if _, err := fmt.Sscanf(recoveryAccepted.ID, "eer_%d", &recoveryEffectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE channel_acquisition_asset_bindings SET state='attempted', fence=$2, lease_expires_at=$3, updated_at=now() WHERE effect_id=$1`, recoveryEffectID, recoveryLease.Fence, recoveryLease.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.PersistAttempt(ctx, recoveryLease); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE external_effects SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, recoveryEffectID); err != nil {
		t.Fatal(err)
	}
	recoveryLease.ExpiresAt = time.Now().Add(-time.Second)
	if recovered, _, err := service.RecoverAttemptedToUnknown(ctx, eer.RecoverAttemptedCommand{Lease: recoveryLease}); err != nil || recovered.State != eer.StateOutcomeUnknown {
		t.Fatalf("recovery=%+v err=%v", recovered, err)
	}
	recoveredTerminal, err := store.GetTerminalOutcome(ctx, recoveryAccepted.ID)
	if err != nil || recoveredTerminal.ResultReferenceDigest != "" || recoveredTerminal.BusinessCallDispatched || recoveredTerminal.RealExternalCallExecuted {
		t.Fatalf("recovered terminal=%+v err=%v", recoveredTerminal, err)
	}
}

type ch02Adapter func(context.Context, eer.EffectEnvelope, eer.Attempt) (eer.AdapterResult, error)

func (adapter ch02Adapter) Execute(ctx context.Context, envelope eer.EffectEnvelope, attempt eer.Attempt) (eer.AdapterResult, error) {
	return adapter(ctx, envelope, attempt)
}

func ch02SeedBinding(t *testing.T, ctx context.Context, pool *pgxpool.Pool, label, effect, acceptReceipt, queueReceipt string, generation int64, fingerprint, acceptDigest, queueDigest eer.Digest) {
	t.Helper()
	var effectID, acceptReceiptID, queueReceiptID, channelID int64
	for _, value := range []struct {
		text   string
		format string
		out    *int64
	}{{effect, "eer_%d", &effectID}, {acceptReceipt, "eerop_%d", &acceptReceiptID}, {queueReceipt, "eerop_%d", &queueReceiptID}} {
		if _, err := fmt.Sscanf(value.text, value.format, value.out); err != nil {
			t.Fatal(err)
		}
	}
	code := "ch02-evidence-" + label
	if err := pool.QueryRow(ctx, `INSERT INTO channels(name, code, config) VALUES ($1, $2, '{"schema_version":1}'::jsonb) RETURNING id`, "CH02 evidence "+label, code).Scan(&channelID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO channel_acquisition_asset_bindings (
  effect_id, channel_id, asset_kind, asset_version, channel_revision, channel_code,
  channel_name, channel_status, scene_value, assignee_wecom_userids, snapshot_digest,
  idempotency_digest, envelope_fingerprint, state, accept_receipt_id, accept_receipt_digest,
  queue_receipt_id, queue_receipt_digest, river_job_id, generation, created_at, updated_at
) VALUES (
  $1, $2, 'contact_way_qrcode', 1, 1, $3, $4, 'active',
  $5, ARRAY['ch02-assignee'], $6, $7, $8, 'queued', $9, $10, $11, $12, 90, $13, now(), now()
)`, effectID, channelID, code, "CH02 evidence "+label, code+"-scene", ch02Digest(label+" snapshot"), ch02Digest(label+" idempotency"), fingerprint, acceptReceiptID, acceptDigest, queueReceiptID, queueDigest, generation); err != nil {
		t.Fatal(err)
	}
}

func ch02Digest(value string) eer.Digest {
	sum := sha256.Sum256([]byte(value))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
