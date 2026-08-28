-- name: CreateHistoricalOutboundTask :one
INSERT INTO outbound_v1_task_history (
    source_id, task_type, status, created_at, broadcast_job_history_id,
    request_payload_digest, response_payload_digest, wecom_task_id_digest,
    trace_id_digest, legacy_broadcast_job_id, source_key_digest,
    source_payload_digest, source_field_digest, redacted_roots
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
) RETURNING *;

-- name: GetHistoricalOutboundTask :one
SELECT * FROM outbound_v1_task_history WHERE id = $1;

-- name: ListHistoricalOutboundTasks :many
SELECT * FROM outbound_v1_task_history ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalOutboundTasks :one
SELECT count(*) FROM outbound_v1_task_history;

-- name: LookupOutboundTaskHistoryParents :many
SELECT id, source_id, legacy_outbound_task_id
FROM outbound_v1_broadcast_job_history
WHERE source_id = $1 ORDER BY id LIMIT 2;
