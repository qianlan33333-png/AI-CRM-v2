-- name: CreateHistoricalMemberView :one
INSERT INTO product_v1_member_view_history (
  source_key_digest, source_view_id, source_service_product_id, product_id,
  name, position, is_default, schema_version, config_digest, version,
  created_at, updated_at, source_payload_digest
) VALUES (
  sqlc.arg(source_key_digest)::bytea, sqlc.arg(source_view_id)::bigint,
  sqlc.arg(source_service_product_id)::bigint, sqlc.narg(product_id)::bigint,
  sqlc.arg(name)::text, sqlc.arg(position)::bigint, sqlc.arg(is_default)::boolean,
  sqlc.arg(schema_version)::smallint, sqlc.arg(config_digest)::bytea, sqlc.arg(version)::bigint,
  sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz,
  sqlc.arg(source_payload_digest)::bytea
) RETURNING *;

-- name: GetHistoricalMemberView :one
SELECT * FROM product_v1_member_view_history WHERE id=sqlc.arg(id)::bigint;

-- name: ListHistoricalMemberViews :many
SELECT * FROM product_v1_member_view_history
WHERE sqlc.narg(product_id)::bigint IS NULL OR product_id=sqlc.narg(product_id)::bigint
ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalMemberViews :one
SELECT count(*)::bigint FROM product_v1_member_view_history
WHERE sqlc.narg(product_id)::bigint IS NULL OR product_id=sqlc.narg(product_id)::bigint;

-- name: CreateHistoricalMemberUsage :one
INSERT INTO product_v1_member_usage_history (
  source_key_digest, customer_id, formally_logged_in, has_token_usage,
  learning_plan_id, learning_plan_current, learning_plan_total, open_count_7d,
  last_open_at, refreshed_at, source_payload_digest, recovery_entry_digest
) VALUES (
  sqlc.arg(source_key_digest)::bytea, sqlc.narg(customer_id)::bigint,
  sqlc.arg(formally_logged_in)::boolean, sqlc.arg(has_token_usage)::boolean,
  sqlc.arg(learning_plan_id)::text, sqlc.narg(learning_plan_current)::bigint,
  sqlc.narg(learning_plan_total)::bigint, sqlc.arg(open_count_7d)::bigint,
  sqlc.narg(last_open_at)::timestamptz, sqlc.arg(refreshed_at)::timestamptz,
  sqlc.arg(source_payload_digest)::bytea, sqlc.arg(recovery_entry_digest)::bytea
) RETURNING *;

-- name: GetHistoricalMemberUsage :one
SELECT * FROM product_v1_member_usage_history WHERE id=sqlc.arg(id)::bigint;

-- name: ListHistoricalMemberUsage :many
SELECT * FROM product_v1_member_usage_history
WHERE sqlc.narg(customer_id)::bigint IS NULL OR customer_id=sqlc.narg(customer_id)::bigint
ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalMemberUsage :one
SELECT count(*)::bigint FROM product_v1_member_usage_history
WHERE sqlc.narg(customer_id)::bigint IS NULL OR customer_id=sqlc.narg(customer_id)::bigint;
