-- name: ListTags :many
SELECT
  t.id,
  t.group_id,
  g.name AS group_name,
  g.sort_order AS group_sort_order,
  t.name,
  t.sort_order
FROM tags AS t
LEFT JOIN tag_groups AS g ON g.id = t.group_id
WHERE t.name NOT LIKE 'archived:%'
ORDER BY
  (t.group_id IS NULL),
  g.sort_order,
  g.id,
  t.sort_order,
  t.id;

-- name: ListLegacyTagGroups :many
SELECT id, name, sort_order FROM tag_groups WHERE name NOT LIKE 'archived:%' ORDER BY sort_order, id;
-- name: ListLegacyTags :many
SELECT t.id, t.group_id, g.name AS group_name, t.name, t.sort_order FROM tags t JOIN tag_groups g ON g.id=t.group_id
WHERE g.name NOT LIKE 'archived:%' AND t.name NOT LIKE 'archived:%' ORDER BY g.sort_order,g.id,t.sort_order,t.id;
-- name: CreateLegacyTagGroup :one
INSERT INTO tag_groups(name, sort_order)
SELECT sqlc.arg(name), COALESCE(MAX(sort_order), -1) + 1
FROM tag_groups
WHERE name NOT LIKE 'archived:%'
RETURNING id,name,sort_order;
-- name: CreateLegacyTag :one
INSERT INTO tags(group_id,name,sort_order)
SELECT g.id, sqlc.arg(name), COALESCE((
  SELECT MAX(t.sort_order) + 1 FROM tags AS t
  WHERE t.group_id = g.id AND t.name NOT LIKE 'archived:%'
), 0)
FROM tag_groups AS g
WHERE g.id = sqlc.arg(group_id) AND g.name NOT LIKE 'archived:%'
RETURNING id,group_id,name,sort_order;
-- name: UpdateLegacyTagGroup :one
UPDATE tag_groups SET name=sqlc.arg(name) WHERE id=sqlc.arg(id) AND name NOT LIKE 'archived:%' RETURNING id,name,sort_order;
-- name: ArchiveLegacyTagGroup :one
WITH archived_tags AS (
  UPDATE tags AS t
  SET name = 'archived:' || t.id::text
  WHERE t.group_id = sqlc.arg(group_id)
    AND t.name NOT LIKE 'archived:%'
)
UPDATE tag_groups AS g
SET name = 'archived:' || g.id::text
WHERE g.id = sqlc.arg(group_id)
  AND g.name NOT LIKE 'archived:%'
RETURNING g.id, g.name, g.sort_order;
-- name: UpdateLegacyTag :one
UPDATE tags SET name=sqlc.arg(name) WHERE id=sqlc.arg(id) AND name NOT LIKE 'archived:%' RETURNING id,group_id,name,sort_order;
-- name: ArchiveLegacyTag :one
UPDATE tags SET name='archived:'||id::text WHERE id=sqlc.arg(id) AND name NOT LIKE 'archived:%' RETURNING id,group_id,name,sort_order;

-- name: ReserveLegacyTagSyncReceipt :one
INSERT INTO legacy_tag_sync_receipts(actor_id, idempotency_key, key_digest, kind, trace_id)
VALUES (sqlc.arg(actor_id), sqlc.arg(idempotency_key), sqlc.arg(key_digest), sqlc.arg(kind), sqlc.arg(trace_id))
ON CONFLICT (actor_id, key_digest) DO NOTHING
RETURNING id, actor_id, idempotency_key, kind, trace_id, state, event_id, river_job_id;

-- name: GetLegacyTagSyncReceipt :one
SELECT id, actor_id, idempotency_key, kind, trace_id, state, event_id, river_job_id
FROM legacy_tag_sync_receipts
WHERE actor_id = sqlc.arg(actor_id) AND key_digest = sqlc.arg(key_digest);

-- name: AcceptLegacyTagSyncReceipt :one
UPDATE legacy_tag_sync_receipts
SET state = 'queued', event_id = sqlc.arg(event_id), river_job_id = sqlc.arg(river_job_id), accepted_at = now()
WHERE id = sqlc.arg(id) AND state = 'reserved'
RETURNING id, actor_id, idempotency_key, kind, trace_id, state, event_id, river_job_id;

-- name: ReserveLegacyTagLiveMutationReceipt :one
INSERT INTO legacy_tag_live_mutation_receipts(actor_id, idempotency_key, key_digest, operation, payload, trace_id)
VALUES (sqlc.arg(actor_id), sqlc.arg(idempotency_key), sqlc.arg(key_digest), sqlc.arg(operation), sqlc.arg(payload), sqlc.arg(trace_id))
ON CONFLICT (actor_id, key_digest) DO NOTHING
RETURNING id, actor_id, idempotency_key, operation, payload, trace_id, state, event_id, river_job_id;

-- name: GetLegacyTagLiveMutationReceipt :one
SELECT id, actor_id, idempotency_key, operation, payload, trace_id, state, event_id, river_job_id
FROM legacy_tag_live_mutation_receipts
WHERE actor_id = sqlc.arg(actor_id) AND key_digest = sqlc.arg(key_digest);

-- name: AcceptLegacyTagLiveMutationReceipt :one
UPDATE legacy_tag_live_mutation_receipts
SET state = 'queued', event_id = sqlc.arg(event_id), river_job_id = sqlc.arg(river_job_id), accepted_at = now()
WHERE id = sqlc.arg(id) AND state = 'reserved'
RETURNING id, actor_id, idempotency_key, operation, payload, trace_id, state, event_id, river_job_id;

-- name: GetLegacyTagExecutionStatus :one
SELECT payload, updated_at FROM legacy_tag_execution_status WHERE singleton = true;
