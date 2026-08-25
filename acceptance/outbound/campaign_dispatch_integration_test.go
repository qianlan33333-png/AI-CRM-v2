package outbound_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	eerstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	outboundworker "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/worker"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

var outboundCampaignDispatchDatabaseURL = flag.String("campaign-dispatch-database-url", "", "dedicated PostgreSQL 16.14 C01 outbound campaign dispatch database")

type campaignDispatchFakeAdapter struct{}

func (campaignDispatchFakeAdapter) Execute(_ context.Context, envelope eer.EffectEnvelope, attempt eer.Attempt) (eer.AdapterResult, error) {
	return eer.AdapterResult{Completion: eer.CompletionExecuted, ReceiptDigest: campaignDispatchDigest("fake-receipt", string(envelope.Fingerprint()), string(rune(attempt.Number)))}, nil
}

type campaignDispatchUnknownAdapter struct{}

func (campaignDispatchUnknownAdapter) Execute(context.Context, eer.EffectEnvelope, eer.Attempt) (eer.AdapterResult, error) {
	return eer.AdapterResult{}, errors.New("fake transport outcome unknown")
}

func TestCampaignDispatchPG16FakeReceiptUnknownAndManualReconcile(t *testing.T) {
	pool := openCampaignDispatchPool(t)
	ctx := context.Background()
	var migrationApplied bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.goose_db_version WHERE version_id=78 AND is_applied)`).Scan(&migrationApplied); err != nil || !migrationApplied {
		t.Fatalf("migration 78 applied=%t err=%v, want true", migrationApplied, err)
	}
	ensureOutboundRiverCatalog(t, ctx, pool)

	planID := outboundCampaignHandoffPlanID('d')
	source := &approvedCampaignHandoffSource{snapshot: outboundport.ApprovedCampaignHandoffSnapshot{
		CampaignCode: "c01-dispatch", PlanID: planID, ReviewVersion: 3,
		SourceDigest: strings.Repeat("11", 32), TargetDigest: strings.Repeat("22", 32), ContentDigest: strings.Repeat("33", 32),
		CustomerIDs: []int64{101, 202}, Steps: []outbound.CampaignHandoffStep{{Index: 1, Content: "approved immutable content"}},
		ApprovedAt: time.Now().UTC().Truncate(time.Microsecond),
	}}
	if _, err := newCampaignHandoffService(t, pool, source).Accept(ctx, outboundapp.AcceptCampaignHandoffCommand{
		CampaignCode: source.snapshot.CampaignCode, PlanID: planID, ExpectedReviewVersion: 3, ActorID: 71, IdempotencyKey: "c01-dispatch-handoff-accept",
	}); err != nil {
		t.Fatal(err)
	}

	uow := platformstore.NewUnitOfWork(pool)
	runtimeStore := eerstore.NewRepository(pool, uow)
	runtime, err := eer.NewService(runtimeStore)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := outboundstore.NewCampaignDispatchRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	service, err := outboundapp.NewCampaignDispatchService(uow, repository, runtime, repository)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.Dispatch(ctx, outboundapp.CampaignDispatchCommand{
		CampaignCode: source.snapshot.CampaignCode, PlanID: planID, ActorID: 71, IdempotencyKey: "c01-dispatch-operator-command", ExternalGate: true,
	})
	if err != nil || summary.Queued != 2 || summary.DeliveryProven || summary.RealExternalCallExecuted {
		t.Fatalf("queued summary=%+v err=%v", summary, err)
	}

	var effectIDs []string
	if rows, queryErr := pool.Query(ctx, `SELECT 'eer_' || external_effect_id::text FROM public.outbound_campaign_dispatches ORDER BY customer_id`); queryErr != nil {
		t.Fatal(queryErr)
	} else {
		defer rows.Close()
		for rows.Next() {
			var effectID string
			if scanErr := rows.Scan(&effectID); scanErr != nil {
				t.Fatal(scanErr)
			}
			effectIDs = append(effectIDs, effectID)
		}
		if rows.Err() != nil {
			t.Fatal(rows.Err())
		}
	}
	if len(effectIDs) != 2 {
		t.Fatalf("effects=%v, want two", effectIDs)
	}

	worker, err := outboundworker.NewCampaignDispatchWorker(service, campaignDispatchFakeAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, &river.Job[outboundstore.CampaignDispatchArgs]{
		JobRow: &rivertype.JobRow{ID: 901, Attempt: 1, MaxAttempts: 1, State: rivertype.JobStateRunning}, Args: outboundstore.CampaignDispatchArgs{EffectID: effectIDs[0]},
	}); err != nil {
		t.Fatal(err)
	}

	lease, _, err := runtime.Claim(ctx, eer.ClaimCommand{EffectID: effectIDs[1], WorkerDigest: campaignDispatchDigest("unknown-worker")})
	if err != nil {
		t.Fatal(err)
	}
	unknown, _, err := runtime.RunAttempt(ctx, lease, campaignDispatchUnknownAdapter{})
	if !errors.Is(err, eer.ErrAdapterFailure) || unknown.State != eer.StateOutcomeUnknown {
		t.Fatalf("unknown projection=%+v err=%v", unknown, err)
	}
	if _, err = service.ManualReconcile(ctx, outboundapp.CampaignDispatchReconcileCommand{
		CampaignCode: source.snapshot.CampaignCode, PlanID: planID, EffectID: effectIDs[1], ActorID: 71, IdempotencyKey: "c01-dispatch-manual-reconcile", Generation: lease.Generation, Fence: lease.Fence, LeaseExpiresAt: lease.ExpiresAt, EvidenceDigest: string(campaignDispatchDigest("operator-evidence")),
	}); err != nil {
		t.Fatal(err)
	}

	summary, err = service.Reconciliation(ctx, source.snapshot.CampaignCode, planID)
	if err != nil || summary.Executed != 1 || summary.Reconciled != 1 || summary.OutcomeUnknown != 0 || summary.DeliveryProven || summary.RealExternalCallExecuted {
		t.Fatalf("terminal summary=%+v err=%v", summary, err)
	}
	var receiptCount int
	var proven bool
	if err = pool.QueryRow(ctx, `SELECT count(*), bool_or(delivery_proven) FROM public.outbound_campaign_provider_attempt_receipts`).Scan(&receiptCount, &proven); err != nil || receiptCount != 2 || proven {
		t.Fatalf("provider attempt receipts=%d proven=%t err=%v, want 2/false", receiptCount, proven, err)
	}
}

func openCampaignDispatchPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if *outboundCampaignDispatchDatabaseURL == "" {
		t.Skip("-campaign-dispatch-database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(*outboundCampaignDispatchDatabaseURL, acceptancefixtures.C01CampaignDispatchDatabaseName); err != nil {
		t.Fatalf("unsafe campaign dispatch database URL: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), *outboundCampaignDispatchDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(context.Background(), `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v, want 160014", version, err)
	}
	return pool
}

func campaignDispatchDigest(values ...string) eer.Digest {
	sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
