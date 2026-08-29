-- name: BindSurveyExternalPush :one
INSERT INTO questionnaire_submission_external_push_bindings(questionnaire_id,public_submission_id,customer_id,external_effect_id,source_ref_digest,target_ref_digest,payload_digest,policy_version_hash)
VALUES(sqlc.arg(questionnaire_id),sqlc.arg(public_submission_id),sqlc.arg(customer_id),sqlc.arg(external_effect_id),sqlc.arg(source_ref_digest),sqlc.arg(target_ref_digest),sqlc.arg(payload_digest),sqlc.arg(policy_version_hash))
ON CONFLICT (public_submission_id) DO UPDATE SET public_submission_id=EXCLUDED.public_submission_id
RETURNING id,questionnaire_id,public_submission_id,customer_id,external_effect_id,created_at;

-- name: GetSurveyExternalPush :one
SELECT b.id,b.questionnaire_id,b.public_submission_id,b.customer_id,b.external_effect_id,b.created_at,e.state,
COALESCE((SELECT bool_or(r.provider_accepted) FROM questionnaire_external_push_delivery_receipts r WHERE r.binding_id=b.id),false) provider_accepted,
COALESCE((SELECT bool_or(r.delivery_proven) FROM questionnaire_external_push_delivery_receipts r WHERE r.binding_id=b.id),false) delivery_proven
FROM questionnaire_submission_external_push_bindings b JOIN external_effects e ON e.id=b.external_effect_id
WHERE b.questionnaire_id=sqlc.arg(questionnaire_id) AND b.public_submission_id=sqlc.arg(public_submission_id);

-- name: CountSurveyExternalPushLogs :one
SELECT count(*)::bigint
FROM questionnaire_submission_external_push_bindings b
JOIN external_effects e ON e.id = b.external_effect_id
  AND e.owner = 'survey' AND e.kind = 'survey_webhook'
WHERE (sqlc.narg(questionnaire_id)::bigint IS NULL OR b.questionnaire_id = sqlc.narg(questionnaire_id)::bigint);

-- name: ListSurveyExternalPushLogs :many
SELECT b.id, b.questionnaire_id, b.public_submission_id, b.customer_id,
       b.external_effect_id, e.state, e.attempt_count,
       COALESCE(bool_or(r.provider_accepted), FALSE)::boolean AS provider_accepted,
       COALESCE(bool_or(r.delivery_proven), FALSE)::boolean AS delivery_proven,
       b.created_at, e.updated_at
FROM questionnaire_submission_external_push_bindings b
JOIN external_effects e ON e.id = b.external_effect_id
  AND e.owner = 'survey' AND e.kind = 'survey_webhook'
LEFT JOIN questionnaire_external_push_delivery_receipts r ON r.binding_id = b.id
WHERE (sqlc.narg(questionnaire_id)::bigint IS NULL OR b.questionnaire_id = sqlc.narg(questionnaire_id)::bigint)
GROUP BY b.id, e.id
ORDER BY b.created_at DESC, b.id DESC
LIMIT sqlc.arg(row_limit)::integer
OFFSET sqlc.arg(row_offset)::integer;

-- name: LockSurveyExternalPushReconcile :one
SELECT b.id,b.questionnaire_id,b.public_submission_id,b.customer_id,b.external_effect_id,b.created_at,e.state,e.owner,e.kind
FROM questionnaire_submission_external_push_bindings b
JOIN external_effects e ON e.id=b.external_effect_id
WHERE b.questionnaire_id=sqlc.arg(questionnaire_id) AND b.public_submission_id=sqlc.arg(public_submission_id)
FOR UPDATE OF b,e;

-- name: LockSurveyExternalPushUnknownAttempt :one
SELECT id
FROM external_effect_attempts
WHERE effect_id=sqlc.arg(effect_id) AND generation=sqlc.arg(generation) AND fence=sqlc.arg(fence) AND completion='outcome_unknown'
FOR UPDATE;

-- name: GetSurveyExternalPushReconciliationEvidence :one
SELECT evidence_digest
FROM external_effect_reconciliations
WHERE effect_id=sqlc.arg(effect_id) AND generation=sqlc.arg(generation) AND fence=sqlc.arg(fence)
FOR SHARE;

-- name: LockSurveyExternalPushDeliveryReceipt :one
SELECT provider_accepted,delivery_proven,COALESCE(evidence_digest,'')::text AS evidence_digest
FROM questionnaire_external_push_delivery_receipts
WHERE binding_id=sqlc.arg(binding_id) AND effect_attempt_id=sqlc.arg(effect_attempt_id)
FOR UPDATE;

-- name: InsertSurveyExternalPushDeliveryReceipt :exec
INSERT INTO questionnaire_external_push_delivery_receipts(binding_id,effect_attempt_id,provider_accepted,delivery_proven,evidence_digest)
VALUES(sqlc.arg(binding_id),sqlc.arg(effect_attempt_id),sqlc.arg(provider_accepted),sqlc.arg(delivery_proven),sqlc.narg(evidence_digest));
