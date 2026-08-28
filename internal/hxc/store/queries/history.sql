-- name: CreateHistoricalHXCMeta :one
INSERT INTO hxc_v1_dashboard_refresh_history (source_id, source_key_digest, source_payload_digest, started_at, finished_at, status, row_count, member_hit, user_hit, only_member, trigger_source) VALUES (sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(started_at), sqlc.narg(finished_at), sqlc.arg(status), sqlc.arg(row_count), sqlc.arg(member_hit), sqlc.arg(user_hit), sqlc.arg(only_member), sqlc.arg(trigger_source)) RETURNING id, source_id, source_key_digest, source_payload_digest, started_at, finished_at, status, row_count, member_hit, user_hit, only_member, trigger_source;

-- name: GetHistoricalHXCMeta :one
SELECT id, source_id, source_key_digest, source_payload_digest, started_at, finished_at, status, row_count, member_hit, user_hit, only_member, trigger_source FROM hxc_v1_dashboard_refresh_history WHERE id = sqlc.arg(id);

-- name: ListHistoricalHXCMeta :many
SELECT id, source_id, source_key_digest, source_payload_digest, started_at, finished_at, status, row_count, member_hit, user_hit, only_member, trigger_source FROM hxc_v1_dashboard_refresh_history ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalHXCMeta :one
SELECT count(*)::bigint FROM hxc_v1_dashboard_refresh_history;

-- name: CreateHistoricalHXCSnapshot :one
INSERT INTO hxc_v1_dashboard_observations (source_id, source_key_digest, source_payload_digest, customer_id, observation, observed_at, in_lead_pool, in_people, in_questionnaire, class_term_no, class_term_label, crm_hxc_state, crm_created_at, last_questionnaire_at, hxc_member_hit, hxc_user_hit, funnel_state, hxc_member_status, hxc_registered_at, hxc_last_login_at, membership_type, membership_status, membership_end_at, membership_days_left, consultation_used, consultation_limit, conversation_chat, conversation_consult, conversation_lesson, messages_user, messages_ai, consult_completed, last_message_at, subscription_tier, subscription_expires, subscription_quota, subscription_used, subscription_period_start) VALUES (sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.narg(customer_id), sqlc.arg(observation), sqlc.arg(observed_at), sqlc.arg(in_lead_pool), sqlc.arg(in_people), sqlc.arg(in_questionnaire), sqlc.narg(class_term_no), sqlc.arg(class_term_label), sqlc.arg(crm_hxc_state), sqlc.narg(crm_created_at)::text::date, sqlc.narg(last_questionnaire_at)::text::date, sqlc.arg(hxc_member_hit), sqlc.arg(hxc_user_hit), sqlc.arg(funnel_state), sqlc.arg(hxc_member_status), sqlc.narg(hxc_registered_at), sqlc.narg(hxc_last_login_at), sqlc.arg(membership_type), sqlc.arg(membership_status), sqlc.narg(membership_end_at), sqlc.narg(membership_days_left), sqlc.narg(consultation_used), sqlc.narg(consultation_limit), sqlc.arg(conversation_chat), sqlc.arg(conversation_consult), sqlc.arg(conversation_lesson), sqlc.arg(messages_user), sqlc.arg(messages_ai), sqlc.arg(consult_completed), sqlc.narg(last_message_at), sqlc.arg(subscription_tier), sqlc.narg(subscription_expires), sqlc.narg(subscription_quota), sqlc.narg(subscription_used), sqlc.narg(subscription_period_start)::text::date) RETURNING id, source_id, source_key_digest, source_payload_digest, customer_id, observation, observed_at, in_lead_pool, in_people, in_questionnaire, class_term_no, class_term_label, crm_hxc_state, crm_created_at, last_questionnaire_at, hxc_member_hit, hxc_user_hit, funnel_state, hxc_member_status, hxc_registered_at, hxc_last_login_at, membership_type, membership_status, membership_end_at, membership_days_left, consultation_used, consultation_limit, conversation_chat, conversation_consult, conversation_lesson, messages_user, messages_ai, consult_completed, last_message_at, subscription_tier, subscription_expires, subscription_quota, subscription_used, subscription_period_start;

-- name: GetHistoricalHXCSnapshot :one
SELECT id, source_id, source_key_digest, source_payload_digest, customer_id, observation, observed_at, in_lead_pool, in_people, in_questionnaire, class_term_no, class_term_label, crm_hxc_state, crm_created_at, last_questionnaire_at, hxc_member_hit, hxc_user_hit, funnel_state, hxc_member_status, hxc_registered_at, hxc_last_login_at, membership_type, membership_status, membership_end_at, membership_days_left, consultation_used, consultation_limit, conversation_chat, conversation_consult, conversation_lesson, messages_user, messages_ai, consult_completed, last_message_at, subscription_tier, subscription_expires, subscription_quota, subscription_used, subscription_period_start FROM hxc_v1_dashboard_observations WHERE id = sqlc.arg(id);

