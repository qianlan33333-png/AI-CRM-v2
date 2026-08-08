#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(CDPATH= cd -- "$script_dir/../.." && pwd -P)"
[[ -f "$repo_root/go.mod" &&
  -f "$repo_root/internal/api/generated/server.gen.go" ]] || {
  echo "p0-s02-static: invalid repository root: $repo_root" >&2
  exit 1
}
cd "$repo_root"

implementation="internal/platform/http/health.go"
unit_test="internal/platform/http/health_test.go"
for path in "$implementation" "$unit_test"; do
  [[ -f "$path" ]] || {
    echo "p0-s02-static: missing required Slice file: $path" >&2
    exit 1
  }
done

forbidden='"encoding/json"|"net/http"|"net"|"database/sql"|"os"|"os/exec"|"io"|"io/fs"|"path/filepath"|"time"|"log"|"embed"|github\.com/jackc/pgx|github\.com/riverqueue/river|os\.(Getenv|LookupEnv|Open|OpenFile|ReadFile|ReadDir|Stat|Lstat)|time\.|(^|[^[:alnum:]_])go[[:space:]]+|\.ListenAndServe|\b(readiness|ready|database|river|queue|settings|uptime|hostname|version)\b'
if rg -ni "$forbidden" "$implementation"; then
  echo "p0-s02-static: forbidden dependency, side effect, or readiness claim found" >&2
  exit 1
fi

rg -Uq 'type[[:space:]]+HealthHandler[[:space:]]+struct[[:space:]]*\{[[:space:]]*\}' "$implementation" || {
  echo "p0-s02-static: HealthHandler must be stateless" >&2
  exit 1
}
rg -q 'var[[:space:]]+_[[:space:]]+generated\.StrictServerInterface[[:space:]]*=[[:space:]]*\(\*HealthHandler\)\(nil\)' "$implementation" || {
  echo "p0-s02-static: missing strict-interface compile-time assertion" >&2
  exit 1
}

for required in \
  'generated.StrictServerInterface' \
  'generated.GetHealthz200JSONResponse' \
  'generated.Ok'; do
  rg -Fq "$required" "$implementation" || {
    echo "p0-s02-static: missing frozen generated contract reference: $required" >&2
    exit 1
  }
done

rg -Uq 'return[[:space:]]+generated\.GetHealthz200JSONResponse[[:space:]]*\{[[:space:]]*Status:[[:space:]]*generated\.Ok[[:space:]]*\}[[:space:]]*,[[:space:]]*nil' "$implementation" || {
  echo "p0-s02-static: handler must return generated OK response and nil error" >&2
  exit 1
}

GOFLAGS="${GOFLAGS:--mod=readonly}" go test -race ./internal/platform/http

echo "p0-s02-static: PASS"
