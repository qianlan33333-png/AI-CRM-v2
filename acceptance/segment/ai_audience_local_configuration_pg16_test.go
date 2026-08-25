package segment_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	authacceptance "github.com/qianlan33333-png/AI-CRM-v2/acceptance/auth"
	automationfixture "github.com/qianlan33333-png/AI-CRM-v2/acceptance/automationfixture"
	contactfixture "github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	automationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	legacyaudience "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudience"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

// TestLocalConfigurationSQLRepositoryPG16 runs against a database migrated
// through 00084_ai_audience_local_configuration_closure.sql. All fixtures roll back.
func TestLocalConfigurationSQLRepositoryPG16(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("CI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CI_TEST_DATABASE_URL is required for the isolated migrated PG16 test")
	}
	requireAudience84Database(t, dsn)
	connection, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	var versionText string
	if err = connection.QueryRow(ctx, "SHOW server_version_num").Scan(&versionText); err != nil {
		t.Fatal(err)
	}
	version, err := strconv.Atoi(versionText)
	if err != nil {
		t.Fatalf("parse PostgreSQL version %q: %v", versionText, err)
	}
	if version/10000 != 16 {
		t.Fatalf("PostgreSQL major=%d, want 16", version/10000)
	}
	actorID := seedLocalConfigurationActor(t, ctx, dsn)
	transaction, err := connection.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback(ctx) //nolint:errcheck -- fixture cleanup.

	assertLocalConfigurationSchema(t, ctx, transaction)
	firstPackage := insertLocalConfigurationPackage(t, ctx, transaction, actorID, "first")
	secondPackage := insertLocalConfigurationPackage(t, ctx, transaction, actorID, "second")
	agentID := insertLocalConfigurationAgent(t, ctx, transaction)
	repository, err := legacyaudience.NewSQLRepository(localConfigurationPGProvider{transaction: transaction})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 5, 6, 7, 0, time.UTC)
	definition := json.RawMessage(`{"field":"is_deleted","op":"eq","value":false}`)
	definitionDigest := sha256.Sum256(definition)
	configuration, err := repository.InsertConfigurationVersion(ctx, legacyaudience.ConfigurationVersion{
		PackageID: firstPackage, Version: 1, SchemaVersion: legacyaudience.ConfigurationSchemaVersion, PackageVersion: 1,
		Definition: segmentport.Definition(definition), DefinitionDigest: fmt.Sprintf("%x", definitionDigest), RefreshMode: "manual", CreatedBy: actorID, CreatedAt: now,
	})
	if err != nil || configuration.PackageID != firstPackage || configuration.Version != 1 || configuration.CreatedBy != actorID || configuration.CreatedAt.IsZero() {
		t.Fatalf("InsertConfigurationVersion configuration=%+v err=%v", configuration, err)
	}
	if _, err = transaction.Exec(ctx, "SAVEPOINT immutable_configuration_version"); err != nil {
		t.Fatal(err)
	}
	if _, err = transaction.Exec(ctx, `UPDATE public.ai_audience_package_configuration_versions SET version = 2 WHERE package_id = $1`, firstPackage); err == nil {
		t.Fatal("immutable configuration version unexpectedly updated")
	}
	if _, err = transaction.Exec(ctx, "ROLLBACK TO SAVEPOINT immutable_configuration_version"); err != nil {
		t.Fatal(err)
	}
	if _, err = transaction.Exec(ctx, "RELEASE SAVEPOINT immutable_configuration_version"); err != nil {
		t.Fatal(err)
	}
	first, err := repository.SaveAutomationBinding(ctx, legacyaudience.AutomationBinding{PackageID: firstPackage, AutomationAgentID: agentID}, actorID, 0, now)
	if err != nil || first.PackageID != firstPackage || first.AutomationAgentID != agentID || first.CreatedBy != actorID || first.UpdatedBy != actorID || first.Version != 1 {
		t.Fatalf("SaveAutomationBinding first=%+v err=%v", first, err)
	}
	if _, err = transaction.Exec(ctx, "SAVEPOINT active_binding_collision"); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.SaveAutomationBinding(ctx, legacyaudience.AutomationBinding{PackageID: secondPackage, AutomationAgentID: agentID}, actorID, 0, now); !errors.Is(err, legacyaudience.ErrConflict) {
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
	if _, err = repository.SaveAutomationBinding(ctx, legacyaudience.AutomationBinding{PackageID: secondPackage, AutomationAgentID: agentID}, actorID, 0, now); err != nil {
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

	wanted := []legacyaudience.PackageSender{{SenderUserID: "alpha", SortOrder: 1, IsEnabled: true}, {SenderUserID: "beta", SortOrder: 2, IsEnabled: false}}
	stored, changed, err := repository.ReplacePackageSenders(ctx, secondPackage, wanted, actorID, now)
	if err != nil || !changed || !reflect.DeepEqual(stored, wanted) {
		t.Fatalf("ReplacePackageSenders stored=%+v changed=%t err=%v", stored, changed, err)
	}
	stored, changed, err = repository.ReplacePackageSenders(ctx, secondPackage, wanted, actorID, now)
	if err != nil || changed || !reflect.DeepEqual(stored, wanted) {
		t.Fatalf("idempotent sender replacement stored=%+v changed=%t err=%v", stored, changed, err)
	}

	keyDigest := sha256.Sum256([]byte("configuration-pg-receipt-key"))
	payloadDigest := sha256.Sum256([]byte(`{"package_id":1}`))
	receipt, owned, err := repository.ReserveConfigurationReceipt(ctx, legacyaudience.ReceiptReservation{
		Operation: "senders_put", ActorID: actorID, KeyDigest: keyDigest, PayloadDigest: payloadDigest, CreatedAt: now,
	})
	if err != nil || !owned || receipt.ID < 1 || receipt.State != "in_progress" {
		t.Fatalf("ReserveConfigurationReceipt receipt=%+v owned=%t err=%v", receipt, owned, err)
	}
	completed, err := repository.CompleteConfigurationReceipt(ctx, receipt.ID, json.RawMessage(`{"package_id":1,"items":[]}`), now)
	if err != nil || completed.State != "completed" {
		t.Fatalf("CompleteConfigurationReceipt receipt=%+v err=%v", completed, err)
	}
}

func TestLocalConfigurationHTTPServicePG16ReceiptsAndRollback(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("CI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CI_TEST_DATABASE_URL is required for the isolated migrated PG16 test")
	}
	requireAudience84Database(t, dsn)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	actorID := seedLocalConfigurationActor(t, ctx, dsn)
	setup, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	packageID := insertLocalConfigurationPackage(t, ctx, setup, actorID, "http-service")
	agentID := insertLocalConfigurationAgent(t, ctx, setup)
	if err = setup.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	senderUserID := fmt.Sprintf("audience_local_http_%d", time.Now().UnixNano())
	staffID, err := contactfixture.CreateStaffRecord(ctx, pool, senderUserID, "Audience local HTTP", true, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM public.segments WHERE id = $1`, packageID)
		_ = contactfixture.DeleteStaff(context.Background(), pool, staffID)
	})

	repository, err := legacyaudience.NewSQLRepository(localConfigurationPoolProvider{pool: pool})
	if err != nil {
		t.Fatal(err)
	}
	staff := contactstore.NewStaffDirectoryRepository(pool)
	service, err := legacyaudience.NewLocalConfigurationService(
		platformstore.NewUnitOfWork(pool), repository,
		localConfigurationAutomationReader{store: automationstore.NewAgentRepository()}, staff, staff,
		segmentapp.NewAudienceDefinitionEngine(segmentstore.NewRefreshRepository()),
		localConfigurationEventAppender{appender: eventstore.NewAppender()},
	)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := legacyaudience.NewLocalConfigurationHandler(service, localConfigurationSecurity{actorID: actorID})
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := legacyaudience.NewLocalConfigurationRouteFragment(handler)
	if err != nil {
		t.Fatal(err)
	}

	mutations := []struct {
		method, suffix, key, body, changed string
	}{
		{http.MethodPut, "/automation-binding", "audience-binding-http-key", fmt.Sprintf(`{"automation_agent_id":%d,"expected_version":0}`, agentID), fmt.Sprintf(`{"automation_agent_id":%d,"expected_version":1}`, agentID)},
		{http.MethodPut, "/senders", "audience-senders-http-key", fmt.Sprintf(`{"items":[{"sender_userid":%q,"sort_order":1,"is_enabled":true}]}`, senderUserID), `{"items":[]}`},
		{http.MethodPut, "/configuration", "audience-config-http-key", `{"expected_version":0,"expected_package_version":1}`, `{"expected_version":0,"expected_package_version":2}`},
	}
	for _, mutation := range mutations {
		path := fmt.Sprintf("%s/packages/%d%s", legacyaudience.RoutePrefix, packageID, mutation.suffix)
		assertLocalConfigurationHTTPStatus(t, fragment, mutation.method, path, mutation.key, mutation.body, http.StatusOK)
		assertLocalConfigurationHTTPStatus(t, fragment, mutation.method, path, mutation.key, mutation.body, http.StatusOK)
		assertLocalConfigurationHTTPStatus(t, fragment, mutation.method, path, mutation.key, mutation.changed, http.StatusConflict)
	}
	deletePath := fmt.Sprintf("%s/packages/%d/automation-binding", legacyaudience.RoutePrefix, packageID)
	assertLocalConfigurationHTTPStatus(t, fragment, http.MethodDelete, deletePath, "audience-delete-http-key", `{"expected_version":1}`, http.StatusOK)
	assertLocalConfigurationHTTPStatus(t, fragment, http.MethodDelete, deletePath, "audience-delete-http-key", `{"expected_version":1}`, http.StatusOK)
	assertLocalConfigurationHTTPStatus(t, fragment, http.MethodDelete, deletePath, "audience-delete-http-key", `{"expected_version":0}`, http.StatusConflict)

	previewPath := fmt.Sprintf("%s/packages/%d/configuration-preview?configuration_version=1&evaluated_at=2026-08-25T01%%3A02%%3A03Z", legacyaudience.RoutePrefix, packageID)
	assertLocalConfigurationHTTPStatus(t, fragment, http.MethodGet, previewPath, "", "", http.StatusOK)
	materializePath := fmt.Sprintf("%s/packages/%d/configuration-materialize", legacyaudience.RoutePrefix, packageID)
	materializeBody := `{"configuration_version":1,"expected_package_version":1}`
	assertLocalConfigurationHTTPStatus(t, fragment, http.MethodPost, materializePath, "audience-materialize-http-key", materializeBody, http.StatusOK)
	assertLocalConfigurationHTTPStatus(t, fragment, http.MethodPost, materializePath, "audience-materialize-http-key", materializeBody, http.StatusOK)
	assertLocalConfigurationHTTPStatus(t, fragment, http.MethodPost, materializePath, "audience-materialize-http-key", `{"configuration_version":1,"expected_package_version":2}`, http.StatusConflict)

	var receipts, events int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.ai_audience_local_configuration_receipts WHERE actor_id = $1 AND state = 'completed'`, actorID).Scan(&receipts); err != nil || receipts != 5 {
		t.Fatalf("completed receipts=%d err=%v, want 5", receipts, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.event_log WHERE idempotency_key LIKE 'ai-audience:%'`).Scan(&events); err != nil || events != 5 {
		t.Fatalf("events=%d err=%v, want 5", events, err)
	}
	before := receipts
	failurePath := fmt.Sprintf("%s/packages/%d/senders", legacyaudience.RoutePrefix, packageID)
	assertLocalConfigurationHTTPStatus(t, fragment, http.MethodPut, failurePath, "audience-rollback-http-key", `{"items":[{"sender_userid":"not-local","sort_order":1,"is_enabled":true}]}`, http.StatusConflict)
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.ai_audience_local_configuration_receipts WHERE actor_id = $1`, actorID).Scan(&receipts); err != nil || receipts != before {
		t.Fatalf("failed mutation receipt count=%d before=%d err=%v", receipts, before, err)
	}
}

func assertLocalConfigurationHTTPStatus(t *testing.T, handler http.Handler, method, path, key, body string, want int) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != want {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.Code, want, response.Body.String())
	}
}

