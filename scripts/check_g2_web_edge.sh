#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'g2-web-edge-contract: %s\n' "$1" >&2
  exit 2
}

repository_root=''
case "${1:-}" in
  --root=*) repository_root="${1#--root=}" ;;
  '') repository_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)" ;;
  *) fail 'usage: check_g2_web_edge.sh [--root=<absolute-directory>]' ;;
esac

[[ "$repository_root" = /* && -d "$repository_root" && ! -L "$repository_root" ]] ||
  fail 'repository root must be an absolute regular directory'

service_file="$repository_root/deploy/aicrm-edge.service"
caddy_file="$repository_root/deploy/Caddyfile"
for contract_file in "$service_file" "$caddy_file"; do
  [[ -f "$contract_file" && ! -L "$contract_file" ]] || fail 'edge contract files must be regular files'
done

require_service() {
  grep -Fqx -- "$1" "$service_file" || fail "missing systemd contract: $1"
}

require_caddy() {
  grep -Fqx -- "$1" "$caddy_file" || fail "missing Caddy contract: $1"
}

require_service 'User=aicrm-edge'
require_service 'Group=aicrm-edge'
require_service 'ExecStart=/usr/local/bin/caddy-aicrm run --environ --config /etc/aicrm-edge/Caddyfile --adapter caddyfile'
require_service 'AmbientCapabilities=CAP_NET_BIND_SERVICE'
require_service 'CapabilityBoundingSet=CAP_NET_BIND_SERVICE'
require_service 'NoNewPrivileges=true'
require_service 'ProtectSystem=strict'
require_service 'ProtectHome=true'
require_service 'ReadWritePaths=/var/lib/aicrm-edge'
require_service 'RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6'
require_service 'SystemCallFilter=@system-service'

grep -Eq '^User=(root|0)$' "$service_file" && fail 'root edge service is forbidden'
grep -Eq '^CapabilityBoundingSet=.*CAP_(SYS_ADMIN|DAC_OVERRIDE|NET_ADMIN)' "$service_file" &&
  fail 'broad edge capabilities are forbidden'

require_caddy $'\tadmin off'
require_caddy 'aa.youcangogogo.com {'
require_caddy $'\t@backend path /api/* /healthz'
require_caddy $'\t\treverse_proxy 127.0.0.1:8080'
require_caddy $'\t\ttry_files {path} /index.html'
require_caddy $'\t\tfile_server'
require_caddy $'\t\troot * /opt/aicrm/web/current'
require_caddy $'\t\tContent-Security-Policy "default-src '\''self'\''; base-uri '\''none'\''; connect-src '\''self'\''; font-src '\''self'\''; form-action '\''self'\''; frame-ancestors '\''none'\''; img-src '\''self'\'' data:; object-src '\''none'\''; script-src '\''self'\''; style-src '\''self'\''"'
require_caddy $'\t\tStrict-Transport-Security "max-age=31536000; includeSubDomains"'
require_caddy $'\t\tX-Content-Type-Options "nosniff"'
require_caddy $'\t\tX-Frame-Options "DENY"'

grep -Eq '^[[:space:]]*reverse_proxy[[:space:]]+[^1]' "$caddy_file" &&
  fail 'only the literal loopback backend may be proxied'
grep -Eq '^[[:space:]]*tls[[:space:]]+(internal|off)' "$caddy_file" &&
  fail 'public TLS may not be disabled or replaced by an internal issuer'

printf 'g2-web-edge-contract: PASS\n'
