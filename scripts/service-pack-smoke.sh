#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${MEGAVPN_PUBLIC_BASE_URL:-${MEGAVPN_API:-http://127.0.0.1:8080}}"
AUTH_TOKEN="${MEGAVPN_AUTH_TOKEN:-}"
WAIT_ATTEMPTS="${MEGAVPN_WAIT_ATTEMPTS:-120}"
WAIT_INTERVAL="${MEGAVPN_WAIT_INTERVAL:-2}"
SMOKE_PROVISION="${MEGAVPN_SMOKE_PROVISION:-1}"
SMOKE_SHARE_LINKS="${MEGAVPN_SMOKE_SHARE_LINKS:-0}"
SMOKE_SHARE_LINK_TTL_HOURS="${MEGAVPN_SMOKE_SHARE_LINK_TTL_HOURS:-24}"
CLIENT_EMAIL_DOMAIN="${MEGAVPN_CLIENT_EMAIL_DOMAIN:-example.invalid}"
SCRIPT_PATH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"

usage() {
  cat <<'EOF'
Usage:
  scripts/service-pack-smoke.sh --list
  scripts/service-pack-smoke.sh --matrix <node-id> <endpoint-domain> [certificate-id]
  scripts/service-pack-smoke.sh [--no-provision] [--with-share-links] <node-id> <pack-key> <endpoint-host> [base-name] [certificate-id]

Environment:
  MEGAVPN_PUBLIC_BASE_URL / MEGAVPN_API   API base URL (default: http://127.0.0.1:8080)
  MEGAVPN_AUTH_TOKEN                      Bearer token for authenticated API calls
  MEGAVPN_WAIT_ATTEMPTS                   Poll attempts for jobs (default: 120)
  MEGAVPN_WAIT_INTERVAL                   Poll interval in seconds (default: 2)
  MEGAVPN_SMOKE_PROVISION                 Create client + run client.provision when supported (default: 1)
  MEGAVPN_SMOKE_SHARE_LINKS               Publish share links for ready artifacts (default: 0)
  MEGAVPN_SMOKE_SHARE_LINK_TTL_HOURS      Share-link TTL in hours (default: 24)
  MEGAVPN_CLIENT_EMAIL_DOMAIN             Synthetic email domain for smoke clients

Examples:
  scripts/service-pack-smoke.sh --list
  scripts/service-pack-smoke.sh node-uuid xray_vless_reality vpn.example.com edge-reality
  scripts/service-pack-smoke.sh --with-share-links node-uuid openvpn_tcp_11994 ovpn.example.com
  scripts/service-pack-smoke.sh --matrix node-uuid smoke.example.com cert-uuid
EOF
}

auth_args=()
if [[ -n "$AUTH_TOKEN" ]]; then
  auth_args=(-H "Authorization: Bearer $AUTH_TOKEN")
fi

MODE="single"
MATRIX_NODE_ID=""
MATRIX_ENDPOINT_DOMAIN=""
MATRIX_CERTIFICATE_ID=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --list)
      MODE="list"
      shift
      ;;
    --matrix)
      MODE="matrix"
      MATRIX_NODE_ID="${2:-}"
      MATRIX_ENDPOINT_DOMAIN="${3:-}"
      MATRIX_CERTIFICATE_ID="${4:-}"
      shift 4 || true
      break
      ;;
    --no-provision)
      SMOKE_PROVISION="0"
      shift
      ;;
    --with-share-links)
      SMOKE_SHARE_LINKS="1"
      shift
      ;;
    --no-share-links)
      SMOKE_SHARE_LINKS="0"
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    -*)
      echo "unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
    *)
      break
      ;;
  esac
done

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

pack_requires_certificate() {
  case "$1" in
    xray_nginx_grpc_edge|xray_nginx_http_edge)
      return 0
      ;;
  esac
  return 1
}

is_provisionable_service() {
  case "$1" in
    openvpn|wireguard|mtproto|xray-core|ipsec|shadowsocks|http_proxy)
      return 0
      ;;
  esac
  return 1
}

create_smoke_client() {
  local base_name="$1"
  local client_username
  client_username="${base_name}-client"
  request_json POST "/api/v1/clients" "$(jq -n \
    --arg username "$client_username" \
    --arg display_name "Smoke ${base_name}" \
    --arg email "${client_username}@${CLIENT_EMAIL_DOMAIN}" \
    --arg notes "Generated by scripts/service-pack-smoke.sh" \
    '{username: $username, display_name: $display_name, email: $email, notes: $notes}')" \
    | jq -r '.id'
}

