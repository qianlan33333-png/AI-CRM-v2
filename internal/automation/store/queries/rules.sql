-- name: CreateAutomationRule :one
INSERT INTO automations
  (automation_code,automation_name,status,current_version,trigger_type,condition_json,action_json,created_by,updated_by,created_at,updated_at)
VALUES
  (sqlc.arg(automation_code),sqlc.arg(automation_name),sqlc.arg(status),1,sqlc.arg(trigger_type),sqlc.arg(condition_json),sqlc.arg(action_json),sqlc.arg(actor_id),sqlc.arg(actor_id),sqlc.arg(now),sqlc.arg(now))
RETURNING *;

-- name: UpdateAutomationRule :one
UPDATE automations
   SET automation_name=sqlc.arg(automation_name), status=sqlc.arg(status), current_version=current_version+1,
       condition_json=sqlc.arg(condition_json), action_json=sqlc.arg(action_json), updated_by=sqlc.arg(actor_id), updated_at=sqlc.arg(now)
 WHERE id=sqlc.arg(id) AND status <> 'archived'
RETURNING *;

-- name: SetAutomationRuleStatus :one
UPDATE automations
   SET status=sqlc.arg(status), updated_by=sqlc.arg(actor_id), updated_at=sqlc.arg(now)
 WHERE id=sqlc.arg(id) AND status <> 'archived'
RETURNING *;

-- name: GetAutomationRule :one
SELECT * FROM automations WHERE id=$1;

-- name: ListAutomationRules :many
SELECT * FROM automations WHERE status <> 'archived' ORDER BY updated_at DESC,id DESC;

-- name: ListActiveAutomationRulesForTagApplied :many
SELECT * FROM automations WHERE status='active' AND trigger_type='customer.tag_applied' ORDER BY id;

-- name: CreateAutomationRuleVersion :exec
INSERT INTO automation_rule_versions (automation_id,version,trigger_type,condition_json,action_json,published_at,published_by)
VALUES (sqlc.arg(automation_id),sqlc.arg(version),sqlc.arg(trigger_type),sqlc.arg(condition_json),sqlc.arg(action_json),sqlc.arg(now),sqlc.arg(actor_id));

-- name: ReserveAutomationEnrollment :one
INSERT INTO automation_enrollments (automation_id,automation_version,source_event_id,customer_id,trigger_payload,state,enrolled_at)
VALUES (sqlc.arg(automation_id),sqlc.arg(automation_version),sqlc.arg(source_event_id),sqlc.arg(customer_id),sqlc.arg(trigger_payload),'enrolled',sqlc.arg(now))
ON CONFLICT (automation_id,source_event_id) DO UPDATE SET source_event_id=EXCLUDED.source_event_id
RETURNING *;

-- name: CreateAutomationExecutionAction :one
INSERT INTO automation_execution_actions (enrollment_id,action_type,action_snapshot,state,created_at)
VALUES (sqlc.arg(enrollment_id),sqlc.arg(action_type),sqlc.arg(action_snapshot),'queued',sqlc.arg(now))
ON CONFLICT (enrollment_id) DO UPDATE SET enrollment_id=EXCLUDED.enrollment_id
RETURNING *;

-- name: CompleteAutomationRecordAction :one
WITH action AS (
  UPDATE automation_execution_actions
     SET state='completed', receipt_digest=sqlc.arg(receipt_digest), completed_at=sqlc.arg(now)
   WHERE automation_execution_actions.id=sqlc.arg(action_id) AND automation_execution_actions.state='queued'
  RETURNING enrollment_id
), enrollment AS (
  UPDATE automation_enrollments
     SET state='completed', completed_at=sqlc.arg(now)
   WHERE id=(SELECT enrollment_id FROM action)
  RETURNING id
)
SELECT enrollment.id FROM enrollment;

-- name: MarkAutomationActionOutcomeUnknown :one
WITH action AS (
  UPDATE automation_execution_actions
     SET state='outcome_unknown', receipt_digest=sqlc.arg(receipt_digest), completed_at=sqlc.arg(now)
   WHERE automation_execution_actions.id=sqlc.arg(action_id) AND automation_execution_actions.state='queued'
  RETURNING enrollment_id
), enrollment AS (
  UPDATE automation_enrollments
     SET state='outcome_unknown', completed_at=sqlc.arg(now)
   WHERE id=(SELECT enrollment_id FROM action)
  RETURNING id
)
SELECT enrollment.id FROM enrollment;

-- name: AttachAutomationActionExternalEffect :one
UPDATE automation_execution_actions
   SET external_effect_id=sqlc.arg(external_effect_id)
 WHERE id=sqlc.arg(action_id)
   AND action_type='outbound_message'
   AND state='queued'
   AND external_effect_id IS NULL
RETURNING *;

-- name: GetAutomationActionForReconcile :one
SELECT *
  FROM automation_execution_actions
 WHERE id=sqlc.arg(action_id)
   AND action_type='outbound_message'
 FOR UPDATE;

-- name: ProjectAutomationActionTerminalEffect :one
WITH action AS (
  UPDATE automation_execution_actions
     SET state=sqlc.arg(state), receipt_digest=sqlc.arg(receipt_digest), completed_at=sqlc.arg(now)
   WHERE external_effect_id=sqlc.arg(external_effect_id)
     AND action_type='outbound_message'
     AND state IN ('queued','outcome_unknown')
  RETURNING enrollment_id
), enrollment AS (
  UPDATE automation_enrollments
     SET state=sqlc.arg(state), completed_at=sqlc.arg(now)
   WHERE id=(SELECT enrollment_id FROM action)
  RETURNING id
)
SELECT enrollment.id FROM enrollment;

-- name: ListAutomationExecutionActions :many
SELECT a.id,a.enrollment_id,a.action_type,a.state,a.external_effect_id,a.receipt_digest,a.created_at,a.completed_at,
       e.automation_id,e.automation_version,e.source_event_id,e.customer_id,e.enrolled_at
  FROM automation_execution_actions a
  JOIN automation_enrollments e ON e.id=a.enrollment_id
 ORDER BY a.id DESC
 LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: ReserveAutomationRuleOperation :one
INSERT INTO automation_operation_receipts (operation,actor_scope,key_digest,payload_digest)
VALUES (sqlc.arg(operation),sqlc.arg(actor_scope),sqlc.arg(key_digest),sqlc.arg(payload_digest))
ON CONFLICT (operation,actor_scope,key_digest) DO UPDATE SET operation=EXCLUDED.operation
RETURNING id,operation,actor_scope,key_digest,payload_digest,result_snapshot,completed_at;

-- name: CompleteAutomationRuleOperation :one
UPDATE automation_operation_receipts
   SET result_snapshot=sqlc.arg(result_snapshot), completed_at=sqlc.arg(now)
 WHERE id=sqlc.arg(id) AND result_snapshot IS NULL
RETURNING id,operation,actor_scope,key_digest,payload_digest,result_snapshot,completed_at;