-- name: ListHistoricalHXCSnapshot :many
SELECT id, source_id, source_key_digest, source_payload_digest, customer_id, observation, observed_at, in_lead_pool, in_people, in_questionnaire, class_term_no, class_term_label, crm_hxc_state, crm_created_at, last_questionnaire_at, hxc_member_hit, hxc_user_hit, funnel_state, hxc_member_status, hxc_registered_at, hxc_last_login_at, membership_type, membership_status, membership_end_at, membership_days_left, consultation_used, consultation_limit, conversation_chat, conversation_consult, conversation_lesson, messages_user, messages_ai, consult_completed, last_message_at, subscription_tier, subscription_expires, subscription_quota, subscription_used, subscription_period_start FROM hxc_v1_dashboard_observations WHERE (sqlc.narg(customer_id)::bigint IS NULL OR customer_id = sqlc.narg(customer_id)::bigint) ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalHXCSnapshot :one
SELECT count(*)::bigint FROM hxc_v1_dashboard_observations WHERE (sqlc.narg(customer_id)::bigint IS NULL OR customer_id = sqlc.narg(customer_id)::bigint);

-- name: CreateHistoricalHXCActivation :one
INSERT INTO hxc_v1_activation_observations (source_id, source_key_digest, source_payload_digest, source_table, original_state, is_active, legacy_import_batch_ref, created_at, updated_at) VALUES (sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(source_table), sqlc.arg(original_state), sqlc.arg(is_active), sqlc.narg(legacy_import_batch_ref), sqlc.arg(created_at), sqlc.arg(updated_at)) RETURNING id, source_id, source_key_digest, source_payload_digest, source_table, original_state, is_active, legacy_import_batch_ref, created_at, updated_at;

-- name: GetHistoricalHXCActivation :one
SELECT id, source_id, source_key_digest, source_payload_digest, source_table, original_state, is_active, legacy_import_batch_ref, created_at, updated_at FROM hxc_v1_activation_observations WHERE id = sqlc.arg(id);

-- name: ListHistoricalHXCActivation :many
SELECT id, source_id, source_key_digest, source_payload_digest, source_table, original_state, is_active, legacy_import_batch_ref, created_at, updated_at FROM hxc_v1_activation_observations WHERE (sqlc.arg(source_table)::text = '' OR source_table = sqlc.arg(source_table)::text) ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalHXCActivation :one
SELECT count(*)::bigint FROM hxc_v1_activation_observations WHERE (sqlc.arg(source_table)::text = '' OR source_table = sqlc.arg(source_table)::text);

-- name: CreateHistoricalHXCLead :one
INSERT INTO hxc_v1_experience_lead_history (source_id, source_key_digest, source_payload_digest, original_type, is_active, legacy_import_batch_ref, created_at, updated_at) VALUES (sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(original_type), sqlc.arg(is_active), sqlc.narg(legacy_import_batch_ref), sqlc.arg(created_at), sqlc.arg(updated_at)) RETURNING id, source_id, source_key_digest, source_payload_digest, original_type, is_active, legacy_import_batch_ref, created_at, updated_at;

-- name: GetHistoricalHXCLead :one
SELECT id, source_id, source_key_digest, source_payload_digest, original_type, is_active, legacy_import_batch_ref, created_at, updated_at FROM hxc_v1_experience_lead_history WHERE id = sqlc.arg(id);

-- name: ListHistoricalHXCLead :many
SELECT id, source_id, source_key_digest, source_payload_digest, original_type, is_active, legacy_import_batch_ref, created_at, updated_at FROM hxc_v1_experience_lead_history ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalHXCLead :one
SELECT count(*)::bigint FROM hxc_v1_experience_lead_history;

-- name: CreateHistoricalHXCBatch :one
INSERT INTO hxc_v1_import_batch_history (source_id, source_key_digest, source_payload_digest, import_type, total_rows, success_rows, failed_rows, created_at) VALUES (sqlc.arg(source_id), sqlc.arg(source_key_digest), sqlc.arg(source_payload_digest), sqlc.arg(import_type), sqlc.arg(total_rows), sqlc.arg(success_rows), sqlc.arg(failed_rows), sqlc.arg(created_at)) RETURNING id, source_id, source_key_digest, source_payload_digest, import_type, total_rows, success_rows, failed_rows, created_at;

-- name: GetHistoricalHXCBatch :one
SELECT id, source_id, source_key_digest, source_payload_digest, import_type, total_rows, success_rows, failed_rows, created_at FROM hxc_v1_import_batch_history WHERE id = sqlc.arg(id);

-- name: ListHistoricalHXCBatch :many
SELECT id, source_id, source_key_digest, source_payload_digest, import_type, total_rows, success_rows, failed_rows, created_at FROM hxc_v1_import_batch_history ORDER BY id ASC LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistoricalHXCBatch :one
SELECT count(*)::bigint FROM hxc_v1_import_batch_history;
