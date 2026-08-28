-- +goose Up
-- Immutable V1 observations; never consumed by the current task worker.
CREATE TABLE outbound_v1_task_history (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id BIGINT NOT NULL UNIQUE,
    task_type TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    broadcast_job_history_id BIGINT REFERENCES outbound_v1_broadcast_job_history(id),
    request_payload_digest BYTEA NOT NULL CHECK (octet_length(request_payload_digest) = 32),
    response_payload_digest BYTEA NOT NULL CHECK (octet_length(response_payload_digest) = 32),
    wecom_task_id_digest BYTEA CHECK (wecom_task_id_digest IS NULL OR octet_length(wecom_task_id_digest) = 32),
    trace_id_digest BYTEA NOT NULL CHECK (octet_length(trace_id_digest) = 32),
    legacy_broadcast_job_id BIGINT,
    source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
    source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
    source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
    redacted_roots TEXT[] NOT NULL
);

-- +goose Down
DROP TABLE outbound_v1_task_history;
