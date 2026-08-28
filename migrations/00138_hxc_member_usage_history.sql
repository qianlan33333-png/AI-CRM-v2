-- +goose Up
-- HXC-owned immutable generation observations, never current rights or owner state.
-- Preserve source JSON text byte-for-byte for typed digest verification.
CREATE TABLE hxc_v1_member_usage_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  generation BIGINT NOT NULL,
  unionid TEXT NOT NULL,
  owner_userid TEXT NOT NULL,
  mobile_hash TEXT NOT NULL,
  is_member BOOLEAN NOT NULL,
  is_registered BOOLEAN NOT NULL,
  registered_at TIMESTAMPTZ,
  has_real_usage BOOLEAN NOT NULL,
  first_used_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ,
  member_since TIMESTAMPTZ,
  membership_expires_at TIMESTAMPTZ,
  membership_tier TEXT NOT NULL,
  membership_status TEXT NOT NULL,
  membership_source TEXT NOT NULL,
  registration_source TEXT NOT NULL,
  usage_source TEXT NOT NULL,
  updated_at TIMESTAMPTZ,
  payload_json TEXT NOT NULL,
  projected_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX hxc_v1_member_usage_history_generation_id ON hxc_v1_member_usage_history (generation, id);

-- +goose Down
DROP TABLE hxc_v1_member_usage_history;
