package campaign_acceptance

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignapp "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/app"
	campaignstore "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestCampaignTouchPlanReviewHandoffPostgreSQLFreshReplayScopeAndImmutability(t *testing.T) {
	pool, ctx := openCampaignPool(t)
	if campaignMigrationWaterline(t, ctx, pool) < 67 {
		t.Fatal("campaign review handoff migration is not applied")
	}
	clearCampaignFacts(t, ctx, pool)
	t.Cleanup(func() { clearCampaignFacts(t, ctx, pool) })

	prefix := fmt.Sprintf("review-store-%d", time.Now().UnixNano())
	planID := initiationPlanID('a')
	if err := seedCompletedInitiationPlan(ctx, pool, prefix, planID, 1, 1, "1a"); err != nil {
		t.Fatal(err)
	}
	service := reviewHandoffService(t, pool)

	reserved, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reserved.Exec(ctx, `INSERT INTO public.cloud_campaign_touch_plan_review_receipts (
  actor_id, operation, key_digest, payload_digest, plan_id, campaign_code, created_at
) VALUES (
  70, 'submit', decode(repeat('aa', 32), 'hex'), decode(repeat('bb', 32), 'hex'), $1, $2, now()
)`, planID, prefix); err != nil {
		reserved.Rollback(ctx)
		t.Fatal(err)
	}
	assertCampaignInitiationSQLState(t, reserved.Commit(ctx), "23514")
	assertReviewReceiptCount(t, ctx, pool, prefix, planID, 0)

	wrongCode := prefix + "-other"
	wrongExistingCode := prefix + "-other-existing"
	seedCampaignFact(t, ctx, pool, wrongExistingCode)
	for _, campaignCode := range []string{wrongCode, wrongExistingCode} {
		_, err = service.Submit(ctx, campaign.SubmitTouchPlanReviewCommand{
			CampaignCode: campaignCode, PlanID: planID, ExpectedVersion: 1, Actor: campaign.Actor{ID: 71}, IdempotencyKey: "review-scope-wrong-key-" + campaignCode,
		})
		if !errors.Is(err, campaign.ErrNotFound) {
			t.Fatalf("cross-campaign submit code=%q error=%v, want not found", campaignCode, err)
		}
		assertReviewReceiptCount(t, ctx, pool, campaignCode, planID, 0)
	}

	submit := campaign.SubmitTouchPlanReviewCommand{
		CampaignCode: prefix, PlanID: planID, ExpectedVersion: 1, Actor: campaign.Actor{ID: 71}, IdempotencyKey: "review-submit-fresh-key",
	}
	submitted, err := service.Submit(ctx, submit)
	if err != nil || submitted.Status != campaign.TouchPlanReviewPending || submitted.Version != 2 {
		t.Fatalf("fresh submit=%#v err=%v", submitted, err)
	}
	assertReviewReceiptCount(t, ctx, pool, prefix, planID, 1)
	assertReviewReceiptSnapshots(t, ctx, pool, planID, 1)
	assertTamperedReviewSnapshotRejected(t, ctx, pool, prefix, planID)

	approved, err := service.Approve(ctx, campaign.DecideTouchPlanReviewCommand{
		CampaignCode: prefix, PlanID: planID, ExpectedVersion: 2, Actor: campaign.Actor{ID: 72}, IdempotencyKey: "review-approve-fresh-key", Confirmation: campaign.ReviewConfirmation("approve", planID),
	})
	if err != nil || approved.Review.Status != campaign.TouchPlanReviewApproved || approved.Review.Version != 3 || approved.Handoff == nil || approved.Handoff.Status != campaign.HandoffPendingOutboundAccept {
		t.Fatalf("fresh approve=%#v err=%v", approved, err)
	}
	assertReviewReceiptCount(t, ctx, pool, prefix, planID, 2)
	assertReviewReceiptSnapshots(t, ctx, pool, planID, 2)

	replayed, err := service.Submit(ctx, submit)
	if err != nil || !reflect.DeepEqual(replayed, submitted) {
		t.Fatalf("submit replay=%#v err=%v, want original=%#v", replayed, err, submitted)
	}
	assertReviewReceiptCount(t, ctx, pool, prefix, planID, 2)

	for _, statement := range []string{
		`UPDATE public.cloud_campaign_touch_plan_reviews SET version = version + 1 WHERE plan_id = $1`,
		`DELETE FROM public.cloud_campaign_touch_plan_reviews WHERE plan_id = $1`,
		`UPDATE public.cloud_campaign_touch_plan_handoffs SET status = 'pending_outbound_acceptance' WHERE plan_id = $1`,
		`DELETE FROM public.cloud_campaign_touch_plan_handoffs WHERE plan_id = $1`,
	} {
		_, err = pool.Exec(ctx, statement, planID)
		assertCampaignInitiationSQLState(t, err, "55000")
	}

	for _, campaignCode := range []string{wrongCode, wrongExistingCode} {
		if _, err = service.GetReview(ctx, campaignCode, planID); !errors.Is(err, campaign.ErrNotFound) {
			t.Fatalf("cross-campaign review code=%q error=%v, want not found", campaignCode, err)
		}
	}
	assertStandaloneReviewTransitionsRejected(t, ctx, pool)
}

