#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${MEGAVPN_PUBLIC_BASE_URL:-${MEGAVPN_API:-http://127.0.0.1:8080}}"
AUTH_TOKEN="${MEGAVPN_AUTH_TOKEN:-}"
NODE_ID="${1:-${MEGAVPN_NODE_ID:-}}"
PACK_KEY="${2:-${MEGAVPN_SERVICE_PACK_KEY:-}}"
ENDPOINT_HOST="${3:-${MEGAVPN_ENDPOINT_HOST:-}}"
BASE_NAME="${4:-${MEGAVPN_BASE_NAME:-}}"
CERTIFICATE_ID="${5:-${MEGAVPN_CERTIFICATE_ID:-}}"
WAIT_ATTEMPTS="${MEGAVPN_WAIT_ATTEMPTS:-120}"
WAIT_INTERVAL="${MEGAVPN_WAIT_INTERVAL:-2}"

usage() {
  cat <<'EOF'
Usage:
  scripts/service-pack-smoke.sh --list
  scripts/service-pack-smoke.sh <node-id> <pack-key> <endpoint-host> [base-name] [certificate-id]

Environment:
  MEGAVPN_PUBLIC_BASE_URL / MEGAVPN_API   API base URL (default: http://127.0.0.1:8080)
  MEGAVPN_AUTH_TOKEN                      Bearer token for authenticated API calls
  MEGAVPN_NODE_ID                         Default node id
  MEGAVPN_SERVICE_PACK_KEY                Default service pack key
  MEGAVPN_ENDPOINT_HOST                   Default endpoint host
  MEGAVPN_BASE_NAME                       Default base name
  MEGAVPN_CERTIFICATE_ID                  Optional managed certificate id
  MEGAVPN_WAIT_ATTEMPTS                   Poll attempts for jobs (default: 120)
  MEGAVPN_WAIT_INTERVAL                   Poll interval in seconds (default: 2)

Examples:
  scripts/service-pack-smoke.sh --list
  scripts/service-pack-smoke.sh node-uuid xray_vless_reality vpn.example.com edge-reality
  scripts/service-pack-smoke.sh node-uuid xray_nginx_grpc_edge edge.example.com edge-grpc cert-uuid
EOF
}

auth_args=()
if [[ -n "$AUTH_TOKEN" ]]; then
  auth_args=(-H "Authorization: Bearer $AUTH_TOKEN")
fi

request_json() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local tmp status
  tmp="$(mktemp)"
  if [[ -n "$body" ]]; then
    status="$(curl -sS -o "$tmp" -w "%{http_code}" -X "$method" "${auth_args[@]}" -H 'Content-Type: application/json' -d "$body" "$BASE_URL$path")"
  else
    status="$(curl -sS -o "$tmp" -w "%{http_code}" -X "$method" "${auth_args[@]}" "$BASE_URL$path")"
  fi
  if [[ "$status" -lt 200 || "$status" -ge 300 ]]; then
    echo "request failed: $method $path -> HTTP $status" >&2
    cat "$tmp" >&2
    rm -f "$tmp"
    return 1
  fi
  cat "$tmp"
  rm -f "$tmp"
}

poll_job() {
  local job_id="$1"
  local label="$2"
  local attempt status
  for ((attempt=1; attempt<=WAIT_ATTEMPTS; attempt++)); do
    status="$(request_json GET "/api/v1/jobs/$job_id" | jq -r '.status')"
    echo "[$label] job=$job_id attempt=$attempt status=$status"
    case "$status" in
      succeeded)
        request_json GET "/api/v1/jobs/$job_id"
        return 0
        ;;
      failed|cancelled)
        echo "[$label] job failed, dumping job + logs" >&2
        request_json GET "/api/v1/jobs/$job_id" | jq . >&2
        request_json GET "/api/v1/jobs/$job_id/logs" | jq . >&2
        return 1
        ;;
    esac
    sleep "$WAIT_INTERVAL"
  done
  echo "[$label] job timeout after $WAIT_ATTEMPTS attempts" >&2
  request_json GET "/api/v1/jobs/$job_id" | jq . >&2
  return 1
}

first_apply_job_for_instance() {
  local instance_id="$1"
  request_json GET "/api/v1/jobs?limit=500" \
    | jq -r --arg iid "$instance_id" '.[] | select(.instance_id == $iid and .type == "instance.apply") | .id' \
    | head -n1
}

queue_apply_if_missing() {
  local instance_id="$1"
  local job_id
  job_id="$(first_apply_job_for_instance "$instance_id")"
  if [[ -n "$job_id" ]]; then
    printf '%s\n' "$job_id"
    return 0
  fi
  request_json POST "/api/v1/instances/$instance_id/apply" | jq -r '.id'
}

if [[ "${1:-}" == "--list" ]]; then
  request_json GET "/api/v1/service-packs" | jq -r '.[] | [.key, .label, .description] | @tsv'
  exit 0
fi

if [[ -z "$NODE_ID" || -z "$PACK_KEY" || -z "$ENDPOINT_HOST" ]]; then
  usage >&2
  exit 1
fi

if [[ -z "$BASE_NAME" ]]; then
  BASE_NAME="smoke-${PACK_KEY}-$(date +%H%M%S)"
fi

echo "base_url: $BASE_URL"
echo "node_id: $NODE_ID"
echo "pack_key: $PACK_KEY"
echo "endpoint_host: $ENDPOINT_HOST"
echo "base_name: $BASE_NAME"
if [[ -n "$CERTIFICATE_ID" ]]; then
  echo "certificate_id: $CERTIFICATE_ID"
fi

payload="$(jq -n \
  --arg node_id "$NODE_ID" \
  --arg base_name "$BASE_NAME" \
  --arg endpoint_host "$ENDPOINT_HOST" \
  --arg certificate_id "$CERTIFICATE_ID" \
  '{
    node_id: $node_id,
    base_name: $base_name,
    endpoint_host: $endpoint_host
  } + (if $certificate_id != "" then {certificate_id: $certificate_id} else {} end)')"

create_resp="$(request_json POST "/api/v1/service-packs/$PACK_KEY/instances" "$payload")"
echo "$create_resp" | jq .

instance_rows="$(echo "$create_resp" | jq -c '.created_instances[]')"
if [[ -z "$instance_rows" ]]; then
  echo "service pack created no instances" >&2
  exit 1
fi

while IFS= read -r row; do
  [[ -n "$row" ]] || continue
  instance_id="$(echo "$row" | jq -r '.id')"
  instance_name="$(echo "$row" | jq -r '.name')"
  service_code="$(echo "$row" | jq -r '.service_code')"
  job_id="$(queue_apply_if_missing "$instance_id")"
  echo "[apply] instance=$instance_name service=$service_code job=$job_id"
  poll_job "$job_id" "$instance_name" >/tmp/megavpn-service-pack-job.json
  cat /tmp/megavpn-service-pack-job.json | jq '{id, type, status, result}'
  rm -f /tmp/megavpn-service-pack-job.json
  request_json GET "/api/v1/instances/$instance_id" | jq '{id, name, service_code, status, enabled, endpoint_host, endpoint_port, systemd_unit}'
done <<< "$instance_rows"

echo "service-pack smoke succeeded: $PACK_KEY"
