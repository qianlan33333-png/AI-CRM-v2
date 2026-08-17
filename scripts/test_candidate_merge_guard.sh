#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
guard="$repo_root/scripts/check_candidate_merge_guard.sh"
test_root="$(mktemp -d -t aicrm-v2-candidate-guard.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT
required_paths=$'docs/api-mapping.jsonl\ndocs/ci/go-acceptance-manifest.tsv\ncmd/aicrm/legacy_push_center_api.go\ninternal/pushcenter/store/repository.go\nmigrations/00044_push_center_read_model.sql'
without_mapping=$'docs/ci/go-acceptance-manifest.tsv\ncmd/aicrm/legacy_push_center_api.go\ninternal/pushcenter/store/repository.go\nmigrations/00044_push_center_read_model.sql'
without_acceptance=$'docs/api-mapping.jsonl\ncmd/aicrm/legacy_push_center_api.go\ninternal/pushcenter/store/repository.go\nmigrations/00044_push_center_read_model.sql'
rollout_proof_paths=$'docs/ci/repo-contract-fingerprints.tsv\ndocs/evidence/governance/ci-merge-gate-rollout-proof.md'
rollout_proof_business_without_mapping=$'docs/ci/go-acceptance-manifest.tsv\ndocs/ci/repo-contract-fingerprints.tsv\ndocs/evidence/governance/ci-merge-gate-rollout-proof.md\ncmd/aicrm/legacy_push_center_api.go\ninternal/pushcenter/store/repository.go\nmigrations/00044_push_center_read_model.sql'
rollout_proof_business_without_acceptance=$'docs/api-mapping.jsonl\ndocs/ci/repo-contract-fingerprints.tsv\ndocs/evidence/governance/ci-merge-gate-rollout-proof.md\ncmd/aicrm/legacy_push_center_api.go\ninternal/pushcenter/store/repository.go\nmigrations/00044_push_center_read_model.sql'

write_event() {
  local output_path="$1" title="$2" body="$3"
  python3 - "$output_path" "$title" "$body" <<'PY'
import json
import sys
with open(sys.argv[1], "w", encoding="utf-8") as target:
    json.dump({"pull_request": {"title": sys.argv[2], "body": sys.argv[3]}}, target)
PY
}

write_diff_event() {
  local output_path="$1" base="$2" head="$3"
  python3 - "$output_path" "$base" "$head" <<'PY'
import json
import sys
with open(sys.argv[1], "w", encoding="utf-8") as target:
    json.dump({"pull_request": {"title": "feat: 正式业务集成", "body": "正式业务集成。", "base": {"sha": sys.argv[2]}, "head": {"sha": sys.argv[3]}}}, target)
PY
}

run_case() {
  local name="$1" expected="$2" title="$3" body="$4" paths="$5" event_path
  event_path="$test_root/$name.json"
  write_event "$event_path" "$title" "$body"
  if GITHUB_EVENT_NAME=pull_request CANDIDATE_GUARD_EVENT_PATH="$event_path" CANDIDATE_GUARD_CHANGED_PATHS="$paths" "$guard" >/dev/null 2>&1
  then
    [[ "$expected" = pass ]] || { echo "candidate-merge-guard-tests: accepted $name" >&2; exit 1; }
  else
    [[ "$expected" = fail ]] || { echo "candidate-merge-guard-tests: rejected $name" >&2; exit 1; }
  fi
}

