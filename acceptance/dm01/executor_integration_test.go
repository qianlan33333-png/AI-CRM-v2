package dm01_acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/contact/migration"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	contactfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/acceptancefixture"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var (
	sourceDatabaseURL = flag.String("source-database-url", "", "dedicated PostgreSQL 16 DM01 source database")
	targetDatabaseURL = flag.String("target-database-url", "", "dedicated PostgreSQL 16 DM01 target database")
)

func TestDM01ExecutorTwoPostgreSQLDatabases(t *testing.T) {
	if *sourceDatabaseURL == "" || *targetDatabaseURL == "" {
		t.Fatal("source-database-url and target-database-url are required")
	}
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(*sourceDatabaseURL, acceptancefixtures.DM01SourceDatabaseName); err != nil {
		t.Fatal("unsafe source database URL")
	}
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(*targetDatabaseURL, acceptancefixtures.DM01TargetDatabaseName); err != nil {
		t.Fatal("unsafe target database URL")
	}
	ctx := context.Background()
	source, err := pgxpool.New(ctx, *sourceDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	target, err := pgxpool.New(ctx, *targetDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	assertPG16(t, ctx, source)
	assertPG16(t, ctx, target)
	var sourceServer, sourceDB, targetServer, targetDB string
	if err = source.QueryRow(ctx, `SELECT system_identifier::text,current_database() FROM pg_control_system()`).Scan(&sourceServer, &sourceDB); err != nil {
		t.Fatal(err)
	}
	if err = target.QueryRow(ctx, `SELECT system_identifier::text,current_database() FROM pg_control_system()`).Scan(&targetServer, &targetDB); err != nil {
		t.Fatal(err)
	}
	if sourceServer != targetServer || sourceDB == targetDB {
		t.Fatalf("expected two databases on one PostgreSQL server, got %s/%s", sourceDB, targetDB)
	}

	repoRoot := repositoryRoot(t)
	schema, err := os.ReadFile(filepath.Join(repoRoot, "tools/sqlc/dm01_legacy_schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if _, err = source.Exec(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}
	resetTarget(t, ctx, target)
	seedSource(t, ctx, source, "Customer One", time.Date(2026, 8, 25, 1, 0, 0, 0, time.UTC))

	hmacKey := []byte("dm01-source-hmac-key-32-bytes!!!")
	archiveKey := []byte("dm01-archive-aes-key-32-bytes!!!")
	if len(hmacKey) < 32 || len(archiveKey) != 32 {
		t.Fatal("test key length")
	}
	fullManifestPath, fullManifestDigest := writeManifest(t, ctx, source, hmacKey, "full")
	if output, runErr := runDM01CLI(ctx, repoRoot, "preflight", 0, fullManifestPath, fullManifestDigest, *sourceDatabaseURL, strings.Replace(*sourceDatabaseURL, "127.0.0.1", "localhost", 1), hmacKey, archiveKey); runErr == nil {
		t.Fatal("same physical database accepted: " + string(output))
	}
	assertCount(t, ctx, target, `SELECT count(*) FROM legacy_contact_identity_import_runs`, 0)
	driftPath, driftDigest := driftManifest(t, fullManifestPath)
	if output, runErr := runDM01CLI(ctx, repoRoot, "preflight", 0, driftPath, driftDigest, *sourceDatabaseURL, *targetDatabaseURL, hmacKey, archiveKey); runErr == nil {
		t.Fatal("schema drift accepted: " + string(output))
	}
	assertCount(t, ctx, target, `SELECT count(*) FROM legacy_contact_identity_import_runs`, 0)
	if output, runErr := runDM01CLI(ctx, repoRoot, "incremental", 0, fullManifestPath, fullManifestDigest, *sourceDatabaseURL, *targetDatabaseURL, hmacKey, archiveKey); runErr == nil {
		t.Fatal("incremental accepted full table mode: " + string(output))
	}
	assertCount(t, ctx, target, `SELECT count(*) FROM legacy_contact_identity_import_runs`, 0)
	mustRun := func(mode string, parent int64, path, digest string) {
		t.Helper()
		if output, runErr := runDM01CLI(ctx, repoRoot, mode, parent, path, digest, *sourceDatabaseURL, *targetDatabaseURL, hmacKey, archiveKey); runErr != nil {
			t.Fatalf("%s: %v: %s", mode, runErr, output)
		}
	}
	mustRun("preflight", 0, fullManifestPath, fullManifestDigest)
	mustRun("full", 0, fullManifestPath, fullManifestDigest)
	assertCount(t, ctx, target, `SELECT count(*) FROM staff`, 1)
	assertCount(t, ctx, target, `SELECT count(*) FROM customers`, 1)
	assertCount(t, ctx, target, `SELECT count(*) FROM identities`, 1)
	mustRun("full", 0, fullManifestPath, fullManifestDigest)
	assertCount(t, ctx, target, `SELECT count(*) FROM legacy_contact_identity_import_runs WHERE mode='full'`, 1)
	assertStaleLeasePageRollback(t, ctx, target, hmacKey)
	boundPath, boundDigest := rewriteManifestSnapshot(t, fullManifestPath, "dm01-bound")
	assertBoundExcludesLateRow(t, ctx, repoRoot, source, target, boundPath, boundDigest, hmacKey, archiveKey)
	resetSource(t, ctx, source, schema, "Customer One", time.Date(2026, 8, 25, 1, 0, 0, 0, time.UTC))
	resumePath, resumeDigest := rewriteManifestSnapshot(t, fullManifestPath, "dm01-resume")
	assertExpiredRunResumes(t, ctx, repoRoot, target, resumePath, resumeDigest, hmacKey, archiveKey)
	var fullParent int64
	if err = target.QueryRow(ctx, `SELECT id FROM legacy_contact_identity_import_runs WHERE mode='full' AND snapshot_id='dm01-e2e'`).Scan(&fullParent); err != nil {
		t.Fatal(err)
	}
	if err = contactfixture.DeleteDM01Archives(ctx, target); err != nil {
		t.Fatal(err)
	}
	if output, runErr := runDM01CLI(ctx, repoRoot, "reconcile", fullParent, fullManifestPath, fullManifestDigest, *sourceDatabaseURL, *targetDatabaseURL, hmacKey, archiveKey); runErr == nil {
		t.Fatal("companion tamper reconciled: " + string(output))
	}
	assertCount(t, ctx, target, `SELECT count(*) FROM legacy_contact_identity_historical_archives`, 0)
	assertCount(t, ctx, target, `SELECT count(*) FROM legacy_contact_identity_import_receipts`, 0)
	if err = contactfixture.EditDM01CustomerName(ctx, target, "Operator Edit"); err != nil {
		t.Fatal(err)
	}
	resetSource(t, ctx, source, schema, "Source Edit", time.Date(2026, 8, 25, 2, 0, 0, 0, time.UTC))
	incrementalPath, incrementalDigest := writeManifest(t, ctx, source, hmacKey, "incremental")
	mustRun("incremental", 0, incrementalPath, incrementalDigest)
	var customerName string
	if err = target.QueryRow(ctx, `SELECT name FROM customers`).Scan(&customerName); err != nil || customerName != "Operator Edit" {
		t.Fatalf("target drift overwritten: %q/%v", customerName, err)
	}
	assertCount(t, ctx, target, `SELECT count(*) FROM legacy_contact_identity_import_quarantines q JOIN legacy_contact_identity_import_runs r ON r.id=q.run_id WHERE r.mode='incremental' AND q.reason_code='target_drift_since_last_import'`, 1)
	var parent int64
	if err = target.QueryRow(ctx, `SELECT id FROM legacy_contact_identity_import_runs WHERE mode='incremental' AND state='imported'`).Scan(&parent); err != nil {
		t.Fatal(err)
	}
	assertArchiveDecrypts(t, ctx, target, hmacKey, archiveKey, parent)
	mustRun("reconcile", parent, incrementalPath, incrementalDigest)
	assertCount(t, ctx, target, `SELECT count(*) FROM event_log`, 0)
	assertCount(t, ctx, target, `SELECT count(*) FROM customer_merges`, 0)
	assertCount(t, ctx, target, `SELECT count(*) FROM pending_events`, 0)
	assertCount(t, ctx, target, `SELECT count(*) FROM outbound_task_job_links`, 0)
	assertCount(t, ctx, target, `SELECT count(*) FROM admin_ops_jobs`, 0)
	assertCount(t, ctx, target, `SELECT count(*) FROM order_export_jobs`, 0)

	var preflighted, imported, reconciled, staff, customers, identities, receipts, checkpoints, archives int
	queries := []struct {
		query  string
		target *int
	}{
		{`SELECT count(*) FROM legacy_contact_identity_import_runs WHERE state='preflighted'`, &preflighted},
		{`SELECT count(*) FROM legacy_contact_identity_import_runs WHERE state='imported'`, &imported},
		{`SELECT count(*) FROM legacy_contact_identity_import_runs WHERE state='reconciled'`, &reconciled},
		{`SELECT count(*) FROM staff WHERE wecom_userid='staff-1'`, &staff}, {`SELECT count(*) FROM customers`, &customers},
		{`SELECT count(*) FROM identities WHERE kind='wecom_external_userid'`, &identities},
		{`SELECT count(*) FROM legacy_contact_identity_import_row_receipts`, &receipts},
		{`SELECT count(*) FROM legacy_contact_identity_import_checkpoints`, &checkpoints},
		{`SELECT count(*) FROM legacy_contact_identity_historical_archives`, &archives},
	}
	for _, item := range queries {
		if err = target.QueryRow(ctx, item.query).Scan(item.target); err != nil {
			t.Fatal(err)
		}
	}
	if preflighted != 1 || imported != 4 || reconciled != 1 || staff != 1 || customers != 1 || identities != 1 || receipts != 16 || checkpoints != 66 || archives != 1 {
		t.Fatalf("states=%d/%d/%d roots=%d/%d/%d receipts=%d checkpoints=%d archives=%d", preflighted, imported, reconciled, staff, customers, identities, receipts, checkpoints, archives)
	}
}

func assertPG16(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var version int
	if err := pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&version); err != nil || version/10000 != 16 {
		t.Fatalf("PostgreSQL 16 required: %d/%v", version, err)
	}
}
func repositoryRoot(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return path
}
func jsonNumber(value int64) string { encoded, _ := json.Marshal(value); return string(encoded) }

func runDM01CLI(ctx context.Context, repoRoot, mode string, parent int64, manifestPath, manifestDigest, sourceURL, targetURL string, hmacKey, archiveKey []byte) ([]byte, error) {
	args := []string{"run", "./cmd/aicrm-dm01", "--mode=" + mode, "--source-manifest=" + manifestPath, "--manifest-sha256=" + manifestDigest}
	if parent > 0 {
		args = append(args, "--parent-run-id="+jsonNumber(parent))
	}
	command := exec.CommandContext(ctx, "go", args...)
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "GOWORK=off", "AICRM_DM01_SOURCE_DATABASE_URL="+sourceURL, "AICRM_DATABASE_URL="+targetURL, "AICRM_DM01_SOURCE_HMAC_KEY="+string(hmacKey), "AICRM_DM01_ARCHIVE_KEY="+string(archiveKey))
	return command.CombinedOutput()
}

func assertCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, query).Scan(&got); err != nil || got != want {
		t.Fatalf("count=%d want=%d err=%v query=%s", got, want, err, query)
	}
}

