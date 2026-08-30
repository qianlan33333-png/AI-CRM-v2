#!/usr/bin/env bash
set -euo pipefail
fail() { printf 'test-final-v1-domain-migration-apply: %s\n' "$1" >&2; exit 1; }
root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
apply="$root/scripts/final_v1_domain_migration_apply.sh"
fixture="$(mktemp -d -t aicrm-final-apply.XXXXXX)"
trap 'rm -rf -- "$fixture"' EXIT
sha='0123456789abcdef0123456789abcdef01234567'
archive_source_sha='e7f7d1b5a52d1a25c72f30932349685417b42c14'
source_slice="$fixture/sealed-source.slice"; usage_recovery="$fixture/member-grid-usage.jsonl"; runtime_env="$fixture/runtime.env"; generated_env="$fixture/generated.env"; log="$fixture/commands.log"
printf 'sealed archive fixture\n' >"$source_slice"; printf '{"archive_run_id":"archive-final"}\n' >"$usage_recovery"
source_seal="$(if command -v sha256sum >/dev/null 2>&1; then sha256sum "$source_slice" | awk '{print $1}'; else shasum -a 256 "$source_slice" | awk '{print $1}'; fi)"
usage_recovery_seal="$(if command -v sha256sum >/dev/null 2>&1; then sha256sum "$usage_recovery" | awk '{print $1}'; else shasum -a 256 "$usage_recovery" | awk '{print $1}'; fi)"
cat >"$runtime_env" <<'EOF'
AICRM_DATABASE_URL=postgres://target-not-live
AICRM_V1_ARCHIVE_TARGET_DATABASE_URL=postgres://target-not-live
AICRM_V1_ARCHIVE_SOURCE_HMAC_KEY=test
AICRM_V1_ARCHIVE_ENCRYPTION_KEY=test
AICRM_DM01_SOURCE_HMAC_KEY=test
AICRM_RELEASE_SHA=0123456789abcdef0123456789abcdef01234567
AICRM_IMAGE=registry.example/aicrm@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
AICRM_ROLLBACK_RELEASE_SHA=abcdef0123456789abcdef0123456789abcdef01
AICRM_ROLLBACK_IMAGE=registry.example/aicrm@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789
AICRM_WECOM_OUTBOUND_ENABLED=false
AICRM_WECOM_CUSTOMER_ACQUISITION_ENABLED=false
AICRM_WECHAT_PAY_ENABLED=false
AICRM_WECHAT_SHOP_ORDER_SYNC_ENABLED=false
AICRM_WECHAT_SHOP_REFUND_ENABLED=false
AICRM_WECOM_DIRECTORY_SYNC_ENABLED=false
AICRM_WECOM_TAG_CATALOG_ENABLED=false
EOF
chmod 600 "$runtime_env"
cat >"$generated_env" <<'EOF'
AICRM_WECOM_OUTBOUND_ENABLED=false
AICRM_WECOM_CUSTOMER_ACQUISITION_ENABLED=false
AICRM_WECHAT_PAY_ENABLED=false
AICRM_WECHAT_SHOP_ORDER_SYNC_ENABLED=false
AICRM_WECHAT_SHOP_REFUND_ENABLED=false
AICRM_WECOM_DIRECTORY_SYNC_ENABLED=false
AICRM_WECOM_TAG_CATALOG_ENABLED=false
EOF
chmod 600 "$generated_env"
printf 'AICRM_GENERATED_ENV_FILE=%s\n' "$generated_env" >>"$runtime_env"
fake="$fixture/fake-command"
cat >"$fake" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${AICRM_FINAL_TEST_LOG:?}"
printf 'env-outbound=%s\n' "${AICRM_WECOM_OUTBOUND_ENABLED:-}" >>"${AICRM_FINAL_TEST_LOG:?}"
case "${1:-}" in
  -C) [[ "${3:-}" = rev-parse ]] && printf '%s\n' "${AICRM_FINAL_TEST_SHA:?}" ;;
  --validate) ;;
  --check=compose-config) ;;
  --check=release) [[ "$*" = *"--expected-sha=${AICRM_FINAL_TEST_SHA:?}"* ]] ;;
  --stop=app,api,worker) ;;
  --check=schema) [[ "$*" = *--expect=135* || "$*" = *--expect=144* ]] ;;
  --check=external-effects) [[ "$*" = *--expect=0* ]] ;;
  --check=archive) [[ "$*" = *"--expected-sha=${AICRM_FINAL_TEST_ARCHIVE_SHA:?}"* ]] ;;
  --mode=final-preflight)
    [[ "${AICRM_FINAL_TEST_FAIL_CHECK:-}" != final-preflight ]] || exit 96
    printf '%s\n' "${AICRM_FINAL_TEST_PREFLIGHT_DOMAINS:?}"
    ;;
  --check=stopped) [[ "$*" = *--services=app,api,worker* ]] ;;
  --from=135) [[ "$*" = *--to=144* ]] ;;
  --mode=import) [[ "${AICRM_FINAL_TEST_FAIL_DOMAIN:-}" = '' || "$*" != *"--domain=$AICRM_FINAL_TEST_FAIL_DOMAIN"* ]] ;;
  --mode=reconcile) ;;
  --mode=final-project) [[ "$*" = *--domain=final* && "$*" = *--migration-actor=1* ]] ;;
  --mode=final-reconcile) [[ "$*" = *--domain=final* && "$*" = *--dm01-run-id=1* && "$*" = *--usage-recovery-file=* ]] ;;
  --start=api,worker) [[ "$*" = *--web=api* ]] ;;
  *) exit 97 ;;
