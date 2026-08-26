-- AI Audience inbound webhooks record authenticated local ingress only. They
-- never configure an outbound subscription, invoke a Provider, or imply send
-- or delivery success.

-- +goose Up
CREATE TABLE public.ai_audience_inbound_webhook_receipts (
  id                       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  package_id               BIGINT NOT NULL REFERENCES public.ai_audience_package_metadata(segment_id) ON DELETE RESTRICT,
  external_event_id_digest BYTEA NOT NULL,
  payload_digest           BYTEA NOT NULL,
  member_event_id          BIGINT,
  callback_status          TEXT NOT NULL,
  message_json             JSONB NOT NULL,
  action_json              JSONB NOT NULL,
  state                    TEXT NOT NULL DEFAULT 'received',
  created_at               TIMESTAMPTZ NOT NULL,
  CONSTRAINT ai_audience_inbound_webhook_receipts_external_event_digest CHECK (octet_length(external_event_id_digest) = 32),
  CONSTRAINT ai_audience_inbound_webhook_receipts_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT ai_audience_inbound_webhook_receipts_member_event CHECK (member_event_id IS NULL OR member_event_id > 0),
  CONSTRAINT ai_audience_inbound_webhook_receipts_callback_status CHECK (
    callback_status = btrim(callback_status) AND char_length(callback_status) <= 64
  ),
  CONSTRAINT ai_audience_inbound_webhook_receipts_message_json CHECK (jsonb_typeof(message_json) = 'object'),
  CONSTRAINT ai_audience_inbound_webhook_receipts_action_json CHECK (jsonb_typeof(action_json) = 'object'),
  CONSTRAINT ai_audience_inbound_webhook_receipts_state CHECK (state = 'received'),
  CONSTRAINT ai_audience_inbound_webhook_receipts_event_once UNIQUE (package_id, external_event_id_digest)
);

CREATE TABLE public.ai_audience_webhook_transport_replays (
  client_id       TEXT NOT NULL,
  event_id_digest BYTEA NOT NULL,
  receipt_id      BIGINT NOT NULL REFERENCES public.ai_audience_inbound_webhook_receipts(id) ON DELETE RESTRICT,
  payload_digest  BYTEA NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL,
  CONSTRAINT ai_audience_webhook_transport_replays_client CHECK (
    client_id = btrim(client_id) AND char_length(client_id) BETWEEN 1 AND 128
  ),
  CONSTRAINT ai_audience_webhook_transport_replays_event_digest CHECK (octet_length(event_id_digest) = 32),
  CONSTRAINT ai_audience_webhook_transport_replays_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT ai_audience_webhook_transport_replays_event_once UNIQUE (client_id, event_id_digest)
);

-- +goose Down
LOCK TABLE public.ai_audience_webhook_transport_replays,
           public.ai_audience_inbound_webhook_receipts
  IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.ai_audience_inbound_webhook_receipts)
     OR EXISTS (SELECT 1 FROM public.ai_audience_webhook_transport_replays) THEN
    RAISE EXCEPTION 'cannot roll back populated AI Audience inbound webhook facts'
      USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE public.ai_audience_webhook_transport_replays;
DROP TABLE public.ai_audience_inbound_webhook_receipts;
