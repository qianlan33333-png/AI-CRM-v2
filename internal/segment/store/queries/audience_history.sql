-- name: CreateHistoricalAudienceGroup :one
INSERT INTO segment_v1_audience_groups (source_id, name, created_at, updated_at)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetHistoricalAudienceGroup :one
SELECT * FROM segment_v1_audience_groups WHERE id = $1;

-- name: ListHistoricalAudienceGroups :many
SELECT * FROM segment_v1_audience_groups ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalAudienceGroups :one
SELECT count(*) FROM segment_v1_audience_groups;

-- name: CreateHistoricalAudiencePackage :one
INSERT INTO segment_v1_audience_packages (source_id, group_history_id, current_version_source_id, package_key, name, natural_language_definition, original_status, query_mode, identity_policy, incremental_enabled, daily_enabled, incremental_interval_seconds, daily_refresh_time, timezone, lookback_seconds, last_incremental_at, last_daily_refreshed_at, next_incremental_at, next_daily_at, paused_reason, created_at, updated_at, runtime_digest)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23) RETURNING *;

-- name: GetHistoricalAudiencePackage :one
SELECT * FROM segment_v1_audience_packages WHERE id = $1;

-- name: ListHistoricalAudiencePackages :many
SELECT * FROM segment_v1_audience_packages ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalAudiencePackages :one
SELECT count(*) FROM segment_v1_audience_packages;

-- name: CreateHistoricalAudienceVersion :one
INSERT INTO segment_v1_audience_versions (source_id, package_history_id, version_number, original_status, ai_prompt, ai_rationale, natural_language_explanation, created_at, published_at, template_key, template_version, template_fingerprint, definition_digest)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING *;

-- name: GetHistoricalAudienceVersion :one
SELECT * FROM segment_v1_audience_versions WHERE id = $1;

-- name: ListHistoricalAudienceVersions :many
SELECT * FROM segment_v1_audience_versions WHERE package_history_id = $1 ORDER BY id LIMIT $2 OFFSET $3;

-- name: CountHistoricalAudienceVersions :one
SELECT count(*) FROM segment_v1_audience_versions WHERE package_history_id = $1;

-- name: CreateHistoricalAudienceSender :one
INSERT INTO segment_v1_audience_senders (source_id, package_history_id, staff_id, display_name, priority, original_status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: GetHistoricalAudienceSender :one
SELECT * FROM segment_v1_audience_senders WHERE id = $1;

-- name: ListHistoricalAudienceSenders :many
SELECT * FROM segment_v1_audience_senders WHERE package_history_id = $1 ORDER BY id LIMIT $2 OFFSET $3;

-- name: CountHistoricalAudienceSenders :one
SELECT count(*) FROM segment_v1_audience_senders WHERE package_history_id = $1;

-- name: CreateHistoricalAudienceRule :one
INSERT INTO segment_v1_audience_rules (source_id, rule_key, display_name, description, rule_type, owner_staff_id, original_status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: GetHistoricalAudienceRule :one
SELECT * FROM segment_v1_audience_rules WHERE id = $1;

-- name: ListHistoricalAudienceRules :many
SELECT * FROM segment_v1_audience_rules ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalAudienceRules :one
SELECT count(*) FROM segment_v1_audience_rules;

-- name: CreateHistoricalAudienceRuleVersion :one
INSERT INTO segment_v1_audience_rule_versions (source_id, rule_history_id, version, executor_type, original_status, published_at, created_at, definition_digest)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: GetHistoricalAudienceRuleVersion :one
SELECT * FROM segment_v1_audience_rule_versions WHERE id = $1;

-- name: ListHistoricalAudienceRuleVersions :many
SELECT * FROM segment_v1_audience_rule_versions WHERE rule_history_id = $1 ORDER BY id LIMIT $2 OFFSET $3;

-- name: CountHistoricalAudienceRuleVersions :one
SELECT count(*) FROM segment_v1_audience_rule_versions WHERE rule_history_id = $1;

-- name: CreateHistoricalAudienceDefinition :one
INSERT INTO segment_v1_definitions (source_id, code, display_name, description, source_type, sql_dialect, original_status, version, cached_headcount, last_refreshed_at, usage_count, created_at, updated_at, definition_digest)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING *;

-- name: GetHistoricalAudienceDefinition :one
SELECT * FROM segment_v1_definitions WHERE id = $1;

-- name: ListHistoricalAudienceDefinitions :many
SELECT * FROM segment_v1_definitions ORDER BY id LIMIT $1 OFFSET $2;

-- name: CountHistoricalAudienceDefinitions :one
SELECT count(*) FROM segment_v1_definitions;

-- name: CreateHistoricalAudienceMember :one
INSERT INTO segment_v1_audience_members (source_id, package_history_id, customer_id, identity_kind, original_status, first_entered_at, last_seen_at, last_updated_at, exited_at, created_at, updated_at, payload_digest)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

-- name: GetHistoricalAudienceMember :one
SELECT * FROM segment_v1_audience_members WHERE id = $1;

-- name: ListHistoricalAudienceMembers :many
SELECT * FROM segment_v1_audience_members WHERE package_history_id = $1 ORDER BY id LIMIT $2 OFFSET $3;

-- name: CountHistoricalAudienceMembers :one
SELECT count(*) FROM segment_v1_audience_members WHERE package_history_id = $1;
