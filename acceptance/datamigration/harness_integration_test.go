package datamigration_acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	migration "github.com/qianlan33333-png/AI-CRM-v2/internal/migration"
	migrationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/migration/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var databaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 data-migration harness database")

var errInjectedSourceCrash = errors.New("injected source crash")

type sourceFixture struct {
	mu        sync.RWMutex
	identity  string
	schema    migration.Digest
	table     migration.TableID
	tableHash migration.Digest
	bound     []byte
	rows      []migration.SourceRow
	failAfter int
	failed    bool
}

func (source *sourceFixture) Preflight(context.Context) (migration.SourcePreflight, error) {
	source.mu.RLock()
	defer source.mu.RUnlock()
	return migration.SourcePreflight{
		Identity: source.identity, SchemaDigest: source.schema,
		Bounds: []migration.TableBound{{
			Table: source.table, SourceIdentity: "legacy." + string(source.table), SchemaDigest: source.tableHash,
			UpperBound: migration.UpperBound{Value: append([]byte(nil), source.bound...)},
		}},
	}, nil
}

func (source *sourceFixture) Stream(_ context.Context, request migration.StreamRequest, each func(migration.SourceRow) error) (migration.StreamResult, error) {
	source.mu.RLock()
	rows := append([]migration.SourceRow(nil), source.rows...)
	failAfter, alreadyFailed := source.failAfter, source.failed
	source.mu.RUnlock()
	start := 0
	if request.After != "" {
		for start < len(rows) && rows[start].Cursor != request.After {
			start++
		}
		if start == len(rows) {
			return migration.StreamResult{}, migration.ErrUnboundedStream
		}
		start++
	}
	end := start + request.Limit
	if end > len(rows) {
		end = len(rows)
	}
	for index, row := range rows[start:end] {
		if err := each(row); err != nil {
			return migration.StreamResult{}, err
		}
		if failAfter > 0 && !alreadyFailed && index+1 == failAfter {
			source.mu.Lock()
			source.failed = true
			source.mu.Unlock()
			return migration.StreamResult{}, errInjectedSourceCrash
		}
	}
	return migration.StreamResult{Complete: end == len(rows)}, nil
}

func (source *sourceFixture) replaceRow(index int, row migration.SourceRow) {
	source.mu.Lock()
	defer source.mu.Unlock()
	source.rows[index] = row
}

type cursorCodec struct{}

func (cursorCodec) Compare(left, right migration.Cursor) (int, error) {
	switch {
	case left < right:
		return -1, nil
	case left > right:
		return 1, nil
	default:
		return 0, nil
	}
}

type fixturePayload struct {
	Adapter string `json:"adapter"`
	ID      int64  `json:"id"`
	Value   string `json:"value"`
}

type fixtureMapper struct{ adapter migration.AdapterID }

func (fixtureMapper) MappingDigest(migration.TableID) migration.Digest {
	return migration.Sum([]byte("fixture mapper v1"))
}

func (mapper fixtureMapper) Map(_ context.Context, _ migration.TableSpec, row migration.SourceRow) (migration.MappedRow, error) {
	var payload fixturePayload
	if err := json.Unmarshal(row.Payload, &payload); err != nil || payload.Adapter != string(mapper.adapter) || payload.ID <= 0 || payload.Value == "" {
		return migration.MappedRow{}, migration.ErrInvalidRun
	}
	return migration.MappedRow{Operation: "fixture.upsert", Payload: append([]byte(nil), row.Payload...), Digest: migration.Sum(row.Payload)}, nil
}

type harness struct {
	runner     *migration.Service
	reconciler *migration.ReconcileService
	repository *migrationstore.Repository
}

