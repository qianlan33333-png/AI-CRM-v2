-- name: InsertCustomerAcquisitionLinkReceipt :one
INSERT INTO public.wecom_customer_acquisition_link_receipts (
  actor_id, key_digest, request_digest, operation, link_id, command_input, state
) VALUES (
  sqlc.arg(actor_id)::bigint, sqlc.arg(key_digest)::bytea, sqlc.arg(request_digest)::bytea,
  sqlc.arg(operation)::text, sqlc.arg(link_id)::text, sqlc.arg(command_input)::jsonb, 'accepted'
)
ON CONFLICT (actor_id, key_digest) DO NOTHING
RETURNING *;

-- name: GetCustomerAcquisitionLinkReceiptByKey :one
SELECT *
FROM public.wecom_customer_acquisition_link_receipts
WHERE actor_id = sqlc.arg(actor_id)::bigint
  AND key_digest = sqlc.arg(key_digest)::bytea;

-- name: GetCustomerAcquisitionLinkReceipt :one
SELECT *
FROM public.wecom_customer_acquisition_link_receipts
WHERE id = sqlc.arg(id)::bigint;

-- name: MarkCustomerAcquisitionLinkReceiptAttempted :one
UPDATE public.wecom_customer_acquisition_link_receipts
SET state = 'attempted', updated_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND state = 'accepted'
RETURNING *;

-- name: CompleteCustomerAcquisitionLinkReceipt :one
UPDATE public.wecom_customer_acquisition_link_receipts
SET state = sqlc.arg(state)::text,
    provider_link = sqlc.narg(provider_link)::jsonb,
    outcome_digest = sqlc.arg(outcome_digest)::bytea,
    business_endpoint_dispatched = sqlc.arg(business_endpoint_dispatched)::boolean,
    real_external_call_executed = sqlc.arg(real_external_call_executed)::boolean,
    reconcile_actor_id = sqlc.narg(reconcile_actor_id)::bigint,
    reconcile_key_digest = sqlc.arg(reconcile_key_digest)::bytea,
    evidence_digest = sqlc.arg(evidence_digest)::bytea,
    resolution = sqlc.narg(resolution)::text,
    updated_at = now()
WHERE id = sqlc.arg(id)::bigint
  AND (
    (state = 'attempted' AND sqlc.arg(state)::text IN ('executed', 'final_failed', 'outcome_unknown'))
    OR (state = 'outcome_unknown' AND sqlc.arg(state)::text = 'reconciled')
  )
RETURNING *;
