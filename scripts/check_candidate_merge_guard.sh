#!/usr/bin/env bash
set -euo pipefail

if [[ "${GITHUB_EVENT_NAME:-}" != "pull_request" ]]; then
  echo "candidate-merge-guard: NOT_APPLICABLE"
  exit 0
fi

event_path="${CANDIDATE_GUARD_EVENT_PATH:-${GITHUB_EVENT_PATH:-}}"
repo_root="${CANDIDATE_GUARD_REPO_ROOT:-$(git rev-parse --show-toplevel)}"
changed_paths="${CANDIDATE_GUARD_CHANGED_PATHS:-}"
[[ -n "$event_path" && -f "$event_path" ]] || { echo "candidate-merge-guard: pull_request event payload is required" >&2; exit 1; }

exec python3 - "$event_path" "$repo_root" "$changed_paths" <<'PY'
import json
import re
import subprocess
import sys


def fail(message: str) -> None:
    print(f"candidate-merge-guard: {message}", file=sys.stderr)
    raise SystemExit(1)


event_path, repo_root, changed_override = sys.argv[1:]
try:
    with open(event_path, encoding="utf-8") as source:
        pull_request = json.load(source)["pull_request"]
except (OSError, KeyError, TypeError, json.JSONDecodeError) as err:
    fail(f"invalid pull_request event payload: {err.__class__.__name__}")

title = str(pull_request.get("title") or "")
body = str(pull_request.get("body") or "")
metadata = f"{title}\n{body}"
if re.search(r"candidate_only", metadata, re.IGNORECASE):
    fail("candidate-only title or body is not mergeable")
if "禁止合并" in metadata:
    fail("prohibited-merge marker is not mergeable")
if re.search(r"(?:evidence|证据)[ _-]*(?:status|状态)\s*[:：]\s*(?:domain_leaf_ready|candidate(?:_ready)?)\b", metadata, re.IGNORECASE):
    fail("candidate evidence status is not mergeable")
if re.search(r"not[ _-]*wired", metadata, re.IGNORECASE):
    fail("not-wired title or body is not mergeable")

if changed_override:
    changed = [line for line in changed_override.splitlines() if line]
    added = ""
else:
    try:
        base = str(pull_request["base"]["sha"])
        head = str(pull_request["head"]["sha"])
    except (KeyError, TypeError):
        fail("pull_request base and head SHAs are required")
    if not re.fullmatch(r"[0-9a-f]{40}", base) or not re.fullmatch(r"[0-9a-f]{40}", head):
        fail("pull_request base and head SHAs are invalid")
    try:
        changed = subprocess.check_output(
            ["git", "-C", repo_root, "diff", "--name-only", "--no-renames", base, head], text=True
        ).splitlines()
        patch = subprocess.check_output(
            [
                "git", "-C", repo_root, "diff", "--unified=0", base, head, "--",
                ":(exclude)scripts/check_candidate_merge_guard.sh",
                ":(exclude)scripts/test_candidate_merge_guard.sh",
            ],
            text=True,
        )
    except subprocess.CalledProcessError:
        fail("cannot inspect the exact pull_request diff")
    added = "\n".join(line[1:] for line in patch.splitlines() if line.startswith("+") and not line.startswith("+++"))

if re.search(r"\bDOMAIN_LEAF_READY\b|(?:evidence|证据)[ _-]*(?:status|状态)\s*[:：]\s*Candidate\b", added, re.IGNORECASE):
    fail("candidate evidence in the pull_request diff is not mergeable")
if re.search(r"\bnot[ _-]*wired\b", added, re.IGNORECASE):
    fail("not-wired implementation in the pull_request diff is not mergeable")

required_exact = {"docs/api-mapping.jsonl", "docs/ci/go-acceptance-manifest.tsv"}
missing = sorted(required_exact.difference(changed))
if missing:
    fail("formal mapping or central acceptance is missing: " + ", ".join(missing))
if not any(path.startswith("cmd/aicrm/") for path in changed):
    fail("HTTP composition closure is missing")
if not any(path.startswith("internal/") for path in changed):
    fail("Store or application closure is missing")
if not any(re.fullmatch(r"migrations/[0-9]{5}_[A-Za-z0-9_]+\.sql", path) for path in changed):
    fail("migration closure is missing")

print("candidate-merge-guard: PASS")
PY
