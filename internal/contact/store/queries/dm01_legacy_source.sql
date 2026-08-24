-- name: ListDM01SourceColumns :many
SELECT a.attnum::integer AS ordinal, a.attname::text AS column_name,
       pg_catalog.format_type(a.atttypid, a.atttypmod)::text AS data_type,
       a.attnotnull AS not_null
FROM pg_catalog.pg_attribute AS a
JOIN pg_catalog.pg_class AS c ON c.oid = a.attrelid
JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
WHERE n.nspname = 'public'
  AND c.relname = sqlc.arg(table_name)::text
  AND c.relkind IN ('r', 'p')
  AND a.attnum > 0
  AND NOT a.attisdropped
ORDER BY a.attnum;

-- name: GetDM01SourceDatabaseIdentity :one
SELECT system_identifier::text AS server_id, current_database()::text AS database
FROM pg_control_system();

-- name: GetDM01OwnerRoleMapUpperBound :one
SELECT updated_at, userid AS source_key FROM owner_role_map ORDER BY updated_at DESC, userid DESC LIMIT 1;
-- name: GetDM01CustomerIdentityUpperBound :one
SELECT updated_at, unionid AS source_key FROM crm_user_identity ORDER BY updated_at DESC, unionid DESC LIMIT 1;
-- name: GetDM01ExternalIdentityUpperBound :one
SELECT updated_at, id::text AS source_key FROM wecom_external_contact_identity_map ORDER BY updated_at DESC, id DESC LIMIT 1;
-- name: GetDM01MergeAuditUpperBound :one
SELECT created_at AS updated_at, id::text AS source_key FROM crm_user_identity_merge_audit ORDER BY created_at DESC, id DESC LIMIT 1;
-- name: GetDM01ResolutionQueueUpperBound :one
SELECT updated_at, id::text AS source_key FROM crm_user_identity_resolution_queue ORDER BY updated_at DESC, id DESC LIMIT 1;
-- name: GetDM01DirectoryMemberUpperBound :one
SELECT last_synced_at AS updated_at, id::text AS source_key FROM admin_wecom_directory_members ORDER BY last_synced_at DESC, id DESC LIMIT 1;
-- name: GetDM01ContactUpperBound :one
SELECT updated_at, id::text AS source_key FROM contacts ORDER BY updated_at DESC, id DESC LIMIT 1;
-- name: GetDM01IdentityConflictUpperBound :one
SELECT updated_at, id::text AS source_key FROM crm_user_identity_conflicts ORDER BY updated_at DESC, id DESC LIMIT 1;
-- name: GetDM01ExternalBindingUpperBound :one
SELECT updated_at, external_userid AS source_key FROM external_contact_bindings ORDER BY updated_at DESC, external_userid DESC LIMIT 1;
-- name: GetDM01PersonUpperBound :one
SELECT updated_at, id::text AS source_key FROM people ORDER BY updated_at DESC, id DESC LIMIT 1;
-- name: GetDM01FollowUserUpperBound :one
SELECT updated_at, id::text AS source_key FROM wecom_external_contact_follow_users ORDER BY updated_at DESC, id DESC LIMIT 1;

-- name: ListDM01OwnerRoleMap :many
SELECT userid, display_name, active, created_at, updated_at,
       jsonb_build_object('userid', userid, 'display_name', display_name,
         'role', role, 'active', active, 'source', source,
         'raw_payload_json', raw_payload_json, 'created_at', created_at,
         'updated_at', updated_at) AS payload
