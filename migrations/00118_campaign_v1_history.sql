-- +goose Up
-- Campaign-owned, non-executable V1 history. Never used by current dispatch.

CREATE TABLE campaign_v1_history_segments (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    campaign_source_id bigint NOT NULL CHECK (campaign_source_id > 0),
    segment_source_id bigint NOT NULL,
    source_parent_state text NOT NULL CHECK (source_parent_state IN ('observed','missing_campaign')),
    code text NOT NULL,
    priority integer NOT NULL,
    label text NOT NULL,
    created_at timestamptz NOT NULL,
    source_payload_digest bytea NOT NULL CHECK (octet_length(source_payload_digest) = 32)
);
CREATE INDEX campaign_v1_hist_0_campaign_source_id_idx ON campaign_v1_history_segments (campaign_source_id, id);

CREATE TABLE campaign_v1_history_members (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    campaign_source_id bigint NOT NULL,
    campaign_segment_source_id bigint NOT NULL,
    segment_source_id bigint NOT NULL,
    member_source_id bigint NOT NULL,
    segment_history_id bigint NOT NULL REFERENCES campaign_v1_history_segments(id),
    customer_id bigint REFERENCES customers(id),
    joined_at timestamptz NOT NULL,
    anchor_date text NOT NULL,
    current_step_index integer NOT NULL,
    next_due_at timestamptz,
    original_status text NOT NULL,
    stop_reason text NOT NULL,
    last_step_sent_at timestamptz,
    retry_count integer NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    source_payload_digest bytea NOT NULL CHECK (octet_length(source_payload_digest) = 32)
);
CREATE INDEX campaign_v1_hist_1_segment_history_id_idx ON campaign_v1_history_members (segment_history_id, id);
CREATE INDEX campaign_v1_hist_1_customer_id_idx ON campaign_v1_history_members (customer_id, id);

CREATE TABLE campaign_v1_history_broadcast_plans (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    source_plan_id text NOT NULL UNIQUE CHECK (source_plan_id <> ''),
    campaign_source_id bigint,
    segment_source_id bigint,
    display_name text NOT NULL,
    intent text NOT NULL,
    content_strategy text NOT NULL,
    content_template_masked text NOT NULL,
    max_recipients bigint NOT NULL,
    candidate_count bigint NOT NULL,
    skipped_count bigint NOT NULL,
    requires_manual_copy boolean NOT NULL,
    original_status text NOT NULL,
    original_review_status text NOT NULL,
    original_run_status text NOT NULL,
    committed_at timestamptz,
    expires_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    runtime_digest bytea NOT NULL CHECK (octet_length(runtime_digest) = 32),
    source_payload_digest bytea NOT NULL CHECK (octet_length(source_payload_digest) = 32)
);

CREATE TABLE campaign_v1_history_broadcast_recipients (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    plan_history_id bigint NOT NULL REFERENCES campaign_v1_history_broadcast_plans(id),
    customer_id bigint REFERENCES customers(id),
    display_name text NOT NULL,
    planned_message_count bigint NOT NULL,
    original_approval_status text NOT NULL,
    original_send_status text NOT NULL,
    approved_at timestamptz,
    rejected_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    source_payload_digest bytea NOT NULL CHECK (octet_length(source_payload_digest) = 32),
    UNIQUE (id, plan_history_id)
);
CREATE INDEX campaign_v1_hist_3_plan_history_id_idx ON campaign_v1_history_broadcast_recipients (plan_history_id, id);
CREATE INDEX campaign_v1_hist_3_customer_id_idx ON campaign_v1_history_broadcast_recipients (customer_id, id);

CREATE TABLE campaign_v1_history_broadcast_messages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id bigint NOT NULL UNIQUE CHECK (source_id > 0),
    plan_history_id bigint NOT NULL REFERENCES campaign_v1_history_broadcast_plans(id),
    recipient_history_id bigint NOT NULL REFERENCES campaign_v1_history_broadcast_recipients(id),
    customer_id bigint REFERENCES customers(id),
    sequence_index bigint NOT NULL,
    day_offset bigint NOT NULL,
    original_send_time text NOT NULL,
    content_masked text NOT NULL,
    original_status text NOT NULL,
    sent_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    content_payload_digest bytea NOT NULL CHECK (octet_length(content_payload_digest) = 32),
    attachments_digest bytea NOT NULL CHECK (octet_length(attachments_digest) = 32),
    source_payload_digest bytea NOT NULL CHECK (octet_length(source_payload_digest) = 32),
    FOREIGN KEY (recipient_history_id, plan_history_id) REFERENCES campaign_v1_history_broadcast_recipients(id, plan_history_id)
);
CREATE INDEX campaign_v1_hist_4_recipient_history_id_idx ON campaign_v1_history_broadcast_messages (recipient_history_id, id);
CREATE INDEX campaign_v1_hist_4_customer_id_idx ON campaign_v1_history_broadcast_messages (customer_id, id);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM campaign_v1_history_segments)
       OR EXISTS (SELECT 1 FROM campaign_v1_history_members)
       OR EXISTS (SELECT 1 FROM campaign_v1_history_broadcast_plans)
       OR EXISTS (SELECT 1 FROM campaign_v1_history_broadcast_recipients)
       OR EXISTS (SELECT 1 FROM campaign_v1_history_broadcast_messages) THEN
        RAISE EXCEPTION 'refusing to remove populated V1 Campaign history; use a verified backup';
    END IF;
END
$$;
-- +goose StatementEnd
DROP TABLE campaign_v1_history_broadcast_messages;
DROP TABLE campaign_v1_history_broadcast_recipients;
DROP TABLE campaign_v1_history_broadcast_plans;
DROP TABLE campaign_v1_history_members;
DROP TABLE campaign_v1_history_segments;