provision_client_for_instances() {
  local client_id="$1"
  shift
  local instance_ids=("$@")
  request_json POST "/api/v1/clients/$client_id/provision" "$(jq -n --argjson ids "$(printf '%s\n' "${instance_ids[@]}" | jq -R . | jq -s .)" '{instance_ids: $ids}')" | jq -r '.id'
}

publish_share_link_for_client() {
  local client_id="$1"
  local artifact_id="$2"
  request_json POST "/api/v1/clients/$client_id/share-links" "$(jq -n --arg target_id "$artifact_id" --argjson ttl_hours "$SMOKE_SHARE_LINK_TTL_HOURS" '{target_id: $target_id, ttl_hours: $ttl_hours}')" | jq -r '.id'
}

run_single_pack() {
  local node_id="$1"
  local pack_key="$2"
  local endpoint_host="$3"
  local base_name="${4:-}"
  local certificate_id="${5:-}"
  local create_resp instance_rows summary_file client_id
  local -a provisionable_instance_ids

  if [[ -z "$base_name" ]]; then
    base_name="smoke-${pack_key//_/-}-$(date +%H%M%S)"
  fi

  echo "base_url: $BASE_URL"
  echo "node_id: $node_id"
  echo "pack_key: $pack_key"
  echo "endpoint_host: $endpoint_host"
  echo "base_name: $base_name"
  echo "smoke_provision: $SMOKE_PROVISION"
  echo "smoke_share_links: $SMOKE_SHARE_LINKS"
  if [[ -n "$certificate_id" ]]; then
    echo "certificate_id: $certificate_id"
  fi

  create_resp="$(request_json POST "/api/v1/service-packs/$pack_key/instances" "$(jq -n \
    --arg node_id "$node_id" \
    --arg base_name "$base_name" \
    --arg endpoint_host "$endpoint_host" \
    --arg certificate_id "$certificate_id" \
    '{
      node_id: $node_id,
      base_name: $base_name,
      endpoint_host: $endpoint_host
    } + (if $certificate_id != "" then {certificate_id: $certificate_id} else {} end)')")"
  echo "$create_resp" | jq .

  instance_rows="$(echo "$create_resp" | jq -c '.created_instances[]')"
  if [[ -z "$instance_rows" ]]; then
    echo "service pack created no instances" >&2
    return 1
  fi

  summary_file="$(mktemp)"
  while IFS= read -r row; do
    local instance_id instance_name service_code job_id
    [[ -n "$row" ]] || continue
    instance_id="$(echo "$row" | jq -r '.id')"
    instance_name="$(echo "$row" | jq -r '.name')"
    service_code="$(echo "$row" | jq -r '.service_code')"
    job_id="$(queue_apply_if_missing "$instance_id")"
    echo "[apply] instance=$instance_name service=$service_code job=$job_id"
    poll_job "$job_id" "$instance_name" >"$summary_file"
    cat "$summary_file" | jq '{id, type, status, result}'
    request_json GET "/api/v1/instances/$instance_id" | jq '{id, name, service_code, status, enabled, endpoint_host, endpoint_port, systemd_unit}'
    if is_provisionable_service "$service_code"; then
      provisionable_instance_ids+=("$instance_id")
    fi
  done <<< "$instance_rows"
  rm -f "$summary_file"

  client_id=""
  if [[ "$SMOKE_PROVISION" == "1" && "${#provisionable_instance_ids[@]}" -gt 0 ]]; then
    local provision_job_id artifacts_json ready_artifact_id share_link_id
    client_id="$(create_smoke_client "$base_name")"
    echo "[client] created client_id=$client_id"
    provision_job_id="$(provision_client_for_instances "$client_id" "${provisionable_instance_ids[@]}")"
    echo "[provision] client_id=$client_id job=$provision_job_id instances=${#provisionable_instance_ids[@]}"
    poll_job "$provision_job_id" "client-provision-$base_name" >/tmp/megavpn-service-pack-provision.json
    cat /tmp/megavpn-service-pack-provision.json | jq '{id, type, status, result}'
    rm -f /tmp/megavpn-service-pack-provision.json

    artifacts_json="$(request_json GET "/api/v1/clients/$client_id/artifacts")"
    echo "$artifacts_json" | jq '{count: length, items: map({id, artifact_type, status, size_bytes, service_access_id})}'
    ready_artifact_id="$(echo "$artifacts_json" | jq -r '[.[] | select(.status == "ready")][0].id // empty')"
    if [[ -z "$ready_artifact_id" ]]; then
      echo "client provisioning succeeded but no ready artifacts were created" >&2
      return 1
    fi

    if [[ "$SMOKE_SHARE_LINKS" == "1" ]]; then
      share_link_id="$(publish_share_link_for_client "$client_id" "$ready_artifact_id")"
      echo "[share-link] client_id=$client_id artifact_id=$ready_artifact_id share_link_id=$share_link_id"
      request_json GET "/api/v1/clients/$client_id/share-links" | jq '{count: length, items: map({id, status, target_type, target_id, expires_at})}'
    fi
  else
    echo "[provision] skipped"
  fi

  echo "service-pack smoke succeeded: $pack_key"
}

