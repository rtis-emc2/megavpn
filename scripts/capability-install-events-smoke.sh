#!/usr/bin/env bash
set -euo pipefail

API="${MEGAVPN_API:-http://127.0.0.1:8080}"
NODE_ID="${1:-}"

if [[ -z "$NODE_ID" ]]; then
  echo "usage: $0 <node-id>" >&2
  exit 2
fi

curl -fsS "$API/api/v1/nodes/$NODE_ID/capabilities/install-events?limit=50" | jq .
