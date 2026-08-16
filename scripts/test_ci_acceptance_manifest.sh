#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

test_root="$(mktemp -d -t aicrm-v2-ci-acceptance-manifest.XXXXXX)"
fixture_name=".ci-manifest-fixture-$$"
fixture_dir="acceptance/$fixture_name"
fixture_script="acceptance/${fixture_name}.sh"
symlink_script="acceptance/${fixture_name}-symlink.sh"
trap 'rm -rf "$test_root" "$fixture_dir"; rm -f "$fixture_script" "$symlink_script"' EXIT

manifest="$test_root/manifest.tsv"
log="$test_root/executor.log"
fake_make="$test_root/make"
fake_go="$test_root/go"
fixture_makefile="$test_root/Makefile"
database_url="postgres://fixture"
header='# id|sequence|database environment (- when not needed)|executor|subject|selector (- when not needed)'

mkdir -p "$fixture_dir"
printf '%s\n' 'package acceptance' >"$fixture_dir/fixture_test.go"
printf '%s\n' '#!/usr/bin/env bash' 'set -euo pipefail' 'printf "%s|script|%s\n" "${SCRIPT_TEST_DATABASE_URL:-}" "$0" >>"${CI_ACCEPTANCE_TEST_LOG:?}"' >"$fixture_script"
chmod +x "$fixture_script"

printf '%s\n' '#!/usr/bin/env bash' 'set -euo pipefail' 'printf "%s|make|%s\n" "${LEGACY_TEST_DATABASE_URL:-}" "$*" >>"${CI_ACCEPTANCE_TEST_LOG:?}"' >"$fake_make"
printf '%s\n' '#!/usr/bin/env bash' 'set -euo pipefail' 'printf "%s|go|%s\n" "${PACKAGE_TEST_DATABASE_URL:-}" "$*" >>"${CI_ACCEPTANCE_TEST_LOG:?}"' >"$fake_go"
chmod +x "$fake_make" "$fake_go"
printf '%s\n' \
  'legacy-acceptance:' \
  'one-acceptance:' \
  'two-acceptance:' \
  'zeta-acceptance:' \
  'alpha-acceptance:' \
  'bad-acceptance:' >"$fixture_makefile"

printf '%s\n' \
  "$header" \
  'legacy|0001|LEGACY_TEST_DATABASE_URL|legacy-make|legacy-acceptance|-' \
  "package|0002|PACKAGE_TEST_DATABASE_URL|go-test|./$fixture_dir|^TestManifest$" \
  "script|0003|SCRIPT_TEST_DATABASE_URL|script|$fixture_script|-" >"$manifest"

CI_ACCEPTANCE_MANIFEST="$manifest" \
CI_ACCEPTANCE_DATABASE_URL="$database_url" \
CI_ACCEPTANCE_TEST_LOG="$log" \
MAKE="$fake_make" \
GO="$fake_go" \
CI_ACCEPTANCE_MAKEFILE="$fixture_makefile" \
scripts/run_ci_acceptance_manifest.sh >/dev/null

grep -Fqx "$database_url|make|--no-print-directory legacy-acceptance" "$log" || {
  echo "ci-acceptance-manifest-tests: legacy Make executor was not invoked exactly" >&2
  exit 1
}
grep -Fqx "$database_url|go|test -race -count=1 -timeout=240s -run ^TestManifest$ ./$fixture_dir" "$log" || {
  echo "ci-acceptance-manifest-tests: Go executor was not invoked exactly" >&2
  exit 1
}
grep -Fqx "$database_url|script|$fixture_script" "$log" || {
  echo "ci-acceptance-manifest-tests: script executor was not invoked exactly" >&2
  exit 1
}

: >"$log"
CI_ACCEPTANCE_MANIFEST="$manifest" \
CI_ACCEPTANCE_VALIDATE_ONLY=1 \
CI_ACCEPTANCE_TEST_LOG="$log" \
MAKE="$test_root/not-installed-make" \
GO="$test_root/not-installed-go" \
CI_ACCEPTANCE_MAKEFILE="$fixture_makefile" \
scripts/run_ci_acceptance_manifest.sh >/dev/null
[[ ! -s "$log" ]] || {
  echo "ci-acceptance-manifest-tests: validate-only mode executed an acceptance command" >&2
  exit 1
}

expect_rejected() {
  local name="$1"
  shift
  printf '%s\n' "$header" "$@" >"$manifest"
  if CI_ACCEPTANCE_MANIFEST="$manifest" CI_ACCEPTANCE_DATABASE_URL="$database_url" \
    CI_ACCEPTANCE_TEST_LOG="$log" MAKE="$fake_make" GO="$fake_go" \
    CI_ACCEPTANCE_MAKEFILE="$fixture_makefile" \
    scripts/run_ci_acceptance_manifest.sh >/dev/null 2>&1; then
    echo "ci-acceptance-manifest-tests: $name was accepted" >&2
    exit 1
  fi
}

