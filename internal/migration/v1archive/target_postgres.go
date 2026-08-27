package v1archive

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresTarget implements the 00107 archive contract. Its SQL contains no
// source identifiers: table and column names are stored as values, while all
// archive payloads are encrypted before this type receives them.
type PostgresTarget struct {
	pool   *pgxpool.Pool
	leases map[string]targetLease
}

type targetLease struct {
	generation int64
	fence      [sha256.Size]byte
}

func OpenPostgresTarget(ctx context.Context, dsn string) (*PostgresTarget, error) {
	if dsn == "" {
		return nil, ErrInvalidConfig
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open V1 archive target: %w", err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping V1 archive target: %w", err)
	}
	return &PostgresTarget{pool: pool, leases: make(map[string]targetLease)}, nil
}

func (target *PostgresTarget) Close() {
	if target != nil && target.pool != nil {
		target.pool.Close()
	}
}

func (target *PostgresTarget) Identity(ctx context.Context) (SourceIdentity, error) {
	if target == nil || target.pool == nil {
		return SourceIdentity{}, ErrInvalidConfig
	}
	var identity SourceIdentity
	err := target.pool.QueryRow(ctx, "SELECT system_identifier::text, current_database(), current_user FROM pg_control_system()").Scan(&identity.SystemID, &identity.Database, &identity.Role)
	if err != nil {
		return SourceIdentity{}, fmt.Errorf("identify V1 archive target: %w", err)
	}
	return identity, identity.Validate()
}

