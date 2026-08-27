-- name: InsertHistoricalStaticProduct :one
INSERT INTO public.products
  (product_code,name,description,price_minor,currency,stock_quantity,created_by,created_at,updated_at,version,local_lifecycle,legacy_admin_projection)
VALUES
  (sqlc.arg(product_code)::text,sqlc.arg(name)::text,'',sqlc.arg(price_minor)::bigint,sqlc.arg(currency)::char(3),0,sqlc.arg(created_by)::bigint,sqlc.arg(created_at)::timestamptz,sqlc.arg(updated_at)::timestamptz,1,'disabled',sqlc.arg(legacy_admin_projection)::jsonb)
RETURNING id,product_code,name,description,price_minor,currency,stock_quantity,created_by,created_at,updated_at,version,local_lifecycle,legacy_admin_projection;
