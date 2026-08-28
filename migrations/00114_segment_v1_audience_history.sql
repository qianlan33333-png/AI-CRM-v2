-- +goose Up
-- Segment-owned V1 history only. No current Segment, membership, queue, or effect rows.

CREATE TABLE segment_v1_audience_groups (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    name text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE segment_v1_audience_packages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    group_history_id bigint REFERENCES segment_v1_audience_groups(id),
    current_version_source_id bigint,
    package_key text NOT NULL,
    name text NOT NULL,
    natural_language_definition text NOT NULL,
    original_status text NOT NULL,
    query_mode text NOT NULL,
    identity_policy text NOT NULL,
    incremental_enabled boolean NOT NULL,
    daily_enabled boolean NOT NULL,
    incremental_interval_seconds bigint NOT NULL,
    daily_refresh_time text NOT NULL,
    timezone text NOT NULL,
    lookback_seconds bigint NOT NULL,
    last_incremental_at timestamptz,
    last_daily_refreshed_at timestamptz,
    next_incremental_at timestamptz,
    next_daily_at timestamptz,
    paused_reason text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    runtime_digest bytea NOT NULL CHECK (octet_length(runtime_digest) = 32)
);

CREATE TABLE segment_v1_audience_versions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    package_history_id bigint REFERENCES segment_v1_audience_packages(id) NOT NULL,
    version_number bigint NOT NULL,
    original_status text NOT NULL,
    ai_prompt text NOT NULL,
    ai_rationale text NOT NULL,
    natural_language_explanation text NOT NULL,
    created_at timestamptz NOT NULL,
    published_at timestamptz,
    template_key text NOT NULL,
    template_version bigint,
    template_fingerprint text NOT NULL,
    definition_digest bytea NOT NULL CHECK (octet_length(definition_digest) = 32)
);
CREATE INDEX segment_v1_audience_versions_package_history_id_idx ON segment_v1_audience_versions (package_history_id, id);

CREATE TABLE segment_v1_audience_senders (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    package_history_id bigint REFERENCES segment_v1_audience_packages(id) NOT NULL,
    staff_id bigint REFERENCES staff(id),
    display_name text NOT NULL,
    priority bigint NOT NULL,
    original_status text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX segment_v1_audience_senders_package_history_id_idx ON segment_v1_audience_senders (package_history_id, id);

CREATE TABLE segment_v1_audience_rules (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    rule_key text NOT NULL,
    display_name text NOT NULL,
    description text NOT NULL,
    rule_type text NOT NULL,
    owner_staff_id bigint REFERENCES staff(id),
    original_status text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE segment_v1_audience_rule_versions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    rule_history_id bigint REFERENCES segment_v1_audience_rules(id) NOT NULL,
    version bigint NOT NULL,
    executor_type text NOT NULL,
    original_status text NOT NULL,
    published_at timestamptz,
    created_at timestamptz NOT NULL,
    definition_digest bytea NOT NULL CHECK (octet_length(definition_digest) = 32)
);
CREATE INDEX segment_v1_audience_rule_versions_rule_history_id_idx ON segment_v1_audience_rule_versions (rule_history_id, id);

CREATE TABLE segment_v1_definitions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    code text NOT NULL,
    display_name text NOT NULL,
    description text NOT NULL,
    source_type text NOT NULL,
    sql_dialect text NOT NULL,
    original_status text NOT NULL,
    version bigint NOT NULL,
    cached_headcount bigint NOT NULL,
    last_refreshed_at timestamptz,
    usage_count bigint NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    definition_digest bytea NOT NULL CHECK (octet_length(definition_digest) = 32)
);

CREATE TABLE segment_v1_audience_members (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    package_history_id bigint REFERENCES segment_v1_audience_packages(id) NOT NULL,
    customer_id bigint REFERENCES customers(id),
    identity_kind text NOT NULL,
    original_status text NOT NULL,
    first_entered_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    last_updated_at timestamptz NOT NULL,
    exited_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    payload_digest bytea NOT NULL CHECK (octet_length(payload_digest) = 32)
);
CREATE INDEX segment_v1_audience_members_package_history_id_idx ON segment_v1_audience_members (package_history_id, id);
CREATE INDEX segment_v1_audience_members_customer_id_idx ON segment_v1_audience_members (customer_id, id);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM segment_v1_audience_groups)
       OR EXISTS (SELECT 1 FROM segment_v1_audience_packages)
       OR EXISTS (SELECT 1 FROM segment_v1_audience_versions)
       OR EXISTS (SELECT 1 FROM segment_v1_audience_senders)
       OR EXISTS (SELECT 1 FROM segment_v1_audience_rules)
       OR EXISTS (SELECT 1 FROM segment_v1_audience_rule_versions)
       OR EXISTS (SELECT 1 FROM segment_v1_definitions)
       OR EXISTS (SELECT 1 FROM segment_v1_audience_members) THEN
        RAISE EXCEPTION 'refusing to remove populated V1 audience history; use a verified backup';
    END IF;
END
$$;
-- +goose StatementEnd
DROP TABLE segment_v1_audience_members;
DROP TABLE segment_v1_definitions;
DROP TABLE segment_v1_audience_rule_versions;
DROP TABLE segment_v1_audience_rules;
DROP TABLE segment_v1_audience_senders;
DROP TABLE segment_v1_audience_versions;
DROP TABLE segment_v1_audience_packages;
DROP TABLE segment_v1_audience_groups;