FROM owner_role_map
WHERE (updated_at, userid) <= (sqlc.arg(upper_watermark)::timestamptz, sqlc.arg(upper_key)::text)
ORDER BY updated_at, userid
LIMIT sqlc.arg(page_size)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: ListDM01CustomerIdentity :many
SELECT unionid, customer_name, avatar, gender, primary_owner_userid,
       first_seen_at, last_seen_at, created_at, updated_at,
       (jsonb_build_object('primary_external_userid', primary_external_userid,
         'external_userids_json', external_userids_json, 'mobile', mobile,
         'mobile_normalized', mobile_normalized, 'mobile_verified', mobile_verified,
         'mobile_source', mobile_source, 'customer_name', customer_name,
         'remark', remark, 'description', description, 'avatar', avatar,
         'gender', gender, 'profile_json', profile_json,
         'primary_owner_userid', primary_owner_userid,
         'follow_users_json', follow_users_json, 'legacy_person_id', legacy_person_id,
         'legacy_identity_map_ids_json', legacy_identity_map_ids_json,
         'legacy_sources_json', legacy_sources_json, 'identity_status', identity_status,
         'unionid_resolved_at', unionid_resolved_at, 'first_seen_at', first_seen_at,
         'last_seen_at', last_seen_at, 'last_polled_at', last_polled_at,
         'next_poll_at', next_poll_at, 'poll_attempt_count', poll_attempt_count,
         'last_poll_error', last_poll_error, 'created_at', created_at,
         'updated_at', updated_at)
       || CASE WHEN sqlc.arg(include_unionid)::boolean
          THEN jsonb_build_object('unionid', unionid) ELSE '{}'::jsonb END
       || CASE WHEN sqlc.arg(include_openids)::boolean
          THEN jsonb_build_object('primary_openid', primary_openid, 'openids_json', openids_json)
          ELSE '{}'::jsonb END)::jsonb AS payload
FROM crm_user_identity
WHERE (updated_at, unionid) <= (sqlc.arg(upper_watermark)::timestamptz, sqlc.arg(upper_key)::text)
ORDER BY updated_at, unionid
LIMIT sqlc.arg(page_size)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: ListDM01ExternalIdentityMap :many
SELECT id, external_userid, unionid, corp_id, updated_at,
       (jsonb_build_object('id', id, 'external_userid', external_userid,
         'follow_user_userid', follow_user_userid, 'name', name, 'status', status,
         'updated_at', updated_at, 'corp_id', corp_id, 'avatar', avatar,
         'gender', gender, 'raw_profile', raw_profile, 'first_seen_at', first_seen_at,
         'last_seen_at', last_seen_at, 'created_at', created_at)
       || CASE WHEN sqlc.arg(include_unionid)::boolean
          THEN jsonb_build_object('unionid', unionid) ELSE '{}'::jsonb END
       || CASE WHEN sqlc.arg(include_openids)::boolean
          THEN jsonb_build_object('openid', openid) ELSE '{}'::jsonb END)::jsonb AS payload
FROM wecom_external_contact_identity_map
WHERE (updated_at, id) <= (sqlc.arg(upper_watermark)::timestamptz, sqlc.arg(upper_key)::bigint)
ORDER BY updated_at, id
LIMIT sqlc.arg(page_size)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: ListDM01MergeAudit :many
SELECT id, created_at, jsonb_build_object(
  'id', id, 'from_unionid', from_unionid, 'to_unionid', to_unionid,
  'reason', reason, 'before_json', before_json, 'after_json', after_json,
  'operator', operator, 'created_at', created_at) AS payload
FROM crm_user_identity_merge_audit
WHERE (created_at, id) <= (sqlc.arg(upper_watermark)::timestamptz, sqlc.arg(upper_key)::bigint)
ORDER BY created_at, id LIMIT sqlc.arg(page_size)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: ListDM01ResolutionQueue :many
SELECT id, updated_at, jsonb_build_object(
  'id', id, 'source_type', source_type, 'source_key', source_key,
  'source_table', source_table, 'source_id', source_id, 'corp_id', corp_id,
  'external_userid', external_userid, 'openid', openid, 'mobile', mobile,
  'payload_json', payload_json, 'raw_payload_json', raw_payload_json,
  'reason', reason, 'status', status, 'resolved_unionid', resolved_unionid,
  'conflict_reason', conflict_reason, 'attempts', attempts, 'attempt_count', attempt_count,
  'last_error', last_error, 'next_attempt_at', next_attempt_at, 'resolved_at', resolved_at,
  'first_seen_at', first_seen_at, 'last_seen_at', last_seen_at, 'created_at', created_at,
  'updated_at', updated_at, 'execution_id', execution_id,
  'parent_execution_id', parent_execution_id, 'external_effect_job_id', external_effect_job_id,
  'lane', lane, 'row_version', row_version, 'hold_reason', hold_reason,
  'held_at', held_at, 'completed_at', completed_at) AS payload