func (target *PostgresTarget) EnsureRun(ctx context.Context, run Run, manifest Manifest) error {
	if target == nil || target.pool == nil {
		return ErrInvalidConfig
	}
	if err := run.Validate(); err != nil {
		return err
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	digest, err := manifest.Digest()
	if err != nil || digest != run.SchemaDigest {
		return ErrRunConflict
	}
	targetIdentity, err := target.Identity(ctx)
	if err != nil {
		return err
	}
	if run.Source.Equal(targetIdentity) {
		return ErrSameDatabase
	}
	tx, err := target.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = ensureGenericRun(ctx, tx, run); err != nil {
		return err
	}
	if err = ensureGenericTables(ctx, tx, run, manifest); err != nil {
		return err
	}
	lease, err := acquireLease(ctx, tx, run.ID)
	if err != nil {
		return err
	}
	if err = ensureArchiveManifest(ctx, tx, run, manifest, lease); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	target.leases[run.ID] = lease
	return nil
}

func ensureGenericRun(ctx context.Context, tx pgx.Tx, run Run) error {
	identity := run.Source.SystemID + "/" + run.Source.Database + "/" + run.Source.Role
	_, err := tx.Exec(ctx, `
INSERT INTO data_migration_runs (run_id, adapter_id, source_identity, source_schema_digest, manifest_digest)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (run_id) DO NOTHING`, run.ID, run.AdapterID, identity, run.SchemaDigest[:], run.SourceDumpDigest[:])
	if err != nil {
		return fmt.Errorf("create V1 archive control run: %w", err)
	}
	var foundAdapter, foundIdentity, phase string
	var foundSchema, foundManifest []byte
	err = tx.QueryRow(ctx, `
SELECT adapter_id, source_identity, source_schema_digest, manifest_digest, phase
FROM data_migration_runs WHERE run_id = $1 FOR UPDATE`, run.ID).Scan(&foundAdapter, &foundIdentity, &foundSchema, &foundManifest, &phase)
	if err != nil {
		return fmt.Errorf("read V1 archive control run: %w", err)
	}
	if foundAdapter != run.AdapterID || foundIdentity != identity || !equalBytes(foundSchema, run.SchemaDigest[:]) || !equalBytes(foundManifest, run.SourceDumpDigest[:]) || phase != "running" {
		return ErrRunConflict
	}
	return nil
}

func ensureArchiveManifest(ctx context.Context, tx pgx.Tx, run Run, manifest Manifest, lease targetLease) error {
	var total int64
	for _, table := range manifest.Tables {
		total += table.RowCount
	}
	_, err := tx.Exec(ctx, `
INSERT INTO v1_archive_runs (run_id, source_repository_sha, snapshot_digest, schema_digest, policy_digest, key_version, table_count, row_count, lease_generation, lease_fence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (run_id) DO NOTHING`, run.ID, run.RepositorySHA, run.SnapshotDigest[:], run.SchemaDigest[:], run.PolicyDigest[:], run.ArchiveKeyVersion, len(manifest.Tables), total, lease.generation, lease.fence[:])
	if err != nil {
		return fmt.Errorf("create V1 archive run summary: %w", err)
	}
	var repositorySHA string
	var snapshot, schema, policy []byte
	var keyVersion, tableCount int
	var rowCount int64
	err = tx.QueryRow(ctx, `
SELECT source_repository_sha, snapshot_digest, schema_digest, policy_digest, key_version, table_count, row_count
FROM v1_archive_runs WHERE run_id = $1`, run.ID).Scan(&repositorySHA, &snapshot, &schema, &policy, &keyVersion, &tableCount, &rowCount)
	if err != nil {
		return fmt.Errorf("read V1 archive run summary: %w", err)
	}
	if repositorySHA != run.RepositorySHA || !equalBytes(snapshot, run.SnapshotDigest[:]) || !equalBytes(schema, run.SchemaDigest[:]) || !equalBytes(policy, run.PolicyDigest[:]) || keyVersion != run.ArchiveKeyVersion || tableCount != len(manifest.Tables) || rowCount != total {
		return ErrRunConflict
	}
	for ordinal, table := range manifest.Tables {
		schemaDigest, err := tableDigest(table)
		if err != nil {
			return err
		}
		columns, err := json.Marshal(table.Columns)
		if err != nil {
			return err
		}
		primaryKey, err := json.Marshal(table.PrimaryKey)
		if err != nil {
			return err
		}
		tableID := archiveTableID(table.Name)
		_, err = tx.Exec(ctx, `
INSERT INTO v1_archive_tables (run_id, table_id, ordinal, schema_digest, row_count, column_manifest, pk_columns, disposition, lease_generation, lease_fence)
VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, 'archive', $8, $9)
ON CONFLICT (run_id, table_id) DO NOTHING`, run.ID, tableID, ordinal, schemaDigest[:], table.RowCount, columns, primaryKey, lease.generation, lease.fence[:])
		if err != nil {
			return fmt.Errorf("create V1 archive table summary %s: %w", tableID, err)
		}
		var foundOrdinal int
		var foundSchema, foundColumns, foundPrimaryKey []byte
		var foundRows int64
		var foundDisposition string
		err = tx.QueryRow(ctx, `
SELECT ordinal, schema_digest, row_count, column_manifest, pk_columns, disposition
FROM v1_archive_tables WHERE run_id = $1 AND table_id = $2`, run.ID, tableID).Scan(&foundOrdinal, &foundSchema, &foundRows, &foundColumns, &foundPrimaryKey, &foundDisposition)
		if err != nil {
			return err
		}
		if foundOrdinal != ordinal || !equalBytes(foundSchema, schemaDigest[:]) || foundRows != table.RowCount || !equalJSON(foundColumns, columns) || !equalJSON(foundPrimaryKey, primaryKey) || foundDisposition != "archive" {
			return ErrRunConflict
		}
	}
	return nil
}

func ensureGenericTables(ctx context.Context, tx pgx.Tx, run Run, manifest Manifest) error {
	for ordinal, table := range manifest.Tables {
		schema, err := tableDigest(table)
		if err != nil {
			return err
		}
		empty := table.RowCount == 0
		var bound []byte
		if !empty {
			bound = tableSnapshotBound(table)
		}
		_, err = tx.Exec(ctx, `
INSERT INTO data_migration_run_tables (run_id, table_id, ordinal, source_identity, schema_digest, upper_bound, upper_bound_empty)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (run_id, table_id) DO NOTHING`, run.ID, archiveTableID(table.Name), ordinal, archiveTableID(table.Name), schema[:], bound, empty)
		if err != nil {
			return fmt.Errorf("register V1 archive checkpoint %s: %w", table.Name, err)
		}
	}
	return nil
}

func tableSnapshotBound(table Table) []byte {
	digest := sha256.Sum256([]byte(fmt.Sprintf("aicrm/v1archive/snapshot-bound/v1\x00%s\x00%d", table.Name, table.RowCount)))
	return digest[:]
}

func acquireLease(ctx context.Context, tx pgx.Tx, runID string) (targetLease, error) {
	_, err := tx.Exec(ctx, `
UPDATE data_migration_run_leases
SET active = FALSE, retired_at = statement_timestamp()
WHERE run_id = $1 AND active AND expires_at <= statement_timestamp()`, runID)
	if err != nil {
		return targetLease{}, fmt.Errorf("retire expired V1 archive lease: %w", err)
	}
	var existing bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM data_migration_run_leases WHERE run_id = $1 AND active)`, runID).Scan(&existing); err != nil {
		return targetLease{}, err
	}
	if existing {
		return targetLease{}, ErrRunConflict
	}
	var fence [sha256.Size]byte
	if _, err = io.ReadFull(rand.Reader, fence[:]); err != nil {
		return targetLease{}, err
	}
	var generation int64
	err = tx.QueryRow(ctx, `
INSERT INTO data_migration_run_leases (run_id, generation, fence, acquired_at, expires_at)
VALUES ($1, COALESCE((SELECT max(generation) + 1 FROM data_migration_run_leases WHERE run_id = $1), 1), $2, statement_timestamp(), statement_timestamp() + interval '30 minutes')
RETURNING generation`, runID, fence[:]).Scan(&generation)
	if err != nil {
		return targetLease{}, fmt.Errorf("acquire V1 archive lease: %w", err)
	}
	return targetLease{generation: generation, fence: fence}, nil
}

func (target *PostgresTarget) WriteBatch(ctx context.Context, records []Record) error {
	if target == nil || target.pool == nil || len(records) == 0 {
		return ErrInvalidConfig
	}
	runID := records[0].RunID
	lease, exists := target.leases[runID]
	if !exists {
		return ErrRunConflict
	}
	tx, err := target.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = renewLease(ctx, tx, runID, lease); err != nil {
		return err
	}
	for _, record := range records {
		if record.RunID != runID || record.Ordinal < 1 || record.Table == "" || record.ArchiveKeyVersion < 1 || len(record.Nonce) != 12 || len(record.Ciphertext) <= 16 {
			return ErrInvalidConfig
		}
		if err = writeRecord(ctx, tx, lease, record); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func renewLease(ctx context.Context, tx pgx.Tx, runID string, lease targetLease) error {
	command, err := tx.Exec(ctx, `
UPDATE data_migration_run_leases
SET expires_at = statement_timestamp() + interval '30 minutes'
WHERE run_id = $1 AND generation = $2 AND fence = $3 AND active AND expires_at > statement_timestamp()`, runID, lease.generation, lease.fence[:])
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrRunConflict
	}
	return nil
}

func writeRecord(ctx context.Context, tx pgx.Tx, lease targetLease, record Record) error {
	tableID := archiveTableID(record.Table)
	mapping := archiveMappingDigest(record.Table, record.SchemaDigest)
	mutation := archiveMutationDigest(record)
	policy := ArchivePolicyDigest()
	_, err := tx.Exec(ctx, `
INSERT INTO data_migration_row_receipts (adapter_id, table_id, source_key_digest, payload_digest, field_digest, disposition, mapping_digest, policy_digest, operation, mutation_digest, run_id, lease_generation, lease_fence)
VALUES ($1, $2, $3, $4, $5, 'archive', $6, $7, '', $8, $9, $10, $11)
ON CONFLICT (adapter_id, table_id, source_key_digest) DO NOTHING`, DefaultAdapterID, tableID, record.SourceKeyHMAC[:], record.PayloadHMAC[:], record.FieldHMAC[:], mapping[:], policy[:], mutation[:], record.RunID, lease.generation, lease.fence[:])
	if err != nil {
		return fmt.Errorf("write V1 archive row receipt: %w", err)
	}
	var foundPayload, foundField, foundMapping, foundPolicy, foundMutation []byte
	var disposition, operation string
	err = tx.QueryRow(ctx, `
SELECT payload_digest, field_digest, mapping_digest, policy_digest, mutation_digest, disposition, operation
FROM data_migration_row_receipts
WHERE adapter_id = $1 AND table_id = $2 AND source_key_digest = $3`, DefaultAdapterID, tableID, record.SourceKeyHMAC[:]).Scan(&foundPayload, &foundField, &foundMapping, &foundPolicy, &foundMutation, &disposition, &operation)
	if err != nil {
		return err
	}
	if !equalBytes(foundPayload, record.PayloadHMAC[:]) || !equalBytes(foundField, record.FieldHMAC[:]) || !equalBytes(foundMapping, mapping[:]) || !equalBytes(foundPolicy, policy[:]) || !equalBytes(foundMutation, mutation[:]) || disposition != "archive" || operation != "" {
		return ErrPayloadConflict
	}
	redactedPaths := record.RedactedPaths
	if redactedPaths == nil {
		redactedPaths = []string{}
	}
	metadata, err := json.Marshal(redactedPaths)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO v1_archive_records (adapter_id, table_id, source_key_digest, run_id, source_ordinal, payload_digest, field_digest, schema_digest, nonce, ciphertext, key_version, compression, redaction_metadata, lease_generation, lease_fence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'none', $12::jsonb, $13, $14)
ON CONFLICT (adapter_id, table_id, source_key_digest) DO NOTHING`, DefaultAdapterID, tableID, record.SourceKeyHMAC[:], record.RunID, record.Ordinal, record.PayloadHMAC[:], record.FieldHMAC[:], record.SchemaDigest[:], record.Nonce, record.Ciphertext, record.ArchiveKeyVersion, metadata, lease.generation, lease.fence[:])
	if err != nil {
		return fmt.Errorf("write encrypted V1 archive record: %w", err)
	}
	var foundRunID string
	var foundOrdinal int64
	var foundPayloadRecord, foundFieldRecord, foundSchema []byte
	var foundKeyVersion int
	var foundCompression string
	var foundMetadata []byte
	err = tx.QueryRow(ctx, `
SELECT run_id, source_ordinal, payload_digest, field_digest, schema_digest, key_version, compression, redaction_metadata
FROM v1_archive_records
WHERE adapter_id = $1 AND table_id = $2 AND source_key_digest = $3`, DefaultAdapterID, tableID, record.SourceKeyHMAC[:]).Scan(&foundRunID, &foundOrdinal, &foundPayloadRecord, &foundFieldRecord, &foundSchema, &foundKeyVersion, &foundCompression, &foundMetadata)
	if err != nil {
		return err
	}
	if foundRunID != record.RunID || foundOrdinal != record.Ordinal || !equalBytes(foundPayloadRecord, record.PayloadHMAC[:]) || !equalBytes(foundFieldRecord, record.FieldHMAC[:]) || !equalBytes(foundSchema, record.SchemaDigest[:]) || foundKeyVersion != record.ArchiveKeyVersion || foundCompression != "none" || !equalJSON(foundMetadata, metadata) {
		return ErrPayloadConflict
	}
	return nil
}