func assertLocalConfigurationSchema(t *testing.T, ctx context.Context, transaction pgx.Tx) {
	t.Helper()
	var bindings, senders, receipts, configurations, senderSecurity, sendRecords string
	if err := transaction.QueryRow(ctx, `
SELECT
  COALESCE(to_regclass('public.ai_audience_package_automation_bindings')::text, ''),
  COALESCE(to_regclass('public.ai_audience_package_senders')::text, ''),
	  COALESCE(to_regclass('public.ai_audience_local_configuration_receipts')::text, ''),
  COALESCE(to_regclass('public.ai_audience_package_configuration_versions')::text, ''),
  COALESCE(to_regclass('public.ai_audience_package_sender_security_config')::text, ''),
  COALESCE(to_regclass('public.ai_audience_package_send_record_projections')::text, '')`).Scan(&bindings, &senders, &receipts, &configurations, &senderSecurity, &sendRecords); err != nil {
		t.Fatal(err)
	}
	if bindings == "" || senders == "" || receipts == "" || configurations == "" || senderSecurity != "" || sendRecords != "" {
		t.Fatalf("required tables missing: bindings=%q senders=%q receipts=%q configurations=%q sender_security=%q send_records=%q", bindings, senders, receipts, configurations, senderSecurity, sendRecords)
	}
}

