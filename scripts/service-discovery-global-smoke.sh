#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/smoke.sh
source "$SCRIPT_DIR/lib/smoke.sh"

BASE_URL="${MEGAVPN_BASE_URL:-http://127.0.0.1:8080}"
NODE_ID="${1:-}"
smoke_require curl
smoke_require jq

if [[ -z "$NODE_ID" ]]; then
  NODE_ID="$(smoke_curl "$BASE_URL/api/v1/nodes" | jq -r '.[0].id // empty')"
fi

if [[ -z "$NODE_ID" ]]; then
  echo "no node found" >&2
  exit 1
fi

echo "node: $NODE_ID"

echo "queue service discovery job"
JOB_ID="$(smoke_json_request POST "$BASE_URL/api/v1/nodes/$NODE_ID/services/discover" | jq -r '.id')"
echo "job: $JOB_ID"

echo "waiting for discovery job..."
for i in {1..20}; do
  STATUS="$(smoke_curl "$BASE_URL/api/v1/jobs/$JOB_ID" | jq -r '.status')"
  echo "status: $STATUS"
  if [[ "$STATUS" == "succeeded" || "$STATUS" == "failed" || "$STATUS" == "cancelled" ]]; then
    break
  fi
  sleep 1
done

echo "summary:"
smoke_curl "$BASE_URL/api/v1/nodes/$NODE_ID/services/discovery-summary" | jq

echo "discovered services:"
smoke_curl "$BASE_URL/api/v1/nodes/$NODE_ID/services/discovered" | jq

echo "importable candidates:"
smoke_curl "$BASE_URL/api/v1/nodes/$NODE_ID/services/discovered" | jq '[.[] | select(.status=="available" or .status=="discovered")]'