func TestDataMigrationHarnessPG16(t *testing.T) {
	if *databaseURL == "" {
		t.Skip("-database-url is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	assertCatalog(t, ctx, pool)
	if _, err = pool.Exec(ctx, `CREATE SCHEMA acceptance_fixtures;
CREATE TABLE acceptance_fixtures.data_migration_fixture_targets(
adapter_id TEXT NOT NULL,id BIGINT NOT NULL,value TEXT NOT NULL,mutation_digest BYTEA NOT NULL CHECK(octet_length(mutation_digest)=32),
PRIMARY KEY(adapter_id,id),UNIQUE(adapter_id,mutation_digest))`); err != nil {
		t.Fatal(err)
	}
	target := newFixtureTarget(t)

	t.Run("run exact replay reconcile and readiness", func(t *testing.T) {
		source := newSource("happy", 0, "one", "two", "three")
		h := newHarness(t, pool, target, source, migration.DispositionImport, time.Second, 2)
		result, runErr := h.runner.Run(ctx, migration.RunRequest{ID: "happy-first", Adapter: "happy"})
		if runErr != nil || result.Imported != 3 {
			t.Fatalf("first run=%#v err=%v", result, runErr)
		}
		if err = h.reconciler.Reconcile(ctx, "happy-first"); err != nil {
			t.Fatal(err)
		}
		ready, readyErr := h.repository.Readiness(ctx, "happy-first")
		if readyErr != nil || !ready.Ready || ready.ProcessedRows != 3 || ready.ResultRows != 3 || ready.QuarantinedRows != 0 {
			t.Fatalf("readiness=%#v err=%v", ready, readyErr)
		}
		replay, replayErr := h.runner.Run(ctx, migration.RunRequest{ID: "happy-replay", Adapter: "happy"})
		if replayErr != nil || replay.Replayed != 3 || replay.Imported != 0 {
			t.Fatalf("replay=%#v err=%v", replay, replayErr)
		}
		assertTargetCount(t, ctx, pool, "happy", 3)
		source.replaceRow(1, fixtureRow("happy", 2, "changed"))
		if _, err = h.runner.Run(ctx, migration.RunRequest{ID: "happy-conflict", Adapter: "happy"}); !errors.Is(err, migration.ErrSourcePayloadConflict) {
			t.Fatalf("payload conflict err=%v", err)
		}
		assertTargetCount(t, ctx, pool, "happy", 3)
	})

	t.Run("crash resumes from durable cursor with new generation", func(t *testing.T) {
		source := newSource("resume", 1, "one", "two", "three")
		const leaseDuration = 2 * time.Second
		h := newHarness(t, pool, target, source, migration.DispositionImport, leaseDuration, 2)
		if _, runErr := h.runner.Run(ctx, migration.RunRequest{ID: "resume-run", Adapter: "resume"}); !errors.Is(runErr, errInjectedSourceCrash) {
			t.Fatalf("first crash err=%v", runErr)
		}
		state, loadErr := h.repository.Load(ctx, "resume-run")
		if loadErr != nil || state.Tables["records"].Processed != 1 || state.Tables["records"].Cursor != "000001" {
			t.Fatalf("crash state=%#v err=%v", state, loadErr)
		}
		time.Sleep(leaseDuration + 100*time.Millisecond)
		resumed, resumeErr := h.runner.Run(ctx, migration.RunRequest{ID: "resume-run", Adapter: "resume"})
		if resumeErr != nil || resumed.Imported != 2 {
			t.Fatalf("resume=%#v err=%v", resumed, resumeErr)
		}
		if err = h.reconciler.Reconcile(ctx, "resume-run"); err != nil {
			t.Fatal(err)
		}
		assertTargetCount(t, ctx, pool, "resume", 3)
		var generations int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM data_migration_run_leases WHERE run_id='resume-run'`).Scan(&generations); err != nil || generations < 3 {
			t.Fatalf("lease generations=%d err=%v", generations, err)
		}
	})

	t.Run("source and target compare fail closed", func(t *testing.T) {
		source := newSource("source-tamper", 0, "one")
		h := newHarness(t, pool, target, source, migration.DispositionImport, time.Second, 2)
		if _, err = h.runner.Run(ctx, migration.RunRequest{ID: "source-tamper-run", Adapter: "source-tamper"}); err != nil {
			t.Fatal(err)
		}
		source.replaceRow(0, fixtureRow("source-tamper", 1, "changed"))
		if err = h.reconciler.Reconcile(ctx, "source-tamper-run"); !errors.Is(err, migration.ErrTargetTampered) {
			t.Fatalf("source tamper err=%v", err)
		}

		targetSource := newSource("target-tamper", 0, "one")
		targetHarness := newHarness(t, pool, target, targetSource, migration.DispositionImport, time.Second, 2)
		if _, err = targetHarness.runner.Run(ctx, migration.RunRequest{ID: "target-tamper-run", Adapter: "target-tamper"}); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `UPDATE acceptance_fixtures.data_migration_fixture_targets SET mutation_digest=$1 WHERE adapter_id='target-tamper'`, bytesOf(migration.Sum([]byte("tampered")))); err != nil {
			t.Fatal(err)
		}
		if err = targetHarness.reconciler.Reconcile(ctx, "target-tamper-run"); !errors.Is(err, migration.ErrTargetTampered) {
			t.Fatalf("target tamper err=%v", err)
		}
	})

	t.Run("quarantine is reconciled but blocks cutover", func(t *testing.T) {
		source := newSource("quarantine", 0, "unresolved")
		h := newHarness(t, pool, target, source, migration.DispositionQuarantine, time.Second, 2)
		if _, err = h.runner.Run(ctx, migration.RunRequest{ID: "quarantine-run", Adapter: "quarantine"}); err != nil {
			t.Fatal(err)
		}
		if err = h.reconciler.Reconcile(ctx, "quarantine-run"); err != nil {
			t.Fatal(err)
		}
		ready, readyErr := h.repository.Readiness(ctx, "quarantine-run")
		if readyErr != nil || ready.Ready || ready.QuarantinedRows != 1 || !ready.Reconciled {
			t.Fatalf("quarantine readiness=%#v err=%v", ready, readyErr)
		}
		replay, replayErr := h.runner.Run(ctx, migration.RunRequest{ID: "quarantine-replay", Adapter: "quarantine"})
		if replayErr != nil || replay.Replayed != 1 {
			t.Fatalf("quarantine replay=%#v err=%v", replay, replayErr)
		}
		if err = h.reconciler.Reconcile(ctx, "quarantine-replay"); err != nil {
			t.Fatal(err)
		}
		replayReady, replayReadyErr := h.repository.Readiness(ctx, "quarantine-replay")
		if replayReadyErr != nil || replayReady.Ready || replayReady.QuarantinedRows != 1 {
			t.Fatalf("quarantine replay readiness=%#v err=%v", replayReady, replayReadyErr)
		}
		assertTargetCount(t, ctx, pool, "quarantine", 0)
	})

	t.Run("parallel runs serialize each source key", func(t *testing.T) {
		source := newSource("parallel", 0, "one", "two")
		h := newHarness(t, pool, target, source, migration.DispositionImport, time.Second, 2)
		type outcome struct {
			result migration.RunResult
			err    error
		}
		outcomes := make(chan outcome, 2)
		for index := 0; index < 2; index++ {
			go func(index int) {
				result, runErr := h.runner.Run(ctx, migration.RunRequest{ID: migration.RunID(fmt.Sprintf("parallel-%d", index)), Adapter: "parallel"})
				outcomes <- outcome{result: result, err: runErr}
			}(index)
		}
		var imported, replayed int
		for index := 0; index < 2; index++ {
			outcome := <-outcomes
			if outcome.err != nil {
				t.Fatal(outcome.err)
			}
			imported += outcome.result.Imported
			replayed += outcome.result.Replayed
		}
		if imported != 2 || replayed != 2 {
			t.Fatalf("parallel imported/replayed=%d/%d", imported, replayed)
		}
		assertTargetCount(t, ctx, pool, "parallel", 2)
	})

	t.Run("database guards immutable facts and stale fences", func(t *testing.T) {
		_, updateErr := pool.Exec(ctx, `UPDATE data_migration_run_tables SET processed=0,cursor=NULL WHERE run_id='happy-first' AND table_id='records'`)
		assertSQLState(t, updateErr, "55000")
		_, deleteErr := pool.Exec(ctx, `DELETE FROM data_migration_row_receipts WHERE adapter_id='happy'`)
		assertSQLState(t, deleteErr, "55000")
		var staleFence []byte
		if err = pool.QueryRow(ctx, `SELECT fence FROM data_migration_run_leases WHERE run_id='resume-run' AND generation=1`).Scan(&staleFence); err != nil {
			t.Fatal(err)
		}
		staleDigest := bytesOf(migration.Sum([]byte("stale-fence-probe")))
		_, staleErr := pool.Exec(ctx, `INSERT INTO data_migration_row_receipts(
adapter_id,table_id,source_key_digest,payload_digest,field_digest,disposition,mapping_digest,policy_digest,operation,mutation_digest,run_id,lease_generation,lease_fence
) VALUES('stale-probe','records',$1,$1,$1,'skip',$1,$1,'',$1,'resume-run',1,$2)`, staleDigest, staleFence)
		assertSQLState(t, staleErr, "55000")
		var rawColumns int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
WHERE table_schema='public' AND table_name LIKE 'data_migration_%'
  AND column_name IN ('payload','raw_payload','source_row','dsn','credential')`).Scan(&rawColumns); err != nil || rawColumns != 0 {
			t.Fatalf("raw control-plane columns=%d err=%v", rawColumns, err)
		}
	})
}

func newSource(adapter string, failAfter int, values ...string) *sourceFixture {
	table := migration.TableID("records")
	rows := make([]migration.SourceRow, 0, len(values))
	for index, value := range values {
		rows = append(rows, fixtureRow(adapter, int64(index+1), value))
	}
	return &sourceFixture{
		identity: adapter + "-source", schema: migration.Sum([]byte(adapter + "-source-schema")),
		table: table, tableHash: migration.Sum([]byte(adapter + "-table-schema")),
		bound: []byte(fmt.Sprintf("%s:%06d", adapter, len(values))), rows: rows, failAfter: failAfter,
	}
}

func fixtureRow(adapter string, id int64, value string) migration.SourceRow {
	payload, err := json.Marshal(fixturePayload{Adapter: adapter, ID: id, Value: value})
	if err != nil {
		panic(err)
	}
	return migration.SourceRow{
		Cursor: migration.Cursor(fmt.Sprintf("%06d", id)), SourceKey: []byte(fmt.Sprintf("%s:%d", adapter, id)),
		Payload: payload, FieldDigest: migration.Sum([]byte(fmt.Sprintf("%s:%d:%s", adapter, id, value))),
	}
}

func newHarness(t *testing.T, pool *pgxpool.Pool, target *migrationstore.Target, source *sourceFixture, disposition migration.Disposition, ttl time.Duration, limit int) harness {
	t.Helper()
	adapter := migration.AdapterID(source.identity[:len(source.identity)-len("-source")])
	policyID := migration.PolicyID(string(adapter) + "-policy")
	policy := migration.Policy{ID: policyID, Disposition: disposition}
	policyDigest, err := policy.Digest()
	if err != nil {
		t.Fatal(err)
	}
	manifest := migration.AdapterManifest{
		ID: adapter, Family: migration.FamilyPlatform, SourceIdentity: source.identity, SourceSchemaDigest: source.schema,
		Tables: []migration.TableSpec{{
			ID: source.table, SourceIdentity: "legacy." + string(source.table), SchemaDigest: source.tableHash,
			MappingDigest: migration.Sum([]byte("fixture mapper v1")), PolicyDigest: policyDigest,
			PrimaryKey: "id", Watermark: "updated_at,id", Mode: migration.ModeSnapshot, Policy: policyID,
		}},
	}
	mappings, err := migration.NewStaticMappingRegistry(migration.AdapterDefinition{Manifest: manifest, Source: source, Mapper: fixtureMapper{adapter: adapter}, Cursors: cursorCodec{}})
	if err != nil {
		t.Fatal(err)
	}
	policies, err := migration.NewStaticPolicyRegistry(policy)
	if err != nil {
		t.Fatal(err)
	}
	repository := migrationstore.NewRepository(pool)
	uow := platformstore.NewUnitOfWork(pool)
	runner := migration.NewServiceWithRuntime(realClock{}, ttl, limit, uow, repository, repository, repository, target, repository, repository, mappings, policies)
	reconciler := migration.NewFullReconcileServiceWithRuntime(realClock{}, ttl, limit, uow, repository, repository, repository, repository, target, mappings, policies)
	return harness{runner: runner, reconciler: reconciler, repository: repository}
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

func newFixtureTarget(t *testing.T) *migrationstore.Target {
	t.Helper()
	registry, err := migrationstore.NewTargetRegistry(
		map[string]migrationstore.ApplyOperation{"fixture.upsert": func(ctx context.Context, tx pgx.Tx, _ migration.LeaseFence, raw []byte) error {
			var payload fixturePayload
			if err := json.Unmarshal(raw, &payload); err != nil || payload.Adapter == "" || payload.ID <= 0 || payload.Value == "" {
				return migration.ErrInvalidRun
			}
			mutation := migration.Sum(raw)
			if _, err := tx.Exec(ctx, `INSERT INTO acceptance_fixtures.data_migration_fixture_targets(adapter_id,id,value,mutation_digest)
VALUES($1,$2,$3,$4) ON CONFLICT(adapter_id,id) DO NOTHING`, payload.Adapter, payload.ID, payload.Value, bytesOf(mutation)); err != nil {
				return err
			}
			var storedValue string
			var storedDigest []byte
			if err := tx.QueryRow(ctx, `SELECT value,mutation_digest FROM acceptance_fixtures.data_migration_fixture_targets WHERE adapter_id=$1 AND id=$2`, payload.Adapter, payload.ID).Scan(&storedValue, &storedDigest); err != nil {
				return err
			}
			if storedValue != payload.Value || !bytes.Equal(storedDigest, mutation[:]) {
				return migration.ErrTargetTampered
			}
			return nil
		}},
		map[string]migrationstore.VerifyOperation{"fixture.upsert": func(ctx context.Context, tx pgx.Tx, receipt migration.ResultReceipt) error {
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM acceptance_fixtures.data_migration_fixture_targets WHERE mutation_digest=$1`, bytesOf(receipt.MutationDigest)).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return migration.ErrTargetTampered
			}
			return nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return migrationstore.NewTarget(registry)
}

func assertCatalog(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var version string
	var waterline, tables, invalidConstraints, invalidIndexes int
	err := pool.QueryRow(ctx, `SELECT current_setting('server_version_num'),
  (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
  (SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename LIKE 'data_migration_%'),
  (SELECT count(*) FROM pg_constraint WHERE conrelid IN (
    'data_migration_runs'::regclass,'data_migration_run_tables'::regclass,'data_migration_run_leases'::regclass,
    'data_migration_row_receipts'::regclass,'data_migration_result_receipts'::regclass,
    'data_migration_quarantines'::regclass,'data_migration_archives'::regclass,'data_migration_reconciliation_receipts'::regclass
  ) AND NOT convalidated),
  (SELECT count(*) FROM pg_index WHERE indrelid IN (
    'data_migration_runs'::regclass,'data_migration_run_tables'::regclass,'data_migration_run_leases'::regclass,
    'data_migration_row_receipts'::regclass,'data_migration_result_receipts'::regclass,
    'data_migration_quarantines'::regclass,'data_migration_archives'::regclass,'data_migration_reconciliation_receipts'::regclass
  ) AND NOT indisvalid)`).Scan(&version, &waterline, &tables, &invalidConstraints, &invalidIndexes)
	if err != nil || version != "160014" || waterline != 76 || tables != 8 || invalidConstraints != 0 || invalidIndexes != 0 {
		t.Fatalf("catalog version/waterline/tables/invalid=%s/%d/%d/%d/%d err=%v", version, waterline, tables, invalidConstraints, invalidIndexes, err)
	}
}

func assertTargetCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, adapter string, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM acceptance_fixtures.data_migration_fixture_targets WHERE adapter_id=$1`, adapter).Scan(&count); err != nil || count != want {
		t.Fatalf("target count adapter=%s got=%d want=%d err=%v", adapter, count, want, err)
	}
}

func assertSQLState(t *testing.T, err error, state string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != state {
		t.Fatalf("SQL error=%v state=%v want=%s", err, pgErr, state)
	}
}

func bytesOf(value migration.Digest) []byte { return append([]byte(nil), value[:]...) }
