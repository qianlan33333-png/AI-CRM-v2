-- name: CreateGroupOpsHistoricalPlanMarker :one
INSERT INTO group_ops_v1_history_plans(plan_id,source_plan_id,source_code,plan_type,original_status,owner_staff_id,archived_at)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *;

-- name: GetGroupOpsHistoricalPlan :one
SELECT p.*,h.source_plan_id,h.source_code,h.plan_type,h.original_status,h.owner_staff_id,h.archived_at
FROM group_ops_plans p JOIN group_ops_v1_history_plans h ON h.plan_id=p.id WHERE p.id=$1;

-- name: ListGroupOpsHistoricalPlans :many
SELECT p.*,h.source_plan_id,h.source_code,h.plan_type,h.original_status,h.owner_staff_id,h.archived_at
FROM group_ops_plans p JOIN group_ops_v1_history_plans h ON h.plan_id=p.id ORDER BY p.id
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountGroupOpsHistoricalPlans :one
SELECT count(*) FROM group_ops_v1_history_plans;

-- name: CreateGroupOpsHistoricalDirectory :one
INSERT INTO group_ops_v1_history_directory(source_kind,source_id,chat_reference,display_name,owner_staff_id,owner_name,member_count,internal_member_count,external_member_count,original_status,recorded_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING *;

-- name: GetGroupOpsHistoricalDirectory :one
SELECT * FROM group_ops_v1_history_directory WHERE id=$1;

-- name: ListGroupOpsHistoricalDirectory :many
SELECT * FROM group_ops_v1_history_directory ORDER BY id LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountGroupOpsHistoricalDirectory :one
SELECT count(*) FROM group_ops_v1_history_directory;

-- name: CreateGroupOpsHistoricalGroup :one
INSERT INTO group_ops_v1_history_groups(source_group_id,source_plan_id,plan_id,chat_reference,display_name,owner_staff_id,internal_member_count,external_member_count,original_status,created_at,removed_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING *;

-- name: GetGroupOpsHistoricalGroup :one
SELECT * FROM group_ops_v1_history_groups WHERE id=$1;

-- name: ListGroupOpsHistoricalGroups :many
SELECT * FROM group_ops_v1_history_groups WHERE plan_id=$1 ORDER BY id LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountGroupOpsHistoricalGroups :one
SELECT count(*) FROM group_ops_v1_history_groups WHERE plan_id=$1;

-- name: CreateGroupOpsHistoricalNode :one
INSERT INTO group_ops_v1_history_nodes(source_node_id,source_plan_id,plan_id,day_index,trigger_time,sort_order,original_status,content_package,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;

-- name: GetGroupOpsHistoricalNode :one
SELECT * FROM group_ops_v1_history_nodes WHERE id=$1;

-- name: ListGroupOpsHistoricalNodes :many
SELECT * FROM group_ops_v1_history_nodes WHERE plan_id=$1 ORDER BY sort_order,id LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountGroupOpsHistoricalNodes :one
SELECT count(*) FROM group_ops_v1_history_nodes WHERE plan_id=$1;
