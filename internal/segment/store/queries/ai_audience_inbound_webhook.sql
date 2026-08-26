-- name: AIAudienceInboundPackageExists :one
SELECT EXISTS (
  SELECT 1
  FROM public.ai_audience_package_metadata
  WHERE segment_id = sqlc.arg(package_id)::bigint
);

-- name: CreateAIAudienceInboundWebhookReceipt :one
INSERT INTO public.ai_audience_inbound_webhook_receipts (
  package_id,
  external_event_id_digest,
  payload_digest,
  member_event_id,
  callback_status,
  message_json,
  action_json,
  state,
  created_at
) VALUES (
  sqlc.arg(package_id)::bigint,
  sqlc.arg(external_event_id_digest)::bytea,
  sqlc.arg(payload_digest)::bytea,
  sqlc.narg(member_event_id)::bigint,
  sqlc.arg(callback_status)::text,
  sqlc.arg(message_json)::jsonb,
  sqlc.arg(action_json)::jsonb,
  'received',
  sqlc.arg(created_at)::timestamptz
)
ON CONFLICT (package_id, external_event_id_digest) DO NOTHING
RETURNING id, package_id, state, external_event_id_digest, payload_digest, created_at;

-- name: GetAIAudienceInboundWebhookReceipt :one
SELECT id, package_id, state, external_event_id_digest, payload_digest, created_at
FROM public.ai_audience_inbound_webhook_receipts
WHERE package_id = sqlc.arg(package_id)::bigint
  AND external_event_id_digest = sqlc.arg(external_event_id_digest)::bytea;

-- name: CreateAIAudienceInboundWebhookTransportReplay :one
INSERT INTO public.ai_audience_webhook_transport_replays (
  client_id,
  event_id_digest,
  receipt_id,
  payload_digest,
  created_at
) VALUES (
  sqlc.arg(client_id)::text,
  sqlc.arg(event_id_digest)::bytea,
  sqlc.arg(receipt_id)::bigint,
  sqlc.arg(payload_digest)::bytea,
  sqlc.arg(created_at)::timestamptz
)
ON CONFLICT (client_id, event_id_digest) DO NOTHING
RETURNING receipt_id, payload_digest;

-- name: GetAIAudienceInboundWebhookTransportReplay :one
SELECT receipt_id, payload_digest
FROM public.ai_audience_webhook_transport_replays
WHERE client_id = sqlc.arg(client_id)::text
  AND event_id_digest = sqlc.arg(event_id_digest)::bytea;
