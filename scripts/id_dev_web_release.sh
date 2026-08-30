#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'id-dev-web-release: %s\n' "$1" >&2
  exit 2
}

usage='Usage: id_dev_web_release.sh --root=<absolute-directory> --release=<absolute-dist-directory> --source-sha=<40-char-sha> [--check|--activate|--rollback]'
[[ "$#" -eq 4 ]] || fail "$usage"
root=''
release=''
source_sha=''
mode=''
for argument in "$@"; do
  case "$argument" in
    --root=*) root="${argument#--root=}" ;;
    --release=*) release="${argument#--release=}" ;;
    --source-sha=*) source_sha="${argument#--source-sha=}" ;;
    --check|--activate|--rollback) mode="${argument#--}" ;;
    *) fail "$usage" ;;
  esac
done
[[ "$root" = /* && -d "$root" && ! -L "$root" ]] || fail 'root must be an absolute regular directory'
[[ "$release" = /* && -d "$release" && ! -L "$release" ]] || fail 'release must be an absolute regular directory'
[[ "$source_sha" =~ ^[0-9a-f]{40}$ ]] || fail 'source SHA must be lowercase and 40 characters'
[[ "$mode" = check || "$mode" = activate || "$mode" = rollback ]] || fail 'exactly one mode is required'

script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"
current="$root/id-dev-current"
previous="$root/id-dev-previous"
if [[ "$mode" = rollback ]]; then
  [[ -L "$previous" ]] || fail 'previous release link is missing'
  release="$(readlink "$previous")"
  [[ "$release" = /* && -d "$release" && ! -L "$release" ]] || fail 'previous release target is invalid'
  source_sha="$(node -e 'const fs=require("node:fs"); const manifest=JSON.parse(fs.readFileSync(process.argv[1],"utf8")); process.stdout.write(String(manifest.source_sha||""))' "$release/asset-manifest.json")"
  [[ "$source_sha" =~ ^[0-9a-f]{40}$ ]] || fail 'previous release manifest has an invalid source SHA'
fi
node "$repository_root/web/scripts/validate-release.mjs" "$release" "$source_sha" >/dev/null
[[ "$mode" = check ]] && { printf 'id-dev-web-release: CHECK PASS\n'; exit 0; }

current_target=''
if [[ -L "$current" ]]; then
  current_target="$(readlink "$current")"
  [[ "$current_target" = /* && -d "$current_target" ]] || fail 'current release target is invalid'
fi
switch_directory="$(mktemp -d "$root/.id-dev-switch.XXXXXX")"
cleanup() { rm -rf -- "$switch_directory"; }
trap cleanup EXIT
temporary_current="$switch_directory/current"
ln -s "$release" "$temporary_current"
if [[ -n "$current_target" && "$current_target" != "$release" ]]; then
  temporary_previous="$switch_directory/previous"
  ln -s "$current_target" "$temporary_previous"
  if mv --help >/dev/null 2>&1; then mv -Tf "$temporary_previous" "$previous"; else mv -fh "$temporary_previous" "$previous"; fi
fi
if mv --help >/dev/null 2>&1; then mv -Tf "$temporary_current" "$current"; else mv -fh "$temporary_current" "$current"; fi
trap - EXIT
cleanup
printf 'id-dev-web-release: %s PASS target=%s\n' "${mode^^}" "$release"
