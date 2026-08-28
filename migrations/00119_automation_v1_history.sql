-- +goose Up
-- Historical configuration is not a live automation, rule, prompt, or agent.
-- Source references and original enabled/published states are never executable.
CREATE TABLE automation_v1_sop_history (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 source_id BIGINT NOT NULL CHECK (source_id > 0),
 source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
 source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
 pool_key TEXT NOT NULL,
 day_index INTEGER NOT NULL,
 content_masked TEXT NOT NULL,
 images_digest BYTEA NOT NULL CHECK (octet_length(images_digest) = 32),
 original_enabled BOOLEAN NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE automation_v1_agent_config_history (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 source_id BIGINT NOT NULL CHECK (source_id > 0),
 source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
 source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
 agent_code TEXT NOT NULL,
 display_name TEXT NOT NULL,
 scenario_code TEXT NOT NULL,
 original_enabled BOOLEAN NOT NULL,
 draft_version INTEGER NOT NULL,
 published_version INTEGER NOT NULL,
 published_at TEXT NOT NULL,
 last_modified_at TEXT NOT NULL,
 last_modified_source TEXT NOT NULL,
 submitted_for_publish BOOLEAN NOT NULL,
 submitted_at TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL,
 actors_digest BYTEA NOT NULL CHECK (octet_length(actors_digest) = 32),
 config_digest BYTEA NOT NULL CHECK (octet_length(config_digest) = 32)
);

CREATE TABLE automation_v1_prompt_history (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 source_id BIGINT NOT NULL CHECK (source_id > 0),
 source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
 source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
 agent_code TEXT NOT NULL,
 display_name TEXT NOT NULL,
 original_enabled BOOLEAN NOT NULL,
 version INTEGER NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL,
 prompt_digest BYTEA NOT NULL CHECK (octet_length(prompt_digest) = 32)
);

CREATE TABLE automation_v1_agent_history (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 source_id BIGINT NOT NULL CHECK (source_id > 0),
 source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
 source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
 program_source_id BIGINT NOT NULL,
 workflow_source_id BIGINT NOT NULL,
 node_source_id BIGINT NOT NULL,
 task_source_id BIGINT NOT NULL,
 agent_code TEXT NOT NULL,
 agent_name TEXT NOT NULL,
 original_type TEXT NOT NULL,
 original_status TEXT NOT NULL,
 sort_order INTEGER NOT NULL,
 original_enabled BOOLEAN NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL,
 archived_at TEXT NOT NULL,
 actors_digest BYTEA NOT NULL CHECK (octet_length(actors_digest) = 32),
 configuration_digest BYTEA NOT NULL CHECK (octet_length(configuration_digest) = 32)
);

-- +goose Down
DROP TABLE automation_v1_agent_history;
DROP TABLE automation_v1_prompt_history;
DROP TABLE automation_v1_agent_config_history;
DROP TABLE automation_v1_sop_history;
