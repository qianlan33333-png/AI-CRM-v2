-- name: ListStages :many
SELECT id, name, sort_order, config
FROM stages
ORDER BY sort_order ASC, id ASC;

-- name: InsertStage :one
INSERT INTO stages (name, sort_order, config)
VALUES (
  sqlc.arg(name),
  sqlc.arg(sort_order),
  sqlc.arg(config)::jsonb
)
RETURNING id, name, sort_order, config;

-- name: RenameStage :one
UPDATE stages
SET name = sqlc.arg(name)
WHERE id = sqlc.arg(id)
RETURNING id, name, sort_order, config;
