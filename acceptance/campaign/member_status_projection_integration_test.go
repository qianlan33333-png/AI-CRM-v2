package campaign_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignstore "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var errRollbackCampaignMemberProjection = errors.New("rollback campaign member projection fixture")

func TestCampaignMemberStatusProjectionUsesNewestPlanAndLocalReviewFilter(t *testing.T) {
	pool, ctx := openCampaignPool(t)
	if campaignMigrationWaterline(t, ctx, pool) < 105 {
		t.Fatal("campaign recipient review migration is not applied")
	}
	code := fmt.Sprintf("member-projection-%d", time.Now().UnixNano())
	oldPlanID := campaignMemberProjectionPlanID(code + "-old")
	latestPlanID := campaignMemberProjectionPlanID(code + "-latest")
	repository := campaignstore.NewRepository()
	uow := platformstore.NewUnitOfWork(pool)

	err := uow.Within(ctx, func(txContext context.Context) error {
		tx, txErr := platformstore.TxFromContext(txContext)
		if txErr != nil {
			return txErr
		}
		createdAt := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
		if _, txErr = tx.Exec(txContext, `INSERT INTO public.cloud_campaigns (campaign_code,name,approval_status,runtime_status,version,created_by,updated_by,created_at,updated_at)
VALUES ($1,'member projection','draft','idle',1,1,1,$2,$2)`, code, createdAt); txErr != nil {
			return txErr
		}
		if _, txErr = tx.Exec(txContext, `INSERT INTO public.cloud_campaigns (campaign_code,name,approval_status,runtime_status,version,created_by,updated_by,created_at,updated_at)
VALUES ($1,'empty member projection','draft','idle',1,1,1,$2,$2)`, code+"-empty", createdAt); txErr != nil {
			return txErr
		}
		if txErr = seedMemberProjectionPlan(txContext, tx, code, oldPlanID, createdAt, []int64{1, 2}); txErr != nil {
			return txErr
		}
		if txErr = seedMemberProjectionPlan(txContext, tx, code, latestPlanID, createdAt.Add(time.Hour), []int64{11, 12, 13}); txErr != nil {
			return txErr
		}
		if _, txErr = tx.Exec(txContext, `UPDATE public.cloud_campaign_touch_plan_reviews
SET status='pending_review', version=2, submitted_by_actor_id=1, submitted_at=$2
WHERE plan_id=$1`, latestPlanID, createdAt.Add(2*time.Hour)); txErr != nil {
			return txErr
		}
		if _, txErr = tx.Exec(txContext, `INSERT INTO public.cloud_campaign_touch_plan_recipient_reviews
  (plan_id,customer_id,campaign_code,status,version,updated_by_actor_id,updated_at)
VALUES ($1,12,$2,'approved',1,1,$3), ($1,13,$2,'rejected',1,1,$3)`, latestPlanID, code, createdAt.Add(3*time.Hour)); txErr != nil {
			return txErr
		}

		all, readErr := repository.ListLatestCampaignMemberStatuses(txContext, code, "", 2, 1)
		if readErr != nil {
			return readErr
		}
		if all.PlanID != latestPlanID || all.Total != 3 || len(all.Items) != 2 || all.Items[0].CustomerID != 12 || all.Items[0].Status != campaign.TouchPlanRecipientReviewApproved || all.Items[1].CustomerID != 13 || all.Items[1].Status != campaign.TouchPlanRecipientReviewRejected {
			return fmt.Errorf("all projection=%+v", all)
		}
		pending, readErr := repository.ListLatestCampaignMemberStatuses(txContext, code, campaign.TouchPlanRecipientReviewPending, 100, 0)
		if readErr != nil {
			return readErr
		}
		if pending.PlanID != latestPlanID || pending.Total != 1 || len(pending.Items) != 1 || pending.Items[0].CustomerID != 11 || pending.Items[0].Status != campaign.TouchPlanRecipientReviewPending {
			return fmt.Errorf("pending projection=%+v", pending)
		}
		empty, readErr := repository.ListLatestCampaignMemberStatuses(txContext, code+"-empty", "", 100, 0)
		if readErr != nil || empty.PlanID != "" || empty.Total != 0 || len(empty.Items) != 0 {
			return fmt.Errorf("empty projection=%+v err=%v", empty, readErr)
		}
		if _, readErr = repository.ListLatestCampaignMemberStatuses(txContext, code+"-missing", "", 100, 0); !errors.Is(readErr, campaign.ErrNotFound) {
			return fmt.Errorf("missing campaign error=%v", readErr)
		}
		var providerEligible, runtimeExecuted, realExternalCall, deliveryProven bool
		if txErr = tx.QueryRow(txContext, `SELECT provider_execution_eligible,runtime_executed,real_external_call_executed,delivery_proven
FROM public.cloud_campaign_touch_plans WHERE id=$1`, latestPlanID).Scan(&providerEligible, &runtimeExecuted, &realExternalCall, &deliveryProven); txErr != nil {
			return txErr
		}
		if providerEligible || runtimeExecuted || realExternalCall || deliveryProven {
			return fmt.Errorf("unsafe plan flags provider=%t runtime=%t external=%t delivery=%t", providerEligible, runtimeExecuted, realExternalCall, deliveryProven)
		}
		return errRollbackCampaignMemberProjection
	})
	if !errors.Is(err, errRollbackCampaignMemberProjection) {
		t.Fatal(err)
	}
	var campaigns int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.cloud_campaigns WHERE campaign_code=$1`, code).Scan(&campaigns); err != nil || campaigns != 0 {
		t.Fatalf("fixture rollback campaigns=%d err=%v", campaigns, err)
	}
}

func campaignMemberProjectionPlanID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "ctp_" + hex.EncodeToString(digest[:])
}

func seedMemberProjectionPlan(ctx context.Context, tx pgx.Tx, campaignCode, planID string, createdAt time.Time, customerIDs []int64) error {
	if _, err := tx.Exec(ctx, `INSERT INTO public.cloud_campaign_touch_plans (
  id,campaign_code,campaign_version,source_kind,customer_selection_id,customer_selection_version,
  source_digest,target_digest,content_digest,target_count,content_step_count,candidate_count,
  active_customer_count,inactive_excluded_count,policy_excluded_count,owner_actor_id,created_at
) VALUES ($1,$2,1,'customer_selection','local_selection','v1',decode(repeat('01',32),'hex'),
  decode(repeat('02',32),'hex'),decode(repeat('03',32),'hex'),$3,1,$3,$3,0,0,1,$4)`, planID, campaignCode, len(customerIDs), createdAt); err != nil {
		return err
	}
	for _, customerID := range customerIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO public.cloud_campaign_touch_plan_targets (plan_id,customer_id) VALUES ($1,$2)`, planID, customerID); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `INSERT INTO public.cloud_campaign_touch_plan_steps (plan_id,step_index,delay_minutes,content) VALUES ($1,1,0,'local only')`, planID)
	return err
}
