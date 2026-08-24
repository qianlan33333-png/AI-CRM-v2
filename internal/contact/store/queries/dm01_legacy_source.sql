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
SELECT userid, display_name, active, updated_at,
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
       first_seen_at, last_seen_at, updated_at,
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
