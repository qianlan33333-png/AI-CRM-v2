#!/usr/bin/env bash
set -euo pipefail

: "${P4GROUP_OPS_TEST_DATABASE_URL:?P4GROUP_OPS_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4GROUP_OPS_TEST_DATABASE_URL"
temporary_database="aicrm_test_groupops_00085"
database_url="${base_database_url/aicrm_test/$temporary_database}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url

cleanup() {
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS $temporary_database WITH (FORCE)" >/dev/null
}
trap cleanup EXIT

fresh_database() {
  cleanup
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 85 >/dev/null
}

fresh_latest_database() {
  cleanup
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c "CREATE DATABASE $temporary_database" >/dev/null
  "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up >/dev/null
}

waterline() {
  psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT max(version_id) FROM goose_db_version WHERE is_applied"
}

expect_facts_reject_down() {
  local output
  if output="$("$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down 2>&1)"; then
    printf '%s\n' "$output" >&2
    echo "expected populated Group Ops runtime down to fail" >&2
    exit 1
  fi
  [[ "$output" == *"cannot roll back populated group ops runtime facts"* ]]
  [[ "$(waterline)" = "85" ]]
}

expect_material_facts_reject_down() {
  local output
  if output="$("$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down 2>&1)"; then
    printf '%s\n' "$output" >&2
    echo "expected populated Group Ops material delivery down to fail" >&2
    exit 1
  fi
  [[ "$output" == *"cannot roll back populated group ops material delivery"* ]]
  [[ "$(waterline)" = "98" ]]
}

expect_sql_rejected() {
  local expected="$1"
  local statement="$2"
  local output
  if output="$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "$statement" 2>&1)"; then
    printf '%s\n' "$output" >&2
    echo "expected SQL statement to be rejected" >&2
    exit 1
  fi
  [[ "$output" == *"$expected"* ]]
}

fresh_database
[[ "$(waterline)" = "85" ]]

fresh_database
[[ "$(waterline)" = "85" ]]
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "WITH plan AS (INSERT INTO group_ops_plans (name, status, revision, created_by, updated_by, created_at, updated_at) VALUES ('Material Guard', 'draft', 1, 1, 1, now(), now()) RETURNING id) INSERT INTO group_ops_plan_nodes (plan_id, position, kind, message_text, delay_minutes, material_reference) SELECT id, 1, 'message', 'guard', 0, 'material:guard' FROM plan"
expect_facts_reject_down
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "DELETE FROM group_ops_plan_nodes WHERE material_reference = 'material:guard'; DELETE FROM group_ops_plans WHERE name = 'Material Guard'"

