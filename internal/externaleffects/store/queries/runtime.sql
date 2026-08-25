-- name: CreateEffect :one
INSERT INTO external_effects(owner,kind,source_ref_digest,target_ref_digest,payload_digest,policy_version_hash,envelope_fingerprint,state)
VALUES($1,$2,$3,$4,$5,$6,$7,'accepted')
RETURNING *;

-- name: LockEffect :one
SELECT * FROM external_effects WHERE id = $1 FOR UPDATE;

-- name: LockEffectByFingerprint :one
SELECT * FROM external_effects WHERE envelope_fingerprint = $1 FOR UPDATE;

-- name: ListEffects :many
SELECT * FROM external_effects ORDER BY id DESC LIMIT $1;

-- name: GetEffect :one
SELECT * FROM external_effects WHERE id = $1;

-- name: GetTerminalOutcome :one
SELECT e.owner, e.kind, e.state, a.number AS attempt_number, a.generation, a.fence,
       a.completed_at AS lease_expires_at, a.receipt_digest, r.id AS receipt_id
FROM external_effects e
JOIN LATERAL (
  SELECT number, generation, fence, completed_at, receipt_digest
  FROM external_effect_attempts
  WHERE effect_id = e.id AND completion IS NOT NULL
  ORDER BY number DESC
  LIMIT 1
) a ON true
JOIN external_effect_receipts r
  ON r.effect_id = e.id
 AND r.receipt_key_digest = a.receipt_digest
 AND r.operation IN ('complete_attempt', 'recover_attempted')
WHERE e.id = $1
  AND e.state IN ('executed','outcome_unknown','reconciled','retryable_failed','final_failed');

-- name: QueueEffect :exec
UPDATE external_effects
SET state='queued', generation=generation+1, lease_fence=0, lease_expires_at=NULL,
    river_job_id=$2, river_queue=$3, river_args_digest=$4, river_scheduled_at=$5, updated_at=now()
WHERE id=$1;

-- name: ClaimEffect :one
UPDATE external_effects
SET lease_fence=lease_fence+1, lease_expires_at=now()+interval '30 seconds', updated_at=now()
WHERE id=$1
RETURNING lease_fence, lease_expires_at;

-- name: CancelEffect :exec
UPDATE external_effects SET state='cancelled', lease_expires_at=NULL, updated_at=now() WHERE id=$1;

-- name: StartAttempt :one
UPDATE external_effects SET state='attempted', attempt_count=attempt_count+1, updated_at=now() WHERE id=$1
RETURNING attempt_count, updated_at;

-- name: InsertAttempt :exec
INSERT INTO external_effect_attempts(effect_id,number,generation,fence,started_at) VALUES($1,$2,$3,$4,$5);

-- name: CompleteAttempt :exec
UPDATE external_effect_attempts
SET completion=$4, receipt_digest=$5, completed_at=now()
WHERE effect_id=$1 AND number=$2 AND generation=$3 AND completion IS NULL;

-- name: CompleteChannelAcquisitionAssetAttempt :exec
UPDATE external_effect_attempts
SET completion=$4, receipt_digest=$5, result_reference_digest=sqlc.narg(result_reference_digest)::text,
    business_call_dispatched=sqlc.arg(business_call_dispatched)::boolean,
    real_external_call_executed=sqlc.arg(real_external_call_executed)::boolean, completed_at=now()
WHERE effect_id=$1 AND number=$2 AND generation=$3 AND completion IS NULL;

-- name: GetChannelAcquisitionAssetTerminalEvidence :one
SELECT result_reference_digest, business_call_dispatched, real_external_call_executed
FROM external_effect_attempts
WHERE effect_id = sqlc.arg(effect_id)::bigint
  AND number = sqlc.arg(attempt_number)::integer
  AND generation = sqlc.arg(generation)::bigint
  AND fence = sqlc.arg(fence)::bigint
  AND completion IS NOT NULL;

-- name: ReconcileAttempt :exec
INSERT INTO external_effect_reconciliations(effect_id,generation,fence,evidence_digest) VALUES($1,$2,$3,$4);

-- name: MarkEffectReconciled :exec
UPDATE external_effects SET state='reconciled', updated_at=now() WHERE id=$1;

-- name: LockOpenAttempt :one
SELECT number FROM external_effect_attempts
WHERE effect_id=$1 AND generation=$2 AND fence=$3 AND completion IS NULL
FOR UPDATE;

-- name: MarkAttemptOutcomeUnknown :exec
UPDATE external_effect_attempts
SET completion='outcome_unknown', receipt_digest=$2, completed_at=now()
WHERE effect_id=$1 AND number=$3;

-- name: UpdateEffectState :exec
UPDATE external_effects SET state=$2, lease_expires_at=NULL, updated_at=now() WHERE id=$1;

-- name: GetReceipt :one
SELECT * FROM external_effect_receipts
WHERE operation=$1 AND effect_id IS NOT DISTINCT FROM $2 AND receipt_key_digest=$3;

-- name: GetAcceptReceipt :one
SELECT * FROM external_effect_receipts WHERE operation='accept' AND receipt_key_digest=$1;

-- name: InsertReceipt :one
INSERT INTO external_effect_receipts(operation,effect_id,receipt_key_digest,command_digest,state)
VALUES($1,$2,$3,$4,$5)
RETURNING *;

-- name: GetDiagnostics :one
SELECT
  COUNT(*) FILTER (WHERE state='accepted') AS accepted,
  COUNT(*) FILTER (WHERE state='queued') AS queued,
  COUNT(*) FILTER (WHERE state='attempted') AS attempted,
  COUNT(*) FILTER (WHERE state='outcome_unknown') AS outcome_unknown,
  COUNT(*) FILTER (WHERE state='retryable_failed') AS retryable_failed
FROM external_effects;

-- name: CreatePE01AcceptanceEffect :one
INSERT INTO external_effects (
  owner, kind, source_ref_digest, target_ref_digest, payload_digest,
  policy_version_hash, envelope_fingerprint, state
) VALUES (
  'order', sqlc.arg(kind), sqlc.arg(source_ref_digest),
  sqlc.arg(target_ref_digest), sqlc.arg(payload_digest),
  sqlc.arg(policy_version_hash), sqlc.arg(envelope_fingerprint), sqlc.arg(state)
)
RETURNING id;
