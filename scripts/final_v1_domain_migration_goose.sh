#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'final-v1-domain-migration-goose: %s\n' "$1" >&2
  exit 2
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{ print $1 }'
  else
    fail 'sha256sum or shasum is required to verify bundled goose'
  fi
}

runtime_env_file='' from='' to=''
for argument in "$@"; do
  case "$argument" in
    --from=*) [[ -z "$from" ]] || fail 'duplicate --from'; from="${argument#--from=}" ;;
    --to=*) [[ -z "$to" ]] || fail 'duplicate --to'; to="${argument#--to=}" ;;
    --runtime-env-file=*) [[ -z "$runtime_env_file" ]] || fail 'duplicate --runtime-env-file'; runtime_env_file="${argument#--runtime-env-file=}" ;;
    *) fail 'only --from=135 --to=143 --runtime-env-file=<absolute-file> are accepted' ;;
  esac
done

[[ "$from" = 135 && "$to" = 143 ]] || fail 'only one bounded Goose migration from 135 to 143 is accepted'
[[ "$runtime_env_file" = /* && -f "$runtime_env_file" && ! -L "$runtime_env_file" ]] || fail 'runtime environment file is invalid'

read_env_value() {
  local key="$1" value='' line
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" = "$key="* ]] && value="${line#*=}"
  done <"$runtime_env_file"
  printf '%s' "$value"
}

database_url="$(read_env_value AICRM_DATABASE_URL)"
goose_command="$(read_env_value AICRM_FINAL_BUNDLED_GOOSE_COMMAND)"
goose_sha256="$(read_env_value AICRM_FINAL_BUNDLED_GOOSE_SHA256)"
[[ -n "$database_url" ]] || fail 'AICRM_DATABASE_URL is required'
[[ "$goose_command" = /* && -f "$goose_command" && ! -L "$goose_command" && -x "$goose_command" ]] || fail 'AICRM_FINAL_BUNDLED_GOOSE_COMMAND must be an executable absolute regular file'
[[ "$goose_sha256" =~ ^[a-f0-9]{64}$ ]] || fail 'AICRM_FINAL_BUNDLED_GOOSE_SHA256 is invalid'
[[ "$(sha256_file "$goose_command")" = "$goose_sha256" ]] || fail 'bundled goose SHA-256 does not match runtime control'

script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"
migrations="$repository_root/migrations"
[[ -d "$migrations" && ! -L "$migrations" ]] || fail 'migrations directory is invalid'
for version in $(seq 136 143); do
  matches=("$migrations"/"$(printf '%05d' "$version")"_*.sql)
  [[ -f "${matches[0]:-}" && ! -L "${matches[0]:-}" ]] || fail "required migration $version is missing"
done

# The DSN remains process environment data; it is never sourced, formatted, or
# placed in the Goose argument vector.
env -u BASH_ENV -u ENV GOOSE_DRIVER=postgres GOOSE_DBSTRING="$database_url" \
  "$goose_command" -dir "$migrations" up-to 143