func archiveTableID(table string) string { return "public/" + table }

func equalJSON(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	leftCanonical, leftErr := json.Marshal(leftValue)
	rightCanonical, rightErr := json.Marshal(rightValue)
	return leftErr == nil && rightErr == nil && equalBytes(leftCanonical, rightCanonical)
}

func archiveMappingDigest(table string, schema [sha256.Size]byte) [sha256.Size]byte {
	return sha256.Sum256(append([]byte("aicrm/v1archive/mapping/v1\x00"+table+"\x00"), schema[:]...))
}

func archiveMutationDigest(record Record) [sha256.Size]byte {
	buffer := make([]byte, 0, sha256.Size*4+16)
	buffer = append(buffer, []byte("aicrm/v1archive/mutation/v1\x00")...)
	buffer = append(buffer, record.SourceKeyHMAC[:]...)
	buffer = append(buffer, record.PayloadHMAC[:]...)
	buffer = append(buffer, record.FieldHMAC[:]...)
	buffer = append(buffer, record.SchemaDigest[:]...)
	return sha256.Sum256(buffer)
}

func (target *PostgresTarget) Complete(ctx context.Context, summary Summary) error {
	if target == nil || target.pool == nil || summary.Validate() != nil {
		return ErrInvalidConfig
	}
	lease, exists := target.leases[summary.RunID]
	if !exists {
		return ErrRunConflict
	}
	tx, err := target.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = renewLease(ctx, tx, summary.RunID, lease); err != nil {
		return err
	}
	for _, table := range summary.Tables {
		var expected int64
		if err = tx.QueryRow(ctx, `SELECT row_count FROM v1_archive_tables WHERE run_id = $1 AND table_id = $2`, summary.RunID, archiveTableID(table.Table)).Scan(&expected); err != nil {
			return err
		}
		if expected != table.Count {
			return ErrPayloadConflict
		}
		var cursor any
		if table.Count > 0 {
			cursor = hex.EncodeToString(table.Digest[:])
		}
		command, updateErr := tx.Exec(ctx, `
UPDATE data_migration_run_tables
SET processed = $1, cursor = $2, complete = TRUE, last_lease_generation = $3, last_lease_fence = $4
WHERE run_id = $5 AND table_id = $6`, table.Count, cursor, lease.generation, lease.fence[:], summary.RunID, archiveTableID(table.Table))
		if updateErr != nil {
			return updateErr
		}
		if command.RowsAffected() != 1 {
			return ErrRunConflict
		}
	}
	if err = retireLease(ctx, tx, summary.RunID, lease); err != nil {
		return err
	}
	command, err := tx.Exec(ctx, `
UPDATE data_migration_runs
SET phase = 'completed', completed_at = statement_timestamp()
WHERE run_id = $1 AND phase = 'running'`, summary.RunID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrRunConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	delete(target.leases, summary.RunID)
	return nil
}

func retireLease(ctx context.Context, tx pgx.Tx, runID string, lease targetLease) error {
	command, err := tx.Exec(ctx, `
UPDATE data_migration_run_leases
SET active = FALSE, retired_at = statement_timestamp()
WHERE run_id = $1 AND generation = $2 AND fence = $3 AND active`, runID, lease.generation, lease.fence[:])
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrRunConflict
	}
	return nil
}

