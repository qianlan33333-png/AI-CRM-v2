-- name: ReadCustomerProjection :one
SELECT id, name
FROM customers
WHERE id = sqlc.arg(customer_id)::bigint AND NOT is_deleted;
