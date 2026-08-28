-- +goose Up
-- Historical observations only; never a Campaign command or worker input.
CREATE TABLE campaign_v1_definition_history (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id BIGINT NOT NULL UNIQUE,
    code TEXT NOT NULL,
    display_name TEXT NOT NULL,
    intent TEXT NOT NULL,
    anchor_mode TEXT NOT NULL,
    anchor_date TEXT NOT NULL,
    review_status TEXT NOT NULL,
    run_status TEXT NOT NULL,
    approved_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    paused_at TIMESTAMPTZ,
    paused_reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    original_disposition TEXT NOT NULL CHECK (original_disposition IN ('archive', 'quarantine')),
    original_reason TEXT NOT NULL CHECK (btrim(original_reason) <> ''),
    private_digest BYTEA NOT NULL CHECK (octet_length(private_digest) = 32),
    source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
    source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
    source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
    redacted_roots TEXT[] NOT NULL
);

CREATE TABLE campaign_v1_definition_step_history (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id BIGINT NOT NULL UNIQUE,
    campaign_source_id BIGINT NOT NULL,
    segment_source_id BIGINT NOT NULL,
    history_definition_id BIGINT REFERENCES campaign_v1_definition_history(id),
    current_campaign_code TEXT REFERENCES cloud_campaigns(campaign_code),
    source_parent_state TEXT NOT NULL CHECK (source_parent_state IN ('history_definition', 'current_definition', 'unresolved_definition')),
    step_index INTEGER NOT NULL,
    day_offset INTEGER NOT NULL,
    send_time TEXT NOT NULL,
    timezone TEXT NOT NULL,
    content_masked TEXT NOT NULL,
    stop_on_reply BOOLEAN NOT NULL,
    skip_recent_days INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    original_disposition TEXT NOT NULL CHECK (original_disposition IN ('archive', 'quarantine')),
    original_reason TEXT NOT NULL CHECK (btrim(original_reason) <> ''),
    content_digest BYTEA NOT NULL CHECK (octet_length(content_digest) = 32),
    private_digest BYTEA NOT NULL CHECK (octet_length(private_digest) = 32),
    source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
    source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
    source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
    redacted_roots TEXT[] NOT NULL,
    CHECK (
        (source_parent_state = 'history_definition' AND history_definition_id IS NOT NULL AND current_campaign_code IS NULL)
        OR (source_parent_state = 'current_definition' AND current_campaign_code IS NOT NULL AND history_definition_id IS NULL)
        OR (source_parent_state = 'unresolved_definition' AND history_definition_id IS NULL AND current_campaign_code IS NULL)
    )
);
CREATE INDEX campaign_v1_definition_step_history_source_parent
    ON campaign_v1_definition_step_history(campaign_source_id, id);

-- +goose Down
DROP TABLE campaign_v1_definition_step_history;
DROP TABLE campaign_v1_definition_history;
