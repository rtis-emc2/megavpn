#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/smoke.sh
source "$SCRIPT_DIR/lib/smoke.sh"

API="${MEGAVPN_API:-http://127.0.0.1:8080}"
NODE_ID="${1:-${MEGAVPN_NODE_ID:-}}"
SERVICE_CODE="${2:-nginx}"
STRATEGY="${3:-}"
CHANNEL="${4:-}"
smoke_require curl
smoke_require jq

if [[ -z "$NODE_ID" ]]; then
  NODE_ID="$(smoke_curl "$API/api/v1/nodes" | jq -r '.[0].id // empty')"
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
JOB_ID="$(smoke_json_request POST "$API/api/v1/nodes/$NODE_ID/capabilities/install" "$(jq -cn \
  --arg service_code "$SERVICE_CODE" \
  --arg strategy "$STRATEGY" \
  --arg channel "$CHANNEL" \
  '{service_code:$service_code,strategy:$strategy,channel:$channel}')" | jq -r '.id')"

echo "job: $JOB_ID"
for i in $(seq 1 120); do
  status="$(smoke_curl "$API/api/v1/jobs/$JOB_ID" | jq -r '.status')"
  echo "status: $status"
  case "$status" in
    succeeded) break ;;
    failed|cancelled)
      smoke_curl "$API/api/v1/jobs/$JOB_ID" | jq .
      smoke_curl "$API/api/v1/jobs/$JOB_ID/logs" | jq .
      exit 1
      ;;
  esac
  sleep 2
done

echo "capabilities:"
smoke_curl "$API/api/v1/nodes/$NODE_ID/capabilities" | jq .

echo "drift:"
smoke_curl "$API/api/v1/nodes/$NODE_ID/capabilities/drift" | jq .
