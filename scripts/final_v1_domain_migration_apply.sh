#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'final-v1-domain-migration-apply: %s\n' "$1" >&2
  exit 2
}

usage='Usage: final_v1_domain_migration_apply.sh --apply --runtime-env-file=<absolute-file> --expected-sha=<40-lowercase-hex> --expected-archive-source-sha=<40-lowercase-hex> --expected-start-schema=<version> --source-slice=<absolute-file> --source-seal-sha256=<64-lowercase-hex> --archive-run-id=<id> --campaign-actors=<owner=actor,...> --migration-actor=<id> --dm01-run-id=<id> --reference-corp-id=<id> --usage-recovery-file=<absolute-file> --usage-recovery-sha256=<64-lowercase-hex>'

require_command() {
  local name="$1"
  local path="${!name:-}"
  [[ "$path" = /* && -f "$path" && ! -L "$path" && -x "$path" ]] ||
    fail "$name must name an executable, regular absolute file"
}

file_mode() {
  case "$(uname -s)" in
    Darwin) stat -f '%Lp' "$1" ;;
    Linux) stat -c '%a' "$1" ;;
    *) fail 'unsupported platform for runtime environment permissions' ;;
  esac
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{ print $1 }'
  else
    fail 'sha256sum or shasum is required to verify source slice seal'
  fi
}

resolve_command() {
  local name="$1"
  local path
  path="$(command -v "$name" 2>/dev/null || true)"
  [[ "$path" = /* && -f "$path" && -x "$path" ]] || fail "$name must resolve to an executable absolute path"
  printf '%s\n' "$path"
}

apply_seen=0
runtime_env_file='' expected_sha='' expected_archive_source_sha='' expected_start_schema='' source_slice='' source_seal_sha256='' archive_run_id=''
campaign_actors='' migration_actor='' dm01_run_id='' reference_corp_id='' usage_recovery_file='' usage_recovery_sha256=''
for argument in "$@"; do
  case "$argument" in
    --apply) [[ "$apply_seen" -eq 0 ]] || fail 'duplicate --apply'; apply_seen=1 ;;
    --runtime-env-file=*) [[ -z "$runtime_env_file" ]] || fail 'duplicate --runtime-env-file'; runtime_env_file="${argument#--runtime-env-file=}" ;;
    --expected-sha=*) [[ -z "$expected_sha" ]] || fail 'duplicate --expected-sha'; expected_sha="${argument#--expected-sha=}" ;;
    --expected-archive-source-sha=*) [[ -z "$expected_archive_source_sha" ]] || fail 'duplicate --expected-archive-source-sha'; expected_archive_source_sha="${argument#--expected-archive-source-sha=}" ;;
    --expected-start-schema=*) [[ -z "$expected_start_schema" ]] || fail 'duplicate --expected-start-schema'; expected_start_schema="${argument#--expected-start-schema=}" ;;
    --source-slice=*) [[ -z "$source_slice" ]] || fail 'duplicate --source-slice'; source_slice="${argument#--source-slice=}" ;;
    --source-seal-sha256=*) [[ -z "$source_seal_sha256" ]] || fail 'duplicate --source-seal-sha256'; source_seal_sha256="${argument#--source-seal-sha256=}" ;;
    --archive-run-id=*) [[ -z "$archive_run_id" ]] || fail 'duplicate --archive-run-id'; archive_run_id="${argument#--archive-run-id=}" ;;
    --campaign-actors=*) campaign_actors="${argument#--campaign-actors=}" ;;
    --migration-actor=*) migration_actor="${argument#--migration-actor=}" ;;
    --dm01-run-id=*) dm01_run_id="${argument#--dm01-run-id=}" ;;
    --reference-corp-id=*) reference_corp_id="${argument#--reference-corp-id=}" ;;
    --usage-recovery-file=*) usage_recovery_file="${argument#--usage-recovery-file=}" ;;
    --usage-recovery-sha256=*) usage_recovery_sha256="${argument#--usage-recovery-sha256=}" ;;
    --help) printf '%s\n' "$usage"; exit 0 ;;
    *) fail "$usage" ;;
  esac
done

[[ "$apply_seen" -eq 1 ]] || fail 'only explicit --apply is accepted'
[[ "$runtime_env_file" = /* && -f "$runtime_env_file" && ! -L "$runtime_env_file" ]] || fail '--runtime-env-file must be an absolute regular file'
runtime_env_mode="$(file_mode "$runtime_env_file")"
(( (8#$runtime_env_mode & 8#077) == 0 )) || fail '--runtime-env-file permissions must not be broader than 0600'
[[ "$expected_sha" =~ ^[a-f0-9]{40}$ ]] || fail '--expected-sha must be 40 lowercase hexadecimal characters'
[[ "$expected_archive_source_sha" =~ ^[a-f0-9]{40}$ ]] || fail '--expected-archive-source-sha must be 40 lowercase hexadecimal characters'
[[ "$expected_start_schema" =~ ^[1-9][0-9]*$ ]] || fail '--expected-start-schema must be a positive integer'
[[ "$expected_start_schema" = '135' ]] || fail '--expected-start-schema must be 135 for formal apply'
[[ "$source_slice" = /* && -f "$source_slice" && ! -L "$source_slice" ]] || fail '--source-slice must be an absolute regular file'
[[ "$source_seal_sha256" =~ ^[a-f0-9]{64}$ ]] || fail '--source-seal-sha256 must be 64 lowercase hexadecimal characters'
[[ "$(sha256_file "$source_slice")" = "$source_seal_sha256" ]] || fail 'source slice SHA-256 seal does not match'
[[ "$archive_run_id" =~ ^[A-Za-z0-9._:-]{1,128}$ ]] || fail '--archive-run-id is invalid'
[[ -n "$campaign_actors" ]] || fail '--campaign-actors is required'
[[ "$migration_actor" =~ ^[1-9][0-9]*$ ]] || fail '--migration-actor must be a positive integer'
[[ "$dm01_run_id" =~ ^[1-9][0-9]*$ ]] || fail '--dm01-run-id must be a positive integer'
[[ "$reference_corp_id" =~ ^[^[:space:]]+$ ]] || fail '--reference-corp-id is required'
[[ "$usage_recovery_file" = /* && -f "$usage_recovery_file" && ! -L "$usage_recovery_file" ]] || fail '--usage-recovery-file must be an absolute regular file'
[[ "$usage_recovery_sha256" =~ ^[a-f0-9]{64}$ ]] || fail '--usage-recovery-sha256 must be 64 lowercase hexadecimal characters'
[[ "$(sha256_file "$usage_recovery_file")" = "$usage_recovery_sha256" ]] || fail 'usage recovery SHA-256 seal does not match'

# Treat the secret file as data, never as executable shell. Other compose
# settings remain in the file for the injected runtime command to consume.
allowed_runtime_keys=(AICRM_DATABASE_URL AICRM_V1_ARCHIVE_TARGET_DATABASE_URL AICRM_V1_ARCHIVE_SOURCE_DATABASE_URL AICRM_DM01_SOURCE_DATABASE_URL AICRM_V1_ARCHIVE_SOURCE_HMAC_KEY AICRM_V1_ARCHIVE_ENCRYPTION_KEY AICRM_DM01_SOURCE_HMAC_KEY AICRM_GENERATED_ENV_FILE)
for allowed_key in "${allowed_runtime_keys[@]}"; do unset "$allowed_key"; done
while IFS= read -r line || [[ -n "$line" ]]; do
  [[ -z "$line" || "$line" = \#* ]] && continue
  [[ "$line" =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]] || fail 'runtime environment contains an invalid assignment'
  key="${BASH_REMATCH[1]}"; value="${BASH_REMATCH[2]}"; allowed=0
  [[ "$key" != COMPOSE_* ]] || fail 'runtime environment must not set COMPOSE_*'
  for allowed_key in "${allowed_runtime_keys[@]}"; do [[ "$key" = "$allowed_key" ]] && allowed=1 && break; done
  [[ "$allowed" -eq 1 ]] || continue
  export "$key=$value"
done <"$runtime_env_file"

[[ -n "${AICRM_DATABASE_URL:-}" && -n "${AICRM_V1_ARCHIVE_TARGET_DATABASE_URL:-}" ]] || fail 'runtime environment must provide target database URLs'
[[ "$AICRM_DATABASE_URL" = "$AICRM_V1_ARCHIVE_TARGET_DATABASE_URL" ]] || fail 'AICRM_DATABASE_URL and AICRM_V1_ARCHIVE_TARGET_DATABASE_URL must match'
[[ -z "${AICRM_V1_ARCHIVE_SOURCE_DATABASE_URL:-}" ]] || fail 'live V1 archive source database URL is forbidden'
[[ -z "${AICRM_DM01_SOURCE_DATABASE_URL:-}" ]] || fail 'live DM01 source database URL is forbidden'
generated_env_file="${AICRM_GENERATED_ENV_FILE:-}"
[[ "$generated_env_file" = /* && -f "$generated_env_file" && ! -L "$generated_env_file" ]] || fail 'AICRM_GENERATED_ENV_FILE must be an absolute regular file'
generated_env_mode="$(file_mode "$generated_env_file")"
(( (8#$generated_env_mode & 8#077) == 0 )) || fail 'AICRM_GENERATED_ENV_FILE permissions must not be broader than 0600'
for external_switch in AICRM_WECOM_OUTBOUND_ENABLED AICRM_WECOM_CUSTOMER_ACQUISITION_ENABLED AICRM_WECHAT_PAY_ENABLED AICRM_WECHAT_SHOP_ORDER_SYNC_ENABLED AICRM_WECHAT_SHOP_REFUND_ENABLED AICRM_WECOM_DIRECTORY_SYNC_ENABLED AICRM_WECOM_TAG_CATALOG_ENABLED; do
  unset "$external_switch"
  external_value=''
  while IFS= read -r generated_line || [[ -n "$generated_line" ]]; do
    [[ "$generated_line" = "$external_switch="* ]] && external_value="${generated_line#*=}"
  done <"$generated_env_file"
  [[ "$external_value" = false ]] || fail "$external_switch must be exactly false in generated environment"
  export "$external_switch=false"
done

script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"
: "${AICRM_FINAL_PLAN_COMMAND:=$repository_root/scripts/final_v1_domain_migration_plan.sh}"
: "${AICRM_FINAL_GIT_COMMAND:=$(resolve_command git)}"
: "${AICRM_FINAL_STATUS_COMMAND:=$repository_root/scripts/final_v1_domain_migration_status.sh}"
: "${AICRM_FINAL_RUNTIME_COMMAND:=$repository_root/scripts/final_v1_domain_migration_runtime.sh}"
for command_name in AICRM_FINAL_PLAN_COMMAND AICRM_FINAL_GIT_COMMAND AICRM_FINAL_STATUS_COMMAND AICRM_FINAL_RUNTIME_COMMAND; do require_command "$command_name"; done
go_command="$(resolve_command go)"

[[ "$("$AICRM_FINAL_GIT_COMMAND" -C "$repository_root" rev-parse HEAD)" = "$expected_sha" ]] || fail '--expected-sha must equal the current checkout HEAD'
[[ -z "$("$AICRM_FINAL_GIT_COMMAND" -C "$repository_root" status --porcelain --untracked-files=normal)" ]] || fail 'the release checkout must be clean'
[[ "$expected_sha" != "$expected_archive_source_sha" ]] || fail 'final release SHA must differ from archive source SHA'
"$AICRM_FINAL_PLAN_COMMAND" --validate
"$AICRM_FINAL_RUNTIME_COMMAND" --check=compose-config --runtime-env-file="$runtime_env_file"
"$AICRM_FINAL_RUNTIME_COMMAND" --stop=app,api,worker --runtime-env-file="$runtime_env_file"
"$AICRM_FINAL_RUNTIME_COMMAND" --check=stopped --services=app,api,worker --runtime-env-file="$runtime_env_file"
"$AICRM_FINAL_RUNTIME_COMMAND" --check=release --expected-sha="$expected_sha" --runtime-env-file="$runtime_env_file"
"$AICRM_FINAL_STATUS_COMMAND" --check=schema --expect="$expected_start_schema" --runtime-env-file="$runtime_env_file"
"$AICRM_FINAL_STATUS_COMMAND" --check=external-effects --expect=0 --runtime-env-file="$runtime_env_file"
"$AICRM_FINAL_STATUS_COMMAND" --check=archive --archive-run-id="$archive_run_id" --expected-sha="$expected_archive_source_sha" --source-slice="$source_slice" --source-seal-sha256="$source_seal_sha256" --runtime-env-file="$runtime_env_file"
run_final_preflight() {
  if [[ -n "${AICRM_FINAL_IMPORT_COMMAND:-}" ]]; then
    require_command AICRM_FINAL_IMPORT_COMMAND
    "$AICRM_FINAL_IMPORT_COMMAND" --mode=final-preflight --domain=final --archive-run-id="$archive_run_id" --preflight-output=lines
  else
    (cd "$repository_root"; GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./cmd/aicrm-v1-domain-import --mode=final-preflight --domain=final --archive-run-id="$archive_run_id" --preflight-output=lines)
  fi
}

# The preflight uses the importer's 40-domain/36-scope registry in a read-only
# transaction. Its output is data, so validate every line before allowing it
# to select the bounded import loop.
preflight_output="$(run_final_preflight)" || fail 'final preflight rejected the existing import baseline'
manifest_domains="$(sed -n 's/.*"domain"[[:space:]]*:[[:space:]]*"\([a-z0-9-]*\)".*/\1/p' "$repository_root/docs/release/final-v1-domain-migration-manifest.json")"
pending_domains=''
if [[ -n "$preflight_output" ]]; then
  while IFS= read -r pending_domain; do
    [[ "$pending_domain" =~ ^[a-z0-9][a-z0-9-]*$ ]] || fail 'final preflight returned an invalid domain'
    printf '%s\n' "$manifest_domains" | grep -Fxq "$pending_domain" || fail 'final preflight returned a domain outside the manifest'
    case " $pending_domains " in *" $pending_domain "*) fail 'final preflight returned a duplicate domain' ;; esac
    pending_domains="${pending_domains:+$pending_domains }$pending_domain"
  done <<<"$preflight_output"
fi
[[ -n "$pending_domains" ]] || fail 'final preflight found no missing domains; refusing replay'
# The injected commands must use the restricted env file themselves and fail
# closed: status verifies the named checks, runtime verifies stopped services,
# Goose performs its one bounded migration, and reconcile seals all imports.
if [[ -n "${AICRM_FINAL_GOOSE_COMMAND:-}" ]]; then
  require_command AICRM_FINAL_GOOSE_COMMAND
  "$AICRM_FINAL_GOOSE_COMMAND" --from="$expected_start_schema" --to=142 --runtime-env-file="$runtime_env_file"
else
  (cd "$repository_root"; GOOSE_DRIVER=postgres GOOSE_DBSTRING="$AICRM_DATABASE_URL" GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" tool -modfile=tools/go.mod goose -dir migrations up-to 142)
fi
"$AICRM_FINAL_STATUS_COMMAND" --check=schema --expect=142 --runtime-env-file="$runtime_env_file"
while IFS= read -r domain; do
  [[ -n "$domain" ]] || continue
  case " $pending_domains " in *" $domain "*) ;; *) continue ;; esac
  if [[ -n "${AICRM_FINAL_IMPORT_COMMAND:-}" ]]; then
    require_command AICRM_FINAL_IMPORT_COMMAND
    "$AICRM_FINAL_IMPORT_COMMAND" --mode=import --domain="$domain" --archive-run-id="$archive_run_id" --campaign-actors="$campaign_actors" --migration-actor="$migration_actor" --dm01-run-id="$dm01_run_id" --reference-corp-id="$reference_corp_id" --usage-recovery-file="$usage_recovery_file"
  else
    (cd "$repository_root"; GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./cmd/aicrm-v1-domain-import --mode=import --domain="$domain" --archive-run-id="$archive_run_id" --campaign-actors="$campaign_actors" --migration-actor="$migration_actor" --dm01-run-id="$dm01_run_id" --reference-corp-id="$reference_corp_id" --usage-recovery-file="$usage_recovery_file")
  fi
done < <(sed -n 's/.*"domain"[[:space:]]*:[[:space:]]*"\([a-z0-9-]*\)".*/\1/p' "$repository_root/docs/release/final-v1-domain-migration-manifest.json")
run_reconcile() {
  local domain="$1"
  if [[ -n "${AICRM_FINAL_IMPORT_COMMAND:-}" ]]; then
    "$AICRM_FINAL_IMPORT_COMMAND" --mode=reconcile --domain="$domain" --archive-run-id="$archive_run_id" --campaign-actors="$campaign_actors" --migration-actor="$migration_actor" --dm01-run-id="$dm01_run_id" --reference-corp-id="$reference_corp_id" --usage-recovery-file="$usage_recovery_file"
  else
    (cd "$repository_root"; GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./cmd/aicrm-v1-domain-import --mode=reconcile --domain="$domain" --archive-run-id="$archive_run_id" --campaign-actors="$campaign_actors" --migration-actor="$migration_actor" --dm01-run-id="$dm01_run_id" --reference-corp-id="$reference_corp_id" --usage-recovery-file="$usage_recovery_file")
  fi
}

# The first five packages share the existing domainImportVersion journal; its
# only safe reconciliation entry point is domain=all. Every later package has
# a domain-specific reconciliation command.
run_reconcile all
while IFS= read -r domain; do
  case "$domain" in campaign|survey|media|radar|shop) continue ;; esac
  run_reconcile "$domain"