func (target *PostgresTarget) Run(ctx context.Context, runID string) (Run, bool, error) {
	if target == nil || target.pool == nil || runID == "" {
		return Run{}, false, ErrInvalidConfig
	}
	var run Run
	var sourceIdentity string
	var phase string
	var snapshotDigest, schemaDigest, policyDigest, sourceDumpDigest []byte
	err := target.pool.QueryRow(ctx, `
SELECT control.run_id, control.adapter_id, control.source_identity, archive.source_repository_sha, archive.snapshot_digest, archive.schema_digest, archive.policy_digest, archive.key_version, control.manifest_digest, control.phase
FROM data_migration_runs AS control
JOIN v1_archive_runs AS archive ON archive.run_id = control.run_id
WHERE control.run_id = $1`, runID).Scan(&run.ID, &run.AdapterID, &sourceIdentity, &run.RepositorySHA, &snapshotDigest, &schemaDigest, &policyDigest, &run.ArchiveKeyVersion, &sourceDumpDigest, &phase)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, err
	}
	if phase != "completed" {
		return Run{}, false, ErrRunConflict
	}
	if len(snapshotDigest) != sha256.Size || len(schemaDigest) != sha256.Size || len(policyDigest) != sha256.Size || len(sourceDumpDigest) != sha256.Size {
		return Run{}, false, ErrRunConflict
	}
	copy(run.SnapshotDigest[:], snapshotDigest)
	copy(run.SchemaDigest[:], schemaDigest)
	copy(run.PolicyDigest[:], policyDigest)
	copy(run.SourceDumpDigest[:], sourceDumpDigest)
	parts := strings.SplitN(sourceIdentity, "/", 3)
	if len(parts) != 3 {
		return Run{}, false, ErrRunConflict
	}
	run.Source = SourceIdentity{SystemID: parts[0], Database: parts[1], Role: parts[2]}
	return run, true, run.Validate()
}

