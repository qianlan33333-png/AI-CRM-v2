#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

test_root="$(mktemp -d -t aicrm-v2-ci-acceptance-manifest.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

manifest="$test_root/manifest.tsv"
log="$test_root/make.log"
fake_make="$test_root/make"
database_url="postgres://fixture"

printf '%s\n' 'one|ONE_TEST_DATABASE_URL|one-target' 'two|-|two-target' >"$manifest"
printf '%s\n' '#!/usr/bin/env bash' 'set -euo pipefail' 'printf "%s|%s\\n" "${ONE_TEST_DATABASE_URL:-}" "$*" >>"${CI_ACCEPTANCE_TEST_LOG:?}"' >"$fake_make"
chmod +x "$fake_make"

CI_ACCEPTANCE_MANIFEST="$manifest" \
CI_ACCEPTANCE_DATABASE_URL="$database_url" \
CI_ACCEPTANCE_TEST_LOG="$log" \
MAKE="$fake_make" \
scripts/run_ci_acceptance_manifest.sh >/dev/null

grep -Fqx "$database_url|--no-print-directory one-target" "$log" || {
  echo "ci-acceptance-manifest-tests: database target was not invoked exactly once" >&2
  exit 1
}
grep -Fqx "|--no-print-directory two-target" "$log" || {
  echo "ci-acceptance-manifest-tests: target without database was not invoked exactly once" >&2
  exit 1
}

printf '%s\n' 'bad|ONE_TEST_DATABASE_URL|bad-target|extra' >"$manifest"
if CI_ACCEPTANCE_MANIFEST="$manifest" CI_ACCEPTANCE_DATABASE_URL="$database_url" MAKE="$fake_make" scripts/run_ci_acceptance_manifest.sh >/dev/null 2>&1; then
  echo "ci-acceptance-manifest-tests: malformed manifest was accepted" >&2
  exit 1
fi

CI_ACCEPTANCE_MANIFEST=docs/ci/go-acceptance-manifest.tsv \
CI_ACCEPTANCE_DATABASE_URL="$database_url" \
CI_ACCEPTANCE_TEST_LOG="$log" \
MAKE="$fake_make" \
scripts/run_ci_acceptance_manifest.sh >/dev/null

echo "ci-acceptance-manifest-tests: PASS"