done < <(sed -n 's/.*"domain"[[:space:]]*:[[:space:]]*"\([a-z0-9-]*\)".*/\1/p' "$repository_root/docs/release/final-v1-domain-migration-manifest.json")
[[ "$(sha256_file "$source_slice")" = "$source_seal_sha256" ]] || fail 'source slice SHA-256 seal drifted before final reconcile'
[[ "$(sha256_file "$usage_recovery_file")" = "$usage_recovery_sha256" ]] || fail 'usage recovery SHA-256 seal drifted before final reconcile'
if [[ -n "${AICRM_FINAL_RECONCILE_COMMAND:-}" ]]; then
  require_command AICRM_FINAL_RECONCILE_COMMAND
  "$AICRM_FINAL_RECONCILE_COMMAND" --mode=final-reconcile --domain=final --archive-run-id="$archive_run_id" --dm01-run-id="$dm01_run_id" --campaign-actors="$campaign_actors" --migration-actor="$migration_actor" --reference-corp-id="$reference_corp_id" --usage-recovery-file="$usage_recovery_file"
else
  (cd "$repository_root"; GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$go_command" run ./cmd/aicrm-v1-domain-import --mode=final-reconcile --domain=final --archive-run-id="$archive_run_id" --dm01-run-id="$dm01_run_id" --campaign-actors="$campaign_actors" --migration-actor="$migration_actor" --reference-corp-id="$reference_corp_id" --usage-recovery-file="$usage_recovery_file")
fi
"$AICRM_FINAL_STATUS_COMMAND" --check=external-effects --expect=0 --runtime-env-file="$runtime_env_file"
"$AICRM_FINAL_RUNTIME_COMMAND" --start=api,worker --web=api --runtime-env-file="$runtime_env_file"
"$AICRM_FINAL_RUNTIME_COMMAND" --check=release --expected-sha="$expected_sha" --runtime-env-file="$runtime_env_file"
printf 'final-v1-domain-migration-apply: PASS (schema=%s->142 imported-domains=%s; reconciled-scopes=36; split api+worker started)\n' "$expected_start_schema" "$(printf '%s\n' "$pending_domains" | tr ' ' '\n' | wc -l | tr -d ' ')"
