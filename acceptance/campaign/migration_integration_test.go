package campaign_acceptance

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
)

var campaignMigrationDatabaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 campaign migration database")

func TestCloudCampaignMigrationDownFailsClosedAndSerializesFacts(t *testing.T) {
	pool, ctx := openCampaignPool(t)
	repoRoot := campaignRepoRoot(t)
	ensureCampaignMigrationAt60(t, ctx, pool)
	t.Cleanup(func() {
		clearCampaignFacts(t, ctx, pool)
		runCampaignGoose(t, ctx, repoRoot, "up-to", "60")
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

func openCampaignPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *campaignMigrationDatabaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*campaignMigrationDatabaseURL); err != nil {
		t.Fatal(err)
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

func assertCampaignMigrationVersion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int64) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&got); err != nil || got != want {
		t.Fatalf("migration waterline=%d err=%v, want %d", got, err, want)
	}
}

func clearCampaignFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `DELETE FROM public.cloud_campaign_operation_receipts`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM public.cloud_campaigns`); err != nil {
		t.Fatal(err)
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
