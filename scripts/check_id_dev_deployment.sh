#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'id-dev-deployment-check: %s\n' "$1" >&2
  exit 2
}

usage='Usage: check_id_dev_deployment.sh --base-url=https://id-dev.youcangogogo.com --source-sha=<40-char-sha>'
[[ "$#" -eq 2 ]] || fail "$usage"
base_url=''
source_sha=''
for argument in "$@"; do
  case "$argument" in
    --base-url=*) base_url="${argument#--base-url=}" ;;
    --source-sha=*) source_sha="${argument#--source-sha=}" ;;
    *) fail "$usage" ;;
  esac
done
[[ "$base_url" = 'https://id-dev.youcangogogo.com' ]] || fail 'base URL must be the id-dev HTTPS origin'
[[ "$source_sha" =~ ^[0-9a-f]{40}$ ]] || fail 'source SHA must be lowercase and 40 characters'

script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"
local_manifest="$repository_root/web/dist/asset-manifest.json"
[[ -f "$local_manifest" && ! -L "$local_manifest" ]] || fail 'local built manifest is missing'

temporary_directory="$(mktemp -d -t aicrm-id-dev-check.XXXXXX)"
cleanup() { rm -rf -- "$temporary_directory"; }
trap cleanup EXIT

fetch() {
  local endpoint="$1" headers="$2" body="$3"
  curl --fail --silent --show-error --proto '=https' --tlsv1.2 \
    --dump-header "$headers" --output "$body" "$base_url$endpoint"
}

header_value() {
  local headers="$1" name="$2"
  awk -v wanted="$name" 'BEGIN { IGNORECASE=1; wanted=tolower(wanted) } {
    key=$1; sub(/:$/, "", key)
    if (tolower(key) == wanted) {
      sub(/^[^:]+:[[:space:]]*/, "")
      sub(/\r$/, "")
      value=$0
    }
  } END { print value }' "$headers"
}

health_headers="$temporary_directory/health.headers"
fetch '/healthz' "$health_headers" "$temporary_directory/health.json"
[[ "$(header_value "$health_headers" 'x-aicrm-release-sha')" = "$source_sha" ]] || fail 'health release SHA does not match the merged SHA'
[[ "$(header_value "$health_headers" 'cache-control')" = 'private, no-store' ]] || fail 'health response is not private/no-store'

login_headers="$temporary_directory/login.headers"
fetch '/login' "$login_headers" "$temporary_directory/login.html"
[[ "$(header_value "$login_headers" 'cache-control')" = 'private, no-store' ]] || fail 'login response is not private/no-store'

admin_headers="$temporary_directory/admin.headers"
fetch '/admin/customers.html' "$admin_headers" "$temporary_directory/admin.html"
[[ "$(header_value "$admin_headers" 'cache-control')" = 'no-cache, must-revalidate' ]] || fail 'admin HTML is not revalidated'

sidebar_headers="$temporary_directory/sidebar.headers"
fetch '/sidebar/' "$sidebar_headers" "$temporary_directory/sidebar.html"
[[ "$(header_value "$sidebar_headers" 'cache-control')" = 'no-cache, must-revalidate' ]] || fail 'sidebar HTML is not revalidated'

remote_manifest="$temporary_directory/asset-manifest.json"
manifest_headers="$temporary_directory/manifest.headers"
fetch '/asset-manifest.json' "$manifest_headers" "$remote_manifest"
[[ "$(header_value "$manifest_headers" 'cache-control')" = 'no-cache, must-revalidate' ]] || fail 'release manifest is not revalidated'
cmp -s "$local_manifest" "$remote_manifest" || fail 'served asset manifest differs from the exact local build'
node "$repository_root/web/scripts/validate-release.mjs" "$repository_root/web/dist" "$source_sha" >/dev/null

while IFS= read -r asset; do
  [[ "$asset" == assets/* ]] || fail 'manifest returned an unsafe asset path'
  asset_headers="$temporary_directory/asset.headers"
  curl --fail --silent --show-error --compressed --proto '=https' --tlsv1.2 \
    --dump-header "$asset_headers" --output /dev/null "$base_url/$asset"
  [[ "$(header_value "$asset_headers" 'cache-control')" = 'public, max-age=31536000, immutable' ]] || fail "asset is not immutable: $asset"
done < <(node -e 'const manifest=require(process.argv[1]); for (const file of Object.keys(manifest.files).sort()) console.log(file)' "$remote_manifest")

sidebar_entry="$(node -e 'const manifest=require(process.argv[1]); process.stdout.write(manifest.entries.sidebar)' "$remote_manifest")"
compressed_headers="$temporary_directory/compressed.headers"
curl --fail --silent --show-error --compressed --proto '=https' --tlsv1.2 \
  --dump-header "$compressed_headers" --output /dev/null "$base_url/$sidebar_entry"
case "$(header_value "$compressed_headers" 'content-encoding')" in
  gzip|zstd) ;;
  *) fail 'sidebar entry was not served with gzip or zstd compression' ;;
esac

while IFS= read -r page; do
  curl --fail --silent --show-error --proto '=https' --tlsv1.2 \
    --output /dev/null "$base_url/admin/$page.html"
done < <(node -e 'const registry=require(process.argv[1]); for (const screen of registry.screens) console.log(screen.key)' "$repository_root/web/src/admin/registry.json")

printf 'id-dev-deployment-check: PASS sha=%s\n' "$source_sha"