esac
EOF
chmod 700 "$fake"
run_apply() {
  AICRM_FINAL_TEST_LOG="$log" AICRM_FINAL_TEST_SHA="$sha" AICRM_FINAL_TEST_ARCHIVE_SHA="$archive_source_sha" AICRM_FINAL_TEST_PREFLIGHT_DOMAINS="${AICRM_FINAL_TEST_PREFLIGHT_DOMAINS:-hxc-chat-job-history
hxc-member-usage-history
cycle-observation-history
customer-timeline-history
audience-activity-history}" AICRM_FINAL_PLAN_COMMAND="$fake" AICRM_FINAL_GIT_COMMAND="$fake" AICRM_FINAL_STATUS_COMMAND="$fake" AICRM_FINAL_RUNTIME_COMMAND="$fake" AICRM_FINAL_GOOSE_COMMAND="$fake" AICRM_FINAL_IMPORT_COMMAND="$fake" AICRM_FINAL_RECONCILE_COMMAND="$fake" "$apply" "$@"
}
arguments=(--apply --runtime-env-file="$runtime_env" --expected-sha="$sha" --expected-archive-source-sha="$archive_source_sha" --expected-start-schema=135 --source-slice="$source_slice" --source-seal-sha256="$source_seal" --archive-run-id=archive-final --campaign-actors=owner=1 --migration-actor=1 --dm01-run-id=1 --reference-corp-id=corp --usage-recovery-file="$usage_recovery" --usage-recovery-sha256="$usage_recovery_seal")
if run_apply "${arguments[@]:1}" >"$fixture/no-apply.log" 2>&1; then fail 'execution without explicit --apply was accepted'; fi
grep -Fq 'only explicit --apply is accepted' "$fixture/no-apply.log" || fail 'missing apply rejection changed'
[[ ! -e "$log" ]] || fail 'a rejected non-apply invocation ran a command'
chmod 644 "$runtime_env"
if run_apply "${arguments[@]}" >"$fixture/permissions.log" 2>&1; then fail 'over-broad runtime environment permissions were accepted'; fi
grep -Fq 'permissions must not be broader than 0600' "$fixture/permissions.log" || fail 'runtime environment permission rejection changed'
chmod 600 "$runtime_env"
printf 'AICRM_V1_ARCHIVE_SOURCE_DATABASE_URL=postgres://forbidden\n' >>"$runtime_env"
if run_apply "${arguments[@]}" >"$fixture/live-source.log" 2>&1; then fail 'live V1 source DSN was accepted'; fi
grep -Fq 'live V1 archive source database URL is forbidden' "$fixture/live-source.log" || fail 'live V1 source rejection changed'
sed -i.bak '$d' "$runtime_env" && rm -f "$runtime_env.bak"
printf 'COMPOSE_PROFILES=combined\n' >>"$runtime_env"
if run_apply "${arguments[@]}" >"$fixture/compose-control.log" 2>&1; then fail 'runtime COMPOSE_* setting was accepted'; fi
grep -Fq 'runtime environment must not set COMPOSE_*' "$fixture/compose-control.log" || fail 'runtime COMPOSE_* rejection changed'
sed -i.bak '$d' "$runtime_env" && rm -f "$runtime_env.bak"
AICRM_WECOM_OUTBOUND_ENABLED=true run_apply "${arguments[@]}" >"$fixture/success.log"
grep -Fq 'PASS (schema=135->144 imported-domains=5; reconciled-scopes=36; split api+worker started)' "$fixture/success.log" || fail 'success receipt is missing'
[[ "$(grep -c '^--mode=import ' "$log")" = 5 ]] || fail 'did not import exactly the pending domains'
[[ "$(grep -c '^--mode=reconcile ' "$log")" = 36 ]] || fail 'did not reconcile every manifest package'
[[ "$(grep -c '^--mode=final-project ' "$log")" = 1 ]] || fail 'editable current objects were not projected exactly once'
[[ "$(grep -c '^--mode=final-reconcile ' "$log")" = 1 ]] || fail 'final reconcile was not invoked exactly once'
[[ "$(grep -c '^--start=api,worker ' "$log")" = 1 ]] || fail 'split runtime was not started once'
grep -Fqx -- "--check=stopped --services=app,api,worker --runtime-env-file=$runtime_env" "$log" || fail 'app/api/worker stop gate is missing'
grep -Fqx -- "--check=external-effects --expect=0 --runtime-env-file=$runtime_env" "$log" || fail 'external effect zero gate is missing'
[[ "$(grep -c '^--check=external-effects ' "$log")" = 2 ]] || fail 'external effects must be checked before and after reconciliation'
project_line="$(grep -n '^--mode=final-project ' "$log" | cut -d: -f1)"
final_line="$(grep -n '^--mode=final-reconcile ' "$log" | cut -d: -f1)"
last_domain_reconcile_line="$(grep -n '^--mode=reconcile ' "$log" | tail -1 | cut -d: -f1)"
last_effect_line="$(grep -n '^--check=external-effects ' "$log" | tail -1 | cut -d: -f1)"
start_line="$(grep -n '^--start=api,worker ' "$log" | cut -d: -f1)"
[[ "$last_domain_reconcile_line" -lt "$project_line" && "$project_line" -lt "$final_line" ]] || fail 'editable projection must follow immutable reconciliation and precede aggregate reconciliation'
[[ "$final_line" -lt "$last_effect_line" && "$last_effect_line" -lt "$start_line" ]] || fail 'post-reconcile effects gate must precede startup'
grep -Fqx -- "--from=135 --to=144 --runtime-env-file=$runtime_env" "$log" || fail 'Goose was not one bounded 135->144 command'
grep -Fqx -- "--check=schema --expect=144 --runtime-env-file=$runtime_env" "$log" || fail 'post-Goose schema gate is missing'
grep -Fqx -- "--mode=final-preflight --domain=final --archive-run-id=archive-final --preflight-output=lines" "$log" || fail 'formal baseline preflight is missing'
preflight_line="$(grep -n '^--mode=final-preflight ' "$log" | cut -d: -f1)"
goose_line="$(grep -n '^--from=135 ' "$log" | cut -d: -f1)"
[[ "$preflight_line" -lt "$goose_line" ]] || fail 'formal baseline preflight must precede Goose'
if grep -Fq 'target-not-live' "$log"; then fail 'a command argument leaked the target DSN'; fi
if grep -Fq 'env-outbound=true' "$log"; then fail 'generated external switch did not override parent environment'; fi
stop_line="$(grep -n '^--stop=app,api,worker ' "$log" | cut -d: -f1)"
schema_line="$(grep -n '^--check=schema ' "$log" | head -n1 | cut -d: -f1)"
[[ "$stop_line" -lt "$schema_line" ]] || fail 'runtime stop must precede database checks'
expected_domains="hxc-chat-job-history
hxc-member-usage-history
cycle-observation-history
customer-timeline-history
audience-activity-history"
actual_domains="$(grep '^--mode=import ' "$log" | sed -n 's/.*--domain=\([^ ]*\).*/\1/p')"
[[ "$actual_domains" = "$expected_domains" ]] || fail 'domain import order differs from the manifest preflight selection'
all_manifest_domains="$(sed -n 's/.*"domain"[[:space:]]*:[[:space:]]*"\([a-z0-9-]*\)".*/\1/p' "$root/docs/release/final-v1-domain-migration-manifest.json")"
expected_reconciliations="all
$(printf '%s\n' "$all_manifest_domains" | awk '!/^(campaign|survey|media|radar|shop)$/')"
actual_reconciliations="$(grep '^--mode=reconcile ' "$log" | sed -n 's/.*--domain=\([^ ]*\).*/\1/p')"
[[ "$actual_reconciliations" = "$expected_reconciliations" ]] || fail 'reconciliation sequence does not match real importer semantics'
grep '^--mode=final-reconcile ' "$log" | grep -Fq -- '--campaign-actors=owner=1' || fail 'final reconcile does not bind campaign actors'
: >"$log"
if AICRM_FINAL_TEST_FAIL_CHECK=final-preflight run_apply "${arguments[@]}" >"$fixture/preflight.log" 2>&1; then fail 'invalid formal baseline was accepted'; fi
[[ "$(grep -c '^--from=135 ' "$log")" = 0 ]] || fail 'preflight rejection reached Goose'
[[ "$(grep -c '^--mode=import ' "$log")" = 0 ]] || fail 'preflight rejection reached import'
: >"$log"
if AICRM_FINAL_TEST_PREFLIGHT_DOMAINS=$'hxc-chat-job-history\nhxc-chat-job-history' run_apply "${arguments[@]}" >"$fixture/duplicate-preflight.log" 2>&1; then fail 'duplicate preflight domain was accepted'; fi
[[ "$(grep -c '^--from=135 ' "$log")" = 0 ]] || fail 'duplicate preflight output reached Goose'
: >"$log"
if AICRM_FINAL_TEST_PREFLIGHT_DOMAINS='not-in-manifest' run_apply "${arguments[@]}" >"$fixture/unknown-preflight.log" 2>&1; then fail 'unknown preflight domain was accepted'; fi
[[ "$(grep -c '^--mode=import ' "$log")" = 0 ]] || fail 'unknown preflight output reached import'
: >"$log"
if AICRM_FINAL_TEST_FAIL_DOMAIN=hxc-member-usage-history run_apply "${arguments[@]}" >"$fixture/failure.log" 2>&1; then fail 'a failed domain import was accepted'; fi
[[ "$(grep -c '^--mode=import .*--domain=hxc-member-usage-history' "$log")" = 1 ]] || fail 'failed import was retried'
[[ "$(grep -c '^--mode=final-reconcile ' "$log")" = 0 ]] || fail 'failure continued to final reconciliation'
[[ "$(grep -c '^--mode=final-project ' "$log")" = 0 ]] || fail 'failure continued to editable projection'
[[ "$(grep -c '^--mode=reconcile ' "$log")" = 0 ]] || fail 'failure continued to domain reconciliation'
[[ "$(grep -c '^--start=api,worker ' "$log")" = 0 ]] || fail 'failure started the runtime'
: >"$log"
no_go_path='/usr/bin:/bin:/usr/sbin:/sbin'
if PATH="$no_go_path" command -v go >/dev/null 2>&1; then fail 'no-Go fixture PATH unexpectedly resolves go'; fi
PATH="$no_go_path" run_apply "${arguments[@]}" >"$fixture/no-go.log"
grep -Fq 'PASS (schema=135->144 imported-domains=5; reconciled-scopes=36; split api+worker started)' "$fixture/no-go.log" || fail 'fully injected execution required Go on PATH'
printf 'test-final-v1-domain-migration-apply: PASS\n'
