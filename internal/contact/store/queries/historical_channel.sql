-- Contact-only, non-executing V1 channel definitions. No operation receipt,
-- asset binding, event or current customer attribution is created.
-- name: CreateHistoricalChannel :one
WITH inserted AS (
  INSERT INTO channels (code, name, status, config, created_by, updated_by, created_at, updated_at)
  VALUES (sqlc.arg(code)::text, sqlc.arg(name)::text, 'inactive', sqlc.arg(projection)::jsonb,
          sqlc.arg(actor)::bigint, sqlc.arg(actor)::bigint,
          sqlc.arg(created_at)::timestamptz, sqlc.arg(updated_at)::timestamptz)
  ON CONFLICT DO NOTHING
  RETURNING *
), archived AS (
  INSERT INTO channel_acquisition_legacy_archives (channel_id, config_digest, status)
  SELECT id, sqlc.arg(config_digest)::text, 'legacy_unverified' FROM inserted
  RETURNING channel_id, config_digest
)
SELECT i.id, i.code, i.name, i.status, i.config, i.created_by, i.updated_by, i.created_at, i.updated_at, a.config_digest
FROM inserted i JOIN archived a ON a.channel_id=i.id;

-- name: GetHistoricalChannel :one
SELECT c.id, c.code, c.name, c.status, c.config, c.created_by, c.updated_by, c.created_at, c.updated_at, a.config_digest
FROM channels c JOIN channel_acquisition_legacy_archives a ON a.channel_id=c.id
WHERE c.id=sqlc.arg(channel_id)::bigint AND a.status='legacy_unverified'
FOR UPDATE OF c, a;
