-- name: UpdateCustomer :one
UPDATE customers AS c
SET
  name = CASE
    WHEN sqlc.arg(name_set)::boolean THEN sqlc.narg(name)::text
    ELSE c.name
  END,
  avatar_url = CASE
    WHEN sqlc.arg(avatar_url_set)::boolean THEN sqlc.narg(avatar_url)::text
    ELSE c.avatar_url
  END,
  gender = CASE
    WHEN sqlc.arg(gender_set)::boolean THEN sqlc.narg(gender)::smallint
    ELSE c.gender
  END,
  owner_staff_id = CASE
    WHEN sqlc.arg(owner_staff_id_set)::boolean THEN sqlc.narg(owner_staff_id)::bigint
    ELSE c.owner_staff_id
  END,
  channel_id = CASE
    WHEN sqlc.arg(channel_id_set)::boolean THEN sqlc.narg(channel_id)::bigint
    ELSE c.channel_id
  END,
  extra = CASE
    WHEN sqlc.arg(extra_set)::boolean THEN sqlc.narg(extra)::jsonb
    ELSE c.extra
  END,
  updated_at = now()
WHERE c.id = sqlc.arg(customer_id)::bigint
  AND NOT c.is_deleted
RETURNING
  c.id,
  c.name,
  c.avatar_url,
  c.gender,
  c.stage_id,
  c.owner_staff_id,
  c.channel_id,
  c.added_at,
  c.last_interact_at,
  c.is_deleted,
  c.extra,
  c.created_at,
  c.updated_at;

-- name: LockActiveCustomerForMutation :one
SELECT
  c.id,
  c.name,
  c.avatar_url,
  c.gender,
  c.stage_id,
  c.owner_staff_id,
  c.channel_id,
  c.added_at,
  c.last_interact_at,
  c.is_deleted,
  c.extra,
  c.created_at,
  c.updated_at
FROM customers AS c
WHERE c.id = sqlc.arg(customer_id)::bigint
  AND NOT c.is_deleted
FOR UPDATE;

-- name: SetCustomerStage :one
UPDATE customers AS c
SET
  stage_id = sqlc.narg(stage_id)::bigint,
  updated_at = now()
WHERE c.id = sqlc.arg(customer_id)::bigint
  AND NOT c.is_deleted
RETURNING
  c.id,
  c.name,
  c.avatar_url,
  c.gender,
  c.stage_id,
  c.owner_staff_id,
  c.channel_id,
  c.added_at,
  c.last_interact_at,
  c.is_deleted,
  c.extra,
  c.created_at,
  c.updated_at;

-- name: GetCustomerTag :one
SELECT id
FROM tags
WHERE id = sqlc.arg(tag_id)::bigint;

-- name: AddCustomerTag :execrows
INSERT INTO customer_tags (
  customer_id,
  tag_id,
  tagged_by
) VALUES (
  sqlc.arg(customer_id)::bigint,
  sqlc.arg(tag_id)::bigint,
  sqlc.arg(tagged_by)::text
)
ON CONFLICT (customer_id, tag_id) DO NOTHING;

-- name: RemoveCustomerTag :execrows
DELETE FROM customer_tags
WHERE customer_id = sqlc.arg(customer_id)::bigint
  AND tag_id = sqlc.arg(tag_id)::bigint;

-- name: AppendCustomerEvent :one
INSERT INTO customer_events (
  customer_id,
  event_type,
  payload,
  actor,
  occurred_at
) VALUES (
  sqlc.arg(customer_id)::bigint,
  sqlc.arg(event_type)::text,
  sqlc.arg(payload)::jsonb,
  sqlc.arg(actor)::text,
  sqlc.arg(occurred_at)::timestamptz
)
RETURNING id;
