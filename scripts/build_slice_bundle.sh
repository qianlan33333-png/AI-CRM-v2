#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 <base-sha> <slice-id> <paths-file> <output-dir>" >&2
  exit 2
}

[[ $# -eq 4 ]] || usage

base_sha="$1"
slice_id="$2"
paths_file="$3"
output_dir="$4"
repo_root="$(cd "$(git rev-parse --show-toplevel)" && pwd -P)"

[[ "$base_sha" =~ ^[0-9a-f]{40}$ ]] || {
  echo "base SHA must be 40 lowercase hex characters" >&2
  exit 2
}
[[ "$slice_id" =~ ^[A-Z0-9-]+$ ]] || {
  echo "slice ID contains unsupported characters" >&2
  exit 2
}
[[ -f "$paths_file" ]] || {
  echo "paths file does not exist: $paths_file" >&2
  exit 2
}
git -C "$repo_root" cat-file -e "${base_sha}^{commit}"
command -v gitleaks >/dev/null || {
  echo "gitleaks is required and must be installed before packaging" >&2
  exit 2
}
[[ "$(gitleaks version)" == "8.30.1" ]] || {
  echo "gitleaks 8.30.1 is required for reproducible packaging" >&2
  exit 2
}
command -v zip >/dev/null
command -v zipinfo >/dev/null
command -v sha256sum >/dev/null

mkdir -p "$output_dir"
output_dir="$(cd "$output_dir" && pwd -P)"
case "$output_dir" in
  "$repo_root"|"$repo_root"/*)
    echo "bundle output directory must be outside the repository" >&2
    exit 2
    ;;
esac
if find "$output_dir" -mindepth 1 -print -quit | grep -q .; then
  echo "bundle output directory must be empty and dedicated to this build" >&2
  exit 2
fi
work_dir="$(mktemp -d -t aicrm-v2-bundle.XXXXXX)"
trap 'rm -rf "$work_dir"' EXIT
payload_dir="$work_dir/${slice_id}-source"
mkdir -p "$payload_dir"

selected_paths="$work_dir/selected-paths.txt"
: >"$selected_paths"

while IFS= read -r raw_path || [[ -n "$raw_path" ]]; do
  path="${raw_path%%#*}"
  path="${path%$'\r'}"
  [[ -z "$path" ]] && continue
  [[ "$path" != /* && "$path" != *".."* && "$path" != .git* ]] || {
    echo "unsafe selected path: $path" >&2
    exit 1
  }
  git -C "$repo_root" cat-file -e "${base_sha}:${path}" || {
    echo "selected path does not exist at base SHA: $path" >&2
    exit 1
  }
  printf '%s\n' "$path" >>"$selected_paths"
done <"$paths_file"

[[ -s "$selected_paths" ]] || {
  echo "paths file selected no repository content" >&2
  exit 1
}

while IFS= read -r path; do
  git -C "$repo_root" archive --format=tar "$base_sha" -- "$path" |
    tar -xf - -C "$payload_dir"
done <"$selected_paths"

if find "$payload_dir" -mindepth 1 ! -type f ! -type d -print | grep -q .; then
  find "$payload_dir" -mindepth 1 ! -type f ! -type d -print >&2
  echo "bundle contains a symbolic link or special file" >&2
  exit 1
fi

while IFS= read -r -d '' payload_path; do
  relative_path="${payload_path#"$payload_dir"/}"
  case "$relative_path" in
    *$'\n'*|*$'\r'*|*\\*)
      echo "bundle path contains a newline, carriage return, or backslash" >&2
      exit 1
      ;;
  esac
  [[ ! "$relative_path" =~ ^[A-Za-z]: ]] || {
    echo "bundle path resembles a Windows drive path" >&2
    exit 1
  }
done < <(find "$payload_dir" -mindepth 1 -print0)

forbidden_name_pattern='(^|/)\.env[^/]*(/|$)|(^|/)(id_rsa[^/]*|cookies[^/]*\.json|credentials[^/]*\.json)$|\.(pem|key|p12|pfx|db|sqlite|sqlite3|dump)$'
if find "$payload_dir" -type f -print | sed "s#^${payload_dir}/##" |
  grep -E "$forbidden_name_pattern" >"$work_dir/forbidden-paths.txt"; then
  sed -n '1,80p' "$work_dir/forbidden-paths.txt" >&2
  echo "bundle contains a forbidden sensitive or runtime path" >&2
  exit 1
fi

gitleaks_report="$output_dir/${slice_id}-gitleaks.json"
gitleaks detect --no-git --source "$payload_dir" --redact --report-format json \
  --report-path "$gitleaks_report" --exit-code 1

gitleaks_version="$(gitleaks version | tr '\n' ' ')"
payload_files="$work_dir/payload-files.txt"
find "$payload_dir" -type f -print | sed "s#^${payload_dir}/##" | LC_ALL=C sort >"$payload_files"
{
  printf 'slice_id=%s\n' "$slice_id"
  printf 'base_sha=%s\n' "$base_sha"
  printf 'gitleaks_version=%s\n' "$gitleaks_version"
  printf 'secret_scan=PASS\n'
  printf 'selected_paths:\n'
  sed 's/^/  - /' "$selected_paths"
  printf 'payload_files:\n'
  sed 's/^/  - /' "$payload_files"
  printf '  - BUNDLE-MANIFEST.txt (generated)\n'
} >"$payload_dir/BUNDLE-MANIFEST.txt"

archive_path="$output_dir/${slice_id}-source-${base_sha:0:12}.zip"
rm -f "$archive_path"
(cd "$work_dir" && zip -X -q -r "$archive_path" "${slice_id}-source")
zip -T "$archive_path"

zip_entries="$work_dir/zip-entries.txt"
zipinfo -1 "$archive_path" >"$zip_entries"
set +e
awk 'BEGIN{bad=0}
     /^\//{bad=1}
     substr($0,2,1)==":"{bad=1}
     index($0,sprintf("%c",92))>0{bad=1}
     /(^|\/)\.\.($|\/)/{bad=1}
     END{exit bad ? 0 : 1}' "$zip_entries"
zip_path_status=$?
set -e
case "$zip_path_status" in
  0)
    echo "archive contains an absolute, traversal, or ambiguous cross-platform path" >&2
    exit 1
    ;;
  1) ;;
  *)
    echo "archive path validation failed with exit code $zip_path_status" >&2
    exit 1
    ;;
esac

archive_size="$(stat -f '%z' "$archive_path" 2>/dev/null || stat -c '%s' "$archive_path")"
archive_hash="$(sha256sum "$archive_path" | awk '{print $1}')"
report_hash="$(sha256sum "$gitleaks_report" | awk '{print $1}')"

{
  printf '%s  %s\n' "$archive_hash" "$(basename "$archive_path")"
  printf '%s  %s\n' "$report_hash" "$(basename "$gitleaks_report")"
} >"$output_dir/SHA256SUMS"

printf 'archive=%s\nbytes=%s\nsha256=%s\nsecret_scan=PASS\n' \
  "$archive_path" "$archive_size" "$archive_hash"
