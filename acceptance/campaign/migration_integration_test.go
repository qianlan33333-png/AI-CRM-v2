package campaign_acceptance

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventsfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store/acceptancefixture"
)

var campaignMigrationDatabaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 campaign migration database")

func TestCloudCampaignMigrationDownFailsClosedAndSerializesFacts(t *testing.T) {
	pool, ctx := openCampaignPool(t)
	repoRoot := campaignRepoRoot(t)
	restoreWaterline := campaignMigrationWaterlineAtLeast(t, ctx, pool, 60)
	runCampaignGoose(t, ctx, repoRoot, "down-to", "60")
	ensureCampaignMigrationAt60(t, ctx, pool)
	t.Cleanup(func() {
		clearCampaignFacts(t, ctx, pool)
		runCampaignGoose(t, ctx, repoRoot, "up-to", restoreWaterline)
	})

	t.Run("facts and receipts reject rollback without loss", func(t *testing.T) {
		clearCampaignFacts(t, ctx, pool)
		seedCampaignFact(t, ctx, pool, "campaign-guard")
		assertCampaignDownRejected(t, ctx, repoRoot)
		assertCampaignCounts(t, ctx, pool, 1, 1, 0)

		clearCampaignFacts(t, ctx, pool)
		seedCampaignReceipt(t, ctx, pool, "receipt-guard")
		assertCampaignDownRejected(t, ctx, repoRoot)
		assertCampaignCounts(t, ctx, pool, 0, 0, 1)
	})

	t.Run("empty tables roll back and upgrade", func(t *testing.T) {
		clearCampaignFacts(t, ctx, pool)
		runCampaignGoose(t, ctx, repoRoot, "down-to", "59")
		var campaigns *string
		if err := pool.QueryRow(ctx, `SELECT to_regclass('public.cloud_campaigns')::text`).Scan(&campaigns); err != nil {
			t.Fatal(err)
		}
		if campaigns != nil {
			t.Fatalf("cloud_campaigns remains after empty rollback: %q", *campaigns)
		}
		assertCampaignMigrationVersion(t, ctx, pool, 59)
		runCampaignGoose(t, ctx, repoRoot, "up-to", "60")
		ensureCampaignMigrationAt60(t, ctx, pool)
	})

	for _, fixture := range []struct {
		name                                   string
		seed                                   func(*testing.T, context.Context, *pgxpool.Pool, string)
		lockRelation                           string
		wantCampaigns, wantSteps, wantReceipts int
	}{
		{"campaign writer", seedCampaignFact, "public.cloud_campaigns", 1, 1, 0},
		{"receipt writer", seedCampaignReceipt, "public.cloud_campaign_operation_receipts", 0, 0, 1},
	} {
		fixture := fixture
		t.Run("concurrent "+fixture.name+" commits before rollback guard", func(t *testing.T) {
			clearCampaignFacts(t, ctx, pool)
			conn, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			code := "concurrent-" + strings.ReplaceAll(fixture.name, " ", "-")
			seedCampaignFactInTx(t, ctx, tx, code, fixture.name == "campaign writer")

			downURL := campaignDownURL(t)
			done := make(chan error, 1)
			go func() { done <- campaignGoose(ctx, repoRoot, downURL, "down-to", "59") }()
			waitForCampaignDownLock(t, ctx, pool, fixture.lockRelation)
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				assertCampaignRollbackError(t, err)
			case <-time.After(15 * time.Second):
				t.Fatalf("rollback did not finish after concurrent %s commit", fixture.name)
			}
			assertCampaignCounts(t, ctx, pool, fixture.wantCampaigns, fixture.wantSteps, fixture.wantReceipts)
			assertCampaignMigrationVersion(t, ctx, pool, 60)
		})
	}
}

