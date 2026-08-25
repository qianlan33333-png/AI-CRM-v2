-- name: InsertRun :one
INSERT INTO data_migration_runs(run_id,adapter_id,source_identity,source_schema_digest,manifest_digest)
VALUES($1,$2,$3,$4,$5)
ON CONFLICT(run_id) DO NOTHING
RETURNING run_id;

-- name: InsertRunTable :exec
INSERT INTO data_migration_run_tables(run_id,table_id,ordinal,source_identity,schema_digest,upper_bound,upper_bound_empty)
VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: GetRun :one
SELECT * FROM data_migration_runs WHERE run_id=$1;

-- name: LockRun :one
SELECT * FROM data_migration_runs WHERE run_id=$1 FOR UPDATE;

-- name: ListRunTables :many
SELECT * FROM data_migration_run_tables WHERE run_id=$1 ORDER BY ordinal;

-- name: LockRunPhaseNow :one
SELECT phase, clock_timestamp()::timestamptz AS database_now FROM data_migration_runs WHERE run_id=$1 FOR UPDATE;

-- name: LockActiveLease :one
SELECT * FROM data_migration_run_leases WHERE run_id=$1 AND active FOR UPDATE;

-- name: RetireActiveLease :exec
UPDATE data_migration_run_leases SET active=FALSE,retired_at=$2 WHERE run_id=$1 AND generation=$3 AND active;

-- name: NextLeaseGeneration :one
SELECT (COALESCE(max(generation),0)+1)::bigint FROM data_migration_run_leases WHERE run_id=$1;

-- name: InsertLease :exec
INSERT INTO data_migration_run_leases(run_id,generation,fence,acquired_at,expires_at) VALUES($1,$2,$3,$4,$5);

-- name: LockLeaseExpiry :one
SELECT expires_at FROM data_migration_run_leases WHERE run_id=$1 AND generation=$2 AND fence=$3 AND active FOR UPDATE;

-- name: DatabaseNow :one
SELECT clock_timestamp()::timestamptz AS database_now;

-- name: RenewLeaseExpiry :exec
UPDATE data_migration_run_leases SET expires_at=$4 WHERE run_id=$1 AND generation=$2 AND fence=$3 AND active;

-- name: AdvanceCheckpoint :execrows
UPDATE data_migration_run_tables SET cursor=$5,processed=$6,complete=$7,last_lease_generation=$3,last_lease_fence=$4
WHERE run_id=$1 AND table_id=$2
  AND aicrm_data_migration_active_fence($1,$3,$4)
  AND upper_bound IS NOT DISTINCT FROM $8 AND upper_bound_empty=$9 AND schema_digest=$10 AND source_identity=$11;

-- name: LockLeaseStatusNow :one
SELECT active,expires_at,clock_timestamp()::timestamptz AS database_now
FROM data_migration_run_leases WHERE run_id=$1 AND generation=$2 AND fence=$3 FOR UPDATE;

-- name: RetireLease :exec
UPDATE data_migration_run_leases SET active=FALSE,retired_at=$4 WHERE run_id=$1 AND generation=$2 AND fence=$3 AND active;

-- name: CompleteRun :execrows
UPDATE data_migration_runs SET phase='completed',completed_at=$2 WHERE run_id=$1 AND phase='running';

-- name: MarkRunReconciled :execrows
UPDATE data_migration_runs SET phase='reconciled',reconciled_at=$2 WHERE run_id=$1 AND phase='completed';

-- name: LockRowReceiptAdvisory :exec
SELECT pg_advisory_xact_lock(hashtextextended($1,0));

-- name: GetRowReceipt :one
SELECT * FROM data_migration_row_receipts WHERE adapter_id=$1 AND table_id=$2 AND source_key_digest=$3;

-- name: InsertRowReceipt :exec
INSERT INTO data_migration_row_receipts(adapter_id,table_id,source_key_digest,payload_digest,field_digest,disposition,mapping_digest,policy_digest,operation,mutation_digest,run_id,lease_generation,lease_fence)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13);

-- name: GetResultReceipt :one
SELECT * FROM data_migration_result_receipts WHERE run_id=$1 AND adapter_id=$2 AND table_id=$3 AND source_key_digest=$4;

-- name: InsertResultReceipt :exec
INSERT INTO data_migration_result_receipts(run_id,adapter_id,table_id,source_key_digest,payload_digest,field_digest,disposition,mapping_digest,policy_digest,operation,mutation_digest,outcome,lease_generation,lease_fence)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14);

-- name: ListResultReceipts :many
SELECT * FROM data_migration_result_receipts WHERE run_id=$1 ORDER BY table_id,source_key_digest;

-- name: InsertQuarantine :exec
INSERT INTO data_migration_quarantines(run_id,adapter_id,table_id,source_key_digest,payload_digest,field_digest,reason,lease_generation,lease_fence)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: InsertArchive :exec
INSERT INTO data_migration_archives(run_id,adapter_id,table_id,source_key_digest,payload_digest,field_digest,lease_generation,lease_fence)
VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: InsertReconciliationReceipt :exec
INSERT INTO data_migration_reconciliation_receipts(run_id,source_row_count,result_row_count,target_verified_count,comparison_digest,lease_generation,lease_fence)
VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: GetReadiness :one
SELECT r.run_id,r.phase,
  count(*) FILTER (WHERE NOT t.complete) AS pending_tables,
  COALESCE(sum(t.processed),0)::bigint AS processed_rows,
  (SELECT count(*) FROM data_migration_result_receipts rr WHERE rr.run_id=r.run_id) AS result_rows,
  (SELECT count(*) FROM data_migration_result_receipts qr WHERE qr.run_id=r.run_id AND qr.disposition='quarantine') AS quarantined_rows,
  EXISTS(SELECT 1 FROM data_migration_reconciliation_receipts x WHERE x.run_id=r.run_id) AS reconciled
FROM data_migration_runs r JOIN data_migration_run_tables t ON t.run_id=r.run_id
WHERE r.run_id=$1 GROUP BY r.run_id,r.phase;
