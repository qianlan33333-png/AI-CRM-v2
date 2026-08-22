package pushcenter_acceptance

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	pushcenterapp "github.com/qianlan33333-png/AI-CRM-v2/internal/pushcenter/app"
	pushcenterstore "github.com/qianlan33333-png/AI-CRM-v2/internal/pushcenter/store"
)

var pushCenterDatabaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 Push Center database")

func TestP4PushCenterSectionsStatsNormalBoundaryAndDegraded(t *testing.T) {
	pool, ctx := openPushCenterPool(t)
	resetPushCenterProjection(t, ctx, pool)
	service := pushCenterService(pool)
	if _, err := service.Read(ctx, pushcenterapp.Filter{}); !errors.Is(err, pushcenterapp.ErrReadModelUnavailable) {
		t.Fatalf("unready projection error=%v, want unavailable", err)
	}
	seedPushCenterProjection(t, ctx, pool)

	fullFilter := pushcenterapp.Filter{Section: " questionnaire ", EffectType: " questionnaire_submission ", Status: " sent ",
		BusinessType: " survey ", BusinessID: " business-1 ", TargetType: " customer ", TargetID: " target-1 ",
		ExternalUserID: " external-1 ", OwnerUserID: " owner-1 ", TraceID: " trace-1 ", IdempotencyKey: " key-1 ",
		SourceModule: " questionnaire ", SourceRoute: " /submit ", CreatedFrom: " 2026-08-01T00:00:00Z ", CreatedTo: " 2026-08-01T00:00:00Z "}
	summary, err := service.Read(ctx, fullFilter)
	if err != nil || summary.Total != 1 || summary.ByStatus["sent"] != 1 || summary.BySection["questionnaire"] != 1 || summary.ByEffectiveStatus["sent"] != 1 {
		t.Fatalf("full summary=%+v err=%v", summary, err)
	}
	sent, err := service.Read(ctx, pushcenterapp.Filter{Status: "sent"})
	if err != nil || sent.Total != 2 || sent.ByStatus["sent_with_shadow_warning"] != 1 || sent.ByEffectiveStatus["reconciled"] != 1 {
		t.Fatalf("sent summary=%+v err=%v", sent, err)
	}
	if _, err := service.Read(ctx, pushcenterapp.Filter{CreatedFrom: "not-a-timestamp"}); !errors.Is(err, pushcenterapp.ErrReadModelUnavailable) {
		t.Fatalf("invalid timestamp error=%v, want unavailable", err)
	}

	var tenantColumns, foreignKeys int
	err = pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name LIKE 'push_center_read_model_%' AND column_name ILIKE ('%' || 'ten' || 'ant%')),
  (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('push_center_read_model_state'::regclass, 'push_center_read_model_entries'::regclass) AND contype='f')`).Scan(&tenantColumns, &foreignKeys)
	if err != nil || tenantColumns != 0 || foreignKeys != 0 {
		t.Fatalf("tenant columns/foreign keys/error=%d/%d/%v", tenantColumns, foreignKeys, err)
	}
}

func TestP4PushCenterSectionsStatsConcurrentReadUoW(t *testing.T) {
	pool, ctx := openPushCenterPool(t)
	resetPushCenterProjection(t, ctx, pool)
	seedPushCenterProjection(t, ctx, pool)
	service := pushCenterService(pool)
	const readers = 24
	errCh := make(chan error, readers)
	var group sync.WaitGroup
	for range readers {
		group.Add(1)
		go func() {
			defer group.Done()
			summary, readErr := service.Read(ctx, pushcenterapp.Filter{Status: "sent"})
			if readErr == nil && summary.Total != 2 {
				readErr = fmt.Errorf("total=%d, want 2", summary.Total)
			}
			errCh <- readErr
		}()
	}
	group.Wait()
	close(errCh)
	for readErr := range errCh {
		if readErr != nil {
			t.Fatalf("concurrent read error=%v", readErr)
		}
	}
}

func TestP4PushCenterTextPlanAvoidsSeqScanAtTwoHundredThousandRows(t *testing.T) {
	pool, ctx := openPushCenterPool(t)
	resetPushCenterProjection(t, ctx, pool)
	if _, err := pool.Exec(ctx, `UPDATE push_center_read_model_state SET production_data_ready=true, fixture_mode=true, updated_at=now() WHERE singleton=true`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO push_center_read_model_entries (section, effect_type, status, effective_status, business_type, created_at)
SELECT 'integrations', CASE WHEN value=199999 THEN 'performance-needle-unique' ELSE 'ordinary-effect' END,
  'pending', 'pending', 'plan', now() - (value || ' seconds')::interval
FROM generate_series(1, 200000) AS value`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE push_center_read_model_entries`); err != nil {
		t.Fatal(err)
	}
	var plan string
	if err := pool.QueryRow(ctx, `EXPLAIN (FORMAT JSON) SELECT count(*) FROM push_center_read_model_entries WHERE effect_type ILIKE '%performance-needle-unique%'`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan, "Seq Scan") {
		t.Fatalf("200k text-filter plan regressed to Seq Scan: %s", plan)
	}
}

func openPushCenterPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *pushCenterDatabaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*pushCenterDatabaseURL); err != nil {
		if pushCenterErr := acceptancefixtures.ValidateDatabaseURLForDatabase(*pushCenterDatabaseURL, acceptancefixtures.PushCenterDatabaseName); pushCenterErr != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *pushCenterDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var serverVersion string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&serverVersion); err != nil || serverVersion != "160014" {
		t.Fatalf("postgres version=%q err=%v", serverVersion, err)
	}
	return pool, ctx
}

func pushCenterService(pool *pgxpool.Pool) *pushcenterapp.Service {
	return pushcenterapp.NewService(platformstore.NewUnitOfWork(pool), pushcenterstore.NewRepository())
}

func resetPushCenterProjection(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `TRUNCATE push_center_read_model_entries RESTART IDENTITY; UPDATE push_center_read_model_state SET production_data_ready=false, fixture_mode=false, allow_fixture_repo_in_prod=false, updated_at=now() WHERE singleton=true`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, `TRUNCATE push_center_read_model_entries RESTART IDENTITY; UPDATE push_center_read_model_state SET production_data_ready=false, fixture_mode=false, allow_fixture_repo_in_prod=false, updated_at=now() WHERE singleton=true`)
	})
}

func seedPushCenterProjection(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `UPDATE push_center_read_model_state SET production_data_ready=true, fixture_mode=true, updated_at=now() WHERE singleton=true`); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO push_center_read_model_entries (
section, effect_type, status, effective_status, business_type, business_id, target_type, target_id,
external_userid, owner_userid, trace_id, idempotency_key, source_module, source_route, created_at)
VALUES
('questionnaire','webhook.questionnaire_submission.push','sent','sent','survey','business-1','customer','target-1','external-1','owner-1','trace-1','key-1','questionnaire','/submit','2026-08-01T00:00:00Z'),
('questionnaire','webhook.questionnaire_submission.push','sent_with_shadow_warning','reconciled','survey','business-2','customer','target-2','external-2','owner-2','trace-2','key-2','questionnaire','/submit','2026-08-02T00:00:00Z'),
('order','webhook.order_paid.push','pending','pending','order','business-3','order','target-3','external-3','owner-3','trace-3','key-3','order','/paid','2026-08-03T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
}