func TestCampaignInitiationSnapshotMigrationGuards(t *testing.T) {
	pool, ctx := openCampaignPool(t)
	repoRoot := campaignRepoRoot(t)
	restoreWaterline := campaignMigrationWaterlineAtLeast(t, ctx, pool, 66)
	clearCampaignFacts(t, ctx, pool)
	runCampaignGoose(t, ctx, repoRoot, "down-to", "65")
	assertCampaignInitiationTablesAbsent(t, ctx, pool)
	runCampaignGoose(t, ctx, repoRoot, "up-to", "66")
	ensureCampaignInitiationMigrationAt66(t, ctx, pool)
	t.Cleanup(func() {
		clearCampaignFacts(t, ctx, pool)
		runCampaignGoose(t, ctx, repoRoot, "up-to", restoreWaterline)
	})

	t.Run("orphan plan is rejected at commit", func(t *testing.T) {
		clearCampaignFacts(t, ctx, pool)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if err := seedInitiationPlanHeader(ctx, tx, "initiation-orphan", initiationPlanID('a'), 1, 1); err != nil {
			t.Fatal(err)
		}
		assertCampaignInitiationSQLState(t, tx.Commit(ctx), "23514")
	})

	t.Run("declared child count mismatch is rejected at commit", func(t *testing.T) {
		clearCampaignFacts(t, ctx, pool)
		planID := initiationPlanID('b')
		err := seedCompletedInitiationPlan(ctx, pool, "initiation-count-mismatch", planID, 2, 1, "0b")
		assertCampaignInitiationSQLState(t, err, "23514")
	})

	t.Run("completed snapshots reject appended targets and steps", func(t *testing.T) {
		clearCampaignFacts(t, ctx, pool)
		planID := initiationPlanID('c')
		if err := seedCompletedInitiationPlan(ctx, pool, "initiation-complete", planID, 1, 1, "0c"); err != nil {
			t.Fatal(err)
		}
		_, err := pool.Exec(ctx, `INSERT INTO public.cloud_campaign_touch_plan_targets (plan_id, customer_id) VALUES ($1, 2)`, planID)
		assertCampaignInitiationSQLState(t, err, "55000")
		_, err = pool.Exec(ctx, `INSERT INTO public.cloud_campaign_touch_plan_steps (plan_id, step_index, delay_minutes, content) VALUES ($1, 2, 0, 'late')`, planID)
		assertCampaignInitiationSQLState(t, err, "55000")
	})

	for _, child := range []struct {
		name           string
		planID         string
		receiptKeyByte string
		append         func(context.Context, pgx.Tx, string) error
	}{
		{
			name:           "target",
			planID:         initiationPlanID('e'),
			receiptKeyByte: "0e",
			append: func(ctx context.Context, tx pgx.Tx, planID string) error {
				_, err := tx.Exec(ctx, `INSERT INTO public.cloud_campaign_touch_plan_targets (plan_id, customer_id) VALUES ($1, 2)`, planID)
				return err
			},
		},
		{
			name:           "step",
			planID:         initiationPlanID('f'),
			receiptKeyByte: "0f",
			append: func(ctx context.Context, tx pgx.Tx, planID string) error {
				_, err := tx.Exec(ctx, `INSERT INTO public.cloud_campaign_touch_plan_steps (plan_id, step_index, delay_minutes, content) VALUES ($1, 2, 0, 'late')`, planID)
				return err
			},
		},
	} {
		child := child
		t.Run("uncommitted snapshot rejects cross-transaction "+child.name+" append", func(t *testing.T) {
			clearCampaignFacts(t, ctx, pool)
			parentConn, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer parentConn.Release()
			childConn, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer childConn.Release()

			parentTx, err := parentConn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = parentTx.Rollback(ctx) }()
			planID := child.planID
			if err = seedCompletedInitiationPlanInTx(ctx, parentTx, "initiation-child-race-"+child.name, planID, 1, 1, child.receiptKeyByte); err != nil {
				t.Fatal(err)
			}

			childTx, err := childConn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = childTx.Rollback(ctx) }()
			appendDone := make(chan error, 1)
			go func() { appendDone <- child.append(ctx, childTx, planID) }()
			select {
			case err := <-appendDone:
				// PostgreSQL's non-deferrable child FK rejects an uncommitted
				// parent immediately. It does not resume the child statement after
				// the parent commits, so the trigger cannot be bypassed by this race.
				assertCampaignInitiationSQLState(t, err, "23503")
			case <-time.After(5 * time.Second):
				t.Fatalf("concurrent %s append did not reject before parent commit", child.name)
			}
			if err = parentTx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			var childCount int
			childTable := "cloud_campaign_touch_plan_" + child.name + "s"
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.`+childTable+` WHERE plan_id=$1`, planID).Scan(&childCount); err != nil {
				t.Fatal(err)
			}
			if childCount != 1 {
				t.Fatalf("%s rows after concurrent rejected append=%d, want 1", child.name, childCount)
			}
		})
	}

	t.Run("facts prevent rollback then empty tables reapply", func(t *testing.T) {
		clearCampaignFacts(t, ctx, pool)
		if err := seedCompletedInitiationPlan(ctx, pool, "initiation-down-guard", initiationPlanID('d'), 1, 1, "0d"); err != nil {
			t.Fatal(err)
		}
		err := campaignGoose(ctx, repoRoot, *campaignMigrationDatabaseURL, "down-to", "65")
		assertCampaignRollbackError(t, err)
		clearCampaignFacts(t, ctx, pool)
		runCampaignGoose(t, ctx, repoRoot, "down-to", "65")
		assertCampaignInitiationTablesAbsent(t, ctx, pool)
		runCampaignGoose(t, ctx, repoRoot, "up-to", "66")
		ensureCampaignInitiationMigrationAt66(t, ctx, pool)
	})
}

func openCampaignPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *campaignMigrationDatabaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*campaignMigrationDatabaseURL); err != nil {
		if dedicatedErr := acceptancefixtures.ValidateDatabaseURLForDatabase(*campaignMigrationDatabaseURL, acceptancefixtures.OutboundCampaignHandoffDatabaseName); dedicatedErr != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *campaignMigrationDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v", version, err)
	}
	return pool, ctx
}

func campaignRepoRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(directory, "migrations")); statErr == nil && info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("repository root with migrations directory not found")
		}
		directory = parent
	}
}

func ensureCampaignMigrationAt60(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	assertCampaignMigrationVersion(t, ctx, pool, 60)
	var campaigns *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.cloud_campaigns')::text`).Scan(&campaigns); err != nil {
		t.Fatal(err)
	}
	if campaigns == nil {
		t.Fatal("cloud_campaigns is absent at migration 60")
	}
}

