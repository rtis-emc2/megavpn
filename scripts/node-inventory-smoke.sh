#!/usr/bin/env bash
set -euo pipefail

API_URL="${MEGAVPN_API_URL:-http://127.0.0.1:8080}"
NODE_ID="${1:-${MEGAVPN_NODE_ID:-}}"

if [[ -z "$NODE_ID" ]]; then
  NODE_ID="$(psql "${MEGAVPN_DATABASE_DSN:-postgres://megavpn:megavpn@127.0.0.1:5432/megavpn?sslmode=disable}" -Atc "select id from nodes where status <> 'retired' order by updated_at desc limit 1" 2>/dev/null || true)"
fi

if [[ -z "$NODE_ID" ]]; then
  echo "node id is required: $0 <node-id>"
  exit 1
fi

echo "queue inventory job for node: $NODE_ID"
curl -fsS -X POST "$API_URL/api/v1/nodes/$NODE_ID/inventory/sync" | jq .

echo "waiting for agent..."
sleep 3

echo "latest jobs:"
curl -fsS "$API_URL/api/v1/jobs?limit=5" | jq .

echo "capabilities:"
curl -fsS "$API_URL/api/v1/nodes/$NODE_ID/capabilities" | jq .

echo "latest inventory snapshot:"
curl -fsS "$API_URL/api/v1/nodes/$NODE_ID/inventory" | jq .