expect_rejected unknown-executor 'bad|0001|-|shell|acceptance/bad.sh|-'
expect_rejected arbitrary-argument-injection "bad|0001|-|go-test|./$fixture_dir|^TestManifest\$;touch"
expect_rejected path-traversal 'bad|0001|-|script|acceptance/../bad.sh|-'
expect_rejected duplicate-id \
  'duplicate|0001|-|legacy-make|one-acceptance|-' \
  'duplicate|0002|-|legacy-make|two-acceptance|-'
expect_rejected unordered-sequence \
  'zeta|0002|-|legacy-make|zeta-acceptance|-' \
  'alpha|0001|-|legacy-make|alpha-acceptance|-'
expect_rejected non-canonical-extra-field 'bad|0001|-|legacy-make|bad-acceptance|-|extra'
expect_rejected non-canonical-whitespace 'bad|0001|-|legacy-make|bad-acceptance|- '

ln -s "${fixture_name}.sh" "$symlink_script"
expect_rejected symlink-script "bad|0001|-|script|$symlink_script|-"

CI_ACCEPTANCE_MANIFEST=docs/ci/go-acceptance-manifest.tsv \
CI_ACCEPTANCE_DATABASE_URL="$database_url" \
CI_ACCEPTANCE_TEST_LOG="$log" \
MAKE="$fake_make" \
GO="$fake_go" \
scripts/run_ci_acceptance_manifest.sh >/dev/null

current_entries="$(awk -F'|' '$0 !~ /^#/ && NF { count++ } END { print count + 0 }' docs/ci/go-acceptance-manifest.tsv)"
(( current_entries >= 43 )) && grep -Fq 'p4-execution-runtime-ab|0027|-|legacy-make|p4-execution-runtime-ab-acceptance|-' docs/ci/go-acceptance-manifest.tsv || {
  echo "ci-acceptance-manifest-tests: latest-main legacy entries were not preserved" >&2
  exit 1
}

expanded_manifest="$test_root/expanded-manifest.tsv"
cp docs/ci/go-acceptance-manifest.tsv "$expanded_manifest"
printf '%s\n' "zz-generic-fixture|0044|-|go-test|./$fixture_dir|-" >>"$expanded_manifest"
CI_ACCEPTANCE_MANIFEST="$expanded_manifest" \
CI_ACCEPTANCE_VALIDATE_ONLY=1 \
CI_ACCEPTANCE_BASE_REF=HEAD \
CI_ACCEPTANCE_BASE_MANIFEST_PATH=docs/ci/go-acceptance-manifest.tsv \
scripts/run_ci_acceptance_manifest.sh >/dev/null || {
  echo "ci-acceptance-manifest-tests: a new declarative acceptance entry required runner changes" >&2
  exit 1
}

reordered_manifest="$test_root/reordered-manifest.tsv"
awk -F'|' 'BEGIN { OFS = "|" }
  NR == 2 { first = $0; next }
  NR == 3 {
    split(first, original, "|")
    print $1, original[2], $3, $4, $5, $6
    print original[1], $2, original[3], original[4], original[5], original[6]
    next
  }
  { print }
' docs/ci/go-acceptance-manifest.tsv >"$reordered_manifest"
if CI_ACCEPTANCE_MANIFEST="$reordered_manifest" \
  CI_ACCEPTANCE_VALIDATE_ONLY=1 \
  CI_ACCEPTANCE_BASE_REF=HEAD \
  CI_ACCEPTANCE_BASE_MANIFEST_PATH=docs/ci/go-acceptance-manifest.tsv \
  scripts/run_ci_acceptance_manifest.sh >/dev/null 2>&1; then
  echo "ci-acceptance-manifest-tests: base acceptance ids were silently reordered" >&2
  exit 1
fi

removed_manifest="$test_root/removed-manifest.tsv"
awk '!/^p4-execution-runtime-ab\|/' docs/ci/go-acceptance-manifest.tsv >"$removed_manifest"
if CI_ACCEPTANCE_MANIFEST="$removed_manifest" \
  CI_ACCEPTANCE_VALIDATE_ONLY=1 \
  CI_ACCEPTANCE_BASE_REF=HEAD \
  CI_ACCEPTANCE_BASE_MANIFEST_PATH=docs/ci/go-acceptance-manifest.tsv \
  scripts/run_ci_acceptance_manifest.sh >/dev/null 2>&1; then
  echo "ci-acceptance-manifest-tests: a base acceptance id was silently removed" >&2
  exit 1
fi

echo "ci-acceptance-manifest-tests: PASS"
