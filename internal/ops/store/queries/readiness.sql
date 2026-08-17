-- name: Ping :one
SELECT 1::bigint AS value;

-- name: CurrentSchemaCompatible :one
SELECT
  to_regclass('public.media_miniprograms') IS NOT NULL
  AND to_regclass('public.media_miniprogram_operation_receipts') IS NOT NULL AS compatible;
