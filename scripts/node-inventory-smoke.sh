#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/smoke.sh
source "$SCRIPT_DIR/lib/smoke.sh"

API_URL="${MEGAVPN_API_URL:-http://127.0.0.1:8080}"
NODE_ID="${1:-${MEGAVPN_NODE_ID:-}}"
smoke_require curl
smoke_require jq

if [[ -z "$NODE_ID" ]]; then
  NODE_ID="$(psql "${MEGAVPN_DATABASE_DSN:-postgres://megavpn:megavpn@127.0.0.1:5432/megavpn?sslmode=disable}" -Atc "select id from nodes where status <> 'retired' order by updated_at desc limit 1" 2>/dev/null || true)"
fi

if [[ -z "$NODE_ID" ]]; then
  echo "node id is required: $0 <node-id>"
  exit 1
fi

echo "queue inventory job for node: $NODE_ID"
smoke_json_request POST "$API_URL/api/v1/nodes/$NODE_ID/inventory/sync" | jq .

echo "waiting for agent..."
sleep 3

echo "latest jobs:"
smoke_curl "$API_URL/api/v1/jobs?limit=5" | jq .

echo "capabilities:"
smoke_curl "$API_URL/api/v1/nodes/$NODE_ID/capabilities" | jq .

echo "latest inventory snapshot:"
smoke_curl "$API_URL/api/v1/nodes/$NODE_ID/inventory" | jq .