run_diff_case() {
  local name="$1" expected="$2" extra_path="$3" extra_content="$4"
  local fixture_repo="$test_root/$name-repo" event_path base head
  mkdir -p "$fixture_repo"
  git init -q "$fixture_repo"
  git -C "$fixture_repo" config user.email 'candidate-guard@example.invalid'
  git -C "$fixture_repo" config user.name 'Candidate Guard Test'
  mkdir -p "$fixture_repo/docs/ci" "$fixture_repo/cmd/aicrm" "$fixture_repo/internal/pushcenter" "$fixture_repo/migrations" "$fixture_repo/scripts"
  : > "$fixture_repo/docs/api-mapping.jsonl"
  : > "$fixture_repo/docs/ci/go-acceptance-manifest.tsv"
  : > "$fixture_repo/cmd/aicrm/legacy_push_center_api.go"
  : > "$fixture_repo/internal/pushcenter/repository.go"
  : > "$fixture_repo/migrations/00044_push_center_read_model.sql"
  git -C "$fixture_repo" add .
  git -C "$fixture_repo" commit -qm 'base'
  base="$(git -C "$fixture_repo" rev-parse HEAD)"
  printf '%s\n' 'route mapping' >> "$fixture_repo/docs/api-mapping.jsonl"
  printf '%s\n' 'acceptance' >> "$fixture_repo/docs/ci/go-acceptance-manifest.tsv"
  printf '%s\n' 'package main' >> "$fixture_repo/cmd/aicrm/legacy_push_center_api.go"
  printf '%s\n' 'package pushcenter' >> "$fixture_repo/internal/pushcenter/repository.go"
  printf '%s\n' '-- migration' >> "$fixture_repo/migrations/00044_push_center_read_model.sql"
  mkdir -p "$(dirname "$fixture_repo/$extra_path")"
  printf '%s\n' "$extra_content" > "$fixture_repo/$extra_path"
  git -C "$fixture_repo" add .
  git -C "$fixture_repo" commit -qm 'formal integration'
  head="$(git -C "$fixture_repo" rev-parse HEAD)"
  event_path="$test_root/$name.json"
  write_diff_event "$event_path" "$base" "$head"
  if GITHUB_EVENT_NAME=pull_request CANDIDATE_GUARD_EVENT_PATH="$event_path" CANDIDATE_GUARD_REPO_ROOT="$fixture_repo" "$guard" >/dev/null 2>&1
  then
    [[ "$expected" = pass ]] || { echo "candidate-merge-guard-tests: accepted $name" >&2; exit 1; }
  else
    [[ "$expected" = fail ]] || { echo "candidate-merge-guard-tests: rejected $name" >&2; exit 1; }
  fi
}

run_no_schema_diff_case() {
  local name="$1" expected="$2" include_slice="$3" matrix_evidence="${4:-yes}" slice_evidence="${5:-yes}" forged_business_evidence="${6:-no}" mapping_evidence="${7:-no}"
  local fixture_repo="$test_root/$name-repo" event_path base head
  mkdir -p "$fixture_repo"
  git init -q "$fixture_repo"
  git -C "$fixture_repo" config user.email 'candidate-guard@example.invalid'
  git -C "$fixture_repo" config user.name 'Candidate Guard Test'
  mkdir -p "$fixture_repo/docs/ci" "$fixture_repo/cmd/aicrm" "$fixture_repo/internal/adminshell" "$fixture_repo/docs/execution/slices"
  : > "$fixture_repo/docs/api-mapping.jsonl"
  : > "$fixture_repo/docs/ci/go-acceptance-manifest.tsv"
  : > "$fixture_repo/docs/feature-matrix.csv"
  : > "$fixture_repo/cmd/aicrm/legacy_admin_shell.go"
  : > "$fixture_repo/internal/adminshell/service.go"
  git -C "$fixture_repo" add .
  git -C "$fixture_repo" commit -qm 'base'
  base="$(git -C "$fixture_repo" rev-parse HEAD)"
  if [[ "$mapping_evidence" = yes ]]; then
    printf '%s\n' '{"disposition_reason":"no_schema_or_external_effect"}' >> "$fixture_repo/docs/api-mapping.jsonl"
  else
    printf '%s\n' 'route mapping' >> "$fixture_repo/docs/api-mapping.jsonl"
  fi
  printf '%s\n' 'acceptance' >> "$fixture_repo/docs/ci/go-acceptance-manifest.tsv"
  if [[ "$matrix_evidence" = yes ]]; then
    printf '%s\n' 'no_schema_or_external_effect' >> "$fixture_repo/docs/feature-matrix.csv"
  else
    printf '%s\n' 'ordinary business row without schema evidence' >> "$fixture_repo/docs/feature-matrix.csv"
  fi
  printf '%s\n' 'package main' >> "$fixture_repo/cmd/aicrm/legacy_admin_shell.go"
  printf '%s\n' 'package adminshell' >> "$fixture_repo/internal/adminshell/service.go"
  if [[ "$forged_business_evidence" = yes ]]; then
    printf '%s\n' '// no_schema_or_external_effect' >> "$fixture_repo/internal/adminshell/service.go"
  fi
  if [[ "$include_slice" = yes ]]; then
    if [[ "$slice_evidence" = yes ]]; then
      printf '%s\n' 'no_schema_or_external_effect' > "$fixture_repo/docs/execution/slices/P4-NO-SCHEMA.md"
    else
      printf '%s\n' 'scope evidence without schema declaration' > "$fixture_repo/docs/execution/slices/P4-NO-SCHEMA.md"
    fi
  fi
  git -C "$fixture_repo" add .
  git -C "$fixture_repo" commit -qm 'formal no-schema integration'
  head="$(git -C "$fixture_repo" rev-parse HEAD)"
  event_path="$test_root/$name.json"
  write_diff_event "$event_path" "$base" "$head"
  if GITHUB_EVENT_NAME=pull_request CANDIDATE_GUARD_EVENT_PATH="$event_path" CANDIDATE_GUARD_REPO_ROOT="$fixture_repo" "$guard" >/dev/null 2>&1
  then
    [[ "$expected" = pass ]] || { echo "candidate-merge-guard-tests: accepted $name" >&2; exit 1; }
  else
    [[ "$expected" = fail ]] || { echo "candidate-merge-guard-tests: rejected $name" >&2; exit 1; }
  fi
}