func ensureCampaignInitiationMigrationAt66(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	assertCampaignMigrationVersion(t, ctx, pool, 66)
	var plans *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.cloud_campaign_touch_plans')::text`).Scan(&plans); err != nil {
		t.Fatal(err)
	}
	if plans == nil {
		t.Fatal("cloud_campaign_touch_plans is absent at migration 66")
	}
}

func assertCampaignInitiationTablesAbsent(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var plans *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.cloud_campaign_touch_plans')::text`).Scan(&plans); err != nil {
		t.Fatal(err)
	}
	if plans != nil {
		t.Fatalf("cloud_campaign_touch_plans remains after rollback: %q", *plans)
	}
}

func assertCampaignMigrationVersion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int64) {
	t.Helper()
	got := campaignMigrationWaterline(t, ctx, pool)
	if got != want {
		t.Fatalf("migration waterline=%d, want %d", got, want)
	}
}

func campaignMigrationWaterlineAtLeast(t *testing.T, ctx context.Context, pool *pgxpool.Pool, minimum int64) string {
	t.Helper()
	got := campaignMigrationWaterline(t, ctx, pool)
	if got < minimum {
		t.Fatalf("migration waterline=%d, want at least %d", got, minimum)
	}
	return strconv.FormatInt(got, 10)
}

func campaignMigrationWaterline(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int64 {
	t.Helper()
	var got int64
	if err := pool.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&got); err != nil {
		t.Fatalf("read migration waterline: %v", err)
	}
	return got
}

func clearCampaignFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var touchPlans *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.cloud_campaign_touch_plans')::text`).Scan(&touchPlans); err != nil {
		t.Fatal(err)
	}
	if touchPlans != nil {
		var reviewReceipts, mediaDeliveryBindings *string
		if err := pool.QueryRow(ctx, `SELECT to_regclass('public.cloud_campaign_touch_plan_review_receipts')::text`).Scan(&reviewReceipts); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT to_regclass('public.media_campaign_delivery_bindings')::text`).Scan(&mediaDeliveryBindings); err != nil {
			t.Fatal(err)
		}
		truncate := `TRUNCATE TABLE public.cloud_campaign_touch_plan_receipts, public.cloud_campaign_touch_plan_targets, public.cloud_campaign_touch_plan_steps, public.cloud_campaign_touch_plans`
		if reviewReceipts != nil {
			truncate = `TRUNCATE TABLE public.cloud_campaign_touch_plan_review_receipts, public.cloud_campaign_touch_plan_handoffs, public.cloud_campaign_touch_plan_reviews, public.cloud_campaign_touch_plan_receipts, public.cloud_campaign_touch_plan_targets, public.cloud_campaign_touch_plan_steps, public.cloud_campaign_touch_plans`
		}
		if mediaDeliveryBindings != nil {
			truncate += ` CASCADE`
		}
		if _, err := pool.Exec(ctx, truncate); err != nil {
			t.Fatal(err)
		}
		if err := eventsfixture.DeleteCloudCampaignFacts(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `DELETE FROM public.cloud_campaign_operation_receipts`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM public.cloud_campaigns`); err != nil {
		t.Fatal(err)
	}
}

