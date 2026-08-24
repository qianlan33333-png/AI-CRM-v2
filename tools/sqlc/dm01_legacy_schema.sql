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
CREATE TABLE crm_user_identity_merge_audit (id bigint PRIMARY KEY, created_at timestamptz NOT NULL);
CREATE TABLE crm_user_identity_resolution_queue (id bigint PRIMARY KEY, updated_at timestamptz NOT NULL);
CREATE TABLE admin_wecom_directory_members (id bigint PRIMARY KEY, last_synced_at timestamptz NOT NULL);
CREATE TABLE contacts (id bigint PRIMARY KEY, updated_at timestamptz NOT NULL);
CREATE TABLE crm_user_identity_conflicts (id bigint PRIMARY KEY, updated_at timestamptz NOT NULL);
CREATE TABLE external_contact_bindings (external_userid text PRIMARY KEY, updated_at timestamptz NOT NULL);
CREATE TABLE people (id bigint PRIMARY KEY, updated_at timestamptz NOT NULL);
CREATE TABLE wecom_external_contact_follow_users (id bigint PRIMARY KEY, updated_at timestamptz NOT NULL);
