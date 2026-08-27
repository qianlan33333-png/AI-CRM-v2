#!/usr/bin/env bash
set -euo pipefail

repository_root="$(git rev-parse --show-toplevel)"
cd "$repository_root"

fail() {
  printf 'G2 runtime image acceptance: %s\n' "$1" >&2
  exit 1
}

dockerfile="$(git show ':deploy/Dockerfile.runtime')"
dockerignore="$(git show ':deploy/Dockerfile.runtime.dockerignore')"
builder="$(git show ':scripts/build_release_binary.sh')"

[[ "$(grep -Fxc 'FROM scratch' <<<"$dockerfile")" -eq 1 ]] ||
  fail 'runtime image must use the empty scratch base'
[[ "$(grep -Fxc 'USER 65532:65532' <<<"$dockerfile")" -eq 1 ]] ||
  fail 'runtime image must run as the fixed non-root identity'
[[ "$(grep -Fxc 'ENTRYPOINT ["/aicrm"]' <<<"$dockerfile")" -eq 1 ]] ||
  fail 'runtime image entrypoint drifted'
grep -Fq 'org.opencontainers.image.revision="${AICRM_SOURCE_SHA}"' <<<"$dockerfile" ||
  fail 'runtime image source SHA label is missing'
! grep -Eiq '(^|[[:space:]])(RUN|ADD|ENV|CMD|SHELL|HEALTHCHECK)([[:space:]]|$)' <<<"$dockerfile" ||
  fail 'runtime image added an executable or mutable layer instruction'
[[ "$dockerignore" = $'**\n!aicrm' ]] ||
  fail 'runtime image context must expose only the release binary'

for required in \
  'CGO_ENABLED=0 GOOS=linux GOARCH=amd64' \
  'GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly' \
  'go build -buildvcs=false -trimpath' \
  '-buildid=$source_sha' \
  "output must remain outside the repository"; do
  grep -Fq -- "$required" <<<"$builder" || fail "release builder lost contract: $required"
done

temporary_directory="$(mktemp -d -t aicrm-g2-image.XXXXXX)"
trap 'rm -rf -- "$temporary_directory"' EXIT

if scripts/build_release_binary.sh --output=relative/aicrm >/dev/null 2>&1; then
  fail 'relative output was accepted'
fi
if scripts/build_release_binary.sh --output="$repository_root/aicrm" >/dev/null 2>&1; then
  fail 'repository output was accepted'
fi
if scripts/build_release_binary.sh --command=unknown --output="$temporary_directory/unknown" >/dev/null 2>&1; then
  fail 'unknown release command was accepted'
fi

scripts/build_release_binary.sh --output="$temporary_directory/aicrm" >"$temporary_directory/receipt"
[[ -x "$temporary_directory/aicrm" ]] || fail 'release binary is not executable'
grep -Eq '^[0-9a-f]{64}[[:space:]]+' "$temporary_directory/receipt" ||
  fail 'release binary SHA-256 receipt is missing'

metadata="$(go version -m "$temporary_directory/aicrm")"
source_sha="$(git rev-parse HEAD)"
grep -Fq $'\tGOOS=linux' <<<"$metadata" || fail 'GOOS receipt mismatch'
grep -Fq $'\tGOARCH=amd64' <<<"$metadata" || fail 'GOARCH receipt mismatch'
[[ "$(go tool buildid "$temporary_directory/aicrm")" = "$source_sha" ]] ||
  fail 'source SHA build ID mismatch'

scripts/build_release_binary.sh \
  --command=aicrm-river-migrate \
  --output="$temporary_directory/aicrm-river-migrate" \
  >"$temporary_directory/migration-receipt"
[[ -x "$temporary_directory/aicrm-river-migrate" ]] || fail 'River migration binary is not executable'
[[ "$(go tool buildid "$temporary_directory/aicrm-river-migrate")" = "$source_sha" ]] ||
  fail 'River migration binary source SHA build ID mismatch'

scripts/build_release_binary.sh \
  --command=aicrm-v1-domain-import \
  --output="$temporary_directory/aicrm-v1-domain-import" \
  >"$temporary_directory/domain-import-receipt"
[[ -x "$temporary_directory/aicrm-v1-domain-import" ]] || fail 'V1 domain import binary is not executable'
[[ "$(go tool buildid "$temporary_directory/aicrm-v1-domain-import")" = "$source_sha" ]] ||
  fail 'V1 domain import binary source SHA build ID mismatch'

printf 'G2 runtime image acceptance: PASS\n'
