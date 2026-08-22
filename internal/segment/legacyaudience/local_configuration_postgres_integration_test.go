package legacyaudience

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestLocalConfigurationSQLRepositoryPG16 runs against a database migrated
// through 00057_ai_audience_local_configuration.sql. All fixtures roll back.
func TestLocalConfigurationSQLRepositoryPG16(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("CI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CI_TEST_DATABASE_URL is required for the isolated migrated PG16 test")
	}
	connection, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	var version int
	if err = connection.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version/10000 != 16 {
		t.Fatalf("PostgreSQL major=%d, want 16", version/10000)
	}
	transaction, err := connection.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback(ctx) //nolint:errcheck -- fixture cleanup.

	assertLocalConfigurationSchema(t, ctx, transaction)
	firstPackage := insertLocalConfigurationPackage(t, ctx, transaction, "first")
	secondPackage := insertLocalConfigurationPackage(t, ctx, transaction, "second")
	agentID := insertLocalConfigurationAgent(t, ctx, transaction)
	repository, err := NewSQLRepository(localConfigurationPGProvider{transaction: transaction})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 5, 6, 7, 0, time.UTC)
	first, err := repository.SaveAutomationBinding(ctx, AutomationBinding{PackageID: firstPackage, AutomationAgentID: agentID}, 7, now)
	if err != nil || !validAutomationBinding(first) || first.PackageID != firstPackage {
		t.Fatalf("SaveAutomationBinding first=%+v err=%v", first, err)
	}
	if _, err = transaction.Exec(ctx, "SAVEPOINT active_binding_collision"); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.SaveAutomationBinding(ctx, AutomationBinding{PackageID: secondPackage, AutomationAgentID: agentID}, 7, now); !errors.Is(err, ErrConflict) {
		t.Fatalf("active package collision error=%v, want conflict", err)
	}
	if _, err = transaction.Exec(ctx, "ROLLBACK TO SAVEPOINT active_binding_collision"); err != nil {
		t.Fatal(err)
	}
	if _, err = transaction.Exec(ctx, "RELEASE SAVEPOINT active_binding_collision"); err != nil {
		t.Fatal(err)
	}
	if _, err = transaction.Exec(ctx, `UPDATE public.ai_audience_package_metadata SET lifecycle = 'archived' WHERE segment_id = $1`, firstPackage); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.SaveAutomationBinding(ctx, AutomationBinding{PackageID: secondPackage, AutomationAgentID: agentID}, 7, now); err != nil {
		t.Fatalf("archived owner must release agent: %v", err)
	}
	if _, err = transaction.Exec(ctx, "SAVEPOINT activation_binding_collision"); err != nil {
		t.Fatal(err)
	}
	if _, err = transaction.Exec(ctx, `UPDATE public.ai_audience_package_metadata SET lifecycle = 'active' WHERE segment_id = $1`, firstPackage); err == nil {
		t.Fatal("reactivation with an occupied agent unexpectedly succeeded")
	}
	if _, err = transaction.Exec(ctx, "ROLLBACK TO SAVEPOINT activation_binding_collision"); err != nil {
		t.Fatal(err)
	}
	if _, err = transaction.Exec(ctx, "RELEASE SAVEPOINT activation_binding_collision"); err != nil {
		t.Fatal(err)
	}

	wanted := []PackageSender{{SenderUserID: "alpha", SortOrder: 1, IsEnabled: true}, {SenderUserID: "beta", SortOrder: 2, IsEnabled: false}}
	stored, changed, err := repository.ReplacePackageSenders(ctx, secondPackage, wanted, 7, now)
	if err != nil || !changed || !reflect.DeepEqual(stored, wanted) {
		t.Fatalf("ReplacePackageSenders stored=%+v changed=%t err=%v", stored, changed, err)
	}
	stored, changed, err = repository.ReplacePackageSenders(ctx, secondPackage, wanted, 7, now)
	if err != nil || changed || !reflect.DeepEqual(stored, wanted) {
		t.Fatalf("idempotent sender replacement stored=%+v changed=%t err=%v", stored, changed, err)
	}

	keyDigest := sha256.Sum256([]byte("configuration-pg-receipt-key"))
	payloadDigest := sha256.Sum256([]byte(`{"package_id":1}`))
	receipt, owned, err := repository.ReserveConfigurationReceipt(ctx, ReceiptReservation{
		Operation: "senders_put", ActorID: 7, KeyDigest: keyDigest, PayloadDigest: payloadDigest, CreatedAt: now,
	})
	if err != nil || !owned || receipt.ID < 1 || receipt.State != "in_progress" {
		t.Fatalf("ReserveConfigurationReceipt receipt=%+v owned=%t err=%v", receipt, owned, err)
	}
	completed, err := repository.CompleteConfigurationReceipt(ctx, receipt.ID, json.RawMessage(`{"package_id":1,"items":[]}`), now)
	if err != nil || completed.State != "completed" {
		t.Fatalf("CompleteConfigurationReceipt receipt=%+v err=%v", completed, err)
	}
}