func initiationPlanID(character rune) string {
	return "ctp_" + strings.Repeat(string(character), 64)
}

func seedInitiationPlanHeader(ctx context.Context, execer campaignExecer, campaignCode, planID string, targetCount, stepCount int32) error {
	if _, err := execer.Exec(ctx, `INSERT INTO public.cloud_campaigns (campaign_code,name,approval_status,runtime_status,version,created_by,updated_by,created_at,updated_at)
VALUES ($1,'initiation guard','draft','idle',1,1,1,now(),now())`, campaignCode); err != nil {
		return err
	}
	if _, err := execer.Exec(ctx, `INSERT INTO public.cloud_campaign_steps (campaign_code,step_index,delay_minutes,content)
VALUES ($1,1,0,'local only')`, campaignCode); err != nil {
		return err
	}
	if _, err := execer.Exec(ctx, `INSERT INTO public.cloud_campaign_touch_plans (
  id, campaign_code, campaign_version, source_kind,
  customer_selection_id, customer_selection_version,
  source_digest, target_digest, content_digest,
  target_count, content_step_count, candidate_count, active_customer_count,
  inactive_excluded_count, policy_excluded_count, owner_actor_id, created_at
) VALUES (
  $1, $2, 1, 'customer_selection',
  'local_selection', 'v1',
  decode(repeat('01', 32), 'hex'), decode(repeat('02', 32), 'hex'), decode(repeat('03', 32), 'hex'),
  $3, $4, $3, $3, 0, 0, 1, now()
)`, planID, campaignCode, targetCount, stepCount); err != nil {
		return err
	}
	return nil
}