fresh_database
[[ "$(waterline)" = "85" ]]
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "INSERT INTO group_ops_directory_groups (chat_reference, owner_staff_id, display_name, member_count, source_digest, refreshed_at) VALUES ('guard-group', 1, 'Guard Group', 0, 'sha256:' || repeat('0', 64), now())"
expect_facts_reject_down
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "DELETE FROM group_ops_directory_groups WHERE chat_reference = 'guard-group'"

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 84 >/dev/null
[[ "$(waterline)" = "84" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 85 >/dev/null
[[ "$(waterline)" = "85" ]]

fresh_latest_database
[[ "$(waterline)" = "98" ]]
AICRM_GROUP_OPS_TEST_DATABASE_URL="$database_url" /usr/bin/env -u BASH_ENV -u ENV \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race -count=1 -timeout=180s ./internal/groupops/... ./internal/externaleffects/... ./acceptance/groupops
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "WITH effect AS (
     INSERT INTO external_effects (owner, kind, source_ref_digest, target_ref_digest, payload_digest, policy_version_hash, envelope_fingerprint, state)
     VALUES ('media', 'media_wecom_upload', 'sha256:' || repeat('1', 64), 'sha256:' || repeat('2', 64), 'sha256:' || repeat('3', 64), 'sha256:' || repeat('4', 64), 'sha256:' || repeat('5', 64), 'accepted')
     RETURNING id
   )
     INSERT INTO media_wecom_upload_preparations (source_kind, source_id, source_digest, provider_scope_digest, upload_kind, external_effect_id, created_at, updated_at)
     SELECT 'image', 7, 'sha256:' || repeat('1', 64), 'sha256:' || repeat('2', 64), 'image', id, now(), now() FROM effect"
psql "$database_url" -X -q -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;
WITH receipt AS (
     INSERT INTO media_wecom_upload_receipts (external_effect_id, preparation_id, provider_media_id, provider_created_at, expires_at, receipt_digest, created_at)
     SELECT external_effect_id, id, 'provider-media-guard', now(), now() + interval '71 hours', 'sha256:' || repeat('6', 64), now()
     FROM media_wecom_upload_preparations
     WHERE source_kind = 'image' AND source_id = 7
     RETURNING preparation_id, provider_media_id, provider_created_at, expires_at, receipt_digest
   )
   UPDATE media_wecom_upload_preparations
   SET state = 'ready', provider_media_id = receipt.provider_media_id,
       provider_created_at = receipt.provider_created_at, expires_at = receipt.expires_at,
       provider_receipt_digest = receipt.receipt_digest, updated_at = now()
   FROM receipt
   WHERE id = receipt.preparation_id;
COMMIT;
SQL
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "WITH plan AS (
     INSERT INTO group_ops_plans (name, status, revision, created_by, updated_by, created_at, updated_at)
     VALUES ('Material Delivery Guard', 'active', 1, 1, 1, now(), now()) RETURNING id
   ), node AS (
     INSERT INTO group_ops_plan_nodes (plan_id, position, kind, message_text, delay_minutes, material_reference, material_plan)
     SELECT id, 1, 'message', 'guard', 0, '', '{\"references\":[{\"kind\":\"image\",\"id\":7}]}'::jsonb FROM plan RETURNING id, plan_id
   ), run AS (
     INSERT INTO group_ops_runs (plan_id, trigger_kind, source_key_digest, plan_revision, scheduled_for, accepted_at, accepted_by)
     SELECT plan_id, 'run_due', decode(repeat('7', 64), 'hex'), 1, now(), now(), 'service:material-guard' FROM node RETURNING id, plan_id
   )
   INSERT INTO group_ops_execution_intents (run_id, plan_id, node_id, plan_revision, node_position, target_reference, target_digest, sender_userid_snapshot, scheduled_for, content_snapshot, content_digest, material_source_snapshot, material_source_digest, execution_key_digest, continuation_job_id, continuation_generation, created_at, updated_at)
   SELECT run.id, run.plan_id, node.id, 1, 1, 'group:material-guard', 'sha256:' || repeat('8', 64), 'staff-guard', now(), '{\"schema_version\":1,\"node_kind\":\"message\",\"message_text\":\"guard\"}'::jsonb, 'sha256:' || repeat('9', 64), '{\"references\":[{\"reference\":{\"kind\":\"image\",\"id\":7},\"source_digest\":\"sha256:1111111111111111111111111111111111111111111111111111111111111111\"}]}'::jsonb, 'sha256:' || repeat('a', 64), decode(repeat('b', 64), 'hex'), 1, 1, now(), now()
   FROM run JOIN node ON node.plan_id = run.plan_id"
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "INSERT INTO group_ops_protocol_replays (client_id, resource_reference, event_id, event_id_digest, payload_digest, created_at)
   VALUES ('aicrm-webhook-group-ops', 'run:material-guard', 'event-material-guard-0001', decode(repeat('c', 64), 'hex'), decode(repeat('d', 64), 'hex'), now())
   ON CONFLICT (client_id, event_id_digest) DO NOTHING;
   INSERT INTO group_ops_protocol_replays (client_id, resource_reference, event_id, event_id_digest, payload_digest, created_at)
   VALUES ('aicrm-webhook-group-ops', 'run:material-guard', 'event-material-guard-0001', decode(repeat('c', 64), 'hex'), decode(repeat('e', 64), 'hex'), now())
   ON CONFLICT (client_id, event_id_digest) DO NOTHING"
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "WITH binding AS (
     SELECT p.id AS plan_id, n.id AS node_id, r.id AS run_id
     FROM group_ops_plans p
     JOIN group_ops_plan_nodes n ON n.plan_id = p.id
     JOIN group_ops_runs r ON r.plan_id = p.id
     WHERE p.name = 'Material Delivery Guard'
   ), effect AS (
     INSERT INTO external_effects (owner, kind, source_ref_digest, target_ref_digest, payload_digest, policy_version_hash, envelope_fingerprint, state)
     VALUES ('group_ops', 'group_ops_broadcast', 'sha256:' || repeat('a', 64), 'sha256:' || repeat('b', 64), 'sha256:' || repeat('c', 64), 'sha256:' || repeat('d', 64), 'sha256:' || repeat('e', 64), 'accepted')
     RETURNING id
   ), execution AS (
     INSERT INTO group_ops_executions (run_id, plan_id, node_id, plan_revision, node_position, target_reference, target_digest, content_snapshot, content_digest, material_snapshot, material_digest, execution_key_digest, external_effect_id, sender_userid_snapshot, created_at, updated_at)
     SELECT binding.run_id, binding.plan_id, binding.node_id, 1, 1, 'group:receipt-guard', 'sha256:' || repeat('f', 64), '{\"message_text\":\"guard\"}'::jsonb, 'sha256:' || repeat('1', 64), '{\"references\":[]}'::jsonb, 'sha256:' || repeat('2', 64), decode(repeat('3', 64), 'hex'), effect.id, 'staff-guard', now(), now()
     FROM binding CROSS JOIN effect
     RETURNING id, external_effect_id
   )
   INSERT INTO group_ops_wecom_group_message_receipts (external_effect_id, execution_id, msgid, sender_userid, chat_id, userid, task_evidence_digest, created_at, updated_at)
   SELECT external_effect_id, id, 'msgid-material-guard', 'staff-guard', 'chat-material-guard', 'member-material-guard', 'sha256:' || repeat('4', 64), now(), now()
   FROM execution"
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "INSERT INTO external_effects (owner, kind, source_ref_digest, target_ref_digest, payload_digest, policy_version_hash, envelope_fingerprint, state)
   VALUES ('group_ops', 'group_ops_broadcast', 'sha256:' || repeat('6', 64), 'sha256:' || repeat('6', 64), 'sha256:' || repeat('6', 64), 'sha256:' || repeat('6', 64), 'sha256:' || repeat('6', 64), 'accepted')"
