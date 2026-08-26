package outbound_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactfixture "github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	eerstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	identityfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store/acceptancefixture"
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

type campaignDispatchUnknownAdapter struct{}

func (campaignDispatchUnknownAdapter) Execute(context.Context, eer.EffectEnvelope, eer.Attempt) (eer.AdapterResult, error) {
	return eer.AdapterResult{}, errors.New("fake transport outcome unknown")
}

type campaignDispatchWeComProviderFake struct{ requests []outboundapp.SendRequest }

func (fake *campaignDispatchWeComProviderFake) Send(_ context.Context, request outboundapp.SendRequest) (outboundapp.ProviderResult, error) {
	fake.requests = append(fake.requests, request)
	return outboundapp.ProviderResult{MessageID: "fake-provider-message-id", BusinessCallDispatched: true, RealExternalCallExecuted: true}, nil
}

func TestCampaignDispatchPG16FakeReceiptUnknownAndManualReconcile(t *testing.T) {
	pool := openCampaignDispatchPool(t)
	ctx := context.Background()
	var migrationsApplied bool
	if err := pool.QueryRow(ctx, `SELECT count(*)=4 FROM public.goose_db_version WHERE version_id IN (78,92,94,99) AND is_applied`).Scan(&migrationsApplied); err != nil || !migrationsApplied {
		t.Fatalf("migrations 78/92/94/99 applied=%t err=%v, want true", migrationsApplied, err)
	}
	ensureOutboundRiverCatalog(t, ctx, pool)
	policyTime := time.Now().UTC().Truncate(time.Microsecond)
	contactFacts, err := contactfixture.CreateCampaignDispatchFacts(ctx, pool, "dispatch-owner", policyTime)
	if err != nil {
		t.Fatal(err)
	}
	if err = identityfixture.CreateCampaignDispatchVerifiedExternalUserID(ctx, pool, contactFacts.EligibleCustomerID, "dispatch-corp", "dispatch-external"); err != nil {
		t.Fatal(err)
	}
	targets, err := contactstore.NewWeComOutboundTargetResolver(pool, "dispatch-corp")
	if err != nil {
		t.Fatal(err)
	}
	sender, externalUserID, resolved, err := targets.Resolve(ctx, contactFacts.EligibleCustomerID)
	if err != nil || !resolved || sender != "dispatch-owner" || externalUserID != "dispatch-external" {
		t.Fatalf("outbound target sender=%q external=%q resolved=%t err=%v", sender, externalUserID, resolved, err)
	}

	planID := outboundCampaignHandoffPlanID('d')
	source := &approvedCampaignHandoffSource{snapshot: outboundport.ApprovedCampaignHandoffSnapshot{
		CampaignCode: "c01-dispatch", PlanID: planID, ReviewVersion: 3,
		SourceDigest: strings.Repeat("11", 32), TargetDigest: strings.Repeat("22", 32), ContentDigest: strings.Repeat("33", 32),
		CustomerIDs: []int64{contactFacts.SuppressedCustomerID, contactFacts.EligibleCustomerID, contactFacts.UnresolvedCustomerID}, Steps: []outbound.CampaignHandoffStep{{Index: 1, Content: "approved immutable content"}},
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
	service, err := outboundapp.NewCampaignDispatchService(uow, repository, runtime, repository, contactstore.NewContactPolicyRepository())
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.Dispatch(ctx, outboundapp.CampaignDispatchCommand{
		CampaignCode: source.snapshot.CampaignCode, PlanID: planID, ActorID: 71, IdempotencyKey: "c01-dispatch-operator-command", ExternalGate: true,
	})
	if err != nil || summary.Blocked != 1 || summary.Queued != 2 || summary.DeliveryProven || summary.RealExternalCallExecuted {
		t.Fatalf("queued summary=%+v err=%v", summary, err)
	}
	var blocked int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.outbound_campaign_dispatches WHERE customer_id=$1 AND state='blocked' AND block_reason='contact_policy' AND external_effect_id IS NULL`, contactFacts.SuppressedCustomerID).Scan(&blocked); err != nil || blocked != 1 {
		t.Fatalf("contact-policy blocked bindings=%d err=%v, want 1", blocked, err)
	}
	if err = contactfixture.DeleteCampaignDispatchPolicy(ctx, pool, contactFacts.SuppressedCustomerID); err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Dispatch(ctx, outboundapp.CampaignDispatchCommand{
		CampaignCode: source.snapshot.CampaignCode, PlanID: planID, ActorID: 71, IdempotencyKey: "c01-dispatch-operator-command", ExternalGate: true,
	})
	if err != nil || replayed.Blocked != 1 || replayed.Queued != 2 {
		t.Fatalf("replayed summary=%+v err=%v", replayed, err)
	}

	var effectIDs []string
	if rows, queryErr := pool.Query(ctx, `SELECT 'eer_' || external_effect_id::text FROM public.outbound_campaign_dispatches WHERE external_effect_id IS NOT NULL ORDER BY customer_id`); queryErr != nil {
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

	provider := &campaignDispatchWeComProviderFake{}
	adapter, err := outboundworker.NewCampaignWeComAdapter(repository, provider)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := outboundworker.NewCampaignDispatchWorker(service, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, &river.Job[outboundstore.CampaignDispatchArgs]{
		JobRow: &rivertype.JobRow{ID: 901, Attempt: 1, MaxAttempts: 1, State: rivertype.JobStateRunning}, Args: outboundstore.CampaignDispatchArgs{EffectID: effectIDs[0]},
	}); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 || provider.requests[0].CustomerID != contactFacts.EligibleCustomerID || provider.requests[0].TemplateKey != outboundapp.TemplateTextNoticeV1 {
		t.Fatalf("controlled WeCom provider requests=%+v", provider.requests)
	}
	var exactReceiptDigest string
	if err = pool.QueryRow(ctx, `SELECT provider_receipt_digest FROM public.outbound_campaign_provider_attempt_receipts WHERE external_effect_id=substring($1 from 5)::bigint AND attempt_number=1 AND completion='executed'`, effectIDs[0]).Scan(&exactReceiptDigest); err != nil {
		t.Fatal(err)
	}
	exactReceipt := outboundport.CampaignDispatchProviderAttemptReceipt{Completion: string(eer.CompletionExecuted), ReceiptDigest: eer.Digest(exactReceiptDigest), BusinessCallDispatched: true, RealExternalCallExecuted: true, ProviderMessageID: "fake-provider-message-id", ProviderResultReceived: true}
	if err = repository.RecordCampaignProviderAttemptReceipt(ctx, effectIDs[0], 1, exactReceipt); err != nil {
		t.Fatalf("exact provider receipt replay: %v", err)
	}
	conflictingReceipt := exactReceipt
	conflictingReceipt.ProviderMessageID = "conflicting-provider-message-id"
	if err = repository.RecordCampaignProviderAttemptReceipt(ctx, effectIDs[0], 1, conflictingReceipt); !errors.Is(err, outbound.ErrCampaignDispatchConflict) {
		t.Fatalf("conflicting provider receipt error=%v, want conflict", err)
	}
	var providerPayload map[string]string
	if err = json.Unmarshal(provider.requests[0].Payload, &providerPayload); err != nil || len(providerPayload) != 1 || providerPayload["text"] != "approved immutable content" {
		t.Fatalf("controlled WeCom payload=%s err=%v", provider.requests[0].Payload, err)
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
	if err != nil || summary.Blocked != 1 || summary.Executed != 1 || summary.Reconciled != 1 || summary.OutcomeUnknown != 0 || summary.DeliveryProven || !summary.BusinessCallDispatched || !summary.RealExternalCallExecuted {
		t.Fatalf("terminal summary=%+v err=%v", summary, err)
	}
	var receiptCount int
	var businessCallDispatched, realExternalCallExecuted, resultReceived, messageIDStored, proven bool
	if err = pool.QueryRow(ctx, `SELECT count(*), bool_or(business_call_dispatched), bool_or(real_external_call_executed), bool_or(provider_result_received), bool_or(provider_message_id='fake-provider-message-id'), bool_or(delivery_proven) FROM public.outbound_campaign_provider_attempt_receipts`).Scan(&receiptCount, &businessCallDispatched, &realExternalCallExecuted, &resultReceived, &messageIDStored, &proven); err != nil || receiptCount != 2 || !businessCallDispatched || !realExternalCallExecuted || !resultReceived || !messageIDStored || proven {
		t.Fatalf("provider attempt receipts=%d business=%t real=%t received=%t msgid=%t proven=%t err=%v, want 2/true/true/true/true/false", receiptCount, businessCallDispatched, realExternalCallExecuted, resultReceived, messageIDStored, proven, err)
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
