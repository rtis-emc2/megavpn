#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/smoke.sh
source "$SCRIPT_DIR/../lib/smoke.sh"

API="${MEGAVPN_API:-http://127.0.0.1:8080}"
NODE_ID="${1:-}"
SERVICE_CODE="${2:-xray-core}"
smoke_require curl
smoke_require jq

if [[ -z "$NODE_ID" ]]; then
  echo "usage: $0 <node-id> [service-code]" >&2
  exit 2
fi

JOB_ID="$(smoke_json_request POST "$API/api/v1/nodes/$NODE_ID/capabilities/install" "$(jq -cn \
  --arg service_code "$SERVICE_CODE" \
  '{service_code:$service_code,strategy:"manual_present",channel:"none"}')" | jq -r .id)"

echo "job: $JOB_ID"
for _ in $(seq 1 45); do
  status="$(smoke_curl "$API/api/v1/jobs/$JOB_ID" | jq -r .status)"
  echo "status: $status"
  [[ "$status" == "succeeded" || "$status" == "failed" || "$status" == "cancelled" ]] && break
  sleep 2
done
smoke_curl "$API/api/v1/jobs/$JOB_ID" | jq .
smoke_curl "$API/api/v1/jobs/$JOB_ID/logs" | jq .
