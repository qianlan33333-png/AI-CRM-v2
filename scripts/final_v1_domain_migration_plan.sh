#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'final-v1-domain-migration-plan: %s\n' "$1" >&2
  exit 2
}

usage='Usage: final_v1_domain_migration_plan.sh --render|--validate'
[[ "$#" -eq 1 ]] || fail "$usage"

case "$1" in
  --render|--validate) mode="$1" ;;
  --help) printf '%s\n' "$usage"; exit 0 ;;
  *) fail "$usage" ;;
esac

script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"
manifest="$repository_root/docs/release/final-v1-domain-migration-manifest.json"
migrations="$repository_root/migrations"
importer="$repository_root/cmd/aicrm-v1-domain-import"

[[ -f "$manifest" && ! -L "$manifest" ]] || fail 'manifest is missing or is a symlink'
[[ -d "$migrations" && ! -L "$migrations" ]] || fail 'migrations directory is missing or is a symlink'
[[ -d "$importer" && ! -L "$importer" ]] || fail 'domain importer directory is missing or is a symlink'
[[ -z "${AICRM_V1_ARCHIVE_SOURCE_DATABASE_URL:-}" ]] || fail 'V1 archive source database URL must be unset'
[[ -z "${AICRM_DM01_SOURCE_DATABASE_URL:-}" ]] || fail 'DM01 source database URL must be unset'

for version in $(seq 132 142); do
  matches=("$migrations"/"$(printf '%05d' "$version")"_*.sql)
  [[ -f "${matches[0]:-}" && ! -L "${matches[0]:-}" ]] || fail "required migration $version is missing"
done

if grep -R -E -q 'hxc_activation_funnel|hxc_activation_broadcast' "$migrations"; then
  fail 'cancelled HXC minimal-A objects are present in migrations'
fi

latest=''
while IFS= read -r file; do
  base="${file##*/}"
  version="${base%%_*}"
  [[ "$version" =~ ^[0-9]+$ ]] || continue
  version=$((10#$version))
  [[ -z "$latest" || "$version" -gt "$latest" ]] && latest="$version"
done < <(find "$migrations" -maxdepth 1 -type f -name '*.sql' -print)
[[ "$latest" = '142' ]] || fail "final schema must be 142, found ${latest:-none}"

grep -Fq '"from": 132' "$manifest" || fail 'manifest must start from schema 132'
grep -Fq '"to": 142' "$manifest" || fail 'manifest must end at schema 142'
if grep -Eq '"domain"[[:space:]]*:[[:space:]]*"all"' "$manifest"; then
  fail 'manifest must enumerate domains and cannot use domain=all'
fi

domain_count=0
while IFS= read -r domain; do
  [[ -n "$domain" ]] || continue
  grep -R -Fq "\"$domain\"" "$importer" || fail "domain importer does not expose $domain"
  domain_count=$((domain_count + 1))
done < <(sed -n 's/.*"domain"[[:space:]]*:[[:space:]]*"\([a-z0-9-]*\)".*/\1/p' "$manifest")
[[ "$domain_count" -gt 0 ]] || fail 'manifest has no domains'

if [[ "$mode" = '--render' ]]; then
  cat "$manifest"
else
  printf 'final-v1-domain-migration-plan: PASS (schema=132->142 domains=%s)\n' "$domain_count"
fi