run_framework_tooling_diff_case() {
  local name="$1" expected="$2" extra_path="${3:-}" extra_content="${4:-}"
  local fixture_repo="$test_root/$name-repo" event_path base head framework_path
  local -a framework_paths=(
    'docs/ci/repo-contract-fingerprints.tsv'
    'scripts/check_generated_sources.sh'
    'scripts/check_repo_contract.sh'
    'scripts/run_ci_acceptance_manifest.sh'
    'scripts/test_ci_acceptance_manifest.sh'
    'scripts/test_gitless_generated_check.sh'
    'scripts/test_repo_contract.sh'
    'tools/migration-mapping/main.go'
    'tools/migration-mapping/main_test.go'
    'tools/p1-reconciliation/main.go'
    'tools/p1-reconciliation/main_test.go'
  )
  mkdir -p "$fixture_repo"
  git init -q "$fixture_repo"
  git -C "$fixture_repo" config user.email 'candidate-guard@example.invalid'
  git -C "$fixture_repo" config user.name 'Candidate Guard Test'
  git -C "$fixture_repo" commit --allow-empty -qm 'base'
  base="$(git -C "$fixture_repo" rev-parse HEAD)"
  for framework_path in "${framework_paths[@]}"; do
    mkdir -p "$(dirname "$fixture_repo/$framework_path")"
    printf '%s\n' 'framework tooling' >"$fixture_repo/$framework_path"
  done
  if [[ -n "$extra_path" ]]; then
    mkdir -p "$(dirname "$fixture_repo/$extra_path")"
    printf '%s\n' "$extra_content" >"$fixture_repo/$extra_path"
  fi
  git -C "$fixture_repo" add .
  git -C "$fixture_repo" commit -qm 'framework tooling change'
  head="$(git -C "$fixture_repo" rev-parse HEAD)"
  event_path="$test_root/$name.json"
  write_diff_event "$event_path" "$base" "$head"
  if GITHUB_EVENT_NAME=pull_request CANDIDATE_GUARD_EVENT_PATH="$event_path" CANDIDATE_GUARD_REPO_ROOT="$fixture_repo" "$guard" >/dev/null 2>&1
  then
    [[ "$expected" = pass ]] || { echo "candidate-merge-guard-tests: accepted $name" >&2; exit 1; }
  else
    [[ "$expected" = fail ]] || { echo "candidate-merge-guard-tests: rejected $name" >&2; exit 1; }
  fi
}

