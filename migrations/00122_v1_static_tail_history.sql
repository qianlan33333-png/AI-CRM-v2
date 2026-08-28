-- +goose Up
-- Immutable V1 static metadata only; no executable payloads or current-domain FKs.

CREATE TABLE media_v1_group_invite_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest)=32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
  name TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  original_state TEXT NOT NULL,
  original_auto_create BOOLEAN NOT NULL,
  room_base_name TEXT NOT NULL,
  room_base_source_id BIGINT,
  original_enabled BOOLEAN NOT NULL,
  original_binding_state TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE product_v1_page_slice_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest)=32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
  product_source_id BIGINT NOT NULL,
  image_source_id BIGINT NOT NULL,
  sort_order BIGINT NOT NULL,
  original_enabled BOOLEAN NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE operation_cycle_v1_strategy_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest)=32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
  strategy_key TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  cadence TEXT NOT NULL,
  timezone TEXT NOT NULL,
  original_status TEXT NOT NULL,
  current_version BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE operation_cycle_v1_version_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest)=32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
  strategy_source_id BIGINT NOT NULL,
  strategy_history_id BIGINT NOT NULL REFERENCES operation_cycle_v1_strategy_history(id),
  version BIGINT NOT NULL,
  label TEXT NOT NULL,
  objective TEXT NOT NULL,
  version_hash TEXT NOT NULL,
  effective_from TIMESTAMPTZ,
  original_governance TEXT NOT NULL,
  confirmed_at TIMESTAMPTZ,
  operation_skill_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE operation_cycle_v1_document_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_id BIGINT NOT NULL,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest)=32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
  strategy_version_source_id BIGINT NOT NULL,
  version_history_id BIGINT NOT NULL REFERENCES operation_cycle_v1_version_history(id),
  schema_version TEXT NOT NULL,
  execution_guide_sha256 TEXT NOT NULL,
  execution_guide_generated_at TIMESTAMPTZ,
  copy_guide_sha256 TEXT NOT NULL,
  copy_guide_generated_at TIMESTAMPTZ,
  measurement_guide_sha256 TEXT NOT NULL,
  measurement_guide_generated_at TIMESTAMPTZ,
  document_pack_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX operation_cycle_v1_version_parent_idx ON operation_cycle_v1_version_history(strategy_history_id,id);
CREATE INDEX operation_cycle_v1_document_parent_idx ON operation_cycle_v1_document_history(version_history_id,id);

-- +goose Down
DROP TABLE operation_cycle_v1_document_history;
DROP TABLE operation_cycle_v1_version_history;
DROP TABLE operation_cycle_v1_strategy_history;
DROP TABLE product_v1_page_slice_history;
DROP TABLE media_v1_group_invite_history;
