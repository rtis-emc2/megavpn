#!/usr/bin/env bash
set -euo pipefail

API="${MEGAVPN_API:-http://127.0.0.1:8080}"
NODE_ID="${1:-}"
SERVICE_CODE="${2:-xray-core}"

if [[ -z "$NODE_ID" ]]; then
  echo "usage: $0 <node-id> [service-code]" >&2
  exit 2
fi

JOB_ID="$(curl -fsS -X POST "$API/api/v1/nodes/$NODE_ID/capabilities/install" \
  -H 'Content-Type: application/json' \
  -d '{"service_code":"'"$SERVICE_CODE"'","strategy":"manual_present","channel":"none"}' | jq -r .id)"

echo "job: $JOB_ID"
for _ in $(seq 1 45); do
  status="$(curl -fsS "$API/api/v1/jobs/$JOB_ID" | jq -r .status)"
  echo "status: $status"
  [[ "$status" == "succeeded" || "$status" == "failed" || "$status" == "cancelled" ]] && break
  sleep 2
done
curl -fsS "$API/api/v1/jobs/$JOB_ID" | jq .
curl -fsS "$API/api/v1/jobs/$JOB_ID/logs" | jq .
