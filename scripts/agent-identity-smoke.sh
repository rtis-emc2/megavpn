#!/usr/bin/env bash
set -euo pipefail
api="${MEGAVPN_API_URL:-http://127.0.0.1:8080}"

echo "== health =="
curl -fsS "$api/health" | jq .

echo "== ready =="
curl -fsS "$api/api/v1/ready" | jq .

echo "== nodes =="
curl -fsS "$api/api/v1/nodes" | jq '.[] | {id,name,status,agent_status,last_heartbeat_at}'

echo "== dashboard =="
curl -fsS "$api/api/v1/dashboard" | jq .

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
