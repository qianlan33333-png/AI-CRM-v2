-- name: CreateAcceptedOutboundTask :one
INSERT INTO outbound_tasks (customer_id, template_key, payload)
VALUES (
  sqlc.arg(customer_id)::bigint,
  sqlc.arg(template_key)::text,
  sqlc.arg(payload)::jsonb
)
RETURNING id;
