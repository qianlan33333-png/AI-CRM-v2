#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'test-package-release-archive: %s\n' "$1" >&2
  exit 1
}

script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"
fixture_root="$(mktemp -d -t aicrm-release-archive-test.XXXXXX)"
cleanup() {
  rm -rf -- "$fixture_root"
}
trap cleanup EXIT

source_sha='0123456789abcdef0123456789abcdef01234567'
source_directory="$fixture_root/$source_sha"
output_directory="$fixture_root/output"
mkdir -p \
  "$source_directory/bin" \
  "$source_directory/config" \
  "$source_directory/deploy" \
  "$source_directory/migrations" \
  "$source_directory/runtime" \
  "$output_directory"

required_files=(
  BUILD-METADATA.txt
  bin/aicrm-river-migrate
  bin/aicrm-river-migrate.sha256
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
  printf 'fixture:%s\n' "$relative_name" >"$source_directory/$relative_name"
done
printf '%s\n' "$source_sha" >"$source_directory/SOURCE_SHA"
(
  cd "$source_directory"
  sha256sum \
    BUILD-METADATA.txt \
    SOURCE_SHA \
    bin/aicrm-river-migrate \
    bin/aicrm-river-migrate.sha256 \
    bin/goose \
    config/aicrm.env \
    config/postgresql.conf \
    deploy/compose.yml \
    migrations/00001_bootstrap.sql \
    migrations/00002_event_log.sql \
    migrations/00003_settings.sql \
    migrations/00004_auth.sql \
    runtime/.dockerignore \
    runtime/Dockerfile.runtime \
    runtime/aicrm \
    runtime/aicrm.sha256 >SHA256SUMS
)

archive_file="$output_directory/aicrm-$source_sha.tar.gz"
"$repository_root/scripts/package_release_archive.sh" \
  --source="$source_directory" --output="$archive_file" >/dev/null
[[ -f "$archive_file" && ! -L "$archive_file" ]] || fail 'valid archive was not created'
case "$(uname -s)" in
  Darwin) archive_mode="$(stat -f '%Lp' "$archive_file")" ;;
  Linux) archive_mode="$(stat -c '%a' "$archive_file")" ;;
  *) fail 'unsupported platform for archive mode verification' ;;
esac
[[ "$archive_mode" = '640' ]] || fail 'archive mode is not 0640'
archive_listing="$(tar -tzf "$archive_file")"
grep -Fqx "$source_sha/" <<<"$archive_listing" || fail 'SHA root directory is missing'
if grep -Eq '(^|/)[.]_[^/]*$' <<<"$archive_listing"; then
  fail 'valid archive contains AppleDouble metadata'
fi

printf 'forbidden metadata\n' >"$source_directory/migrations/._00001_bootstrap.sql"
negative_log="$fixture_root/negative.log"
if "$repository_root/scripts/package_release_archive.sh" \
  --source="$source_directory" --output="$output_directory/forbidden.tar.gz" \
  >"$negative_log" 2>&1; then
  fail 'source containing AppleDouble metadata was accepted'
fi
grep -Fq 'source contains forbidden macOS AppleDouble metadata' "$negative_log" ||
  fail 'AppleDouble rejection did not return the stable error'
[[ ! -e "$output_directory/forbidden.tar.gz" ]] ||
  fail 'rejected source still produced an archive'

rm -f "$source_directory/migrations/._00001_bootstrap.sql"
printf 'untracked release content\n' >"$source_directory/extra.txt"
if "$repository_root/scripts/package_release_archive.sh" \
  --source="$source_directory" --output="$output_directory/untracked.tar.gz" \
  >"$negative_log" 2>&1; then
  fail 'source containing an untracked release file was accepted'
fi
grep -Fq 'SHA256SUMS must cover release files exactly' "$negative_log" ||
  fail 'manifest closure rejection did not return the stable error'
[[ ! -e "$output_directory/untracked.tar.gz" ]] ||
  fail 'manifest closure rejection still produced an archive'

printf 'test-package-release-archive: PASS\n'
