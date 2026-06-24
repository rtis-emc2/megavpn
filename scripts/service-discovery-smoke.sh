#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/smoke.sh
source "$SCRIPT_DIR/lib/smoke.sh"

API_URL="${MEGAVPN_API_URL:-http://127.0.0.1:8080}"
NODE_ID="${MEGAVPN_NODE_ID:-}"
smoke_require curl
smoke_require jq

if [[ -z "$NODE_ID" ]]; then
  NODE_ID="$(smoke_curl "$API_URL/api/v1/nodes" | jq -r '.[0].id // empty')"
fi

if [[ -z "$NODE_ID" ]]; then
  echo "no node found; create/enroll a node first" >&2
  exit 1
fi

echo "queue service discovery job for node: $NODE_ID"
JOB_JSON="$(smoke_json_request POST "$API_URL/api/v1/nodes/$NODE_ID/services/discover")"
echo "$JOB_JSON" | jq
JOB_ID="$(echo "$JOB_JSON" | jq -r '.id')"

echo "waiting for agent..."
for i in {1..20}; do
  sleep 2
  status="$(smoke_curl "$API_URL/api/v1/jobs/$JOB_ID" | jq -r '.status')"
  echo "job status: $status"
  if [[ "$status" == "succeeded" || "$status" == "failed" || "$status" == "cancelled" ]]; then
    break
  fi
done

echo "job:"
smoke_curl "$API_URL/api/v1/jobs/$JOB_ID" | jq

echo "discovered services:"
smoke_curl "$API_URL/api/v1/nodes/$NODE_ID/services/discovered" | jq

echo "latest inventory config_files:"
smoke_curl "$API_URL/api/v1/nodes/$NODE_ID/inventory" | jq '.payload.config_files // {}'
