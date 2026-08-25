#!/usr/bin/env bash
set -euo pipefail

: "${P4MEDIADELIVERY_TEST_DATABASE_URL:?P4MEDIADELIVERY_TEST_DATABASE_URL is required}"
go_command="${GO:-go}"
tools_mod="${TOOLS_MOD:-tools/go.mod}"
base_database_url="$P4MEDIADELIVERY_TEST_DATABASE_URL"
database_url="${base_database_url/aicrm_test/aicrm_test_media_delivery_83}"

MIGRATION_TEST_DATABASE_URL="$base_database_url" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
cleanup() { psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'DROP DATABASE IF EXISTS aicrm_test_media_delivery_83 WITH (FORCE)' >/dev/null; }
recreate_database() {
  cleanup
  psql "$base_database_url" -X -q -v ON_ERROR_STOP=1 -c 'CREATE DATABASE aicrm_test_media_delivery_83' >/dev/null
  MIGRATION_TEST_DATABASE_URL="$database_url" MIGRATION_TEST_DATABASE_NAME=aicrm_test_media_delivery_83 GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
    "$go_command" run ./acceptance/fixtures/cmd/validate-database-url
}
expect_populated_guard() {
  local guard_output
  guard_output="$(mktemp)"
  if "$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 82 >"$guard_output" 2>&1; then
    rm -f "$guard_output"
    printf 'expected populated 00083 down guard to fail\n' >&2
    exit 1
  fi
  grep -Fq 'cannot roll back populated media content package and delivery facts' "$guard_output"
  rm -f "$guard_output"
  [[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id=83 AND is_applied')" = 1 ]]
}
trap cleanup EXIT
recreate_database

"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 83
[[ "$(psql "$database_url" -X -q -At -c 'SHOW server_version_num')" = 160014 ]]
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id=83 AND is_applied')" = 1 ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name IN ('media_content_packages','media_attachment_uploads','media_campaign_delivery_bindings','outbound_media_acceptances') AND column_name ~* 'tenant|workspace|organization|public_url|object_storage'")" = 0 ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" down-to 82
[[ "$(psql "$database_url" -X -q -At -c "SELECT to_regclass('public.media_content_packages') IS NULL")" = t ]]
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 83
[[ "$(psql "$database_url" -X -q -At -c 'SELECT count(*) FROM goose_db_version WHERE version_id=83 AND is_applied')" = 1 ]]

# A completed receipt is an independent business fact and must block rollback
# even when no package/upload/binding row exists.
psql "$database_url" -X -q -v ON_ERROR_STOP=1 -c "INSERT INTO public.media_content_package_mutation_receipts (operation, actor_id, key_digest, payload_digest, result_snapshot, created_at) VALUES ('create', 1, decode(repeat('1', 64), 'hex'), decode(repeat('2', 64), 'hex'), '{\"id\":1}'::jsonb, now())" >/dev/null
expect_populated_guard

recreate_database
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 83
# Campaign owns these prerequisite facts. The Media Go acceptance then writes
# only Media-owned tables while proving a binding to an existing plan.
psql "$database_url" -X -q -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;
INSERT INTO public.cloud_campaigns
  (campaign_code, name, approval_status, runtime_status, version, created_by, updated_by, created_at, updated_at)
VALUES ('media83-campaign', 'media83', 'draft', 'idle', 1, 1, 1, now(), now());
INSERT INTO public.cloud_campaign_steps (campaign_code, step_index, delay_minutes, content)
VALUES ('media83-campaign', 1, 0, 'local');
INSERT INTO public.cloud_campaign_touch_plans
  (id, campaign_code, campaign_version, source_kind, customer_selection_id, customer_selection_version,
   source_digest, target_digest, content_digest, target_count, content_step_count, candidate_count,
   active_customer_count, inactive_excluded_count, policy_excluded_count, owner_actor_id, created_at)
VALUES ('ctp_' || repeat('8', 64), 'media83-campaign', 1, 'customer_selection', 'local_selection', 'v1',
        decode(repeat('01', 32), 'hex'), decode(repeat('02', 32), 'hex'), decode(repeat('03', 32), 'hex'),
        1, 1, 1, 1, 0, 0, 1, now());
INSERT INTO public.cloud_campaign_touch_plan_targets (plan_id, customer_id)
VALUES ('ctp_' || repeat('8', 64), 1);
INSERT INTO public.cloud_campaign_touch_plan_steps (plan_id, step_index, delay_minutes, content)
VALUES ('ctp_' || repeat('8', 64), 1, 0, 'local');
WITH created_event AS (
  INSERT INTO public.event_log (event_type, payload, occurred_at, idempotency_key)
  VALUES (
    'cloud_campaign.fact_recorded',
    jsonb_build_object(
      'audit_type', 'touch_plan_created',
      'plan_id', 'ctp_' || repeat('8', 64),
      'campaign_code', 'media83-campaign',
      'owner_actor_id', 1,
      'target_digest', repeat('02', 32),
      'target_count', 1,
      'content_digest', repeat('03', 32)
    ),
    now(),
    'campaign-initiation-media83-fixture'
  )
  RETURNING id
)
INSERT INTO public.cloud_campaign_touch_plan_receipts
  (actor_id, key_digest, payload_digest, plan_id, event_id, state, created_at, completed_at)
SELECT 1, decode(repeat('04', 32), 'hex'), decode(repeat('05', 32), 'hex'),
       'ctp_' || repeat('8', 64), id, 'completed', now(), now()
FROM created_event;
COMMIT;
SQL
GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  "$go_command" test -race ./acceptance/media -run '^TestP4MediaContentDelivery00083BusinessClosure$' -count=1 -args -database-url "$database_url"

# Recreate once more so the raw compatibility fixture retains deterministic IDs.
recreate_database
"$go_command" tool -modfile="$tools_mod" goose -dir migrations postgres "$database_url" up-to 83

# Keep this fixture entirely local and digest-only.  It proves the 00083
# relationships used by package/PDF upload, outbound-media EER binding and
# manual reconciliation without creating a provider call or a recipient fact.
psql "$database_url" -X -q -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;
DO $$
DECLARE
  attachment_key BIGINT;
  package_key BIGINT;
  upload_key BIGINT;
  effect_key BIGINT;
BEGIN
  INSERT INTO public.media_attachments
    (name, file_name, mime_type, file_size, checksum, created_by, updated_by, created_at, updated_at)
  VALUES
    ('00083 acceptance PDF', '00083-acceptance.pdf', 'application/pdf', 1,
     decode('2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881', 'hex'),
     1, 1, now(), now())
  RETURNING id INTO attachment_key;
  INSERT INTO public.media_attachment_blobs (attachment_id, content, checksum, created_at)
  VALUES (attachment_key, 'x'::bytea,
          decode('2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881', 'hex'), now());

  INSERT INTO public.media_content_packages (name, content_text, enabled, created_by, updated_by, created_at, updated_at)
  VALUES ('00083 acceptance package', 'local-only', TRUE, 1, 1, now(), now())
  RETURNING id INTO package_key;
  INSERT INTO public.media_content_package_refs (package_id, position, ref_kind, attachment_id)
  VALUES (package_key, 1, 'attachment', attachment_key);

  INSERT INTO public.media_attachment_uploads
    (file_name, name, expected_size, expected_digest, created_by, state, attachment_id, created_at, completed_at)
  VALUES
    ('00083-acceptance.pdf', '00083 acceptance PDF', 1,
     decode('2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881', 'hex'),
     1, 'completed', attachment_key, now(), now())
  RETURNING id INTO upload_key;
  INSERT INTO public.media_attachment_upload_parts (upload_id, part_number, digest, content, created_at)
  VALUES (upload_key, 1,
          decode('2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881', 'hex'), 'x'::bytea, now());

  INSERT INTO external_effects
    (owner, kind, source_ref_digest, target_ref_digest, payload_digest, policy_version_hash, envelope_fingerprint, state)
  VALUES
    ('outbound', 'outbound_media', 'sha256:' || repeat('a', 64), 'sha256:' || repeat('b', 64),
     'sha256:' || repeat('c', 64), 'sha256:' || repeat('d', 64), 'sha256:' || repeat('e', 64), 'accepted')
  RETURNING id INTO effect_key;
  INSERT INTO external_effect_receipts (operation, effect_id, receipt_key_digest, command_digest, state)
  VALUES ('accept', effect_key, 'sha256:' || repeat('f', 64), 'sha256:' || repeat('1', 64), 'accepted');
  INSERT INTO public.outbound_media_effect_bindings (content_package_id, target_digest, snapshot_digest, effect_id, created_at)
  VALUES (package_key, 'sha256:' || repeat('b', 64), 'sha256:' || repeat('a', 64), effect_key, now());
  INSERT INTO public.outbound_media_acceptances
    (content_package_id, target_digest, media_refs, source_digest, payload_digest, external_effect_id, state, created_at, updated_at)
  VALUES
    (package_key, 'sha256:' || repeat('b', 64), jsonb_build_array(jsonb_build_object('kind', 'attachment', 'attachmentId', attachment_key)),
     'sha256:' || repeat('a', 64), 'sha256:' || repeat('c', 64), effect_key, 'accepted', now(), now());

  UPDATE external_effects
  SET state = 'outcome_unknown', lease_fence = 1, lease_expires_at = now() + interval '5 minutes'
  WHERE id = effect_key;
  INSERT INTO external_effect_reconciliations (effect_id, generation, fence, evidence_digest)
  VALUES (effect_key, 1, 1, 'sha256:' || repeat('2', 64));
  INSERT INTO external_effect_receipts (operation, effect_id, receipt_key_digest, command_digest, state)
  VALUES ('reconcile', effect_key, 'sha256:' || repeat('3', 64), 'sha256:' || repeat('4', 64), 'reconciled');
  INSERT INTO public.outbound_media_reconciliation_receipts
    (effect_id, generation, fence, lease_expires_at, idempotency_key_digest, evidence_digest, provider_accepted, delivery_proven, eer_receipt_digest, created_at)
  VALUES (effect_key, 1, 1, now() + interval '5 minutes', 'sha256:' || repeat('5', 64), 'sha256:' || repeat('2', 64), TRUE, FALSE,
          'sha256:' || repeat('4', 64), now());
  UPDATE external_effects SET state = 'reconciled' WHERE id = effect_key;
END $$;
COMMIT;
SQL
[[ "$(psql "$database_url" -X -q -At -F '|' -c "SELECT p.id, 'eer_' || e.id, e.state, r.provider_accepted, r.delivery_proven FROM public.media_content_packages p JOIN public.outbound_media_effect_bindings b ON b.content_package_id=p.id JOIN external_effects e ON e.id=b.effect_id JOIN public.outbound_media_reconciliation_receipts r ON r.effect_id=e.id")" = '1|eer_1|reconciled|t|f' ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT count(*) FROM public.outbound_media_acceptances a JOIN public.outbound_media_effect_bindings b ON b.content_package_id=a.content_package_id AND b.target_digest=a.target_digest AND b.effect_id=a.external_effect_id WHERE a.source_digest=b.snapshot_digest AND a.state='accepted' AND NOT a.provider_execution_eligible AND NOT a.real_external_call_executed AND NOT a.delivery_proven")" = 1 ]]
[[ "$(psql "$database_url" -X -q -At -c "SELECT count(*) FROM public.media_attachment_uploads u JOIN public.media_attachment_upload_parts part ON part.upload_id=u.id JOIN public.media_attachments a ON a.id=u.attachment_id JOIN public.media_content_package_refs ref ON ref.attachment_id=a.id WHERE u.state='completed' AND u.expected_size=1 AND octet_length(part.content)=1 AND ref.ref_kind='attachment'")" = 1 ]]

expect_populated_guard
printf 'P4 Media Content Delivery migration compatibility: PASS (PG16.14; exact 83; empty 83/82/83; receipt-only guard; package/PDF/EER acceptance/reconcile/detail; populated guard)\n'