func driftManifest(t *testing.T, path string) (string, string) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest migration.Manifest
	if err = json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Tables[0].SchemaDigest = strings.Repeat("0", 64)
	payload, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	driftPath := filepath.Join(t.TempDir(), "drift-manifest.json")
	if err = os.WriteFile(driftPath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	return driftPath, hex.EncodeToString(digest[:])
}

func rewriteManifestSnapshot(t *testing.T, path, snapshot string) (string, string) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest migration.Manifest
	if err = json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.SnapshotID = snapshot
	payload, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	next := filepath.Join(t.TempDir(), snapshot+".json")
	if err = os.WriteFile(next, payload, 0600); err != nil {
		t.Fatal(err)
	}
	return next, hex.EncodeToString(digest[:])
}

func assertBoundExcludesLateRow(t *testing.T, ctx context.Context, repoRoot string, source, target *pgxpool.Pool, manifestPath, manifestDigest string, hmacKey, archiveKey []byte) {
	t.Helper()
	lock, err := target.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Rollback(ctx) }()
	if _, err = lock.Exec(ctx, `LOCK TABLE staff IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, runErr := runDM01CLI(ctx, repoRoot, "full", 0, manifestPath, manifestDigest, *sourceDatabaseURL, *targetDatabaseURL, hmacKey, archiveKey)
		done <- runErr
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var count int
		if err = target.QueryRow(ctx, `SELECT count(*) FROM legacy_contact_identity_import_runs WHERE snapshot_id='dm01-bound' AND state='importing'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bounded run did not reach importing")
		}
		time.Sleep(10 * time.Millisecond)
	}
	late := time.Date(2026, 8, 25, 3, 0, 0, 0, time.UTC)
	if _, err = source.CopyFrom(ctx, pgx.Identifier{"owner_role_map"}, []string{"userid", "display_name", "role", "active", "source", "raw_payload_json", "created_at", "updated_at"}, pgx.CopyFromRows([][]any{{"late-staff", "Late Staff", "owner", true, "legacy", "{}", late, late}})); err != nil {
		t.Fatal(err)
	}
	if err = lock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	assertCount(t, ctx, target, `SELECT count(*) FROM staff WHERE wecom_userid='late-staff'`, 0)
}

