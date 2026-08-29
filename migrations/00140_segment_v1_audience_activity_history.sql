-- +goose Up
-- Immutable V1 Audience activity observations only. These records never drive
-- a current Audience refresh, membership change, event, queue, or Provider call.

CREATE TABLE segment_v1_audience_activity_runs (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_key_digest bytea NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
    source_payload_digest bytea NOT NULL CHECK (octet_length(source_payload_digest) = 32),
    source_field_digest bytea NOT NULL CHECK (octet_length(source_field_digest) = 32),
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    package_history_id bigint NOT NULL REFERENCES segment_v1_audience_packages(id),
    version_history_id bigint REFERENCES segment_v1_audience_versions(id),
    run_type text NOT NULL,
    original_status text NOT NULL,
    refresh_started_at timestamptz NOT NULL,
    refresh_finished_at timestamptz,
    last_watermark_at timestamptz,
    next_watermark_at timestamptz,
    returned_count integer NOT NULL,
    entered_count integer NOT NULL,
    updated_count integer NOT NULL,
    exited_count integer NOT NULL,
    member_event_count integer NOT NULL,
    duration_ms integer NOT NULL,
    created_at timestamptz NOT NULL,
    private_digest bytea NOT NULL CHECK (octet_length(private_digest) = 32)
);
CREATE INDEX segment_v1_audience_activity_runs_package_history_id_idx ON segment_v1_audience_activity_runs(package_history_id, id);

CREATE TABLE segment_v1_audience_activity_member_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_key_digest bytea NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
    source_payload_digest bytea NOT NULL CHECK (octet_length(source_payload_digest) = 32),
    source_field_digest bytea NOT NULL CHECK (octet_length(source_field_digest) = 32),
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    package_history_id bigint NOT NULL REFERENCES segment_v1_audience_packages(id),
    run_history_id bigint REFERENCES segment_v1_audience_activity_runs(id),
    member_history_id bigint REFERENCES segment_v1_audience_members(id),
    event_type text NOT NULL,
    identity_kind text NOT NULL,
    occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    private_digest bytea NOT NULL CHECK (octet_length(private_digest) = 32)
);
CREATE INDEX segment_v1_audience_activity_member_events_package_history_id_idx ON segment_v1_audience_activity_member_events(package_history_id, id);
CREATE INDEX segment_v1_audience_activity_member_events_run_history_id_idx ON segment_v1_audience_activity_member_events(run_history_id, id);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM segment_v1_audience_activity_member_events)
       OR EXISTS (SELECT 1 FROM segment_v1_audience_activity_runs) THEN
        RAISE EXCEPTION 'refusing to remove populated V1 audience activity history; use a verified backup';
    END IF;
END
$$;
-- +goose StatementEnd
DROP TABLE segment_v1_audience_activity_member_events;
DROP TABLE segment_v1_audience_activity_runs;
