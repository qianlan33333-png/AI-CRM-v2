-- name: ListAppSettingsProjection :many
SELECT s.key, s.value, s.updated_at,
       COALESCE(latest.action_type, 'empty')::text AS last_action_type,
       COALESCE(latest.updated_by, '')::text AS last_modified_by,
       latest.updated_at AS last_modified_at
FROM settings s
LEFT JOIN LATERAL (
  SELECT CASE WHEN sa.old_value IS NULL THEN 'create' ELSE 'update' END AS action_type,
         sa.updated_by, sa.updated_at
  FROM settings_audit sa
  WHERE sa.key = s.key
  ORDER BY sa.updated_at DESC, sa.id DESC
  LIMIT 1
) latest ON true
WHERE s.key IN ('wecom.corp_id', 'wecom.agent_id', 'outbound.rate_per_second', 'outbound.max_attempts')
ORDER BY s.key;

-- name: ListAppSettingsAudit :many
SELECT id, updated_by AS operator,
       CASE WHEN old_value IS NULL THEN 'create' ELSE 'update' END AS action_type,
       key AS target_id, updated_at AS created_at
FROM settings_audit
WHERE key IN ('wecom.corp_id', 'wecom.agent_id', 'outbound.rate_per_second', 'outbound.max_attempts')
ORDER BY updated_at DESC, id DESC
LIMIT 10;
