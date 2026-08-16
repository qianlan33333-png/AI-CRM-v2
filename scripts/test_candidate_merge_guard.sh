#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
guard="$repo_root/scripts/check_candidate_merge_guard.sh"
test_root="$(mktemp -d -t aicrm-v2-candidate-guard.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT
required_paths=$'docs/api-mapping.jsonl\ndocs/ci/go-acceptance-manifest.tsv\ncmd/aicrm/legacy_push_center_api.go\ninternal/pushcenter/store/repository.go\nmigrations/00044_push_center_read_model.sql'
without_mapping=$'docs/ci/go-acceptance-manifest.tsv\ncmd/aicrm/legacy_push_center_api.go\ninternal/pushcenter/store/repository.go\nmigrations/00044_push_center_read_model.sql'
without_acceptance=$'docs/api-mapping.jsonl\ncmd/aicrm/legacy_push_center_api.go\ninternal/pushcenter/store/repository.go\nmigrations/00044_push_center_read_model.sql'

write_event() {
  local output_path="$1" title="$2" body="$3"
  python3 - "$output_path" "$title" "$body" <<'PY'
import json
import sys
with open(sys.argv[1], "w", encoding="utf-8") as target:
    json.dump({"pull_request": {"title": sys.argv[2], "body": sys.argv[3]}}, target)
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

echo 'candidate-merge-guard-tests: PASS'
