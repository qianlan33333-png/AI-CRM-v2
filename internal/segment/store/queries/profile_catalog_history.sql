-- name: CreateHistoricalProfileTemplate :one
INSERT INTO segment_v1_profile_templates (source_id, source_key_digest, source_payload_digest, template_code, template_name, questionnaire_source_id, segmentation_question_source_id, program_source_id, description, original_enabled, version, created_by_digest, updated_by_digest, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING *;

-- name: GetHistoricalProfileTemplate :one
SELECT * FROM segment_v1_profile_templates WHERE id = $1;

-- name: ListHistoricalProfileTemplates :many
SELECT * FROM segment_v1_profile_templates ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalProfileTemplates :one
SELECT count(*) FROM segment_v1_profile_templates;

-- name: CreateHistoricalProfileCategory :one
INSERT INTO segment_v1_profile_categories (source_id, source_key_digest, source_payload_digest, template_source_id, template_history_id, category_key, category_name, description, sort_order, original_enabled, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

-- name: GetHistoricalProfileCategory :one
SELECT * FROM segment_v1_profile_categories WHERE id = $1;

-- name: ListHistoricalProfileCategories :many
SELECT * FROM segment_v1_profile_categories WHERE template_history_id = $1 ORDER BY id LIMIT $2 OFFSET $3;

-- name: CountHistoricalProfileCategories :one
SELECT count(*) FROM segment_v1_profile_categories WHERE template_history_id = $1;

-- name: CreateHistoricalProfileOptionMapping :one
INSERT INTO segment_v1_profile_option_mappings (source_id, source_key_digest, source_payload_digest, template_source_id, category_source_id, template_history_id, category_history_id, question_source_id, option_source_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

-- name: GetHistoricalProfileOptionMapping :one
SELECT * FROM segment_v1_profile_option_mappings WHERE id = $1;

-- name: ListHistoricalProfileOptionMappings :many
SELECT * FROM segment_v1_profile_option_mappings WHERE template_history_id = $1 AND category_history_id = $2 ORDER BY id LIMIT $3 OFFSET $4;

-- name: CountHistoricalProfileOptionMappings :one
SELECT count(*) FROM segment_v1_profile_option_mappings WHERE template_history_id = $1 AND category_history_id = $2;
