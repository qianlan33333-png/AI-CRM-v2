-- sqlc-only source catalog for the pinned legacy DM01 database. This file is
-- compile input, not a target migration.
CREATE TABLE owner_role_map (
  userid text PRIMARY KEY, display_name text NOT NULL, role text NOT NULL,
  active boolean NOT NULL, source text NOT NULL, raw_payload_json jsonb NOT NULL,
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE TABLE crm_user_identity (
  unionid text PRIMARY KEY, primary_external_userid text NOT NULL,
  external_userids_json jsonb NOT NULL, primary_openid text NOT NULL,
  openids_json jsonb NOT NULL, mobile text NOT NULL, mobile_normalized text NOT NULL,
  mobile_verified boolean NOT NULL, mobile_source text NOT NULL,
  customer_name text NOT NULL, remark text NOT NULL, description text NOT NULL,
  avatar text NOT NULL, gender integer, profile_json jsonb NOT NULL,
  primary_owner_userid text NOT NULL, follow_users_json jsonb NOT NULL,
  legacy_person_id text NOT NULL, legacy_identity_map_ids_json jsonb NOT NULL,
  legacy_sources_json jsonb NOT NULL, identity_status text NOT NULL,
  unionid_resolved_at timestamptz, first_seen_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL, last_polled_at timestamptz,
  next_poll_at timestamptz, poll_attempt_count integer NOT NULL,
  last_poll_error text NOT NULL, created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
CREATE TABLE wecom_external_contact_identity_map (
  id bigint PRIMARY KEY, external_userid text NOT NULL, unionid text NOT NULL,
  openid text NOT NULL, follow_user_userid text NOT NULL, name text NOT NULL,
  status text NOT NULL, updated_at timestamptz NOT NULL, corp_id text NOT NULL,
  avatar text NOT NULL, gender integer, raw_profile jsonb NOT NULL,
  first_seen_at timestamptz NOT NULL, last_seen_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL
);
CREATE TABLE crm_user_identity_merge_audit (
  id bigint PRIMARY KEY, from_unionid text NOT NULL, to_unionid text NOT NULL,
  reason text NOT NULL, before_json jsonb NOT NULL, after_json jsonb NOT NULL,
  operator text NOT NULL, created_at timestamptz NOT NULL
);
CREATE TABLE crm_user_identity_resolution_queue (
  id bigint PRIMARY KEY, source_type text NOT NULL, source_key text NOT NULL,
  source_table text NOT NULL, source_id text NOT NULL, corp_id text NOT NULL,
  external_userid text NOT NULL, openid text NOT NULL, mobile text NOT NULL,
  payload_json jsonb NOT NULL, raw_payload_json jsonb NOT NULL, reason text NOT NULL,
  status text NOT NULL, resolved_unionid text NOT NULL, conflict_reason text NOT NULL,
  attempts integer NOT NULL, attempt_count integer NOT NULL, last_error text NOT NULL,
  next_attempt_at timestamptz, resolved_at timestamptz, first_seen_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL, created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL, execution_id text NOT NULL,
  parent_execution_id text NOT NULL, external_effect_job_id bigint,
  lane text NOT NULL, row_version bigint NOT NULL, hold_reason text NOT NULL,
  held_at timestamptz, completed_at timestamptz
);
CREATE TABLE admin_wecom_directory_members (
  id bigint PRIMARY KEY, corp_id text NOT NULL, wecom_userid text NOT NULL,
  display_name text NOT NULL, department_ids_json jsonb NOT NULL,
  department_name text NOT NULL, position text NOT NULL, mobile text NOT NULL,
  avatar_url text NOT NULL, wecom_status integer NOT NULL, is_active boolean NOT NULL,
  raw_payload_json jsonb NOT NULL, first_seen_at timestamptz NOT NULL,
  last_synced_at timestamptz NOT NULL, created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL, updated_by text NOT NULL
);
CREATE TABLE contacts (id bigint PRIMARY KEY, unionid text NOT NULL, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL);
CREATE TABLE crm_user_identity_conflicts (
  id bigint PRIMARY KEY, conflict_type text NOT NULL, unionid text NOT NULL,
  candidate_unionid text NOT NULL, external_userid text NOT NULL, openid text NOT NULL,
  mobile text NOT NULL, source_type text NOT NULL, source_key text NOT NULL,
  payload_json jsonb NOT NULL, source_payload_json jsonb NOT NULL, status text NOT NULL,
  resolution_status text NOT NULL, resolution_note text NOT NULL,
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL, resolved_at timestamptz
);
CREATE TABLE external_contact_bindings (
  external_userid text PRIMARY KEY, person_id text, first_owner_userid text NOT NULL,
  last_owner_userid text NOT NULL, updated_at timestamptz NOT NULL
);
CREATE TABLE people (id bigint PRIMARY KEY, mobile text NOT NULL, third_party_user_id text NOT NULL, updated_at timestamptz NOT NULL);
CREATE TABLE wecom_external_contact_follow_users (
  id bigint PRIMARY KEY, external_userid text NOT NULL, user_id text NOT NULL,
  relation_status text NOT NULL, is_primary boolean NOT NULL, remark text NOT NULL,
  description text NOT NULL, updated_at timestamptz NOT NULL, corp_id text NOT NULL,
  raw_follow_user jsonb NOT NULL, first_seen_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL, created_at timestamptz NOT NULL
);
