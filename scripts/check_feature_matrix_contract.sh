#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C PATH=/usr/bin:/bin

fail() { printf 'feature-matrix-contract: %s\n' "$*" >&2; exit 1; }

phase=""
if [[ "$#" -eq 2 && "$1" == "--completion" ]]; then
  phase="$2"
  [[ "$phase" == p1 || "$phase" == p4 || "$phase" == p5 ]] || fail "invalid completion phase: $phase"
elif [[ "$#" -ne 0 ]]; then
  fail "usage: check_feature_matrix_contract.sh [--completion p1|p4|p5]"
fi

script_source="${BASH_SOURCE[0]:-}"
script_parent="${script_source%/*}"; [[ "$script_parent" != "$script_source" ]] || script_parent=.; [[ -n "$script_parent" ]] || script_parent=/
[[ ! -L "$script_parent" ]] || fail "real script directory required: scripts"
script_dir="$(CDPATH= cd -- "$script_parent" && pwd -P)" || fail "cannot locate script directory"
repo_root="$(CDPATH= cd -- "$script_dir/.." && pwd -P)" || fail "cannot locate repository root"
script_path="$script_dir/${script_source##*/}"
matrix="$repo_root/docs/feature-matrix.csv"
anchor="$repo_root/docs/evidence/p1/feature-matrix-id-anchor.v1"
expected_anchor_sha256="1ab849cb10518e55f5c95716c1fab6f2c9e47477d17ad7f3f125edcc7e01ad75"
expected_header="feature_id,page,section,action,triggered_api,expected_result,notes,disposition,implementation,verification,signoff,legacy_source_sha,source_evidence,decision_evidence,implementation_evidence,verification_evidence,target_feature_id"
g1_d02_evidence="decision=G1-D02-2026-08-10;approved_by=repository_owner;approved_at=2026-08-10;semantics=legacy_behavior_1_to_1;verification=NOT_EXECUTED"

