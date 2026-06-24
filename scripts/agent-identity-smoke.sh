#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/smoke.sh
source "$SCRIPT_DIR/lib/smoke.sh"
api="${MEGAVPN_API_URL:-http://127.0.0.1:8080}"
smoke_require curl
smoke_require jq

echo "== health =="
smoke_curl "$api/health" | jq .

echo "== ready =="
smoke_curl "$api/api/v1/ready" | jq .

echo "== nodes =="
smoke_curl "$api/api/v1/nodes" | jq '.[] | {id,name,status,agent_status,last_heartbeat_at}'

echo "== dashboard =="
smoke_curl "$api/api/v1/dashboard" | jq .

echo "== local agent state =="
if [[ -f /var/lib/megavpn/agent/state.json ]]; then
  jq 'del(.agent_token)' /var/lib/megavpn/agent/state.json
else
  echo "missing /var/lib/megavpn/agent/state.json"
fi

echo "== bootstrap file =="
if [[ -f /etc/megavpn/agent-bootstrap.env ]]; then
  echo "WARNING: bootstrap file still exists: /etc/megavpn/agent-bootstrap.env"
else
  echo "ok: bootstrap file is absent"
fi