func seedLocalConfigurationActor(t *testing.T, ctx context.Context, databaseURL string) int64 {
	t.Helper()
	fixture, err := authacceptance.OpenPostgreSQL(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	actorID, err := fixture.SeedAdmin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return actorID
}

func insertLocalConfigurationPackage(t *testing.T, ctx context.Context, transaction pgx.Tx, actorID int64, suffix string) int64 {
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
VALUES ($1, 'active', 1, $2, $2)`, packageID, actorID); err != nil {
		t.Fatal(err)
	}
	return packageID
}

func insertLocalConfigurationAgent(t *testing.T, ctx context.Context, transaction pgx.Tx) int64 {
	t.Helper()
	code := fmt.Sprintf("audience_local_config_%d", time.Now().UnixNano())
	agentID, err := automationfixture.CreateAgentConfiguration(ctx, transaction, code)
	if err != nil {
		t.Fatal(err)
	}
	return agentID
}

func requireAudience84Database(t *testing.T, dsn string) {
	t.Helper()
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(dsn, acceptancefixtures.Audience84DatabaseName); err != nil {
		if errors.Is(err, acceptancefixtures.ErrUnsafeDatabaseURL) {
			t.Skip("Audience 84 PG16 tests require the isolated Audience 84 database")
		}
		t.Fatal(err)
	}
}

type localConfigurationPGProvider struct{ transaction pgx.Tx }

func (provider localConfigurationPGProvider) Reader(context.Context) (legacyaudience.SQLExecutor, error) {
	return localConfigurationPGExecutor{transaction: provider.transaction}, nil
}
func (provider localConfigurationPGProvider) Transaction(context.Context) (legacyaudience.SQLExecutor, error) {
	return localConfigurationPGExecutor{transaction: provider.transaction}, nil
}
func (localConfigurationPGProvider) IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

type localConfigurationPGExecutor struct{ transaction pgx.Tx }

func (executor localConfigurationPGExecutor) Exec(ctx context.Context, query string, arguments ...any) (int64, error) {
	tag, err := executor.transaction.Exec(ctx, query, arguments...)
	return tag.RowsAffected(), err
}
func (executor localConfigurationPGExecutor) Query(ctx context.Context, query string, arguments ...any) (legacyaudience.SQLRows, error) {
	return executor.transaction.Query(ctx, query, arguments...)
}
func (executor localConfigurationPGExecutor) QueryRow(ctx context.Context, query string, arguments ...any) legacyaudience.SQLRow {
	return executor.transaction.QueryRow(ctx, query, arguments...)
}

type localConfigurationPoolProvider struct{ pool *pgxpool.Pool }

func (provider localConfigurationPoolProvider) Reader(context.Context) (legacyaudience.SQLExecutor, error) {
	return localConfigurationPoolExecutor{queryer: provider.pool, execer: provider.pool}, nil
}
func (localConfigurationPoolProvider) Transaction(ctx context.Context) (legacyaudience.SQLExecutor, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return localConfigurationPoolExecutor{queryer: tx, execer: tx}, nil
}
func (localConfigurationPoolProvider) IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

type localConfigurationPoolQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type localConfigurationPoolExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}
type localConfigurationPoolExecutor struct {
	queryer localConfigurationPoolQueryer
	execer  localConfigurationPoolExecer
}

func (executor localConfigurationPoolExecutor) Exec(ctx context.Context, query string, arguments ...any) (int64, error) {
	tag, err := executor.execer.Exec(ctx, query, arguments...)
	return tag.RowsAffected(), err
}
func (executor localConfigurationPoolExecutor) Query(ctx context.Context, query string, arguments ...any) (legacyaudience.SQLRows, error) {
	return executor.queryer.Query(ctx, query, arguments...)
}
func (executor localConfigurationPoolExecutor) QueryRow(ctx context.Context, query string, arguments ...any) legacyaudience.SQLRow {
	return executor.queryer.QueryRow(ctx, query, arguments...)
}

type localConfigurationAutomationReader struct {
	store *automationstore.AgentRepository
}

func (reader localConfigurationAutomationReader) GetAutomationAgent(ctx context.Context, id int64) (legacyaudience.AutomationAgent, error) {
	agent, err := reader.store.Lock(ctx, automationport.AgentID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return legacyaudience.AutomationAgent{}, legacyaudience.ErrNotFound
	}
	if err != nil {
		return legacyaudience.AutomationAgent{}, err
	}
	return legacyaudience.AutomationAgent{ID: int64(agent.ID), Status: string(agent.Status)}, nil
}

type localConfigurationEventAppender struct{ appender eventport.Appender }

func (adapter localConfigurationEventAppender) Append(ctx context.Context, event legacyaudience.LocalEvent) error {
	_, err := adapter.appender.Append(ctx, eventport.Event{Type: event.Type, Payload: event.Payload, OccurredAt: event.OccurredAt, IdempotencyKey: event.IdempotencyKey})
	return err
}

type localConfigurationSecurity struct{ actorID int64 }

func (security localConfigurationSecurity) Authorize(_ *http.Request, _ legacyaudience.AccessRequirement) (legacyaudience.Actor, error) {
	return legacyaudience.Actor{AdminUserID: security.actorID}, nil
}