func (target *PostgresTarget) Summary(ctx context.Context, runID string) (Summary, error) {
	if target == nil || target.pool == nil || runID == "" {
		return Summary{}, ErrInvalidConfig
	}
	rows, err := target.pool.Query(ctx, `
SELECT table_id, source_ordinal, source_key_digest, payload_digest, field_digest
FROM v1_archive_records
WHERE run_id = $1
ORDER BY table_id, source_ordinal`, runID)
	if err != nil {
		return Summary{}, err
	}
	defer rows.Close()
	accumulators := make(map[string]*summaryAccumulator)
	for rows.Next() {
		var tableID string
		var ordinal int64
		var sourceKey, payload, fields []byte
		if err = rows.Scan(&tableID, &ordinal, &sourceKey, &payload, &fields); err != nil {
			return Summary{}, err
		}
		table, err := tableNameFromID(tableID)
		if err != nil {
			return Summary{}, err
		}
		accumulator := accumulators[table]
		if accumulator == nil {
			accumulator = newSummaryAccumulator(table)
			accumulators[table] = accumulator
		}
		var record Record
		record.Table, record.Ordinal = table, ordinal
		copy(record.SourceKeyHMAC[:], sourceKey)
		copy(record.PayloadHMAC[:], payload)
		copy(record.FieldHMAC[:], fields)
		if err = accumulator.Add(record); err != nil {
			return Summary{}, err
		}
	}
	if err = rows.Err(); err != nil {
		return Summary{}, err
	}
	tables, err := target.pool.Query(ctx, `SELECT table_id, row_count FROM v1_archive_tables WHERE run_id = $1 ORDER BY ordinal`, runID)
	if err != nil {
		return Summary{}, err
	}
	defer tables.Close()
	result := Summary{RunID: runID}
	for tables.Next() {
		var tableID string
		var expected int64
		if err = tables.Scan(&tableID, &expected); err != nil {
			return Summary{}, err
		}
		table, err := tableNameFromID(tableID)
		if err != nil {
			return Summary{}, err
		}
		accumulator := accumulators[table]
		if accumulator == nil {
			accumulator = newSummaryAccumulator(table)
		}
		summary := accumulator.Summary(runID)
		if summary.Count != expected {
			return Summary{}, ErrReconciliationFailed
		}
		result.Tables = append(result.Tables, summary)
	}
	if err = tables.Err(); err != nil {
		return Summary{}, err
	}
	if err = result.Validate(); err != nil {
		return Summary{}, err
	}
	return result, nil
}

