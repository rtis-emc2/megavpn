#!/usr/bin/env bash
set -euo pipefail

API="${MEGAVPN_API:-http://127.0.0.1:8080}"
NODE_ID="${1:-${MEGAVPN_NODE_ID:-}}"
SERVICE_CODE="${2:-nginx}"
STRATEGY="${3:-}"
CHANNEL="${4:-}"

if [[ -z "$NODE_ID" ]]; then
  NODE_ID="$(curl -fsS "$API/api/v1/nodes" | jq -r '.[0].id // empty')"
fi
if [[ -z "$NODE_ID" ]]; then
  echo "no node id provided and no nodes found" >&2
  exit 1
fi

case "$SERVICE_CODE" in
  nginx)
    STRATEGY="${STRATEGY:-nginx_org_repo}"
    CHANNEL="${CHANNEL:-stable}"
    ;;
  xray|xray-core)
    SERVICE_CODE="xray-core"
    STRATEGY="${STRATEGY:-xtls_install_release}"
    CHANNEL="${CHANNEL:-latest}"
    ;;
  *)
    echo "unsupported service code for smoke: $SERVICE_CODE" >&2
    exit 1
    ;;
esac

echo "node: $NODE_ID"
echo "install capability: $SERVICE_CODE strategy=$STRATEGY channel=$CHANNEL"
JOB_ID="$(curl -fsS -X POST "$API/api/v1/nodes/$NODE_ID/capabilities/install" \
  -H 'Content-Type: application/json' \
  -d "{\"service_code\":\"$SERVICE_CODE\",\"strategy\":\"$STRATEGY\",\"channel\":\"$CHANNEL\"}" | jq -r '.id')"

echo "job: $JOB_ID"
for i in $(seq 1 120); do
  status="$(curl -fsS "$API/api/v1/jobs/$JOB_ID" | jq -r '.status')"
  echo "status: $status"
  case "$status" in
    succeeded) break ;;
    failed|cancelled)
      curl -fsS "$API/api/v1/jobs/$JOB_ID" | jq .
      curl -fsS "$API/api/v1/jobs/$JOB_ID/logs" | jq .
      exit 1
      ;;
  esac
  sleep 2
done

echo "capabilities:"
curl -fsS "$API/api/v1/nodes/$NODE_ID/capabilities" | jq .

echo "drift:"
curl -fsS "$API/api/v1/nodes/$NODE_ID/capabilities/drift" | jq .
