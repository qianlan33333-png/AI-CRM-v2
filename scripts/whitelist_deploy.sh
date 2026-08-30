#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'whitelist-deploy: %s\n' "$1" >&2; exit 2; }

base_url='' output_directory='' runtime_env_file=''
for argument in "$@"; do
  case "$argument" in
    --base-url=*) base_url="${argument#--base-url=}" ;;
    --output-dir=*) output_directory="${argument#--output-dir=}" ;;
    --runtime-env-file=*) runtime_env_file="${argument#--runtime-env-file=}" ;;
    --help) printf 'Usage: whitelist_deploy.sh --base-url=<id-dev URL> --output-dir=<absolute dir> --runtime-env-file=<absolute file>\n'; exit 0 ;;
    *) fail 'invalid argument' ;;
  esac
done
[[ "${AICRM_ALLOW_WHITELIST_DEPLOY:-}" = 1 ]] || fail 'AICRM_ALLOW_WHITELIST_DEPLOY=1 is required'
[[ "$base_url" = https://* ]] || fail 'id-dev HTTPS base URL is required'
[[ "$output_directory" = /* ]] || fail 'absolute output directory is required'
[[ "$runtime_env_file" = /* && -f "$runtime_env_file" && ! -L "$runtime_env_file" ]] || fail 'runtime environment file is invalid'
[[ -n "${AICRM_WHITELIST_SOURCE_DATABASE_URL:-}" ]] || fail 'source database URL is required'
[[ -n "${AICRM_WHITELIST_ARCHIVE_RUN_ID:-}" ]] || fail 'sealed V1 archive run is required'
[[ -n "${AICRM_WHITELIST_ADMIN_DATABASE_URL:-}" ]] || fail 'admin database URL is required'
[[ -n "${AICRM_DATABASE_URL:-}" ]] || fail 'target database URL is required'
[[ "${AICRM_RELEASE_SHA:-}" =~ ^[a-f0-9]{40}$ ]] || fail 'release SHA is invalid'
command -v psql >/dev/null 2>&1 || fail 'psql is required'
command -v jq >/dev/null 2>&1 || fail 'jq is required'
command -v go >/dev/null 2>&1 || fail 'Go is required'

runtime_value() {
  local key="$1"
  awk -F= -v key="$key" '$1 == key { value=substr($0,length(key)+2) } END { print value }' "$runtime_env_file"
}

[[ "$(runtime_value AICRM_DATABASE_URL)" = "$AICRM_DATABASE_URL" ]] || fail 'runtime database URL is not the whitelist target'
[[ "$(runtime_value AICRM_POSTGRES_DB)" = aicrm_v2_core ]] || fail 'runtime PostgreSQL database name must be aicrm_v2_core'
[[ "$(runtime_value AICRM_FINAL_RUNTIME_MODE)" = external-postgres-manual ]] || fail 'runtime must use the exact id-dev upstream/Web-root switch mode'
for external_flag in \
  AICRM_WECOM_OUTBOUND_ENABLED \
  AICRM_WECOM_CUSTOMER_ACQUISITION_ENABLED \
  AICRM_WECOM_TAG_CATALOG_ENABLED \
  AICRM_WECOM_DIRECTORY_SYNC_ENABLED \
  AICRM_WECHAT_PAY_ENABLED \
  AICRM_WECHAT_SHOP_ORDER_SYNC_ENABLED \
  AICRM_WECHAT_SHOP_REFUND_ENABLED; do
  [[ "$(runtime_value "$external_flag")" = false ]] || fail "$external_flag must be false"
done

script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"
[[ "$(git -C "$repository_root" rev-parse HEAD)" = "$AICRM_RELEASE_SHA" ]] || fail 'release SHA does not match checkout'
[[ -z "$(git -C "$repository_root" status --porcelain --untracked-files=normal)" ]] || fail 'release checkout must be clean'
mkdir -p "$output_directory"
[[ -d "$output_directory" && ! -L "$output_directory" ]] || fail 'output directory is invalid'

# The fixed database name is intentional. CREATE fails closed if a previous
# attempt already exists; operators must inspect and explicitly remove only
# that failed candidate before retrying.
psql -v ON_ERROR_STOP=1 "$AICRM_WHITELIST_ADMIN_DATABASE_URL" -c 'CREATE DATABASE aicrm_v2_core'
psql -v ON_ERROR_STOP=1 "$AICRM_DATABASE_URL" -f "$repository_root/schema/whitelist_baseline.sql" >"$output_directory/baseline.log"
[[ "$(psql -Atqc 'select current_database()' "$AICRM_DATABASE_URL")" = aicrm_v2_core ]] || fail 'target URL is not aicrm_v2_core'

digest_json="$(cd "$repository_root" && GOWORK=off GOFLAGS=-mod=readonly go run ./cmd/aicrm-whitelist-import --mode=digest)"
AICRM_WHITELIST_SOURCE_DIGEST="$(jq -er '.source_digest' <<<"$digest_json")"
export AICRM_WHITELIST_SOURCE_DIGEST
printf '%s\n' "$digest_json" >"$output_directory/source-digest.json"

run_id="wli_${AICRM_RELEASE_SHA}"
(cd "$repository_root" && GOWORK=off GOFLAGS=-mod=readonly go run ./cmd/aicrm-whitelist-import --mode=import --run-id="$run_id") >"$output_directory/import.json"
(cd "$repository_root" && GOWORK=off GOFLAGS=-mod=readonly go run ./cmd/aicrm-whitelist-import --mode=reconcile --run-id="$run_id") >"$output_directory/reconciliation-detail.json"
jq -e '.required_identity_mapping_coverage=="100%" and .orphan_rows==0 and .identity_conflicts==0 and .external_effects==0 and .real_provider_receipts==0 and .legacy_material_references==0 and .old_campaign_rows==0 and .old_message_rows==0' \
  "$output_directory/reconciliation-detail.json" >/dev/null || fail 'nine-domain reconciliation failed'

reconciliation_receipt="$output_directory/reconciliation-receipt.json"
printf '{"status":"passed","target_database":"aicrm_v2_core","source_digest":"%s","release_sha":"%s"}\n' \
  "$AICRM_WHITELIST_SOURCE_DIGEST" "$AICRM_RELEASE_SHA" >"$reconciliation_receipt"
chmod 600 "$reconciliation_receipt"

# In external-postgres-manual mode this helper starts the split API/Worker and
# atomically replaces only the declared id-dev Caddy tail/Web root.
"$script_directory/final_v1_domain_migration_runtime.sh" --runtime-env-file="$runtime_env_file" --start=api,worker --web=api --expected-sha="$AICRM_RELEASE_SHA"
"$script_directory/whitelist_smoke.sh" --base-url="$base_url" --output="$output_directory/id-dev-smoke-receipt.json"
printf 'whitelist-deploy: import, reconciliation, id-dev switch and smoke passed; cleanup remains a separate --plan/--apply step\n'