func tableNameFromID(tableID string) (string, error) {
	const prefix = "public/"
	if len(tableID) <= len(prefix) || tableID[:len(prefix)] != prefix {
		return "", ErrRunConflict
	}
	return tableID[len(prefix):], nil
}

func (target *PostgresTarget) Reconcile(ctx context.Context, sourceSummary Summary) error {
	if target == nil || target.pool == nil || sourceSummary.Validate() != nil {
		return ErrInvalidConfig
	}
	targetSummary, err := target.Summary(ctx, sourceSummary.RunID)
	if err != nil {
		return err
	}
	if err = RequireReconciled(sourceSummary, targetSummary); err != nil {
		return err
	}
	tx, err := target.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	lease, err := acquireLease(ctx, tx, sourceSummary.RunID)
	if err != nil {
		return err
	}
	comparison := summaryDigest(sourceSummary)
	_, err = tx.Exec(ctx, `
INSERT INTO v1_archive_reconciliation_receipts (run_id, source_table_count, archived_table_count, source_row_count, archive_record_count, terminal_disposition_count, canonical_mapping_count, target_verified_count, comparison_digest, lease_generation, lease_fence)
VALUES ($1, $2, $3, $4, $5, $6, 0, 0, $7, $8, $9)
ON CONFLICT (run_id) DO NOTHING`, sourceSummary.RunID, len(sourceSummary.Tables), len(targetSummary.Tables), sourceSummary.TotalCount(), targetSummary.TotalCount(), targetSummary.TotalCount(), comparison[:], lease.generation, lease.fence[:])
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO data_migration_reconciliation_receipts (run_id, source_row_count, result_row_count, target_verified_count, comparison_digest, lease_generation, lease_fence)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (run_id) DO NOTHING`, sourceSummary.RunID, sourceSummary.TotalCount(), targetSummary.TotalCount(), targetSummary.TotalCount(), comparison[:], lease.generation, lease.fence[:])
	if err != nil {
		return err
	}
	if err = retireLease(ctx, tx, sourceSummary.RunID, lease); err != nil {
		return err
	}
	command, err := tx.Exec(ctx, `
UPDATE data_migration_runs
SET phase = 'reconciled', reconciled_at = statement_timestamp()
WHERE run_id = $1 AND phase = 'completed'`, sourceSummary.RunID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrRunConflict
	}
	return tx.Commit(ctx)
}

func summaryDigest(summary Summary) [sha256.Size]byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte("aicrm/v1archive/reconcile/v1\x00" + summary.RunID + "\x00"))
	for _, table := range summary.Tables {
		_, _ = hash.Write([]byte(table.Table))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(table.Digest[:])
	}
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result
}

var _ TargetWriter = (*PostgresTarget)(nil)
