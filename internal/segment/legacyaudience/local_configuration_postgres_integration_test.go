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
	automationfixture "github.com/qianlan33333-png/AI-CRM-v2/acceptance/automationfixture"
	contactfixture "github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
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
	if err = connection.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&version); err != nil {
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

func TestLocalConfigurationSQLRepositoryPG16SerializesStaffDeactivationAndSenderReplacement(t *testing.T) {
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
	competitor, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer competitor.Close(ctx)

	setup, err := connection.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	packageID := insertLocalConfigurationPackage(t, ctx, setup, "staff-lock")
	senderUserID := insertLocalConfigurationStaff(t, ctx, setup, "staff-lock")
	if err = setup.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = connection.Exec(context.Background(), `DELETE FROM public.segments WHERE id = $1`, packageID)
		_ = contactfixture.DeleteStaffByWeComUserID(context.Background(), connection, senderUserID)
	})

	transaction, err := connection.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := NewSQLRepository(localConfigurationPGProvider{transaction: transaction})
	if err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err = repository.LockPackage(ctx, packageID); err != nil {
		_ = transaction.Rollback(ctx)
		t.Fatal(err)
	}
	eligible, err := repository.LockEligibleSenderUserIDs(ctx, []string{senderUserID})
	if err != nil || !reflect.DeepEqual(eligible, []string{senderUserID}) {
		_ = transaction.Rollback(ctx)
		t.Fatalf("LockEligibleSenderUserIDs eligible=%v err=%v", eligible, err)
	}

	deactivationDone := make(chan error, 1)
	go func() {
		deactivationDone <- contactfixture.SetStaffActiveByWeComUserID(ctx, competitor, senderUserID, false)
	}()
	select {
	case updateErr := <-deactivationDone:
		_ = transaction.Rollback(ctx)
		if updateErr != nil {
			t.Fatalf("staff deactivation returned before sender transaction: %v", updateErr)
		}
		t.Fatal("staff deactivation did not block on sender eligibility lock")
	case <-time.After(200 * time.Millisecond):
	}

	stored, changed, err := repository.ReplacePackageSenders(ctx, packageID, []PackageSender{{
		SenderUserID: senderUserID, SortOrder: 1, IsEnabled: true,
	}}, 7, time.Date(2026, 8, 22, 5, 6, 8, 0, time.UTC))
	if err != nil || !changed || !reflect.DeepEqual(stored, []PackageSender{{SenderUserID: senderUserID, SortOrder: 1, IsEnabled: true}}) {
		_ = transaction.Rollback(ctx)
		t.Fatalf("ReplacePackageSenders stored=%v changed=%t err=%v", stored, changed, err)
	}
	if err = transaction.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case updateErr := <-deactivationDone:
		if updateErr != nil {
			t.Fatalf("staff deactivation after sender commit: %v", updateErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("staff deactivation remained blocked after sender commit")
	}

	var active bool
	if err = connection.QueryRow(ctx, `SELECT is_active FROM public.staff WHERE wecom_userid = $1`, senderUserID).Scan(&active); err != nil || active {
		t.Fatalf("staff active=%t err=%v, want committed deactivation", active, err)
	}
	var senderCount int
	if err = connection.QueryRow(ctx, `SELECT count(*) FROM public.ai_audience_package_senders WHERE package_id = $1 AND sender_userid = $2`, packageID, senderUserID).Scan(&senderCount); err != nil || senderCount != 1 {
		t.Fatalf("sender count=%d err=%v, want committed replacement", senderCount, err)
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
	code := fmt.Sprintf("audience_local_config_%d", time.Now().UnixNano())
	agentID, err := automationfixture.CreateActiveAgent(ctx, transaction, "Audience local configuration agent", code, "agent")
	if err != nil {
		t.Fatal(err)
	}
	return agentID
}

func insertLocalConfigurationStaff(t *testing.T, ctx context.Context, transaction pgx.Tx, suffix string) string {
	t.Helper()
	userID := fmt.Sprintf("audience_local_%s_%d", suffix, time.Now().UnixNano())
	if _, err := contactfixture.CreateStaffWithDetails(ctx, transaction, userID, "Audience local configuration staff", true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	return userID
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
	if len(arguments) == 1 && arguments[0] != nil && query == `
SELECT pg_advisory_xact_lock(hashtextextended('ai_audience.package.senders.v1:' || $1::text, 0))` {
		arguments[0] = fmt.Sprint(arguments[0])
	}
	tag, err := executor.transaction.Exec(ctx, query, arguments...)
	return tag.RowsAffected(), err
}
func (executor localConfigurationPGExecutor) Query(ctx context.Context, query string, arguments ...any) (SQLRows, error) {
	return executor.transaction.Query(ctx, query, arguments...)
}
func (executor localConfigurationPGExecutor) QueryRow(ctx context.Context, query string, arguments ...any) SQLRow {
	return executor.transaction.QueryRow(ctx, query, arguments...)
}
