-- This file documents the package-local PostgreSQL contract. RP01 is kept out
-- of the shared sqlc manifest so central generation remains a Root-owned lane.

-- name: CreateReleaseCandidate :one
INSERT INTO release_candidates (
  commit_sha, artifact_digest, manifest_digest, config_digest,
  target_schema_version, state, created_by, created_at
) VALUES ($1,$2,$3,$4,$5,'draft',$6,$7)
RETURNING id,commit_sha,artifact_digest,manifest_digest,config_digest,target_schema_version,state,created_by,created_at,prepared_at,activated_at,rollback_requested_at,rolled_back_at;

-- name: GetReleaseCandidate :one
SELECT id,commit_sha,artifact_digest,manifest_digest,config_digest,target_schema_version,state,created_by,created_at,prepared_at,activated_at,rollback_requested_at,rolled_back_at
FROM release_candidates WHERE id=$1;

-- name: LockReleaseCandidate :one
SELECT id,commit_sha,artifact_digest,manifest_digest,config_digest,target_schema_version,state,created_by,created_at,prepared_at,activated_at,rollback_requested_at,rolled_back_at
FROM release_candidates WHERE id=$1 FOR UPDATE;

-- name: TransitionReleaseCandidate :execrows
UPDATE release_candidates SET
  state=$3,
  prepared_at=CASE WHEN $3='prepared' THEN $4 ELSE prepared_at END,
  activated_at=CASE WHEN $3='activated' THEN $4 ELSE activated_at END,
  rollback_requested_at=CASE WHEN $3='rollback_pending' THEN $4 ELSE rollback_requested_at END,
  rolled_back_at=CASE WHEN $3='rolled_back' THEN $4 ELSE rolled_back_at END
WHERE id=$1 AND state=$2;

-- name: CreateReleasePrerequisite :one
INSERT INTO release_prerequisite_receipts(
  candidate_id,candidate_commit_sha,candidate_artifact_digest,candidate_manifest_digest,
  candidate_config_digest,candidate_schema_version,kind,evidence_sha,recorded_by,recorded_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id,candidate_id,candidate_commit_sha,candidate_artifact_digest,candidate_manifest_digest,
  candidate_config_digest,candidate_schema_version,kind,evidence_sha,recorded_by,recorded_at;

-- name: ListReleasePrerequisites :many
SELECT id,candidate_id,candidate_commit_sha,candidate_artifact_digest,candidate_manifest_digest,
  candidate_config_digest,candidate_schema_version,kind,evidence_sha,recorded_by,recorded_at
FROM release_prerequisite_receipts WHERE candidate_id=$1 ORDER BY kind;

-- name: StartReleaseWorker :one
INSERT INTO release_worker_leases(candidate_id,generation,fence,started_by,started_at,active)
SELECT $1,COALESCE(max(generation),0)+1,$2,$3,$4,TRUE
FROM release_worker_leases WHERE candidate_id=$1
RETURNING candidate_id,generation,fence,started_by,started_at,active,retired_at;

-- name: GetActiveReleaseWorker :one
SELECT candidate_id,generation,fence,started_by,started_at,active,retired_at
FROM release_worker_leases WHERE candidate_id=$1 AND active FOR UPDATE;

-- name: FindActiveReleaseWorkerSummary :one
SELECT candidate_id,generation,started_by,started_at
FROM release_worker_leases WHERE candidate_id=$1 AND active;

-- name: RetireReleaseWorker :execrows
UPDATE release_worker_leases SET active=FALSE,retired_at=$4
WHERE candidate_id=$1 AND generation=$2 AND fence=$3 AND active;

-- name: AppendReleaseCutoverStep :one
INSERT INTO release_cutover_journal(candidate_id,generation,step,fence,completed_by,completed_at)
VALUES($1,$2,$3,$4,$5,$6)
RETURNING id,candidate_id,generation,step,fence,completed_by,completed_at;

-- name: CreateReleaseRollbackCheck :one
INSERT INTO release_rollback_checks(candidate_id,kind,passed,evidence_sha,recorded_by,recorded_at)
VALUES($1,$2,$3,$4,$5,$6)
RETURNING id,candidate_id,kind,passed,evidence_sha,recorded_by,recorded_at;

-- name: ReserveReleaseOperationReceipt :one
INSERT INTO release_operation_receipts(action,actor_id,key_digest,payload_digest,created_at)
VALUES($1,$2,$3,$4,$5) ON CONFLICT(action,actor_id,key_digest) DO NOTHING
RETURNING id,action,actor_id,key_digest,payload_digest,state,result_snapshot;

-- name: LockReleaseOperationReceipt :one
SELECT id,action,actor_id,key_digest,payload_digest,state,result_snapshot
FROM release_operation_receipts WHERE action=$1 AND actor_id=$2 AND key_digest=$3 FOR UPDATE;

-- name: CompleteReleaseOperationReceipt :execrows
UPDATE release_operation_receipts SET state='completed',result_snapshot=$2,completed_at=$3
WHERE id=$1 AND state='in_progress';

-- name: GetReleaseOperationReceiptByID :one
SELECT id,action,actor_id,key_digest,payload_digest,state,result_snapshot,created_at,completed_at
FROM release_operation_receipts WHERE id=$1;