expect_sql_rejected \
  "group ops WeCom group message receipt must bind its exact group ops broadcast effect" \
  "INSERT INTO group_ops_wecom_group_message_receipts (external_effect_id, execution_id, msgid, sender_userid, chat_id, userid, task_evidence_digest, created_at, updated_at)
   SELECT mismatch.id, execution.id, 'msgid-mismatched-effect', 'staff-guard', 'chat-mismatched-effect', 'member-mismatched-effect', 'sha256:' || repeat('5', 64), now(), now()
   FROM external_effects mismatch
   CROSS JOIN group_ops_executions execution
   WHERE mismatch.envelope_fingerprint = 'sha256:' || repeat('6', 64)
     AND execution.target_reference = 'group:receipt-guard'"
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "WITH binding AS (
     SELECT p.id AS plan_id, n.id AS node_id, r.id AS run_id
     FROM group_ops_plans p
     JOIN group_ops_plan_nodes n ON n.plan_id = p.id
     JOIN group_ops_runs r ON r.plan_id = p.id
     WHERE p.name = 'Material Delivery Guard'
   ), effect AS (
     INSERT INTO external_effects (owner, kind, source_ref_digest, target_ref_digest, payload_digest, policy_version_hash, envelope_fingerprint, state)
     VALUES ('campaign', 'campaign_dispatch', 'sha256:' || repeat('7', 64), 'sha256:' || repeat('7', 64), 'sha256:' || repeat('7', 64), 'sha256:' || repeat('7', 64), 'sha256:' || repeat('7', 64), 'accepted')
     RETURNING id
   )
   INSERT INTO group_ops_executions (run_id, plan_id, node_id, plan_revision, node_position, target_reference, target_digest, content_snapshot, content_digest, material_snapshot, material_digest, execution_key_digest, external_effect_id, sender_userid_snapshot, created_at, updated_at)
   SELECT binding.run_id, binding.plan_id, binding.node_id, 1, 1, 'group:wrong-owner', 'sha256:' || repeat('7', 64), '{\"message_text\":\"guard\"}'::jsonb, 'sha256:' || repeat('7', 64), '{\"references\":[]}'::jsonb, 'sha256:' || repeat('7', 64), decode(repeat('7', 64), 'hex'), effect.id, 'staff-guard', now(), now()
   FROM binding CROSS JOIN effect"
