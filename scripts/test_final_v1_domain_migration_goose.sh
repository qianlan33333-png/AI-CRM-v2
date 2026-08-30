#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'test-final-v1-domain-migration-goose: %s\n' "$1" >&2; exit 1; }
root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
goose_runner="$root/scripts/final_v1_domain_migration_goose.sh"
fixture="$(mktemp -d -t aicrm-final-goose.XXXXXX)"
trap 'rm -rf -- "$fixture"' EXIT

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi
}

log="$fixture/goose.log"; goose="$fixture/goose"; env_file="$fixture/runtime.env"
cat >"$goose" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'args=%s\n' "$*" >>"${AICRM_FINAL_GOOSE_TEST_LOG:?}"
printf 'dsn=%s\n' "${GOOSE_DBSTRING:-}" >>"${AICRM_FINAL_GOOSE_TEST_LOG:?}"
[[ "$GOOSE_DRIVER" = postgres && "$*" = *"-dir ${AICRM_FINAL_GOOSE_TEST_MIGRATIONS:?} up-to 143" ]]
EOF
chmod 700 "$goose"
dsn='postgres://target-user:secret@target.example/aicrm?sslmode=require'
cat >"$env_file" <<EOF
AICRM_DATABASE_URL=$dsn
AICRM_FINAL_BUNDLED_GOOSE_COMMAND=$goose
AICRM_FINAL_BUNDLED_GOOSE_SHA256=$(sha256_file "$goose")
EOF
chmod 600 "$env_file"

AICRM_FINAL_GOOSE_TEST_LOG="$log" AICRM_FINAL_GOOSE_TEST_MIGRATIONS="$root/migrations" \
  "$goose_runner" --from=135 --to=143 --runtime-env-file="$env_file"
[[ "$(grep -c '^args=' "$log")" = 1 ]] || fail 'Goose was not invoked exactly once'
grep -Fq "args=-dir $root/migrations up-to 143" "$log" || fail 'Goose bounds or migration directory changed'
grep -Fq "dsn=$dsn" "$log" || fail 'Goose did not receive the target DSN through environment'
! grep -Fq "$dsn" <(grep '^args=' "$log") || fail 'target DSN leaked into Goose arguments'

if AICRM_FINAL_GOOSE_TEST_LOG="$log" "$goose_runner" --from=135 --to=141 --runtime-env-file="$env_file" >"$fixture/bounds.log" 2>&1; then fail 'wrong final bound was accepted'; fi
grep -Fq 'only one bounded Goose migration from 135 to 143 is accepted' "$fixture/bounds.log" || fail 'wrong bound rejection changed'
printf '0' >>"$goose"
if AICRM_FINAL_GOOSE_TEST_LOG="$log" "$goose_runner" --from=135 --to=143 --runtime-env-file="$env_file" >"$fixture/seal.log" 2>&1; then fail 'modified bundled Goose was accepted'; fi
grep -Fq 'bundled goose SHA-256 does not match runtime control' "$fixture/seal.log" || fail 'bundled Goose seal rejection changed'

printf 'test-final-v1-domain-migration-goose: PASS\n'
