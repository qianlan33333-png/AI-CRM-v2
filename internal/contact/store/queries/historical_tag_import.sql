-- name: LockHistoricalTagImportGroup :one
SELECT id, name, sort_order
FROM public.tag_groups
WHERE id = sqlc.arg(group_id)::bigint
FOR KEY SHARE;

-- name: CreateHistoricalTagImportGroup :one
INSERT INTO public.tag_groups (name, sort_order)
VALUES (sqlc.arg(name)::text, sqlc.arg(sort_order)::integer)
RETURNING id, name, sort_order;

-- name: LockHistoricalTagImport :one
SELECT id, group_id, wecom_tag_id, name, sort_order
FROM public.tags
WHERE id = sqlc.arg(tag_id)::bigint
FOR KEY SHARE;

-- name: LockHistoricalTagImportByProviderID :one
SELECT id, group_id, wecom_tag_id, name, sort_order
FROM public.tags
WHERE wecom_tag_id = sqlc.arg(provider_tag_id)::text
FOR KEY SHARE;

-- name: CreateHistoricalTagImport :one
INSERT INTO public.tags (group_id, wecom_tag_id, name, sort_order)
VALUES (
  sqlc.arg(group_id)::bigint,
  sqlc.arg(provider_tag_id)::text,
  sqlc.arg(name)::text,
  sqlc.arg(sort_order)::integer
)
ON CONFLICT (wecom_tag_id) DO NOTHING
RETURNING id, group_id, wecom_tag_id, name, sort_order;

-- name: LockHistoricalCustomerTagImport :one
SELECT customer_id, tag_id, tagged_at, tagged_by
FROM public.customer_tags
WHERE customer_id = sqlc.arg(customer_id)::bigint
  AND tag_id = sqlc.arg(tag_id)::bigint
FOR KEY SHARE;

-- name: BindHistoricalCustomerTagImport :one
INSERT INTO public.customer_tags (customer_id, tag_id, tagged_at, tagged_by)
VALUES (
  sqlc.arg(customer_id)::bigint,
  sqlc.arg(tag_id)::bigint,
  sqlc.arg(tagged_at)::timestamptz,
  sqlc.arg(tagged_by)::text
)
ON CONFLICT (customer_id, tag_id) DO NOTHING
RETURNING customer_id, tag_id, tagged_at, tagged_by;
