-- name: CreateHistoricalBroadcastJob :one
INSERT INTO outbound_v1_broadcast_job_history (source_id, original_source_type, source_reference_digest, source_table, scheduled_for, priority, batch_key_digest, original_status, requires_approval, approved_by_digest, approved_at, cancelled_by_digest, cancelled_at, cancel_reason_digest, target_count, target_summary_digest, content_type, content_payload_digest, content_summary_digest, attempt_count, last_error_digest, legacy_outbound_task_id, sent_count, failed_count, trace_id_digest, created_by_digest, created_at, updated_at, claimed_at, sent_at, claim_token_digest, lease_expires_at, business_domain, idempotency_key_digest, channel, target_kind, failure_type, retry_policy_digest, metadata_digest, target_union_i_ds_digest, max_attempts, next_retry_at, dispatch_started_at, original_side_effect_executed, original_provider_result_received, result_summary_digest, original_reconciliation_required, completed_at, hold_reason_digest, hold_at, legacy_external_effect_job_id, execution_id_digest, execution_owner_digest, source_key_digest, source_payload_digest, source_field_digest, redacted_roots)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39, $40, $41, $42, $43, $44, $45, $46, $47, $48, $49, $50, $51, $52, $53, $54, $55, $56, $57) RETURNING *;

-- name: GetHistoricalBroadcastJob :one
SELECT * FROM outbound_v1_broadcast_job_history WHERE id = $1;

-- name: ListHistoricalBroadcastJobs :many
SELECT * FROM outbound_v1_broadcast_job_history ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalBroadcastJobs :one
SELECT count(*) FROM outbound_v1_broadcast_job_history;
