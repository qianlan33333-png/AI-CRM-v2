#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'final-v1-domain-migration-plan-tests: %s\n' "$1" >&2
  exit 1
}

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
plan="$root/scripts/final_v1_domain_migration_plan.sh"

[[ "$("$plan" --validate)" = 'final-v1-domain-migration-plan: PASS (schema=132->144 domains=40)' ]] ||
  fail 'baseline validation did not pass'
rendered="$("$plan" --render)"
grep -Fq '"from": 132' <<<"$rendered" || fail 'render omitted schema start'
grep -Fq '"to": 144' <<<"$rendered" || fail 'render omitted schema end'
grep -Fq '"domain": "campaign"' <<<"$rendered" || fail 'render omitted explicit domains'
if grep -Eq '"domain"[[:space:]]*:[[:space:]]*"all"' <<<"$rendered"; then
  fail 'render must not rely on domain=all'
fi

fixture="$(mktemp -d -t aicrm-final-migration-plan.XXXXXX)"
cleanup() { rm -rf -- "$fixture"; }
trap cleanup EXIT
mkdir -p "$fixture/scripts" "$fixture/docs/release" "$fixture/migrations" "$fixture/cmd/aicrm-v1-domain-import"
cp "$plan" "$fixture/scripts/"
cp "$root/docs/release/final-v1-domain-migration-manifest.json" "$fixture/docs/release/"
cp "$root/migrations"/001{32,33,34,35,36,37,38,39,40,41,42,43,44}_*.sql "$fixture/migrations/"
printf 'package main\nconst domain = "campaign"\n' >"$fixture/cmd/aicrm-v1-domain-import/main.go"
for domain in $(sed -n 's/.*"domain"[[:space:]]*:[[:space:]]*"\([a-z0-9-]*\)".*/\1/p' "$fixture/docs/release/final-v1-domain-migration-manifest.json"); do
  printf 'const domain_%s = "%s"\n' "${domain//-/_}" "$domain" >>"$fixture/cmd/aicrm-v1-domain-import/main.go"
done
printf '%s\n' '-- +goose Up' 'CREATE TABLE public.hxc_activation_funnel(id bigint);' >"$fixture/migrations/00142_hxc_activation_funnel_local.sql"
if "$fixture/scripts/final_v1_domain_migration_plan.sh" --validate >"$fixture/reject.log" 2>&1; then
  fail 'cancelled HXC migration was accepted'
fi
grep -Fq 'cancelled HXC minimal-A objects are present' "$fixture/reject.log" || fail 'cancelled HXC rejection was not stable'

if AICRM_V1_ARCHIVE_SOURCE_DATABASE_URL='postgres://v1' "$plan" --validate >"$fixture/source-url.log" 2>&1; then
  fail 'V1 archive source URL was accepted'
fi
grep -Fq 'V1 archive source database URL must be unset' "$fixture/source-url.log" ||
  fail 'V1 archive source URL rejection was not stable'

printf 'final-v1-domain-migration-plan-tests: PASS\n'
