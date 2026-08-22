-- name: GetQuestionnaireOperations :one
SELECT q.id AS questionnaire_id,
       COALESCE(o.navigation_target_id, '') AS navigation_target_id,
       COALESCE(o.channel_id, 0)::bigint AS channel_id,
       COALESCE(o.external_push_enabled, FALSE) AS external_push_enabled,
       COALESCE(o.external_push_configuration_reference, '') AS external_push_configuration_reference
FROM public.questionnaires AS q
LEFT JOIN public.questionnaire_operations AS o ON o.questionnaire_id = q.id
WHERE q.id = sqlc.arg(questionnaire_id)::bigint;

-- name: SaveQuestionnaireCompletionOperations :one
INSERT INTO public.questionnaire_operations (
  questionnaire_id, navigation_target_id, channel_id,
  external_push_enabled, external_push_configuration_reference,
  created_at, updated_at
)
SELECT q.id, sqlc.arg(navigation_target_id)::text, sqlc.arg(channel_id)::bigint,
       FALSE, '', sqlc.arg(updated_at)::timestamptz, sqlc.arg(updated_at)::timestamptz
FROM public.questionnaires AS q
WHERE q.id = sqlc.arg(questionnaire_id)::bigint
ON CONFLICT (questionnaire_id) DO UPDATE
SET navigation_target_id = EXCLUDED.navigation_target_id,
    channel_id = EXCLUDED.channel_id,
    updated_at = EXCLUDED.updated_at
RETURNING questionnaire_id;

-- name: SaveQuestionnaireExternalPushOperations :one
INSERT INTO public.questionnaire_operations (
  questionnaire_id, navigation_target_id, channel_id,
  external_push_enabled, external_push_configuration_reference,
  created_at, updated_at
)
SELECT q.id, '', 0, sqlc.arg(external_push_enabled)::boolean,
       sqlc.arg(external_push_configuration_reference)::text,
       sqlc.arg(updated_at)::timestamptz, sqlc.arg(updated_at)::timestamptz
FROM public.questionnaires AS q
WHERE q.id = sqlc.arg(questionnaire_id)::bigint
ON CONFLICT (questionnaire_id) DO UPDATE
SET external_push_enabled = EXCLUDED.external_push_enabled,
    external_push_configuration_reference = EXCLUDED.external_push_configuration_reference,
    updated_at = EXCLUDED.updated_at
RETURNING questionnaire_id;

-- name: ReserveQuestionnaireOperationsReceipt :one
INSERT INTO public.questionnaire_operations_receipts (
  operation, actor_scope, key_digest, payload_digest, created_at
)
VALUES (
  sqlc.arg(operation)::text, sqlc.arg(actor_scope)::text,
  sqlc.arg(key_digest)::bytea, sqlc.arg(payload_digest)::bytea,
  sqlc.arg(created_at)::timestamptz
)
ON CONFLICT (operation, actor_scope, key_digest) DO NOTHING
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: GetQuestionnaireOperationsReceipt :one
SELECT id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot
FROM public.questionnaire_operations_receipts
WHERE operation = sqlc.arg(operation)::text
  AND actor_scope = sqlc.arg(actor_scope)::text
  AND key_digest = sqlc.arg(key_digest)::bytea
FOR UPDATE;

-- name: CompleteQuestionnaireOperationsReceipt :one
UPDATE public.questionnaire_operations_receipts
SET state = 'completed',
    result_snapshot = sqlc.arg(result_snapshot)::jsonb,
    completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'in_progress'
RETURNING id, operation, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: CreateQueuedQuestionnaireExternalPushTest :one
INSERT INTO public.questionnaire_external_push_test_runs (
  questionnaire_id, operation_receipt_id, created_at, updated_at
)
VALUES (
  sqlc.arg(questionnaire_id)::bigint,
  sqlc.arg(operation_receipt_id)::bigint,
  sqlc.arg(created_at)::timestamptz,
  sqlc.arg(created_at)::timestamptz
)
RETURNING id;

-- name: GetQuestionnaireExternalPushTest :one
SELECT id, questionnaire_id, status, attempt_count, side_effect_executed,
       provider_result_received, unknown_after_dispatch, auto_retry_allowed,
       created_at, updated_at
FROM public.questionnaire_external_push_test_runs
WHERE questionnaire_id = sqlc.arg(questionnaire_id)::bigint
  AND id = sqlc.arg(test_run_id)::bigint;

-- name: CountQuestionnaireExternalPushTests :one
SELECT count(*)
FROM public.questionnaire_external_push_test_runs
WHERE questionnaire_id = sqlc.arg(questionnaire_id)::bigint;

-- name: CountGlobalQuestionnaireExternalPushTests :one
SELECT count(*)
FROM public.questionnaire_external_push_test_runs;

-- name: ListQuestionnaireExternalPushTests :many
SELECT id, questionnaire_id, status, attempt_count, side_effect_executed,
       provider_result_received, unknown_after_dispatch, auto_retry_allowed,
       created_at, updated_at
FROM public.questionnaire_external_push_test_runs
WHERE questionnaire_id = sqlc.arg(questionnaire_id)::bigint
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: ListGlobalQuestionnaireExternalPushTests :many
SELECT id, questionnaire_id, status, attempt_count, side_effect_executed,
       provider_result_received, unknown_after_dispatch, auto_retry_allowed,
       created_at, updated_at
FROM public.questionnaire_external_push_test_runs
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;