mode_of() {
  local path="$1" mode
  mode="$(stat -f '%Lp' "$path" 2>/dev/null || true)"
  if [[ "$mode" =~ ^[0-7]{3,4}$ ]]; then printf '%s\n' "$mode"; return; fi
  mode="$(stat -c '%a' "$path" 2>/dev/null || true)"
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || return 1
  printf '%s\n' "$mode"
}
check_regular() {
  local path="$1" expected="$2" label="$3" actual
  [[ -f "$path" && ! -L "$path" ]] || fail "regular file required: $label"
  actual="$(mode_of "$path")" || fail "cannot read mode: $label"
  (( (8#$actual & 07777) == 8#$expected )) || fail "mode must be exactly $expected: $label"
}
hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}'
  else fail "sha256sum or shasum is required"
  fi
}

for directory in "$repo_root" "$repo_root/docs" "$repo_root/docs/evidence" "$repo_root/docs/evidence/p1" "$script_dir"; do
  [[ -d "$directory" && ! -L "$directory" ]] || fail "real directory required: $directory"
done
check_regular "$script_path" 0755 scripts/check_feature_matrix_contract.sh
check_regular "$matrix" 0644 docs/feature-matrix.csv
check_regular "$anchor" 0644 docs/evidence/p1/feature-matrix-id-anchor.v1
[[ "$(hash_file "$anchor")" == "$expected_anchor_sha256" ]] || fail "anchor is not the frozen revision"

CSV_FIELDS=()
parse_csv_line() {
  local line="$1" label="$2" field="" char next quoted=0 closed=0 i
  CSV_FIELDS=()
  for ((i=0; i<${#line}; i++)); do
    char="${line:i:1}"
    if (( quoted )); then
      if [[ "$char" == '"' ]]; then
        next="${line:i+1:1}"
        if [[ "$next" == '"' ]]; then field="${field}\""; i=$((i + 1))
        else quoted=0; closed=1
        fi
      else field="${field}${char}"
      fi
    elif (( closed )); then
      [[ "$char" == ',' ]] || fail "invalid character after closing quote at $label"
      CSV_FIELDS[${#CSV_FIELDS[@]}]="$field"; field=""; closed=0
    else
      case "$char" in
        ',') CSV_FIELDS[${#CSV_FIELDS[@]}]="$field"; field="" ;;
        '"') [[ -z "$field" ]] || fail "quote inside unquoted field at $label"; quoted=1 ;;
        *) field="${field}${char}" ;;
      esac
    fi
  done
  (( quoted == 0 )) || fail "unterminated quoted field at $label"
  CSV_FIELDS[${#CSV_FIELDS[@]}]="$field"
}

tmp_root="$(mktemp -d -t aicrm-v2-feature-matrix.XXXXXX)" || fail "cannot create temporary directory"
trap 'rm -rf "$tmp_root"' EXIT
ids="$tmp_root/ids"; targets="$tmp_root/targets"; immutable="$tmp_root/immutable"; byte_dump="$tmp_root/bytes"
: >"$ids"; : >"$targets"; : >"$immutable"

file_bytes="$(wc -c <"$matrix" | tr -d ' ')"
[[ "$file_bytes" =~ ^[0-9]+$ ]] || fail "cannot read matrix byte count"
(( file_bytes > 0 && file_bytes <= 262144 )) || fail "matrix size is outside the 1..262144 byte contract"
od -An -v -tu1 "$matrix" >"$byte_dump" || fail "cannot inspect matrix bytes"
set +e; awk '{ for (i = 1; i <= NF; i++) { if ($i == 0) nul = 1; last = $i } } END { exit (nul ? 1 : (last == 10 ? 0 : 2)) }' "$byte_dump"; byte_status=$?; set -e
case "$byte_status" in 0) ;; 1) fail "matrix contains NUL bytes" ;; 2) fail "matrix must end with one LF" ;; *) fail "cannot validate matrix bytes" ;; esac

line_number=0; observed_bytes=0; row_count=0; p1_pending=0; p4_pending=0; p5_pending=0
synthetic=0; staging=0; production=0
while IFS= read -r line || [[ -n "$line" ]]; do
  line_number=$((line_number + 1)); observed_bytes=$((observed_bytes + ${#line} + 1))
  (( ${#line} <= 4096 )) || fail "physical row exceeds 4096 bytes at line $line_number"
  [[ "$line" != *$'\r'* ]] || fail "carriage return is forbidden at line $line_number"
  if (( line_number == 1 )); then
    [[ "$line" == "$expected_header" ]] || fail "header must be the exact ADR-010 17-column schema"
    continue
  fi
  parse_csv_line "$line" "line $line_number"
  [[ "${#CSV_FIELDS[@]}" -eq 17 ]] || fail "expected 17 columns at line $line_number"
  for value in "${CSV_FIELDS[@]}"; do
    [[ "$value" != *$'\t'* ]] || fail "tab is forbidden at line $line_number"
  done
  feature_id="${CSV_FIELDS[0]}"; disposition="${CSV_FIELDS[7]}"; implementation="${CSV_FIELDS[8]}"
  verification="${CSV_FIELDS[9]}"; signoff="${CSV_FIELDS[10]}"; legacy_sha="${CSV_FIELDS[11]}"
  source_evidence="${CSV_FIELDS[12]}"; decision_evidence="${CSV_FIELDS[13]}"
  implementation_evidence="${CSV_FIELDS[14]}"; verification_evidence="${CSV_FIELDS[15]}"; target="${CSV_FIELDS[16]}"
  [[ "$feature_id" =~ ^[A-Z][A-Z0-9-]+$ ]] || fail "invalid feature_id at line $line_number"
  for required_index in 1 2 3 4 5 6; do [[ -n "${CSV_FIELDS[$required_index]}" ]] || fail "required fact is empty at line $line_number"; done
  [[ "$legacy_sha" =~ ^[0-9a-f]{40}$ && -n "$source_evidence" ]] || fail "legacy source evidence is incomplete at line $line_number"
  [[ "$disposition" =~ ^(UNREVIEWED|MIGRATE|MERGED|DEPRECATED)$ ]] || fail "invalid disposition at line $line_number"
  [[ "$implementation" =~ ^(NOT_STARTED|IN_PROGRESS|IMPLEMENTED)$ ]] || fail "invalid implementation at line $line_number"
  [[ "$verification" =~ ^(NOT_RUN|SYNTHETIC_PASS|STAGING_PASS|PRODUCTION_PASS)$ ]] || fail "invalid verification at line $line_number"
  [[ "$signoff" =~ ^(NOT_REQUIRED|PENDING_HUMAN_SIGNOFF|APPROVED)$ ]] || fail "invalid signoff at line $line_number"
  if [[ "$disposition" == UNREVIEWED ]]; then
    [[ "$signoff" == PENDING_HUMAN_SIGNOFF && -z "$decision_evidence" ]] || fail "UNREVIEWED row must await human signoff at line $line_number"
  elif [[ "$disposition" == MIGRATE ]]; then
    [[ "$signoff" == APPROVED && "$decision_evidence" == "$g1_d02_evidence" ]] || fail "MIGRATE row lacks exact G1-D02 decision evidence at line $line_number"
  else
    [[ "$signoff" == APPROVED && "$decision_evidence" =~ approved_by=[^\;]+ && "$decision_evidence" =~ approved_at=[^\;]+ ]] || fail "decided row lacks approved decision evidence at line $line_number"
  fi
  [[ "$disposition" != MERGED || ( -n "$target" && "$decision_evidence" =~ semantics=[^\;]+ ) ]] || fail "MERGED row lacks target or merge semantics at line $line_number"
  [[ "$disposition" != DEPRECATED || "$decision_evidence" =~ reason=[^\;]+ ]] || fail "DEPRECATED row lacks reason at line $line_number"
  [[ -z "$target" || "$target" != "$feature_id" ]] || fail "self target at line $line_number"
  [[ "$implementation" != NOT_STARTED || -z "$implementation_evidence" ]] || fail "NOT_STARTED row has implementation evidence at line $line_number"
  if [[ "$implementation" == IMPLEMENTED ]]; then
    [[ "$implementation_evidence" =~ /pull/[0-9]+ && "$implementation_evidence" =~ [0-9a-f]{40} && "$implementation_evidence" =~ tests=[^\;]+ && "$implementation_evidence" =~ paths=[^\;]+ ]] || fail "IMPLEMENTED evidence lacks PR, merge SHA, tests, or paths at line $line_number"
  fi
  [[ "$verification" == NOT_RUN || "$implementation" == IMPLEMENTED ]] || fail "verification PASS requires IMPLEMENTED at line $line_number"
  case "$verification" in
    NOT_RUN) [[ -z "$verification_evidence" ]] || fail "NOT_RUN row has verification evidence at line $line_number" ;;
    SYNTHETIC_PASS) [[ "$verification_evidence" =~ method=[^\;]+ && "$verification_evidence" =~ command=[^\;]+ ]] || fail "synthetic evidence lacks method or command at line $line_number"; synthetic=$((synthetic + 1)) ;;
    STAGING_PASS) [[ "$verification_evidence" =~ environment=[^\;]+ && "$verification_evidence" =~ build_sha=[0-9a-f]{40} && "$verification_evidence" =~ time=[^\;]+ && "$verification_evidence" =~ evidence=[^\;]+ ]] || fail "staging evidence is incomplete at line $line_number"; staging=$((staging + 1)) ;;
    PRODUCTION_PASS) [[ "$verification_evidence" =~ environment=[^\;]+ && "$verification_evidence" =~ build_sha=[0-9a-f]{40} && "$verification_evidence" =~ time=[^\;]+ && "$verification_evidence" =~ evidence=[^\;]+ && "$verification_evidence" =~ authorization=[^\;]+ ]] || fail "production evidence is incomplete at line $line_number"; production=$((production + 1)) ;;
  esac
  printf '%s:%s\n' "${#feature_id}" "$feature_id" >>"$ids"
  [[ -z "$target" ]] || printf '%s\n' "$target" >>"$targets"
  for fact_index in 0 1 2 3 4 5 6 11 12; do value="${CSV_FIELDS[$fact_index]}"; printf '%s:%s\n' "${#value}" "$value" >>"$immutable"; done
  row_count=$((row_count + 1))
  [[ "$disposition" != UNREVIEWED ]] || p1_pending=$((p1_pending + 1))
  [[ "$disposition" == DEPRECATED || ( "$disposition" != UNREVIEWED && "$implementation" == IMPLEMENTED ) ]] || p4_pending=$((p4_pending + 1))
  [[ "$disposition" == DEPRECATED || ( "$implementation" == IMPLEMENTED && ( "$verification" == STAGING_PASS || "$verification" == PRODUCTION_PASS ) ) ]] || p5_pending=$((p5_pending + 1))
done <"$matrix"
[[ "$observed_bytes" -eq "$file_bytes" ]] || fail "matrix byte stream changed during parse"

duplicate="$(sort "$ids" | uniq -d | head -n 1 || true)"
[[ -z "$duplicate" ]] || fail "duplicate feature_id"
while IFS= read -r target; do grep -Fqx -- "${#target}:$target" "$ids" || fail "dangling target: $target"; done <"$targets"

[[ "$(sed -n '1p' "$anchor")" == version=1 && "$(sed -n '2p' "$anchor")" == "rows=$row_count" ]] || fail "anchor version or row count mismatch"
[[ "$(sed -n '3p' "$anchor")" == "id_sha256=$(hash_file "$ids")" ]] || fail "feature ID order drifted"
[[ "$(sed -n '4p' "$anchor")" == 'immutable_fields=feature_id,page,section,action,triggered_api,expected_result,notes,legacy_source_sha,source_evidence' ]] || fail "immutable field contract drifted"
[[ "$(sed -n '5p' "$anchor")" == "immutable_sha256=$(hash_file "$immutable")" && "$(wc -l <"$anchor" | tr -d ' ')" == 5 ]] || fail "immutable legacy facts drifted"

if [[ -n "$phase" ]]; then
  case "$phase" in p1) pending="$p1_pending" ;; p4) pending="$p4_pending" ;; p5) pending="$p5_pending" ;; esac
  if (( pending > 0 )); then printf 'feature-matrix-completion: PENDING phase=%s rows=%s pending=%s synthetic=%s staging=%s production=%s\n' "$phase" "$row_count" "$pending" "$synthetic" "$staging" "$production"; exit 2; fi
  printf 'feature-matrix-completion: PASS phase=%s rows=%s synthetic=%s staging=%s production=%s\n' "$phase" "$row_count" "$synthetic" "$staging" "$production"
else
  printf 'feature-matrix-contract: PASS (rows=%s)\n' "$row_count"
fi