func TestCampaignTouchPlanReviewHandoffFactsBlockRollback(t *testing.T) {
	pool, ctx := openCampaignPool(t)
	repoRoot := campaignRepoRoot(t)
	restoreWaterline := campaignMigrationWaterlineAtLeast(t, ctx, pool, 67)
	clearCampaignFacts(t, ctx, pool)
	t.Cleanup(func() {
		clearCampaignFacts(t, ctx, pool)
		runCampaignGoose(t, ctx, repoRoot, "up-to", restoreWaterline)
	})

	planID := initiationPlanID('b')
	if err := seedCompletedInitiationPlan(ctx, pool, "review-down-guard", planID, 1, 1, "1b"); err != nil {
		t.Fatal(err)
	}
	if _, err := reviewHandoffService(t, pool).Submit(ctx, campaign.SubmitTouchPlanReviewCommand{
		CampaignCode: "review-down-guard", PlanID: planID, ExpectedVersion: 1, Actor: campaign.Actor{ID: 73}, IdempotencyKey: "review-down-guard-key",
	}); err != nil {
		t.Fatal(err)
	}
	assertCampaignRollbackError(t, campaignGoose(ctx, repoRoot, *campaignMigrationDatabaseURL, "down-to", "66"))
	clearCampaignFacts(t, ctx, pool)
	runCampaignGoose(t, ctx, repoRoot, "down-to", "66")
	assertCampaignReviewHandoffTablesAbsent(t, ctx, pool)
	runCampaignGoose(t, ctx, repoRoot, "up-to", "67")
}

func reviewHandoffService(t *testing.T, pool *pgxpool.Pool) *campaignapp.ReviewHandoffService {
	t.Helper()
	events, err := campaignstore.NewReviewHandoffEventLogAdapter(eventstore.NewAppender())
	if err != nil {
		t.Fatal(err)
	}
	service, err := campaignapp.NewReviewHandoffService(platformstore.NewUnitOfWork(pool), campaignstore.NewRepository(), events)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func assertReviewReceiptCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, campaignCode, planID string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.cloud_campaign_touch_plan_review_receipts WHERE campaign_code = $1 AND plan_id = $2`, campaignCode, planID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("review receipt count=%d, want %d", got, want)
	}
}

func assertReviewReceiptSnapshots(t *testing.T, ctx context.Context, pool *pgxpool.Pool, planID string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.cloud_campaign_touch_plan_review_receipts WHERE plan_id = $1 AND state = 'completed' AND event_id IS NOT NULL AND result_snapshot IS NOT NULL`, planID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("completed durable review receipt count=%d, want %d", got, want)
	}
}

