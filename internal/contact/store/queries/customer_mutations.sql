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
  AND (
    sqlc.narg(scope_owner_staff_id)::bigint IS NULL
    OR c.owner_staff_id = sqlc.narg(scope_owner_staff_id)::bigint
  )
  AND NOT c.is_deleted
FOR UPDATE;

-- name: GetSidebarCustomerProfile :one
SELECT id, name,
       COALESCE(NULLIF(sqlc.arg(owner_staff_id)::bigint, 0), owner_staff_id, 0)::bigint AS owner_staff_id,
       extra, updated_at
FROM public.customers
WHERE id = sqlc.arg(customer_id)::bigint
  AND NOT is_deleted;

-- name: UpdateSidebarCustomerProfile :one
UPDATE public.customers
SET extra = jsonb_set(extra, '{sidebar_profile}', sqlc.arg(profile)::jsonb, true),
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE id = sqlc.arg(customer_id)::bigint
  AND updated_at = sqlc.arg(expected_updated_at)::timestamptz
  AND NOT is_deleted
RETURNING id, name, sqlc.arg(owner_staff_id)::bigint AS owner_staff_id, extra, updated_at;

-- name: ReserveSidebarCustomerProfileReceipt :one
INSERT INTO public.sidebar_customer_profile_operation_receipts (
  actor_scope, key_digest, payload_digest, created_at
) VALUES (
  sqlc.arg(actor_scope)::text, sqlc.arg(key_digest)::bytea,
  sqlc.arg(payload_digest)::bytea, sqlc.arg(created_at)::timestamptz
)
ON CONFLICT (actor_scope, key_digest) DO NOTHING
RETURNING id, actor_scope, key_digest, payload_digest, state, result_snapshot;

-- name: GetSidebarCustomerProfileReceipt :one
SELECT id, actor_scope, key_digest, payload_digest, state, result_snapshot
FROM public.sidebar_customer_profile_operation_receipts
WHERE actor_scope = sqlc.arg(actor_scope)::text
  AND key_digest = sqlc.arg(key_digest)::bytea
FOR UPDATE;

-- name: CompleteSidebarCustomerProfileReceipt :one
UPDATE public.sidebar_customer_profile_operation_receipts
SET state = 'completed', result_snapshot = sqlc.arg(result_snapshot)::jsonb,
    completed_at = sqlc.arg(completed_at)::timestamptz
WHERE id = sqlc.arg(id)::bigint AND state = 'reserved'
RETURNING id, actor_scope, key_digest, payload_digest, state, result_snapshot;

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

-- name: CreateCustomerForIdentity :one
INSERT INTO customers (
  name,
  owner_staff_id,
  channel_id
) VALUES (
  sqlc.arg(name)::text,
  sqlc.narg(owner_staff_id)::bigint,
  sqlc.narg(channel_id)::bigint
)
RETURNING id;

-- name: LockCustomersForMerge :many
SELECT c.id, c.is_deleted
FROM customers AS c
WHERE c.id = ANY(sqlc.arg(customer_ids)::bigint[])
ORDER BY c.id
FOR UPDATE;

-- name: GetCustomerMergeLineage :one
SELECT lineage.primary_customer_id
FROM customer_merge_lineage AS lineage
WHERE lineage.merged_customer_id = sqlc.arg(merged_customer_id)::bigint;

-- name: CopyCustomerTagsForMerge :execrows
INSERT INTO customer_tags (
  customer_id,
  tag_id,
  tagged_at,
  tagged_by
)
SELECT
  sqlc.arg(primary_customer_id)::bigint,
  source.tag_id,
  source.tagged_at,
  source.tagged_by
FROM customer_tags AS source
WHERE source.customer_id = sqlc.arg(merged_customer_id)::bigint
ON CONFLICT (customer_id, tag_id) DO NOTHING;

-- name: MarkCustomerMerged :execrows
UPDATE customers
SET is_deleted = TRUE, updated_at = now()
WHERE id = sqlc.arg(merged_customer_id)::bigint
  AND NOT is_deleted;

-- name: InsertCustomerMergeLineage :execrows
INSERT INTO customer_merge_lineage (
  merged_customer_id,
  primary_customer_id,
  actor,
  reason
) VALUES (
  sqlc.arg(merged_customer_id)::bigint,
  sqlc.arg(primary_customer_id)::bigint,
  sqlc.arg(actor)::text,
  sqlc.arg(reason)::text
)
ON CONFLICT (merged_customer_id) DO NOTHING;

-- name: ResolveEffectiveCustomerRoot :one
WITH RECURSIVE roots AS (
  SELECT c.id, c.is_deleted, ARRAY[c.id]::bigint[] AS path
  FROM customers AS c
  WHERE c.id = sqlc.arg(customer_id)::bigint

  UNION ALL

  SELECT parent.id, parent.is_deleted, roots.path || parent.id
  FROM roots
  JOIN customer_merge_lineage AS lineage
    ON lineage.merged_customer_id = roots.id
  JOIN customers AS parent
    ON parent.id = lineage.primary_customer_id
  WHERE NOT parent.id = ANY(roots.path)
)
SELECT id
FROM roots
WHERE NOT is_deleted
ORDER BY cardinality(path) DESC
LIMIT 1;
