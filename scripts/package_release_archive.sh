#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'package-release-archive: %s\n' "$1" >&2
  exit 2
}

usage='Usage: package_release_archive.sh --source=<absolute-directory> --output=<absolute-tar.gz>'
[[ "$#" -eq 2 ]] || fail "$usage"

source_directory=''
output_value=''
for argument in "$@"; do
  case "$argument" in
    --source=*)
      [[ -z "$source_directory" ]] || fail 'duplicate --source'
      source_directory="${argument#--source=}"
      ;;
    --output=*)
      [[ -z "$output_value" ]] || fail 'duplicate --output'
      output_value="${argument#--output=}"
      ;;
    *) fail "$usage" ;;
  esac
done

[[ "$source_directory" = /* ]] || fail 'source must be an absolute directory'
[[ "$output_value" = /* ]] || fail 'output must be an absolute file'
[[ -d "$source_directory" && ! -L "$source_directory" ]] ||
  fail 'source must be a regular directory'

source_directory="$(CDPATH= cd -- "$source_directory" && pwd -P)"
source_name="$(basename -- "$source_directory")"
[[ "$source_name" =~ ^[0-9a-f]{40}$ ]] ||
  fail 'source directory name must be a lowercase 40-character Git SHA'

output_name="$(basename -- "$output_value")"
[[ -n "$output_name" && "$output_name" != '.' && "$output_name" != '..' ]] ||
  fail 'output file is invalid'
output_directory="$(CDPATH= cd -- "$(dirname -- "$output_value")" && pwd -P)" ||
  fail 'output directory must already exist'
[[ -d "$output_directory" && ! -L "$output_directory" ]] ||
  fail 'output directory must be a regular directory'
output_file="$output_directory/$output_name"
case "$output_file" in
  "$source_directory"|"$source_directory"/*) fail 'output must remain outside source' ;;
esac
[[ ! -e "$output_file" && ! -L "$output_file" ]] ||
  fail 'output must not already exist'

required_files=(
  BUILD-METADATA.txt
  SHA256SUMS
  SOURCE_SHA
  bin/aicrm-river-migrate
  bin/aicrm-river-migrate.sha256
  bin/aicrm-v1-domain-import
  bin/aicrm-v1-domain-import.sha256
  bin/goose
  config/aicrm.env
  config/postgresql.conf
  deploy/compose.yml
  migrations/00001_bootstrap.sql
  migrations/00002_event_log.sql
  migrations/00003_settings.sql
  migrations/00004_auth.sql
  runtime/.dockerignore
  runtime/Dockerfile.runtime
  runtime/aicrm
  runtime/aicrm.sha256
)
for relative_name in "${required_files[@]}"; do
  [[ -f "$source_directory/$relative_name" && ! -L "$source_directory/$relative_name" ]] ||
    fail "required regular file is missing: $relative_name"
done

[[ "$(tr -d '\r\n' <"$source_directory/SOURCE_SHA")" = "$source_name" ]] ||
  fail 'SOURCE_SHA does not match the source directory'
[[ -z "$(find "$source_directory" -type l -print -quit)" ]] ||
  fail 'source must not contain symlinks'
[[ -z "$(find "$source_directory" -name '._*' -print -quit)" ]] ||
  fail 'source contains forbidden macOS AppleDouble metadata'
(cd "$source_directory" && diff -u \
  <(find . -type f ! -name SHA256SUMS -print | sed 's#^[.]/##' | LC_ALL=C sort) \
  <(awk '{ line = $0; sub(/^[0-9a-fA-F]+[[:space:]]+[ *]?/, "", line); sub(/^[.]\//, "", line); print line }' SHA256SUMS | LC_ALL=C sort) \
  >/dev/null) || fail 'SHA256SUMS must cover release files exactly'
(cd "$source_directory" && sha256sum -c SHA256SUMS >/dev/null) ||
  fail 'SHA256SUMS verification failed'

temporary_archive="$(mktemp "$output_directory/.aicrm-release-archive.XXXXXX")"
cleanup() {
  rm -f -- "$temporary_archive"
}
trap cleanup EXIT

COPYFILE_DISABLE=1 tar --no-xattrs -czf "$temporary_archive" \
  -C "$(dirname -- "$source_directory")" "$source_name"

archive_listing="$(tar -tzf "$temporary_archive")"
[[ -n "$archive_listing" ]] || fail 'archive is empty'
if grep -Eq '(^|/)[.][.](/|$)|^/|(^|/)[.]_[^/]*$' <<<"$archive_listing"; then
  fail 'archive contains an unsafe path or macOS AppleDouble metadata'
fi
if awk -v root="$source_name/" '
  index($0, root) != 1 && $0 != substr(root, 1, length(root) - 1) { exit 1 }
' <<<"$archive_listing"; then
  :
else
  fail 'archive contains entries outside the SHA root directory'
fi

chmod 0640 "$temporary_archive"
mv -f -- "$temporary_archive" "$output_file"
trap - EXIT
sha256sum "$output_file"