func assertLocalConfigurationSchema(t *testing.T, ctx context.Context, transaction pgx.Tx) {
	t.Helper()
	var bindings, senders, receipts string
	if err := transaction.QueryRow(ctx, `
SELECT
  COALESCE(to_regclass('public.ai_audience_package_automation_bindings')::text, ''),
  COALESCE(to_regclass('public.ai_audience_package_senders')::text, ''),
  COALESCE(to_regclass('public.ai_audience_local_configuration_receipts')::text, '')`).Scan(&bindings, &senders, &receipts); err != nil {
		t.Fatal(err)
	}
	if bindings == "" || senders == "" || receipts == "" {
		t.Fatalf("required tables missing: bindings=%q senders=%q receipts=%q", bindings, senders, receipts)
	}
}

func insertLocalConfigurationPackage(t *testing.T, ctx context.Context, transaction pgx.Tx, suffix string) int64 {
	t.Helper()
	name := fmt.Sprintf("audience-local-config-%s-%d", suffix, time.Now().UnixNano())
	var packageID int64
	if err := transaction.QueryRow(ctx, `
INSERT INTO public.segments (name, definition, refresh_mode, member_count, refresh_status)
VALUES ($1, '{}'::jsonb, 'manual', 0, 'idle')
RETURNING id`, name).Scan(&packageID); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(ctx, `
INSERT INTO public.ai_audience_package_metadata
  (segment_id, lifecycle, version, created_by, updated_by)
VALUES ($1, 'active', 1, 7, 7)`, packageID); err != nil {
		t.Fatal(err)
	}
	return packageID
}

func insertLocalConfigurationAgent(t *testing.T, ctx context.Context, transaction pgx.Tx) int64 {
	t.Helper()
	var agentID int64
	code := fmt.Sprintf("audience_local_config_%d", time.Now().UnixNano())
	if err := transaction.QueryRow(ctx, `
INSERT INTO public.automation_agent_configurations
  (agent_name, agent_code, automation_type, status, created_by, updated_by, created_at, updated_at)
VALUES ('Audience local configuration agent', $1, 'agent', 'active', 7, 7, now(), now())
RETURNING id`, code).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	return agentID
}

type localConfigurationPGProvider struct{ transaction pgx.Tx }

func (provider localConfigurationPGProvider) Reader(context.Context) (SQLExecutor, error) {
	return localConfigurationPGExecutor{transaction: provider.transaction}, nil
}
func (provider localConfigurationPGProvider) Transaction(context.Context) (SQLExecutor, error) {
	return localConfigurationPGExecutor{transaction: provider.transaction}, nil
}
func (localConfigurationPGProvider) IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

type localConfigurationPGExecutor struct{ transaction pgx.Tx }

func (executor localConfigurationPGExecutor) Exec(ctx context.Context, query string, arguments ...any) (int64, error) {
	tag, err := executor.transaction.Exec(ctx, query, arguments...)
	return tag.RowsAffected(), err
}
func (executor localConfigurationPGExecutor) Query(ctx context.Context, query string, arguments ...any) (SQLRows, error) {
	return executor.transaction.Query(ctx, query, arguments...)
}
func (executor localConfigurationPGExecutor) QueryRow(ctx context.Context, query string, arguments ...any) SQLRow {
	return executor.transaction.QueryRow(ctx, query, arguments...)
}
