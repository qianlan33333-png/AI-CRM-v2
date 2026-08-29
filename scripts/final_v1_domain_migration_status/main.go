// final_v1_domain_migration_status performs narrow read-only guards. The DSN
// is read only from the process environment, never command arguments/output.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"

	"github.com/jackc/pgx/v5/pgxpool"
)

var shaPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)
var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "final-v1-domain-migration-status:", err)
		os.Exit(2)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("final-v1-domain-migration-status", flag.ContinueOnError)
	check := flags.String("check", "", "schema|external-effects|archive|journals-empty")
	expect := flags.String("expect", "", "expected scalar")
	archiveRunID := flags.String("archive-run-id", "", "archive run")
	expectedSHA := flags.String("expected-sha", "", "archive source SHA")
	_ = flags.String("source-slice", "", "outer runner checks its seal")
	sourceSeal := flags.String("source-seal-sha256", "", "source slice SHA-256 seal")
	_ = flags.String("runtime-env-file", "", "outer runner validates this file")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected positional arguments")
	}
	databaseURL := os.Getenv("AICRM_DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("AICRM_DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("target database connection setup failed")
	}
	defer pool.Close()
	switch *check {
	case "schema":
		var expected int64
		if _, err := fmt.Sscan(*expect, &expected); err != nil || expected < 1 {
			return fmt.Errorf("schema check requires a positive expected version")
		}
		var missingOrUnapplied, beyondApplied int64
		if err := pool.QueryRow(ctx, `WITH latest AS (
			SELECT DISTINCT ON (version_id) version_id, is_applied
			FROM goose_db_version
			ORDER BY version_id, id DESC
		)
		SELECT
			count(*) FILTER (WHERE latest.is_applied IS DISTINCT FROM true),
			(SELECT count(*) FROM latest WHERE version_id > $1 AND is_applied)
		FROM generate_series(1, $1) AS expected_version
		LEFT JOIN latest ON latest.version_id=expected_version`, expected).Scan(&missingOrUnapplied, &beyondApplied); err != nil {
			return fmt.Errorf("target database schema query failed")
		}
		if missingOrUnapplied != 0 || beyondApplied != 0 {
			return fmt.Errorf("target database does not have exactly schema %d", expected)
		}
	case "external-effects":
		if *expect != "0" {
			return fmt.Errorf("external effects check requires expect=0")
		}
		var count int64
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.external_effects`).Scan(&count); err != nil {
			return fmt.Errorf("target database effects query failed")
		}
		if count != 0 {
			return fmt.Errorf("expected external_effects=0, found %d", count)
		}
	case "archive":
		if *archiveRunID == "" || !shaPattern.MatchString(*expectedSHA) || !sha256Pattern.MatchString(*sourceSeal) {
			return fmt.Errorf("archive check requires archive-run-id, expected-sha, and source-seal-sha256")
		}
		var phase, sourceSHA, snapshotDigest string
		var reconciled bool
		err := pool.QueryRow(ctx, `SELECT r.phase, a.source_repository_sha,
			encode(a.snapshot_digest, 'hex'),
			EXISTS (SELECT 1 FROM public.v1_archive_reconciliation_receipts receipt WHERE receipt.run_id=a.run_id)
			FROM public.data_migration_runs r JOIN public.v1_archive_runs a ON a.run_id=r.run_id
			WHERE a.run_id=$1`, *archiveRunID).Scan(&phase, &sourceSHA, &snapshotDigest, &reconciled)
		if err != nil {
			return fmt.Errorf("target database archive query failed")
		}
		if phase != "reconciled" || sourceSHA != *expectedSHA || snapshotDigest != *sourceSeal || !reconciled {
			return fmt.Errorf("archive run is not reconciled and sealed for the expected SHA")
		}
	case "journals-empty":
		if *archiveRunID == "" {
			return fmt.Errorf("journals-empty check requires archive-run-id")
		}
		var receipts, reconciliations int64
		if err := pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM public.v1_domain_import_receipts WHERE archive_run_id=$1),
			(SELECT count(*) FROM public.v1_domain_import_reconciliation_receipts WHERE archive_run_id=$1)`,
			*archiveRunID).Scan(&receipts, &reconciliations); err != nil {
			return fmt.Errorf("target database journal query failed")
		}
		if receipts != 0 || reconciliations != 0 {
			return fmt.Errorf("domain import journals must be empty for archive run")
		}
	default:
		return fmt.Errorf("check must be schema, external-effects, archive, or journals-empty")
	}
	return nil
}
