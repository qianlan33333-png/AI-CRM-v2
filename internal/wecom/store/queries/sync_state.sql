-- name: LoadWeComSyncState :one
SELECT cursor, (completed_at IS NOT NULL)::boolean AS completed
FROM wecom_sync_state
WHERE sync_key = sqlc.arg(sync_key)::text;

-- name: AdvanceWeComSyncState :one
INSERT INTO wecom_sync_state (sync_key, cursor, completed_at)
VALUES (
  sqlc.arg(sync_key)::text,
  sqlc.arg(cursor)::text,
  CASE WHEN sqlc.arg(completed)::boolean THEN now() ELSE NULL END
)
ON CONFLICT (sync_key) DO UPDATE
SET cursor = EXCLUDED.cursor,
    completed_at = EXCLUDED.completed_at,
    updated_at = now()
WHERE wecom_sync_state.cursor = sqlc.arg(expected_cursor)::text
  AND wecom_sync_state.completed_at IS NULL
RETURNING cursor, (completed_at IS NOT NULL)::boolean AS completed;
