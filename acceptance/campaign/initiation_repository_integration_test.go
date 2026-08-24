package campaign_acceptance

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactfixture "github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	campaignapp "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/app"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	campaignstore "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

type allEligibleInitiationChecker struct{}

func (allEligibleInitiationChecker) CheckCampaignEligibility(_ context.Context, request campaignport.EligibilityRequest) ([]campaignport.EligibilityDecision, error) {
	if request.Checkpoint != campaignport.EligibilityCheckpointPreview || request.MaximumTargets != campaignport.MaximumEligibilityTargets ||
		len(request.CustomerIDs) < 1 || len(request.CustomerIDs) > campaignport.MaximumEligibilityTargets {
		return nil, fmt.Errorf("unexpected eligibility request: %#v", request)
	}
	decisions := make([]campaignport.EligibilityDecision, len(request.CustomerIDs))
	for index, customerID := range request.CustomerIDs {
		if customerID < 1 || index > 0 && request.CustomerIDs[index-1] >= customerID {
			return nil, fmt.Errorf("non-canonical eligibility candidates: %v", request.CustomerIDs)
		}
		decisions[index] = campaignport.EligibilityDecision{
			CustomerID: customerID, CustomerActive: true, Eligible: true, Exclusion: campaignport.EligibilityExclusionNone,
		}
	}
	return decisions, nil
}

func TestCampaignInitiationRepositoryPostgreSQLStrictReadbackAndReplay(t *testing.T) {
	pool, ctx := openCampaignPool(t)
	if campaignMigrationWaterline(t, ctx, pool) < 66 {
		t.Fatal("campaign initiation migration is not applied")
	}
	clearCampaignFacts(t, ctx, pool)

	prefix := fmt.Sprintf("initiation-store-%d", time.Now().UnixNano())
	customerIDs := seedInitiationCustomers(t, ctx, pool, prefix, 2)
	t.Cleanup(func() {
		clearCampaignFacts(t, ctx, pool)
		deleteInitiationCustomers(t, ctx, pool, customerIDs)
	})
	code := prefix
	if _, err := pool.Exec(ctx, `INSERT INTO public.cloud_campaigns (campaign_code,name,approval_status,runtime_status,version,created_by,updated_by,created_at,updated_at)
VALUES ($1,'initiation store','draft','idle',1,1,1,now(),now())`, code); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO public.cloud_campaign_steps (campaign_code,step_index,delay_minutes,content)
VALUES ($1,1,0,'immutable campaign content')`, code); err != nil {
		t.Fatal(err)
	}

	audits, err := campaignstore.NewInitiationEventLogAdapter(eventstore.NewAppender())
	if err != nil {
		t.Fatal(err)
	}
	repository := campaignstore.NewRepository()
	service, err := campaignapp.NewService(
		platformstore.NewUnitOfWork(pool), repository, nil, allEligibleInitiationChecker{}, repository, audits,
	)
	if err != nil {
		t.Fatal(err)
	}
	command := campaign.CreateDraftTouchPlanCommand{
		CampaignCode: code, ExpectedCampaignVersion: 1,
		Source: campaign.InitiationSourceRequest{Kind: campaign.InitiationSourceCustomerSelection, CustomerIDs: []int64{customerIDs[1], customerIDs[0]}},
		Owner:  campaign.Actor{ID: 1}, IdempotencyKey: prefix + "-idempotency-key-0000000000000001",
	}
	created, err := service.CreateDraftTouchPlan(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if !campaign.ValidDraftTouchPlan(created) || created.Safety != campaign.LocalInitiationSafety() || len(created.Targets.CustomerIDs) != 2 {
		t.Fatalf("created=%#v", created)
	}
	replayed, err := service.CreateDraftTouchPlan(ctx, command)
	if err != nil || replayed.ID != created.ID || replayed.Targets.Digest != created.Targets.Digest {
		t.Fatalf("replay=%#v err=%v", replayed, err)
	}
	page, err := service.ListDraftTouchPlans(ctx, code, "", 1)
	if err != nil || len(page.Items) != 1 || page.NextCursor != "" || page.Items[0].ID != created.ID {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	readback, err := service.GetDraftTouchPlan(ctx, code, created.ID)
	if err != nil || readback.Content.Digest != created.Content.Digest || readback.Targets.Digest != created.Targets.Digest {
		t.Fatalf("readback=%#v err=%v", readback, err)
	}

	var plans, targets, steps, receipts, events, deliveries int
	if err = pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM public.cloud_campaign_touch_plans WHERE id=$1),
  (SELECT count(*) FROM public.cloud_campaign_touch_plan_targets WHERE plan_id=$1),
  (SELECT count(*) FROM public.cloud_campaign_touch_plan_steps WHERE plan_id=$1),
  (SELECT count(*) FROM public.cloud_campaign_touch_plan_receipts WHERE plan_id=$1 AND state='completed'),
  (SELECT count(*) FROM public.event_log WHERE event_type='cloud_campaign.fact_recorded' AND payload ->> 'audit_type'='touch_plan_created' AND payload ->> 'plan_id'=$1),
  (SELECT count(*) FROM public.event_deliveries AS delivery JOIN public.event_log AS event ON event.id=delivery.event_id WHERE event.payload ->> 'plan_id'=$1)`, created.ID).Scan(&plans, &targets, &steps, &receipts, &events, &deliveries); err != nil {
		t.Fatal(err)
	}
	// The business UoW records one bound Campaign Fact. It does not dispatch it
	// itself; the existing Events infrastructure later creates and completes the
	// local audit delivery, without an Outbound or Provider task.
	if plans != 1 || targets != 2 || steps != 1 || receipts != 1 || events != 1 || deliveries != 0 {
		t.Fatalf("persisted plans/targets/steps/receipts/events/deliveries=%d/%d/%d/%d/%d/%d", plans, targets, steps, receipts, events, deliveries)
	}
}

