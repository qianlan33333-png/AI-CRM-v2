#!/usr/bin/env bash
set -euo pipefail

script_directory="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
repository_root="$(CDPATH= cd -- "$script_directory/.." && pwd -P)"
checker="$repository_root/scripts/check_g2_web_edge.sh"

"$checker" --root="$repository_root" >/dev/null

expect_failure() {
  fixture_directory="$1"
  label="$2"
  if "$checker" --root="$fixture_directory" >/dev/null 2>&1; then
    printf 'test-g2-web-edge: expected failure for %s\n' "$label" >&2
    exit 1
  fi
}

new_fixture() {
  fixture_directory="$(mktemp -d)"
  mkdir -p "$fixture_directory/deploy"
  cp "$repository_root/deploy/aicrm-edge.service" "$fixture_directory/deploy/aicrm-edge.service"
  cp "$repository_root/deploy/Caddyfile" "$fixture_directory/deploy/Caddyfile"
  printf '%s\n' "$fixture_directory"
}

cleanup_directories=()
cleanup() {
  for fixture_directory in "${cleanup_directories[@]}"; do
    rm -rf -- "$fixture_directory"
  done
}
trap cleanup EXIT

fixture_directory="$(new_fixture)"
cleanup_directories+=("$fixture_directory")
sed -i.bak 's/User=aicrm-edge/User=root/' "$fixture_directory/deploy/aicrm-edge.service"
rm -f -- "$fixture_directory/deploy/aicrm-edge.service.bak"
expect_failure "$fixture_directory" 'root service user'

fixture_directory="$(new_fixture)"
cleanup_directories+=("$fixture_directory")
sed -i.bak 's/reverse_proxy 127\.0\.0\.1:8080/reverse_proxy app:8080/' "$fixture_directory/deploy/Caddyfile"
rm -f -- "$fixture_directory/deploy/Caddyfile.bak"
expect_failure "$fixture_directory" 'non-loopback backend'

fixture_directory="$(new_fixture)"
cleanup_directories+=("$fixture_directory")
sed -i.bak '/Content-Security-Policy/d' "$fixture_directory/deploy/Caddyfile"
rm -f -- "$fixture_directory/deploy/Caddyfile.bak"
expect_failure "$fixture_directory" 'missing CSP'

fixture_directory="$(new_fixture)"
cleanup_directories+=("$fixture_directory")
sed -i.bak 's/CapabilityBoundingSet=CAP_NET_BIND_SERVICE/CapabilityBoundingSet=CAP_SYS_ADMIN/' "$fixture_directory/deploy/aicrm-edge.service"
rm -f -- "$fixture_directory/deploy/aicrm-edge.service.bak"
expect_failure "$fixture_directory" 'broad service capability'

fixture_directory="$(new_fixture)"
cleanup_directories+=("$fixture_directory")
sed -i.bak 's# /q/\*##' "$fixture_directory/deploy/Caddyfile"
rm -f -- "$fixture_directory/deploy/Caddyfile.bak"
expect_failure "$fixture_directory" 'missing public survey backend route'

printf 'test-g2-web-edge: PASS\n'
