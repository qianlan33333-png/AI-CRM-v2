-- +goose Up
-- Inert source facts only; no current binding, executable asset or public redirect.
CREATE TABLE contact_v1_unbound_tag_history (
id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest)=32) UNIQUE,
source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest)=32),
private_digest BYTEA NOT NULL CHECK (octet_length(private_digest)=32),
redacted_roots TEXT[] NOT NULL,
tag_source_id TEXT NOT NULL,
union_id_digest BYTEA NOT NULL CHECK (octet_length(union_id_digest)=32),
created_at TIMESTAMPTZ NOT NULL,
quarantine_reason TEXT NOT NULL CHECK (quarantine_reason='invalid_contact_tag')
);

CREATE TABLE contact_v1_invalid_channel_history (
id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest)=32) UNIQUE,
source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest)=32),
private_digest BYTEA NOT NULL CHECK (octet_length(private_digest)=32),
redacted_roots TEXT[] NOT NULL,
source_id BIGINT NOT NULL,
code TEXT NOT NULL,
name TEXT NOT NULL,
channel_type TEXT NOT NULL,
carrier_type TEXT NOT NULL,
created_at TIMESTAMPTZ NOT NULL,
updated_at TIMESTAMPTZ NOT NULL,
quarantine_reason TEXT NOT NULL CHECK (quarantine_reason='invalid_channel_definition')
);

CREATE TABLE media_v1_invalid_asset_history (
id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest)=32) UNIQUE,
source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest)=32),
private_digest BYTEA NOT NULL CHECK (octet_length(private_digest)=32),
redacted_roots TEXT[] NOT NULL,
kind TEXT NOT NULL CHECK (kind IN ('image','attachment')),
source_id BIGINT NOT NULL,
name TEXT NOT NULL,
file_name TEXT NOT NULL,
mime_type TEXT NOT NULL,
file_size BIGINT NOT NULL,
original_enabled BOOLEAN NOT NULL,
content_digest BYTEA NOT NULL CHECK (octet_length(content_digest)=32),
created_at TIMESTAMPTZ NOT NULL,
updated_at TIMESTAMPTZ NOT NULL,
quarantine_reason TEXT NOT NULL CHECK (quarantine_reason='invalid_static_media_definition')
);

CREATE TABLE radar_v1_invalid_link_history (
id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
source_key_digest BYTEA NOT NULL CHECK (octet_length(source_key_digest)=32) UNIQUE,
source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest)=32),
source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest)=32),
private_digest BYTEA NOT NULL CHECK (octet_length(private_digest)=32),
redacted_roots TEXT[] NOT NULL,
source_id BIGINT NOT NULL,
code TEXT NOT NULL,
title TEXT NOT NULL,
destination_url_digest BYTEA NOT NULL CHECK (octet_length(destination_url_digest)=32),
created_at TIMESTAMPTZ NOT NULL,
updated_at TIMESTAMPTZ NOT NULL,
quarantine_reason TEXT NOT NULL CHECK (quarantine_reason='invalid_radar_definition')
);

-- +goose Down
DROP TABLE radar_v1_invalid_link_history;
DROP TABLE media_v1_invalid_asset_history;
DROP TABLE contact_v1_invalid_channel_history;
DROP TABLE contact_v1_unbound_tag_history;