run_matrix() {
  local node_id="$1"
  local endpoint_domain="$2"
  local certificate_id="${3:-}"
  local pack_keys pack_key suffix host base_name stamp status summary_tmp
  local ok_count=0
  local failed_count=0
  local skipped_count=0

  if [[ -z "$node_id" || -z "$endpoint_domain" ]]; then
    usage >&2
    exit 1
  fi

  summary_tmp="$(mktemp)"
  pack_keys="$(request_json GET "/api/v1/service-packs" | jq -r '.[].key')"
  while IFS= read -r pack_key; do
    [[ -n "$pack_key" ]] || continue
    if pack_requires_certificate "$pack_key" && [[ -z "$certificate_id" ]]; then
      echo "[$pack_key] skipped: managed certificate id is required for edge TLS smoke" >&2
      printf 'SKIPPED\t%s\t-\t-\n' "$pack_key" >>"$summary_tmp"
      skipped_count=$((skipped_count + 1))
      continue
    fi
    stamp="$(date +%H%M%S)"
    suffix="${pack_key//_/-}"
    base_name="smoke-${suffix}-${stamp}"
    host="${base_name}.${endpoint_domain}"
    if MEGAVPN_SMOKE_PROVISION="$SMOKE_PROVISION" \
      MEGAVPN_SMOKE_SHARE_LINKS="$SMOKE_SHARE_LINKS" \
      MEGAVPN_SMOKE_SHARE_LINK_TTL_HOURS="$SMOKE_SHARE_LINK_TTL_HOURS" \
      MEGAVPN_CLIENT_EMAIL_DOMAIN="$CLIENT_EMAIL_DOMAIN" \
      "$SCRIPT_PATH" "$node_id" "$pack_key" "$host" "$base_name" "$certificate_id"; then
      status="OK"
      ok_count=$((ok_count + 1))
    else
      status="FAILED"
      failed_count=$((failed_count + 1))
    fi
    printf '%s\t%s\t%s\t%s\n' "$status" "$pack_key" "$base_name" "$host" >>"$summary_tmp"
  done <<< "$pack_keys"

  echo
  echo "matrix summary:"
  printf '%-8s %-30s %-40s %s\n' "STATUS" "PACK" "BASE_NAME" "ENDPOINT_HOST"
  while IFS=$'\t' read -r status pack_key base_name host; do
    printf '%-8s %-30s %-40s %s\n' "$status" "$pack_key" "$base_name" "$host"
  done <"$summary_tmp"
  rm -f "$summary_tmp"

  echo
  echo "matrix totals: ok=$ok_count failed=$failed_count skipped=$skipped_count"
  if [[ "$failed_count" -gt 0 ]]; then
    return 1
  fi
}

case "$MODE" in
  list)
    request_json GET "/api/v1/service-packs" | jq -r '.[] | [.key, .label, .description] | @tsv'
    ;;
  matrix)
    run_matrix "$MATRIX_NODE_ID" "$MATRIX_ENDPOINT_DOMAIN" "$MATRIX_CERTIFICATE_ID"
    ;;
  single)
    NODE_ID="${1:-${MEGAVPN_NODE_ID:-}}"
    PACK_KEY="${2:-${MEGAVPN_SERVICE_PACK_KEY:-}}"
    ENDPOINT_HOST="${3:-${MEGAVPN_ENDPOINT_HOST:-}}"
    BASE_NAME="${4:-${MEGAVPN_BASE_NAME:-}}"
    CERTIFICATE_ID="${5:-${MEGAVPN_CERTIFICATE_ID:-}}"
    if [[ -z "$NODE_ID" || -z "$PACK_KEY" || -z "$ENDPOINT_HOST" ]]; then
      usage >&2
      exit 1
    fi
    run_single_pack "$NODE_ID" "$PACK_KEY" "$ENDPOINT_HOST" "$BASE_NAME" "$CERTIFICATE_ID"
    ;;
esac