FROM crm_user_identity_resolution_queue
WHERE (updated_at, id) <= (sqlc.arg(upper_watermark)::timestamptz, sqlc.arg(upper_key)::bigint)
ORDER BY updated_at, id LIMIT sqlc.arg(page_size)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: ListDM01DirectoryMember :many
SELECT id, last_synced_at, jsonb_build_object(
  'id', id, 'corp_id', corp_id, 'wecom_userid', wecom_userid,
  'display_name', display_name, 'department_ids_json', department_ids_json,
  'department_name', department_name, 'position', position, 'mobile', mobile,
  'avatar_url', avatar_url, 'wecom_status', wecom_status, 'is_active', is_active,
  'raw_payload_json', raw_payload_json, 'first_seen_at', first_seen_at,
  'last_synced_at', last_synced_at, 'created_at', created_at,
  'updated_at', updated_at, 'updated_by', updated_by) AS payload
FROM admin_wecom_directory_members
WHERE (last_synced_at, id) <= (sqlc.arg(upper_watermark)::timestamptz, sqlc.arg(upper_key)::bigint)
ORDER BY last_synced_at, id LIMIT sqlc.arg(page_size)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: ListDM01Contact :many
SELECT id, updated_at, jsonb_build_object('id', id, 'unionid', unionid, 'created_at', created_at, 'updated_at', updated_at) AS payload
FROM contacts
WHERE (updated_at, id) <= (sqlc.arg(upper_watermark)::timestamptz, sqlc.arg(upper_key)::bigint)
ORDER BY updated_at, id LIMIT sqlc.arg(page_size)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: ListDM01IdentityConflict :many
SELECT id, updated_at, jsonb_build_object(
  'id', id, 'conflict_type', conflict_type, 'unionid', unionid,
  'candidate_unionid', candidate_unionid, 'external_userid', external_userid,
  'openid', openid, 'mobile', mobile, 'source_type', source_type,
  'source_key', source_key, 'payload_json', payload_json,
  'source_payload_json', source_payload_json, 'status', status,
  'resolution_status', resolution_status, 'resolution_note', resolution_note,
  'created_at', created_at, 'updated_at', updated_at, 'resolved_at', resolved_at) AS payload
FROM crm_user_identity_conflicts
WHERE (updated_at, id) <= (sqlc.arg(upper_watermark)::timestamptz, sqlc.arg(upper_key)::bigint)
ORDER BY updated_at, id LIMIT sqlc.arg(page_size)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: ListDM01ExternalBinding :many
SELECT external_userid, updated_at, jsonb_build_object(
  'external_userid', external_userid, 'person_id', person_id,
  'first_owner_userid', first_owner_userid, 'last_owner_userid', last_owner_userid,
  'updated_at', updated_at) AS payload
FROM external_contact_bindings
WHERE (updated_at, external_userid) <= (sqlc.arg(upper_watermark)::timestamptz, sqlc.arg(upper_key)::text)
ORDER BY updated_at, external_userid LIMIT sqlc.arg(page_size)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: ListDM01Person :many
SELECT id, updated_at, jsonb_build_object('id', id, 'mobile', mobile, 'third_party_user_id', third_party_user_id, 'updated_at', updated_at) AS payload
FROM people
WHERE (updated_at, id) <= (sqlc.arg(upper_watermark)::timestamptz, sqlc.arg(upper_key)::bigint)
ORDER BY updated_at, id LIMIT sqlc.arg(page_size)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: ListDM01FollowUser :many
SELECT id, updated_at, jsonb_build_object(
  'id', id, 'external_userid', external_userid, 'user_id', user_id,
  'relation_status', relation_status, 'is_primary', is_primary, 'remark', remark,
  'description', description, 'updated_at', updated_at, 'corp_id', corp_id,
  'raw_follow_user', raw_follow_user, 'first_seen_at', first_seen_at,
  'last_seen_at', last_seen_at, 'created_at', created_at) AS payload
FROM wecom_external_contact_follow_users
WHERE (updated_at, id) <= (sqlc.arg(upper_watermark)::timestamptz, sqlc.arg(upper_key)::bigint)
ORDER BY updated_at, id LIMIT sqlc.arg(page_size)::integer OFFSET sqlc.arg(page_offset)::integer;