func seedCompletedInitiationPlan(ctx context.Context, pool *pgxpool.Pool, campaignCode, planID string, declaredTargets, actualTargets int32, receiptKeyByte string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = seedCompletedInitiationPlanInTx(ctx, tx, campaignCode, planID, declaredTargets, actualTargets, receiptKeyByte); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func seedCompletedInitiationPlanInTx(ctx context.Context, tx pgx.Tx, campaignCode, planID string, declaredTargets, actualTargets int32, receiptKeyByte string) error {
	if err := seedInitiationPlanHeader(ctx, tx, campaignCode, planID, declaredTargets, 1); err != nil {
		return err
	}
	for customerID := int32(1); customerID <= actualTargets; customerID++ {
		if _, err := tx.Exec(ctx, `INSERT INTO public.cloud_campaign_touch_plan_targets (plan_id, customer_id) VALUES ($1, $2)`, planID, customerID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.cloud_campaign_touch_plan_steps (plan_id, step_index, delay_minutes, content) VALUES ($1, 1, 0, 'local only')`, planID); err != nil {
		return err
	}
	eventID, err := eventsfixture.AppendCloudCampaignTouchPlanCreated(ctx, tx, planID, campaignCode, declaredTargets)
	if err != nil {
		return err
	}
	var receiptID int64
	if err := tx.QueryRow(ctx, `INSERT INTO public.cloud_campaign_touch_plan_receipts (actor_id, key_digest, payload_digest, plan_id, created_at)
VALUES (1, decode(repeat($2::text, 32), 'hex'), decode(repeat('04', 32), 'hex'), $1::text, now())
RETURNING id`, planID, receiptKeyByte).Scan(&receiptID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE public.cloud_campaign_touch_plan_receipts
SET state = 'completed', event_id = $2, completed_at = now()
WHERE id = $1`, receiptID, eventID); err != nil {
		return err
	}
	return nil
}

func assertCampaignInitiationSQLState(t *testing.T, err error, want string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if err == nil || !errors.As(err, &pgErr) || pgErr.Code != want {
		t.Fatalf("error=%v, want SQLSTATE %s", err, want)
	}
}

func seedCampaignFact(t *testing.T, ctx context.Context, pool *pgxpool.Pool, code string) {
	t.Helper()
	seedCampaignFactInTx(t, ctx, pool, code, true)
}

func seedCampaignReceipt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, key string) {
	t.Helper()
	seedCampaignFactInTx(t, ctx, pool, key, false)
}

type campaignExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func seedCampaignFactInTx(t *testing.T, ctx context.Context, execer campaignExecer, code string, campaign bool) {
	t.Helper()
	if campaign {
		if _, err := execer.Exec(ctx, `INSERT INTO public.cloud_campaigns (campaign_code,name,approval_status,runtime_status,version,created_by,updated_by,created_at,updated_at) VALUES ($1,'rollback guard','draft','idle',1,1,1,now(),now())`, code); err != nil {
			t.Fatal(err)
		}
		if _, err := execer.Exec(ctx, `INSERT INTO public.cloud_campaign_steps (campaign_code,step_index,delay_minutes,content) VALUES ($1,1,0,'local only')`, code); err != nil {
			t.Fatal(err)
		}
		return
	}
	if _, err := execer.Exec(ctx, `INSERT INTO public.cloud_campaign_operation_receipts (actor_id,key_digest,operation,payload_digest,state,created_at) VALUES (1,decode('01','hex'),'start',decode('02','hex'),'reserved',now())`); err != nil {
		t.Fatal(err)
	}
}

func assertCampaignDownRejected(t *testing.T, ctx context.Context, repoRoot string) {
	t.Helper()
	err := campaignGoose(ctx, repoRoot, *campaignMigrationDatabaseURL, "down-to", "59")
	assertCampaignRollbackError(t, err)
}

func assertCampaignRollbackError(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "55000") {
		t.Fatalf("rollback error=%v, want SQLSTATE 55000", err)
	}
}

func assertCampaignCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, wantCampaigns, wantSteps, wantReceipts int) {
	t.Helper()
	var campaigns, steps, receipts int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM public.cloud_campaigns), (SELECT count(*) FROM public.cloud_campaign_steps), (SELECT count(*) FROM public.cloud_campaign_operation_receipts)`).Scan(&campaigns, &steps, &receipts); err != nil {
		t.Fatal(err)
	}
	if campaigns != wantCampaigns || steps != wantSteps || receipts != wantReceipts {
		t.Fatalf("campaign facts campaigns/steps/receipts=%d/%d/%d, want %d/%d/%d", campaigns, steps, receipts, wantCampaigns, wantSteps, wantReceipts)
	}
}

func runCampaignGoose(t *testing.T, ctx context.Context, repoRoot, operation, version string) {
	t.Helper()
	if err := campaignGoose(ctx, repoRoot, *campaignMigrationDatabaseURL, operation, version); err != nil {
		t.Fatal(err)
	}
}

func campaignGoose(ctx context.Context, repoRoot, databaseURL, operation, version string) error {
	command := exec.CommandContext(ctx, "go", "tool", "-modfile=tools/go.mod", "goose", "-dir", "migrations", "postgres", databaseURL, operation, version)
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOFLAGS=-mod=readonly")
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("goose %s %s: %w: %s", operation, version, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func campaignDownURL(t *testing.T) string {
	t.Helper()
	parsed, err := url.Parse(*campaignMigrationDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("application_name", "campaign-migration-down")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func waitForCampaignDownLock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, relation string) {
	t.Helper()
	if relation != "public.cloud_campaigns" && relation != "public.cloud_campaign_operation_receipts" {
		t.Fatalf("unexpected campaign rollback lock relation %q", relation)
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		err := pool.QueryRow(ctx, `SELECT count(*)
      FROM pg_locks lock
      JOIN pg_stat_activity activity ON activity.pid=lock.pid
      WHERE activity.application_name='campaign-migration-down'
        AND lock.relation=$1::regclass
        AND lock.mode='ShareRowExclusiveLock'
		AND NOT lock.granted`, relation).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting == 1 {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("campaign rollback never waited on %s lock", relation)
		case <-ticker.C:
		}
	}
}