func assertExpiredRunResumes(t *testing.T, ctx context.Context, repoRoot string, target *pgxpool.Pool, manifestPath, manifestDigest string, hmacKey, archiveKey []byte) {
	t.Helper()
	digest, err := hex.DecodeString(manifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	token := bytesOf(88)
	var runID int64
	runID, err = contactfixture.CreateDM01ExpiredImportingRun(ctx, target, digest, migration.LegacyRepositorySHA, "dm01-resume", time.Date(2026, 8, 25, 1, 0, 0, 0, time.UTC), token)
	if err != nil {
		t.Fatal(err)
	}
	if output, runErr := runDM01CLI(ctx, repoRoot, "full", 0, manifestPath, manifestDigest, *sourceDatabaseURL, *targetDatabaseURL, hmacKey, archiveKey); runErr != nil {
		t.Fatalf("resume: %v: %s", runErr, output)
	}
	var gotID, generation int64
	var state string
	if err = target.QueryRow(ctx, `SELECT id,lease_generation,state FROM legacy_contact_identity_import_runs WHERE snapshot_id='dm01-resume'`).Scan(&gotID, &generation, &state); err != nil {
		t.Fatal(err)
	}
	if gotID != runID || generation != 2 || state != "imported" {
		t.Fatalf("resume=%d/%d/%s want=%d/2/imported", gotID, generation, state, runID)
	}
	assertCount(t, ctx, target, `SELECT count(*) FROM staff`, 1)
	assertCount(t, ctx, target, `SELECT count(*) FROM customers`, 1)
	assertCount(t, ctx, target, `SELECT count(*) FROM identities`, 1)
}

func assertArchiveDecrypts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, hmacKey, archiveKey []byte, runID int64) {
	t.Helper()
	var table string
	var sourceKey, payloadHMAC, fieldDigest, nonce, ciphertext []byte
	var keyVersion int16
	err := pool.QueryRow(ctx, `SELECT source_table,source_key_hmac,payload_hmac,field_digest,archive_nonce,archive_ciphertext,archive_key_version FROM legacy_contact_identity_historical_archives WHERE run_id=$1`, runID).Scan(&table, &sourceKey, &payloadHMAC, &fieldDigest, &nonce, &ciphertext, &keyVersion)
	if err != nil {
		t.Fatal(err)
	}
	aad, err := migration.ArchiveAAD(runID, table, sourceKey, payloadHMAC, fieldDigest, int(keyVersion))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := migration.DecryptArchiveBound(archiveKey, hmacKey, table, aad, nonce, ciphertext, payloadHMAC)
	if err != nil || !json.Valid(plain) {
		t.Fatalf("archive decrypt=%q/%v", plain, err)
	}
}