run_case formal-integration pass 'feat: 集成 Push Center 读模型' '正式业务集成。' "$required_paths"
run_case candidate-title fail 'CANDIDATE_ONLY: Push Center' '正式业务集成。' "$required_paths"
run_case prohibited-merge fail 'feat: Push Center' '禁止合并' "$required_paths"
run_case candidate-evidence fail 'feat: Push Center' 'Evidence-Status: DOMAIN_LEAF_READY' "$required_paths"
run_case not-wired fail 'feat: Push Center' 'not wired' "$required_paths"
run_case missing-mapping fail 'feat: Push Center' '正式业务集成。' "$without_mapping"
run_case missing-acceptance fail 'feat: Push Center' '正式业务集成。' "$without_acceptance"
run_case missing-http fail 'feat: Push Center' '正式业务集成。' $'docs/api-mapping.jsonl\ndocs/ci/go-acceptance-manifest.tsv\ninternal/pushcenter/store/repository.go\nmigrations/00044_push_center_read_model.sql'
run_case missing-store fail 'feat: Push Center' '正式业务集成。' $'docs/api-mapping.jsonl\ndocs/ci/go-acceptance-manifest.tsv\ncmd/aicrm/legacy_push_center_api.go\nmigrations/00044_push_center_read_model.sql'
run_case missing-migration fail 'feat: Push Center' '正式业务集成。' $'docs/api-mapping.jsonl\ndocs/ci/go-acceptance-manifest.tsv\ncmd/aicrm/legacy_push_center_api.go\ninternal/pushcenter/store/repository.go'
run_no_schema_diff_case formal-no-schema pass yes
run_no_schema_diff_case formal-mapping-only-no-schema pass yes no yes no yes
run_no_schema_diff_case no-schema-without-slice fail no
run_no_schema_diff_case no-schema-forged-business-text fail yes no yes yes
run_no_schema_diff_case no-schema-without-slice-declaration fail yes yes no
run_no_schema_diff_case mapping-only-without-slice-declaration fail yes no no no yes
run_case rollout-proof-only pass 'docs: CI rollout proof' '仅记录 CI rollout 证明。' "$rollout_proof_paths"
run_case rollout-proof-business-missing-mapping fail 'feat: Push Center with CI proof' '真实业务集成仍需正式闭环。' "$rollout_proof_business_without_mapping"
run_case rollout-proof-business-missing-acceptance fail 'feat: Push Center with CI proof' '真实业务集成仍需正式闭环。' "$rollout_proof_business_without_acceptance"
run_case policy-only pass 'infra: CI policy' '仅修改 CI 策略。' $'.github/workflows/ci.yml\nscripts/ci/classify_changes.py\nscripts/check_repo_contract.sh\ndocs/ci/repo-contract-fingerprints.tsv'
run_diff_case policy-self-description pass 'scripts/check_repo_contract.sh' 'not-wired implementation in the pull_request diff is not mergeable'
run_diff_case policy-test-self-description pass 'scripts/test_repo_fingerprints.sh' 'not-wired implementation in the pull_request diff is not mergeable'
run_diff_case policy-receipt-self-description pass 'docs/ci/repo-contract-fingerprints.tsv' 'not-wired implementation in the pull_request diff is not mergeable'
run_diff_case openapi-checker-self-description pass 'tools/openapi-contract/main_test.go' 'not-wired implementation in the pull_request diff is not mergeable'
run_diff_case implementation-not-wired fail 'internal/pushcenter/candidate.go' '// not-wired candidate implementation'
run_framework_tooling_diff_case framework-tooling-only pass
run_framework_tooling_diff_case framework-tooling-plus-business-without-formal-closure fail 'internal/pushcenter/store/repository.go' 'package pushcenter'

echo 'candidate-merge-guard-tests: PASS'
