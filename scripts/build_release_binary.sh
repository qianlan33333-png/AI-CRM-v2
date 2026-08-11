#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'build-release-binary: %s\n' "$1" >&2
  exit 2
}

usage='Usage: build_release_binary.sh --output=<absolute-file>'
[[ "$#" -eq 1 ]] || fail "$usage"

output_value=''
case "$1" in
  --output=*) output_value="${1#--output=}" ;;
  *) fail "$usage" ;;
esac
[[ "$output_value" = /* ]] || fail 'output must be an absolute file'

output_name="$(basename -- "$output_value")"
[[ -n "$output_name" && "$output_name" != '.' && "$output_name" != '..' ]] ||
  fail 'output file is invalid'
output_directory="$(CDPATH= cd -- "$(dirname -- "$output_value")" && pwd -P)" ||
  fail 'output directory must already exist'
[[ -d "$output_directory" && ! -L "$output_directory" ]] ||
  fail 'output directory must be a regular directory'

script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"
output_file="$output_directory/$output_name"
case "$output_file" in
  "$repository_root"|"$repository_root"/*) fail 'output must remain outside the repository' ;;
esac
[[ ! -L "$output_file" ]] || fail 'output file must not be a symlink'

cd "$repository_root"
git diff --quiet -- . || fail 'working tree must be clean'
git diff --cached --quiet -- . || fail 'index must be clean'
source_sha="$(git rev-parse --verify HEAD)"
[[ "$source_sha" =~ ^[0-9a-f]{40}$ ]] || fail 'source commit is invalid'

temporary_file="$(mktemp "$output_directory/.aicrm-release.XXXXXX")"
cleanup() {
  rm -f -- "$temporary_file"
}
trap cleanup EXIT

env -u BASH_ENV -u ENV \
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly \
  go build -buildvcs=false -trimpath -ldflags="-s -w -buildid=$source_sha" \
    -o "$temporary_file" ./cmd/aicrm
chmod 0755 "$temporary_file"
mv -f -- "$temporary_file" "$output_file"
trap - EXIT

build_metadata="$(go version -m "$output_file")"
grep -Fq $'\tGOOS=linux' <<<"$build_metadata" || fail 'binary GOOS receipt is missing'
grep -Fq $'\tGOARCH=amd64' <<<"$build_metadata" || fail 'binary GOARCH receipt is missing'
[[ "$(go tool buildid "$output_file")" = "$source_sha" ]] || fail 'binary source SHA build ID is missing'

sha256sum "$output_file"
