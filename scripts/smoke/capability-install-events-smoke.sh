#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/smoke.sh
source "$SCRIPT_DIR/../lib/smoke.sh"

API="${MEGAVPN_API:-http://127.0.0.1:8080}"
NODE_ID="${1:-}"
smoke_require curl
smoke_require jq

if [[ -z "$NODE_ID" ]]; then
  echo "usage: $0 <node-id>" >&2
  exit 2
fi

smoke_curl "$API/api/v1/nodes/$NODE_ID/capabilities/install-events?limit=50" | jq .