func assertTamperedReviewSnapshotRejected(t *testing.T, ctx context.Context, pool *pgxpool.Pool, campaignCode, planID string) {
	t.Helper()
	for _, test := range []struct {
		name, keyHex, version, extra string
	}{
		{name: "wrong numeric version", keyHex: "cc", version: "999"},
		{name: "string version", keyHex: "dd", version: "to_jsonb('2'::text)"},
		{name: "extra key", keyHex: "ee", version: "2", extra: ", 'unexpected', true"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var eventID, receiptID int64
			if err = tx.QueryRow(ctx, `INSERT INTO public.event_log (event_type, payload, occurred_at, idempotency_key)
VALUES ('cloud_campaign.fact_recorded', jsonb_build_object(
  'audit_type', 'touch_plan_submitted', 'plan_id', $1::text, 'campaign_code', $2::text, 'review_version', 2, 'actor_id', 71
), now(), 'review-tampered-snapshot-event:' || $1::text || ':' || $3::text)
RETURNING id`, planID, campaignCode, test.name).Scan(&eventID); err != nil {
				t.Fatal(err)
			}
			if err = tx.QueryRow(ctx, `INSERT INTO public.cloud_campaign_touch_plan_review_receipts (
  actor_id, operation, key_digest, payload_digest, plan_id, campaign_code, created_at
) VALUES (
  71, 'submit', decode(repeat($3::text, 32), 'hex'), decode(repeat('ff', 32), 'hex'), $1, $2, now()
) RETURNING id`, planID, campaignCode, test.keyHex).Scan(&receiptID); err != nil {
				t.Fatal(err)
			}
			statement := fmt.Sprintf(`UPDATE public.cloud_campaign_touch_plan_review_receipts AS receipt
SET state = 'completed', event_id = $2, completed_at = now(),
    result_snapshot = %s
FROM public.cloud_campaign_touch_plan_reviews AS review
WHERE receipt.id = $1 AND review.plan_id = receipt.plan_id`, "jsonb_build_object(\n      'review', jsonb_build_object(\n        'plan_id', review.plan_id, 'campaign_code', review.campaign_code,\n        'status', review.status, 'version', "+test.version+",\n        'submitted_by_actor_id', review.submitted_by_actor_id,\n        'submitted_at_unix_micro', floor(extract(epoch FROM review.submitted_at) * 1000000)::bigint,\n        'reviewed_by_actor_id', 0, 'reviewed_at_unix_micro', 0, 'confirmation_digest', ''"+test.extra+"\n      ),\n      'handoff', 'null'::jsonb, 'event_ids', jsonb_build_array($2::bigint)\n    )")
			if _, err = tx.Exec(ctx, statement, receiptID, eventID); err != nil {
				t.Fatal(err)
			}
			assertCampaignInitiationSQLState(t, tx.Commit(ctx), "23514")
			assertReviewReceiptCount(t, ctx, pool, campaignCode, planID, 1)
		})
	}
}

func assertStandaloneReviewTransitionsRejected(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, transition := range []struct {
		name, campaignCode, planID, receiptByte, terminal string
	}{
		{name: "submit", campaignCode: "review-standalone-submit", planID: initiationPlanID('c'), receiptByte: "1c"},
		{name: "reject", campaignCode: "review-standalone-reject", planID: initiationPlanID('d'), receiptByte: "1d", terminal: "rejected"},
		{name: "approve", campaignCode: "review-standalone-approve", planID: initiationPlanID('e'), receiptByte: "1e", terminal: "approved"},
	} {
		transition := transition
		t.Run(transition.name, func(t *testing.T) {
			if err := seedCompletedInitiationPlan(ctx, pool, transition.campaignCode, transition.planID, 1, 1, transition.receiptByte); err != nil {
				t.Fatal(err)
			}
			if transition.terminal != "" {
				if _, err := reviewHandoffService(t, pool).Submit(ctx, campaign.SubmitTouchPlanReviewCommand{
					CampaignCode: transition.campaignCode, PlanID: transition.planID, ExpectedVersion: 1, Actor: campaign.Actor{ID: 81}, IdempotencyKey: "review-standalone-submit-key-" + transition.name,
				}); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if transition.terminal == "" {
				if _, err = tx.Exec(ctx, `UPDATE public.cloud_campaign_touch_plan_reviews
SET status = 'pending_review', version = 2, submitted_by_actor_id = 81, submitted_at = now()
WHERE plan_id = $1`, transition.planID); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err = tx.Exec(ctx, `UPDATE public.cloud_campaign_touch_plan_reviews
SET status = $2, version = 3, reviewed_by_actor_id = 82, reviewed_at = now(), confirmation_digest = decode(repeat('ee', 32), 'hex')
WHERE plan_id = $1`, transition.planID, transition.terminal); err != nil {
					t.Fatal(err)
				}
			}
			assertCampaignInitiationSQLState(t, tx.Commit(ctx), "23514")
			var status string
			if err = pool.QueryRow(ctx, `SELECT status FROM public.cloud_campaign_touch_plan_reviews WHERE plan_id = $1`, transition.planID).Scan(&status); err != nil {
				t.Fatal(err)
			}
			wantStatus := "draft"
			if transition.terminal != "" {
				wantStatus = "pending_review"
			}
			if status != wantStatus {
				t.Fatalf("standalone %s left status=%q, want %q", transition.name, status, wantStatus)
			}
		})
	}
}

func assertCampaignReviewHandoffTablesAbsent(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, table := range []string{"cloud_campaign_touch_plan_reviews", "cloud_campaign_touch_plan_review_receipts", "cloud_campaign_touch_plan_handoffs"} {
		var relation *string
		if err := pool.QueryRow(ctx, `SELECT to_regclass('public.' || $1)::text`, table).Scan(&relation); err != nil {
			t.Fatal(err)
		}
		if relation != nil {
			t.Fatalf("%s remains after rollback: %q", table, *relation)
		}
	}
}