expect_sql_rejected \
  "group ops WeCom group message receipt must bind its exact group ops broadcast effect" \
  "INSERT INTO group_ops_wecom_group_message_receipts (external_effect_id, execution_id, msgid, sender_userid, chat_id, userid, task_evidence_digest, created_at, updated_at)
   SELECT external_effect_id, id, 'msgid-wrong-owner', 'staff-guard', 'chat-wrong-owner', 'member-wrong-owner', 'sha256:' || repeat('7', 64), now(), now()
   FROM group_ops_executions WHERE target_reference = 'group:wrong-owner'"
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "WITH binding AS (
     SELECT p.id AS plan_id, n.id AS node_id, r.id AS run_id
     FROM group_ops_plans p
     JOIN group_ops_plan_nodes n ON n.plan_id = p.id
     JOIN group_ops_runs r ON r.plan_id = p.id
     WHERE p.name = 'Material Delivery Guard'
   ), effect AS (
     INSERT INTO external_effects (owner, kind, source_ref_digest, target_ref_digest, payload_digest, policy_version_hash, envelope_fingerprint, state)
     VALUES ('group_ops', 'campaign_dispatch', 'sha256:' || repeat('8', 64), 'sha256:' || repeat('8', 64), 'sha256:' || repeat('8', 64), 'sha256:' || repeat('8', 64), 'sha256:' || repeat('8', 64), 'accepted')
     RETURNING id
   )
   INSERT INTO group_ops_executions (run_id, plan_id, node_id, plan_revision, node_position, target_reference, target_digest, content_snapshot, content_digest, material_snapshot, material_digest, execution_key_digest, external_effect_id, sender_userid_snapshot, created_at, updated_at)
   SELECT binding.run_id, binding.plan_id, binding.node_id, 1, 1, 'group:wrong-kind', 'sha256:' || repeat('8', 64), '{\"message_text\":\"guard\"}'::jsonb, 'sha256:' || repeat('8', 64), '{\"references\":[]}'::jsonb, 'sha256:' || repeat('8', 64), decode(repeat('8', 64), 'hex'), effect.id, 'staff-guard', now(), now()
   FROM binding CROSS JOIN effect"
expect_sql_rejected \
  "group ops WeCom group message receipt must bind its exact group ops broadcast effect" \
  "INSERT INTO group_ops_wecom_group_message_receipts (external_effect_id, execution_id, msgid, sender_userid, chat_id, userid, task_evidence_digest, created_at, updated_at)
   SELECT external_effect_id, id, 'msgid-wrong-kind', 'staff-guard', 'chat-wrong-kind', 'member-wrong-kind', 'sha256:' || repeat('8', 64), now(), now()
   FROM group_ops_executions WHERE target_reference = 'group:wrong-kind'"
expect_sql_rejected \
  "group ops WeCom group message task receipt is immutable" \
  "UPDATE group_ops_wecom_group_message_receipts SET msgid = 'msgid-tampered' WHERE msgid = 'msgid-material-guard'"
expect_sql_rejected \
  "group ops WeCom group message receipts cannot be deleted" \
  "DELETE FROM group_ops_wecom_group_message_receipts WHERE msgid = 'msgid-material-guard'"
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "UPDATE group_ops_wecom_group_message_receipts
   SET send_status = 1, delivery_evidence_digest = 'sha256:' || repeat('9', 64), updated_at = now()
   WHERE msgid = 'msgid-material-guard'"
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c \
  "UPDATE group_ops_wecom_group_message_receipts
   SET send_status = 1, delivery_evidence_digest = 'sha256:' || repeat('9', 64), updated_at = updated_at + interval '1 microsecond'
   WHERE msgid = 'msgid-material-guard'"
expect_sql_rejected \
  "group ops WeCom group message delivery is immutable" \
  "UPDATE group_ops_wecom_group_message_receipts SET delivery_evidence_digest = 'sha256:' || repeat('a', 64), updated_at = now() WHERE msgid = 'msgid-material-guard'"
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT count(*) FROM media_wecom_upload_preparations p JOIN media_wecom_upload_receipts r ON r.preparation_id = p.id WHERE p.state = 'ready' AND p.provider_media_id = r.provider_media_id")" = "1" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT count(*) FROM group_ops_execution_intents WHERE state = 'material_pending' AND execution_id IS NULL")" = "1" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT count(*) FROM group_ops_protocol_replays WHERE event_id = 'event-material-guard-0001' AND payload_digest = decode(repeat('d', 64), 'hex')")" = "1" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT count(*) FROM group_ops_wecom_group_message_receipts WHERE msgid = 'msgid-material-guard' AND sender_userid = 'staff-guard'")" = "1" ]]
[[ "$(psql "$database_url" -X -q -v ON_ERROR_STOP=1 -At -c "SELECT count(*) FROM group_ops_wecom_group_message_receipts WHERE msgid = 'msgid-material-guard' AND send_status = 1 AND delivery_evidence_digest = 'sha256:' || repeat('9', 64)")" = "1" ]]
expect_material_facts_reject_down
fresh_latest_database
[[ "$(waterline)" = "98" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 96 >/dev/null
[[ "$(waterline)" = "96" ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 98 >/dev/null
[[ "$(waterline)" = "98" ]]

printf 'P4 Group Ops runtime migration compatibility: PASS (PG16.14, exact 85 and 98, populated guards, empty 84/85 and 96/98 down/up)\n'
