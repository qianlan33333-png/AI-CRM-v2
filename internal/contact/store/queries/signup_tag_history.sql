-- Signup tag rules are immutable V1 history. They never populate the current
-- tag catalogue or trigger a Provider synchronization.

-- name: CreateHistoricalSignupTagRule :one
INSERT INTO contact_v1_signup_tag_rules (
  source_key_digest,
  source_payload_digest,
  tag_source_id,
  tag_name,
  signup_status,
  original_active,
  updated_at
) VALUES (
  sqlc.arg(source_key_digest)::bytea,
  sqlc.arg(source_payload_digest)::bytea,
  sqlc.arg(tag_source_id)::text,
  sqlc.arg(tag_name)::text,
  sqlc.arg(signup_status)::text,
  sqlc.arg(original_active)::boolean,
  sqlc.arg(updated_at)::timestamptz
)
RETURNING *;

-- name: GetHistoricalSignupTagRule :one
SELECT *
FROM contact_v1_signup_tag_rules
WHERE id=sqlc.arg(id)::bigint;

-- name: ListHistoricalSignupTagRules :many
SELECT *
FROM contact_v1_signup_tag_rules
ORDER BY id
LIMIT sqlc.arg(row_limit)::integer
OFFSET sqlc.arg(row_offset)::integer;

-- name: CountHistoricalSignupTagRules :one
SELECT count(*)::bigint
FROM contact_v1_signup_tag_rules;
