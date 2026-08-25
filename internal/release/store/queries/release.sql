-- name: CreateReleaseCandidate :one
INSERT INTO release_candidates (commit_sha, artifact_digest, manifest_digest, config_digest, target_schema_version, state, created_by, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;
-- name: GetReleaseCandidate :one
SELECT * FROM release_candidates WHERE id=$1;
-- name: ListReleaseCandidates :many
SELECT * FROM release_candidates ORDER BY id DESC LIMIT $1;
-- name: UpdateReleaseCandidateState :one
UPDATE release_candidates SET state=$2, prepared_at=CASE WHEN $2='prepared' THEN $3 ELSE prepared_at END, activated_at=CASE WHEN $2='activated' THEN $3 ELSE activated_at END, rollback_requested_at=CASE WHEN $2='rollback_pending' THEN $3 ELSE rollback_requested_at END, rolled_back_at=CASE WHEN $2='rolled_back' THEN $3 ELSE rolled_back_at END WHERE id=$1 RETURNING *;
-- name: CreateReleasePrerequisite :one
INSERT INTO release_prerequisite_receipts (candidate_id,kind,evidence_sha,recorded_by,recorded_at) VALUES ($1,$2,$3,$4,$5) RETURNING *;
-- name: ListReleasePrerequisites :many
SELECT * FROM release_prerequisite_receipts WHERE candidate_id=$1 ORDER BY kind;
-- name: StartReleaseWorker :one
INSERT INTO release_worker_leases (candidate_id,generation,fence,started_by,started_at,active) VALUES ($1,1,$2,$3,$4,true) RETURNING *;
-- name: GetReleaseWorker :one
SELECT * FROM release_worker_leases WHERE candidate_id=$1;
-- name: AppendReleaseCutoverStep :one
INSERT INTO release_cutover_journal (candidate_id,step,fence,completed_by,completed_at) VALUES ($1,$2,$3,$4,$5) RETURNING *;
-- name: ListReleaseCutoverSteps :many
SELECT * FROM release_cutover_journal WHERE candidate_id=$1 ORDER BY id;
-- name: ReserveReleaseOperationReceipt :one
INSERT INTO release_operation_receipts (action,actor_id,key_digest,payload_digest,created_at) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (action,actor_id,key_digest) DO NOTHING RETURNING *;
-- name: GetReleaseOperationReceipt :one
SELECT * FROM release_operation_receipts WHERE action=$1 AND actor_id=$2 AND key_digest=$3;
-- name: CompleteReleaseOperationReceipt :one
UPDATE release_operation_receipts SET state='completed', result_snapshot=$2, completed_at=$3 WHERE id=$1 AND state='in_progress' RETURNING *;