func assertStaleLeasePageRollback(t *testing.T, ctx context.Context, pool *pgxpool.Pool, digestKey []byte) {
	t.Helper()
	baseline := 0
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM staff`).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	service := migration.NewActiveRootService(platformstore.NewUnitOfWork(pool), contactstore.HistoricalImportRepository{}, identitystore.NewRepository())
	for index, kind := range []string{"token", "generation", "expired"} {
		t.Run("stale-"+kind, func(t *testing.T) {
			token := make([]byte, 32)
			for offset := range token {
				token[offset] = byte(index + 20)
			}
			expires := time.Now().Add(time.Hour)
			if kind == "expired" {
				expires = time.Now().Add(-time.Hour)
			}
			runID, createErr := contactfixture.CreateDM01StaleRun(ctx, pool, bytesOf(byte(index+50)), migration.LegacyRepositorySHA, "stale-"+kind, time.Date(2030, time.Month(index+1), 1, 0, 0, 0, 0, time.UTC), token, 2, expires)
			if createErr != nil {
				t.Fatal(createErr)
			}
			if stateErr := contactfixture.SetDM01RunState(ctx, pool, runID, "importing"); stateErr != nil {
				t.Fatal(stateErr)
			}
			passedToken := append([]byte(nil), token...)
			generation := int64(2)
			if kind == "token" {
				passedToken[0]++
			}
			if kind == "generation" {
				generation++
			}
			key, _ := migration.SourceKeyHMAC(digestKey, "owner_role_map", "stale-"+kind)
			payload, _ := migration.SourcePayloadHMAC(digestKey, "owner_role_map", []byte(`{"staff":"stale"}`))
			fields, _ := migration.SourceFieldsHMAC(digestKey, "owner_role_map", []byte(`{"staff":"stale"}`))
			now := time.Now().UTC()
			command := migration.ActiveRootsCommand{Fence: contactport.NonActiveLeaseFence{RunID: runID, Generation: generation, TokenHMAC: passedToken}, CorpID: "corp-1", HMACKeyVersion: 1, DigestKey: digestKey, Staff: []migration.StaffActiveRoot{{Source: contactport.HistoricalImportSourceFact{SourceKeyHMAC: key, PayloadHMAC: payload, FieldDigest: fields}, Target: contactport.HistoricalImportStaffFact{WeComUserID: "stale-" + kind, Name: "Stale", Active: true, CreatedAt: now, UpdatedAt: now}}}}
			if _, processErr := service.Process(ctx, command); processErr == nil {
				t.Fatal("stale lease accepted")
			}
			assertCount(t, ctx, pool, `SELECT count(*) FROM staff`, baseline)
			assertCount(t, ctx, pool, `SELECT count(*) FROM legacy_contact_identity_source_mappings WHERE first_run_id=`+jsonNumber(runID), 0)
			assertCount(t, ctx, pool, `SELECT count(*) FROM legacy_contact_identity_import_row_receipts WHERE run_id=`+jsonNumber(runID), 0)
			if stateErr := contactfixture.SetDM01RunState(ctx, pool, runID, "failed"); stateErr != nil {
				t.Fatal(stateErr)
			}
		})
	}
}

func bytesOf(value byte) []byte {
	result := make([]byte, 32)
	for index := range result {
		result[index] = value
	}
	return result
}

func resetTarget(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	err := contactfixture.ResetDM01(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
}

func resetSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema []byte, customerName string, updated time.Time) {
	t.Helper()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}
	seedSource(t, ctx, pool, customerName, updated)
}

func seedSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, customerName string, updated time.Time) {
	t.Helper()
	created := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	one := time.Date(2026, 8, 25, 1, 0, 0, 0, time.UTC)
	copyRow := func(table string, columns []string, row []any) {
		t.Helper()
		if _, err := pool.CopyFrom(ctx, pgx.Identifier{table}, columns, pgx.CopyFromRows([][]any{row})); err != nil {
			t.Fatal(err)
		}
	}
	copyRow("owner_role_map", []string{"userid", "display_name", "role", "active", "source", "raw_payload_json", "created_at", "updated_at"}, []any{"staff-1", "Staff One", "owner", true, "legacy", "{}", created, one})
	copyRow("crm_user_identity", []string{"unionid", "primary_external_userid", "external_userids_json", "primary_openid", "openids_json", "mobile", "mobile_normalized", "mobile_verified", "mobile_source", "customer_name", "remark", "description", "avatar", "gender", "profile_json", "primary_owner_userid", "follow_users_json", "legacy_person_id", "legacy_identity_map_ids_json", "legacy_sources_json", "identity_status", "unionid_resolved_at", "first_seen_at", "last_seen_at", "last_polled_at", "next_poll_at", "poll_attempt_count", "last_poll_error", "created_at", "updated_at"}, []any{"union-1", "external-1", "[]", "", "[]", "", "", false, "", customerName, "", "", "", nil, "{}", "staff-1", "[]", "", "[]", "[]", "active", nil, created, one, nil, nil, 0, "", created, updated})
	copyRow("wecom_external_contact_identity_map", []string{"id", "external_userid", "unionid", "openid", "follow_user_userid", "name", "status", "updated_at", "corp_id", "avatar", "gender", "raw_profile", "first_seen_at", "last_seen_at", "created_at"}, []any{int64(1), "external-1", "union-1", "", "staff-1", "Customer One", "active", one, "corp-1", "", nil, "{}", created, one, created})
	copyRow("crm_user_identity_merge_audit", []string{"id", "from_unionid", "to_unionid", "reason", "before_json", "after_json", "operator", "created_at"}, []any{int64(1), "from", "union-1", "historical", "{}", "{}", "operator", one})
}

func writeManifest(t *testing.T, ctx context.Context, pool *pgxpool.Pool, hmacKey []byte, tableMode string) (string, string) {
	t.Helper()
	specs := []struct{ name, pk, watermark, action string }{
		{"owner_role_map", "userid", "updated_at+userid", "import_staff"}, {"crm_user_identity", "unionid", "updated_at+unionid", "import_customer"}, {"wecom_external_contact_identity_map", "id", "updated_at+id", "bind_scoped_identity"},
		{"crm_user_identity_merge_audit", "id", "created_at+id", "archive_inactive"}, {"crm_user_identity_resolution_queue", "id", "updated_at+id", "archive_inactive"}, {"admin_wecom_directory_members", "id", "last_synced_at+id", "rebuild"},
		{"contacts", "id", "updated_at+id", "drop"}, {"crm_user_identity_conflicts", "id", "updated_at+id", "defer_quarantine"}, {"external_contact_bindings", "external_userid", "updated_at+external_userid", "rebuild"},
		{"people", "id", "updated_at+id", "defer_quarantine"}, {"wecom_external_contact_follow_users", "id", "updated_at+id", "defer_quarantine"},
	}
	manifest := migration.Manifest{ContractVersion: 1, SourceSystem: "legacy", LegacyRepositorySHA: migration.LegacyRepositorySHA, SnapshotID: "dm01-e2e", SingleCorp: true, WeComCorpID: "corp-1", HMACKeyVersion: 1}
	owner, err := migration.OwnerAllowlistHMAC(hmacKey, "staff-1")
	if err != nil {
		t.Fatal(err)
	}
	manifest.OwnerAllowlistHMACs = []string{hex.EncodeToString(owner)}
	for _, spec := range specs {
		rows, err := pool.Query(ctx, `SELECT a.attnum::integer,a.attname::text,pg_catalog.format_type(a.atttypid,a.atttypmod)::text,a.attnotnull FROM pg_catalog.pg_attribute a JOIN pg_catalog.pg_class c ON c.oid=a.attrelid JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname=$1 AND c.relkind IN ('r','p') AND a.attnum>0 AND NOT a.attisdropped ORDER BY a.attnum`, spec.name)
		if err != nil {
			t.Fatal(err)
		}
		var columns []migration.SourceColumn
		for rows.Next() {
			var column migration.SourceColumn
			if err = rows.Scan(&column.Ordinal, &column.Name, &column.DataType, &column.NotNull); err != nil {
				t.Fatal(err)
			}
			columns = append(columns, column)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		digest, err := migration.CanonicalSchemaDigest(columns)
		if err != nil {
			t.Fatal(err)
		}
		manifest.Tables = append(manifest.Tables, migration.Table{Name: spec.name, PrimaryKey: spec.pk, Watermark: spec.watermark, SchemaDigest: digest, Mode: tableMode, Action: spec.action})
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err = os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	return path, hex.EncodeToString(digest[:])
}