func TestSegmentTouchPlanSnapshotSerializesRefreshWriter(t *testing.T) {
	pool, ctx := openCampaignPool(t)
	if campaignMigrationWaterline(t, ctx, pool) < 66 {
		t.Fatal("campaign initiation migration is not applied")
	}
	prefix := fmt.Sprintf("initiation-segment-%d", time.Now().UnixNano())
	customerIDs := seedInitiationCustomers(t, ctx, pool, prefix, 2)
	refreshedAt := time.Now().UTC().Truncate(time.Microsecond)
	var segmentID int64
	if err := pool.QueryRow(ctx, `INSERT INTO public.segments (name,definition,refresh_mode,member_count,refreshed_at,refresh_status,lifecycle_status,created_at,updated_at)
VALUES ($1,'{}','manual',2,$2,'idle','active',$2,$2)
RETURNING id`, prefix, refreshedAt).Scan(&segmentID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM public.segments WHERE id=$1`, segmentID); err != nil {
			t.Fatal(err)
		}
		deleteInitiationCustomers(t, ctx, pool, customerIDs)
	})
	for _, customerID := range customerIDs {
		if _, err := pool.Exec(ctx, `INSERT INTO public.segment_members (segment_id,customer_id,computed_at) VALUES ($1,$2,$3)`, segmentID, customerID, refreshedAt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO public.ai_audience_package_metadata (segment_id,lifecycle,version,created_by,updated_by,created_at,updated_at)
VALUES ($1,'active',7,1,1,$2,$2)`, segmentID, refreshedAt); err != nil {
		t.Fatal(err)
	}

	uow := platformstore.NewUnitOfWork(pool)
	reader := segmentstore.NewTouchPlanSnapshotRepository()
	if err := uow.Within(ctx, func(tx context.Context) error {
		segment, err := reader.ReadSegmentTouchPlanSnapshot(tx, segmentport.SegmentID(segmentID))
		if err != nil || !segment.Valid() || len(segment.CustomerIDs) != len(customerIDs) {
			return fmt.Errorf("segment snapshot=%#v: %w", segment, err)
		}
		audience, err := reader.ReadAudiencePackageTouchPlanSnapshot(tx, segmentport.SegmentID(segmentID))
		if err != nil || !audience.Valid() || audience.PackageVersion != 7 || audience.Digest == segment.Digest {
			return fmt.Errorf("audience snapshot=%#v: %w", audience, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	holderPID := make(chan int32, 1)
	holderReady := make(chan struct{})
	releaseHolder := make(chan struct{})
	holderReleased := false
	defer func() {
		if !holderReleased {
			close(releaseHolder)
		}
	}()
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- uow.Within(ctx, func(tx context.Context) error {
			snapshot, err := reader.ReadSegmentTouchPlanSnapshot(tx, segmentport.SegmentID(segmentID))
			if err != nil || !snapshot.Valid() {
				return fmt.Errorf("held snapshot=%#v: %w", snapshot, err)
			}
			db, err := platformstore.TxFromContext(tx)
			if err != nil {
				return err
			}
			var pid int32
			if err = db.QueryRow(tx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			holderPID <- pid
			close(holderReady)
			<-releaseHolder
			return nil
		})
	}()
	select {
	case <-holderReady:
	case <-time.After(5 * time.Second):
		t.Fatal("Segment snapshot did not acquire its share lock")
	}

	writerPID := make(chan int32, 1)
	writerDone := make(chan error, 1)
	refresh := segmentstore.NewRefreshRepository()
	go func() {
		writerDone <- uow.Within(ctx, func(tx context.Context) error {
			db, err := platformstore.TxFromContext(tx)
			if err != nil {
				return err
			}
			var pid int32
			if err = db.QueryRow(tx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			writerPID <- pid
			_, err = refresh.LockDefinition(tx, segmentport.SegmentID(segmentID))
			return err
		})
	}()
	holder := <-holderPID
	writer := <-writerPID
	waitForCampaignInitiationLock(t, ctx, pool, holder, writer)
	select {
	case err := <-writerDone:
		t.Fatalf("refresh writer bypassed Segment snapshot lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseHolder)
	holderReleased = true
	if err := <-holderDone; err != nil {
		t.Fatal(err)
	}
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
}

func seedInitiationCustomers(t *testing.T, ctx context.Context, pool *pgxpool.Pool, _ string, count int) []int64 {
	t.Helper()
	result := make([]int64, 0, count)
	for range count {
		customerID, err := contactfixture.CreateCustomerRecord(ctx, pool)
		if err != nil {
			for _, createdID := range result {
				_ = contactfixture.DeleteCustomer(context.Background(), pool, createdID)
			}
			t.Fatal(err)
		}
		result = append(result, customerID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func deleteInitiationCustomers(t *testing.T, ctx context.Context, pool *pgxpool.Pool, customerIDs []int64) {
	t.Helper()
	for _, customerID := range customerIDs {
		if err := contactfixture.DeleteCustomer(ctx, pool, customerID); err != nil {
			t.Fatal(err)
		}
	}
}

func waitForCampaignInitiationLock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, holderPID, writerPID int32) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := pool.QueryRow(ctx, `SELECT $2 = ANY(pg_blocking_pids($1))`, writerPID, holderPID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("refresh writer did not block on Segment snapshot lock")
		case <-ticker.C:
		}
	}
}
