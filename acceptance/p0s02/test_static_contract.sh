#!/usr/bin/env bash
set -euo pipefail

fail() { echo "p0-s02-static-contract-tests: $*" >&2; exit 1; }
[[ "$#" -eq 0 ]] || fail "unexpected argument"
runner_path="${BASH_SOURCE[0]}"
[[ -f "$runner_path" && ! -L "$runner_path" ]] || fail "runner must be a regular file"
script_dir="$(CDPATH= cd -- "$(dirname -- "$runner_path")" && pwd -P)"
canonical_static="$script_dir/static_contract.sh"
[[ -f "$canonical_static" && ! -L "$canonical_static" && -x "$canonical_static" ]] ||
  fail "canonical static contract is unavailable"

test_root="$(mktemp -d -t p0s02-static-contract.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT

new_fixture() {
  fixture="$test_root/$1"
  mkdir -p "$fixture/acceptance/p0s02" "$fixture/internal/api/generated" "$fixture/internal/platform/http"
  cat >"$fixture/go.mod" <<'EOF'
module github.com/qianlan33333-png/AI-CRM-v2

go 1.26.5
EOF
  cat >"$fixture/internal/api/generated/server.gen.go" <<'EOF'
package generated

import "context"

type HealthResponseStatus string
const Ok HealthResponseStatus = "ok"
type GetHealthzRequestObject struct{}
type GetHealthzResponseObject interface{}
type GetHealthz200JSONResponse struct { Status HealthResponseStatus }
type StrictServerInterface interface {
	GetHealthz(context.Context, GetHealthzRequestObject) (GetHealthzResponseObject, error)
  }
EOF
  printf '%s\n' 'package platformhttp' >"$fixture/internal/platform/http/contract.go"
  chmod 644 "$fixture/internal/platform/http/contract.go"
  cat >"$fixture/internal/platform/http/health.go" <<'EOF'
package platformhttp

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/api/generated"
)

type HealthHandler struct{}

var _ generated.StrictServerInterface = (*HealthHandler)(nil)

func NewHealthHandler() *HealthHandler { return &HealthHandler{} }

func (*HealthHandler) GetHealthz(context.Context, generated.GetHealthzRequestObject) (generated.GetHealthzResponseObject, error) {
	return generated.GetHealthz200JSONResponse{Status: generated.Ok}, nil
}
EOF
  cat >"$fixture/internal/platform/http/health_test.go" <<'EOF'
package platformhttp

import "testing"

func TestHealth(t *testing.T) { _ = NewHealthHandler() }
EOF
  cp "$canonical_static" "$fixture/acceptance/p0s02/static_contract.sh"
  cmp -s "$canonical_static" "$fixture/acceptance/p0s02/static_contract.sh" ||
    fail "fixture static contract differs from canonical static contract"
  chmod 755 "$fixture/acceptance/p0s02/static_contract.sh"
  [[ ! -e "$fixture/.git" && ! -L "$fixture/.git" ]] || fail "fixture must not contain .git"
}

write_stat() {
  mkdir -p "$fixture/bin"
  printf '%s\n' '#!/usr/bin/env bash' 'case "$1" in' \
    "  -f) printf '%s\\n' '$1' ;;" "  -c) printf '%s\\n' '$2' ;;" '  *) exit 64 ;;' 'esac' >"$fixture/bin/stat"
  chmod 755 "$fixture/bin/stat"
}

write_rg_sentinel() {
  mkdir -p "$fixture/bin"
  printf '%s\n' '#!/usr/bin/env bash' 'printf "%s\\n" called >>"$RG_SENTINEL_LOG"' 'exit 97' >"$fixture/bin/rg"
  chmod 755 "$fixture/bin/rg"
}

run_static() {
  local stat_dir="${1:-}"
  local rg_sentinel_log="$fixture/rg-called"
  local output_file="${2:-/dev/null}"
  if [[ -n "$stat_dir" ]]; then
    (cd "$fixture" && env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOPROXY=off GOSUMDB=off PATH="$stat_dir:$PATH" RG_SENTINEL_LOG="$rg_sentinel_log" "$fixture/acceptance/p0s02/static_contract.sh") >"$output_file" 2>&1
  else
    (cd "$fixture" && env -u BASH_ENV -u ENV GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly GOPROXY=off GOSUMDB=off RG_SENTINEL_LOG="$rg_sentinel_log" "$fixture/acceptance/p0s02/static_contract.sh") >"$output_file" 2>&1
  fi
}

reject_with() {
  local expected="$1" output="$fixture/static.out"
  if run_static "" "$output"; then fail "invalid fixture was accepted"; fi
  [[ "$(cat "$output")" == "$expected" ]] || fail "unexpected static diagnostic"
}

new_fixture native
run_static || fail "native mode semantics were rejected"
new_fixture rg-unavailable
write_rg_sentinel
run_static "$fixture/bin" || fail "rg-unavailable mode semantics were rejected"
[[ ! -e "$fixture/rg-called" ]] || fail "static contract invoked rg"
new_fixture bsd
write_stat 644 invalid
run_static "$fixture/bin" || fail "BSD mode semantics were rejected"
new_fixture gnu
write_stat invalid 644
run_static "$fixture/bin" || fail "GNU fallback mode semantics were rejected"
new_fixture nonmode
write_stat invalid invalid
if run_static "$fixture/bin"; then fail "non-mode stat output was accepted"; fi
for mode in 650 645; do
  new_fixture "execute-$mode"
  chmod "$mode" "$fixture/internal/platform/http/health.go"
  if run_static; then fail "execute mode was accepted: $mode"; fi
done
for kind in symlink special extra; do
  new_fixture "$kind"
  case "$kind" in
    symlink) rm -f "$fixture/internal/platform/http/health.go"; ln -s health_test.go "$fixture/internal/platform/http/health.go" ;;
    special) rm -f "$fixture/internal/platform/http/health.go"; mkfifo "$fixture/internal/platform/http/health.go" ;;
    extra) : >"$fixture/internal/platform/http/unexpected.go" ;;
  esac
  if run_static; then fail "boundary was accepted: $kind"; fi
done
for ancestor in internal internal/platform internal/platform/http; do
  new_fixture "ancestor-${ancestor//\//-}"
  mv "$fixture/$ancestor" "$fixture/$ancestor.real"
  ln -s "${ancestor##*/}.real" "$fixture/$ancestor"
  reject_with "p0-s02-static: repository directory must be real: $ancestor"
done
new_fixture missing-contract
rm -f "$fixture/internal/platform/http/contract.go"
reject_with "p0-s02-static: contract.go must be a regular non-symlink file: internal/platform/http/contract.go"

echo "p0-s02-static-contract-tests: PASS"
