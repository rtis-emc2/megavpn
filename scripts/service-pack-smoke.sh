#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${MEGAVPN_PUBLIC_BASE_URL:-${MEGAVPN_API:-http://127.0.0.1:8080}}"
AUTH_TOKEN="${MEGAVPN_AUTH_TOKEN:-}"
WAIT_ATTEMPTS="${MEGAVPN_WAIT_ATTEMPTS:-120}"
WAIT_INTERVAL="${MEGAVPN_WAIT_INTERVAL:-2}"
SMOKE_PROVISION="${MEGAVPN_SMOKE_PROVISION:-1}"
SMOKE_SHARE_LINKS="${MEGAVPN_SMOKE_SHARE_LINKS:-0}"
SMOKE_SHARE_LINK_TTL_HOURS="${MEGAVPN_SMOKE_SHARE_LINK_TTL_HOURS:-24}"
SMOKE_RUNTIME_CHECK="${MEGAVPN_SMOKE_RUNTIME_CHECK:-1}"
SMOKE_REQUIRE_AGENT_REPORT="${MEGAVPN_SMOKE_REQUIRE_AGENT_REPORT:-0}"
SMOKE_CLEANUP="${MEGAVPN_SMOKE_CLEANUP:-0}"
SMOKE_CLEANUP_ON_FAILURE="${MEGAVPN_SMOKE_CLEANUP_ON_FAILURE:-0}"
SMOKE_EVIDENCE_DIR="${MEGAVPN_SMOKE_EVIDENCE_DIR:-}"
SMOKE_EVIDENCE_FILE="${MEGAVPN_SMOKE_EVIDENCE_FILE:-}"
MATRIX_SUMMARY_FILE="${MEGAVPN_SMOKE_MATRIX_SUMMARY_FILE:-}"
RUNTIME_WAIT_ATTEMPTS="${MEGAVPN_RUNTIME_WAIT_ATTEMPTS:-$WAIT_ATTEMPTS}"
RUNTIME_WAIT_INTERVAL="${MEGAVPN_RUNTIME_WAIT_INTERVAL:-$WAIT_INTERVAL}"
CLIENT_EMAIL_DOMAIN="${MEGAVPN_CLIENT_EMAIL_DOMAIN:-example.invalid}"
CAMOUFLAGE_PATH="${MEGAVPN_CAMOUFLAGE_PATH:-}"
FALLBACK_UPSTREAM_URL="${MEGAVPN_FALLBACK_UPSTREAM_URL:-}"
FALLBACK_HOST_HEADER="${MEGAVPN_FALLBACK_HOST_HEADER:-}"
FALLBACK_SNI="${MEGAVPN_FALLBACK_SNI:-}"
MATRIX_PACKS="${MEGAVPN_SMOKE_PACKS:-}"
MATRIX_EXCLUDE_PACKS="${MEGAVPN_SMOKE_EXCLUDE_PACKS:-}"
MATRIX_PLAN_ONLY="${MEGAVPN_SMOKE_PLAN_ONLY:-0}"
SCRIPT_PATH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"

usage() {
  cat <<'EOF'
Usage:
  scripts/service-pack-smoke.sh --list
  scripts/service-pack-smoke.sh --matrix <node-id> <endpoint-domain> [certificate-id] [--packs key1,key2] [--exclude key3] [--plan]
  scripts/service-pack-smoke.sh [--no-provision] [--with-share-links] <node-id> <pack-key> <endpoint-host> [base-name] [certificate-id]

Environment:
  MEGAVPN_PUBLIC_BASE_URL / MEGAVPN_API   API base URL (default: http://127.0.0.1:8080)
  MEGAVPN_AUTH_TOKEN                      Bearer token for authenticated API calls
  MEGAVPN_WAIT_ATTEMPTS                   Poll attempts for jobs (default: 120)
  MEGAVPN_WAIT_INTERVAL                   Poll interval in seconds (default: 2)
  MEGAVPN_SMOKE_PROVISION                 Create client + run client.provision when supported (default: 1)
  MEGAVPN_SMOKE_SHARE_LINKS               Publish share links for ready artifacts (default: 0)
  MEGAVPN_SMOKE_SHARE_LINK_TTL_HOURS      Share-link TTL in hours (default: 24)
  MEGAVPN_SMOKE_RUNTIME_CHECK             Wait for runtime_status/health/drift after apply (default: 1)
  MEGAVPN_SMOKE_REQUIRE_AGENT_REPORT      Require agent_reported_at, not only job-derived runtime state (default: 0)
  MEGAVPN_SMOKE_CLEANUP                   Delete smoke client and instances after successful smoke (default: 0)
  MEGAVPN_SMOKE_CLEANUP_ON_FAILURE        Delete partially created smoke resources after failed smoke (default: 0)
  MEGAVPN_SMOKE_EVIDENCE_DIR              Write one JSON evidence file per successful service-pack smoke
  MEGAVPN_SMOKE_EVIDENCE_FILE             Write single-pack JSON evidence to this exact file
  MEGAVPN_SMOKE_MATRIX_SUMMARY_FILE       Write matrix result summary JSON to this exact file
  MEGAVPN_RUNTIME_WAIT_ATTEMPTS           Poll attempts for runtime-state convergence (default: MEGAVPN_WAIT_ATTEMPTS)
  MEGAVPN_RUNTIME_WAIT_INTERVAL           Runtime-state poll interval in seconds (default: MEGAVPN_WAIT_INTERVAL)
  MEGAVPN_CLIENT_EMAIL_DOMAIN             Synthetic email domain for smoke clients
  MEGAVPN_CAMOUFLAGE_PATH                 Hidden VLESS path for configurable camouflage packs
  MEGAVPN_FALLBACK_UPSTREAM_URL           Required fallback website URL for Nginx camouflage packs
  MEGAVPN_FALLBACK_HOST_HEADER            Optional fallback Host header override
  MEGAVPN_FALLBACK_SNI                    Optional fallback HTTPS SNI override
  MEGAVPN_SMOKE_PACKS                     Comma/space-separated service pack keys to include in matrix
  MEGAVPN_SMOKE_EXCLUDE_PACKS             Comma/space-separated service pack keys to skip in matrix
  MEGAVPN_SMOKE_PLAN_ONLY                 Print matrix plan and exit without creating instances (default: 0)

Examples:
  scripts/service-pack-smoke.sh --list
  scripts/service-pack-smoke.sh node-uuid xray_vless_reality vpn.example.com edge-reality
  MEGAVPN_FALLBACK_UPSTREAM_URL=https://target.example.com scripts/service-pack-smoke.sh node-uuid xray_nginx_http_edge enter.example.com edge-camouflage cert-uuid
  scripts/service-pack-smoke.sh --with-share-links node-uuid openvpn_tcp_11994 ovpn.example.com
  MEGAVPN_SMOKE_CLEANUP=1 scripts/service-pack-smoke.sh node-uuid openvpn_tcp_11994 ovpn.example.com
  MEGAVPN_SMOKE_CLEANUP_ON_FAILURE=1 scripts/service-pack-smoke.sh node-uuid openvpn_tcp_11994 ovpn.example.com
  scripts/service-pack-smoke.sh --matrix node-uuid smoke.example.com cert-uuid --packs openvpn_tcp_11994,openvpn_udp_1194,wireguard_roadwarrior
  scripts/service-pack-smoke.sh --matrix node-uuid smoke.example.com --packs xray_vless_reality --plan
EOF
}

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
      if [[ -n "${4:-}" && "${4:-}" != -* ]]; then
        MATRIX_CERTIFICATE_ID="$4"
        shift 4
      else
        shift 3
      fi
      ;;
    --packs)
      if [[ -z "${2:-}" || "${2:-}" == -* ]]; then
        echo "--packs requires a comma-separated service pack key list" >&2
        usage >&2
        exit 1
      fi
      MATRIX_PACKS="${2:-}"
      shift 2
      ;;
    --exclude)
      if [[ -z "${2:-}" || "${2:-}" == -* ]]; then
        echo "--exclude requires a comma-separated service pack key list" >&2
        usage >&2
        exit 1
      fi
      MATRIX_EXCLUDE_PACKS="${2:-}"
      shift 2
      ;;
    --plan|--dry-run)
      MATRIX_PLAN_ONLY="1"
      shift
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
  local -a curl_args
  tmp="$(mktemp)"
  curl_args=(-sS -o "$tmp" -w "%{http_code}" -X "$method")
  if [[ -n "$AUTH_TOKEN" ]]; then
    curl_args+=(-H "Authorization: Bearer $AUTH_TOKEN")
  fi
  if [[ -n "$body" ]]; then
    curl_args+=(-H 'Content-Type: application/json' -d "$body")
  fi
  curl_args+=("$BASE_URL$path")
  if ! status="$(curl "${curl_args[@]}")"; then
    echo "request failed: $method $path -> curl transport error" >&2
    cat "$tmp" >&2 || true
    rm -f "$tmp"
    return 1
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
    echo "[$label] job=$job_id attempt=$attempt status=$status" >&2
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

dump_runtime_diagnostics() {
  local instance_id="$1"
  echo "[runtime] state diagnostic for instance=$instance_id" >&2
  request_json GET "/api/v1/instances/$instance_id/runtime-state" \
    | jq '{instance_id, service_code, systemd_unit, runtime_status, health_status, drift_status, active_state, enabled_state, last_job_type, last_job_status, agent_reported_at, health_reasons, drift_reasons, error_text}' >&2 || true
  echo "[runtime] latest observations for instance=$instance_id" >&2
  request_json GET "/api/v1/instances/$instance_id/runtime-observations?limit=5" \
    | jq 'map({source, service_code, systemd_unit, runtime_status, health_status, drift_status, active_state, enabled_state, agent_reported_at, checked_at, health_reasons, drift_reasons, error_text})' >&2 || true
}

runtime_state_ready() {
  local state_json="$1"
  local runtime health drift agent_reported_at
  runtime="$(echo "$state_json" | jq -r '.runtime_status // ""')"
  health="$(echo "$state_json" | jq -r '.health_status // ""')"
  drift="$(echo "$state_json" | jq -r '.drift_status // ""')"
  agent_reported_at="$(echo "$state_json" | jq -r '.agent_reported_at // ""')"
  if [[ "$runtime" != "active" || "$health" != "healthy" || "$drift" != "in_sync" ]]; then
    return 1
  fi
  if [[ "$SMOKE_REQUIRE_AGENT_REPORT" == "1" && -z "$agent_reported_at" ]]; then
    return 1
  fi
  return 0
}

runtime_state_failed() {
  local state_json="$1"
  local runtime health drift last_job_status
  runtime="$(echo "$state_json" | jq -r '.runtime_status // ""')"
  health="$(echo "$state_json" | jq -r '.health_status // ""')"
  drift="$(echo "$state_json" | jq -r '.drift_status // ""')"
  last_job_status="$(echo "$state_json" | jq -r '.last_job_status // ""')"
  [[ "$runtime" == "failed" || "$health" == "unhealthy" || "$drift" == "drifted" || "$last_job_status" == "failed" ]]
}

wait_runtime_state() {
  local instance_id="$1"
  local label="$2"
  local attempt state_json runtime health drift agent_reported_at
  for ((attempt=1; attempt<=RUNTIME_WAIT_ATTEMPTS; attempt++)); do
    if state_json="$(request_json GET "/api/v1/instances/$instance_id/runtime-state" 2>/dev/null)"; then
      runtime="$(echo "$state_json" | jq -r '.runtime_status // "unknown"')"
      health="$(echo "$state_json" | jq -r '.health_status // "unknown"')"
      drift="$(echo "$state_json" | jq -r '.drift_status // "unknown"')"
      agent_reported_at="$(echo "$state_json" | jq -r '.agent_reported_at // ""')"
      echo "[runtime] label=$label attempt=$attempt runtime=$runtime health=$health drift=$drift agent_reported_at=${agent_reported_at:-none}" >&2
      if runtime_state_ready "$state_json"; then
        echo "$state_json" | jq '{instance_id, service_code, systemd_unit, runtime_status, health_status, drift_status, active_state, enabled_state, agent_reported_at, checked_at}'
        return 0
      fi
      if runtime_state_failed "$state_json"; then
        echo "[runtime] $label reached failed runtime projection" >&2
        dump_runtime_diagnostics "$instance_id"
        return 1
      fi
    else
      echo "[runtime] label=$label attempt=$attempt runtime-state=missing" >&2
    fi
    sleep "$RUNTIME_WAIT_INTERVAL"
  done
  echo "[runtime] $label did not converge after $RUNTIME_WAIT_ATTEMPTS attempts" >&2
  dump_runtime_diagnostics "$instance_id"
  return 1
}

first_apply_job_for_instance() {
  local instance_id="$1"
  request_json GET "/api/v1/jobs?limit=500" \
    | jq -r --arg iid "$instance_id" '.[] | select(.instance_id == $iid and .type == "instance.apply") | .id' \
    | head -n1
}

first_delete_job_for_instance() {
  local instance_id="$1"
  request_json GET "/api/v1/jobs?limit=500" \
    | jq -r --arg iid "$instance_id" '.[] | select(.instance_id == $iid and .type == "instance.delete") | .id' \
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

smoke_truthy() {
  local value
  value="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  case "$value" in
    1|true|yes|on)
      return 0
      ;;
  esac
  return 1
}

cleanup_smoke_resources() {
  local client_id="$1"
  shift
  local instance_ids=("$@")
  local idx instance_id delete_job_id failed=0

  echo "[cleanup] started client_id=${client_id:-none} instances=${#instance_ids[@]}"
  if [[ -n "$client_id" ]]; then
    if request_json DELETE "/api/v1/clients/$client_id" \
      | jq '{client_id, username, deleted, service_accesses_deleted, access_routes_deleted, email_deliveries_deleted, secret_refs_deleted, config_cleanup}'; then
      echo "[cleanup] client deleted client_id=$client_id"
    else
      echo "[cleanup] client delete failed client_id=$client_id" >&2
      failed=1
    fi
  fi

  for ((idx=${#instance_ids[@]}-1; idx>=0; idx--)); do
    instance_id="${instance_ids[$idx]}"
    [[ -n "$instance_id" ]] || continue
    if request_json DELETE "/api/v1/instances/$instance_id" \
      | jq '{id, name, service_code, status, enabled, systemd_unit}'; then
      echo "[cleanup] instance delete queued instance=$instance_id"
    else
      echo "[cleanup] instance delete request failed instance=$instance_id" >&2
      failed=1
      continue
    fi
    delete_job_id="$(first_delete_job_for_instance "$instance_id")"
    if [[ -z "$delete_job_id" ]]; then
      echo "[cleanup] no instance.delete job found instance=$instance_id"
      continue
    fi
    if ! poll_job "$delete_job_id" "instance-delete-$instance_id" | jq '{id, type, status, result}'; then
      failed=1
    fi
  done

  if [[ "$failed" -ne 0 ]]; then
    echo "[cleanup] failed; inspect jobs before reusing the node" >&2
    return 1
  fi
  echo "[cleanup] completed"
}

cleanup_failed_single_pack() {
  local status=$?
  trap - ERR
  if [[ "${single_pack_failed:-1}" != "0" ]] && smoke_truthy "$SMOKE_CLEANUP_ON_FAILURE"; then
    if [[ -n "${client_id:-}" || "${#created_instance_ids[@]}" -gt 0 ]]; then
      echo "[cleanup] smoke failed; MEGAVPN_SMOKE_CLEANUP_ON_FAILURE=1, removing partial resources" >&2
      if [[ "${#created_instance_ids[@]}" -gt 0 ]]; then
        cleanup_smoke_resources "${client_id:-}" "${created_instance_ids[@]}" || true
      else
        cleanup_smoke_resources "${client_id:-}" || true
      fi
    else
      echo "[cleanup] smoke failed before resources were created" >&2
    fi
  fi
  return "$status"
}

resolve_evidence_file() {
  local base_name="$1"
  if [[ -n "$SMOKE_EVIDENCE_FILE" ]]; then
    printf '%s\n' "$SMOKE_EVIDENCE_FILE"
    return 0
  fi
  if [[ -n "$SMOKE_EVIDENCE_DIR" ]]; then
    mkdir -p "$SMOKE_EVIDENCE_DIR"
    printf '%s/%s.json\n' "${SMOKE_EVIDENCE_DIR%/}" "$base_name"
    return 0
  fi
  return 1
}

resolve_matrix_summary_file() {
  if [[ -n "$MATRIX_SUMMARY_FILE" ]]; then
    mkdir -p "$(dirname "$MATRIX_SUMMARY_FILE")"
    printf '%s\n' "$MATRIX_SUMMARY_FILE"
    return 0
  fi
  if [[ -n "$SMOKE_EVIDENCE_DIR" ]]; then
    mkdir -p "$SMOKE_EVIDENCE_DIR"
    printf '%s/_matrix-summary.json\n' "${SMOKE_EVIDENCE_DIR%/}"
    return 0
  fi
  return 1
}

matrix_pack_evidence_path() {
  local base_name="$1"
  if [[ -n "$SMOKE_EVIDENCE_DIR" && -n "$base_name" && "$base_name" != "-" ]]; then
    printf '%s/%s.json\n' "${SMOKE_EVIDENCE_DIR%/}" "$base_name"
    return 0
  fi
  printf '%s\n' '-'
}

write_matrix_summary() {
  local matrix_summary_file="$1"
  local summary_file="$2"
  local node_id="$3"
  local endpoint_domain="$4"
  local certificate_id="$5"
  local ok_count="$6"
  local failed_count="$7"
  local skipped_count="$8"
  local selected_count="$9"
  local generated_at

  generated_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  mkdir -p "$(dirname "$matrix_summary_file")"
  jq -Rn \
    --arg generated_at "$generated_at" \
    --arg base_url "$BASE_URL" \
    --arg node_id "$node_id" \
    --arg endpoint_domain "$endpoint_domain" \
    --arg certificate_id "$certificate_id" \
    --arg matrix_packs "$MATRIX_PACKS" \
    --arg matrix_exclude_packs "$MATRIX_EXCLUDE_PACKS" \
    --arg fallback_upstream_url "$FALLBACK_UPSTREAM_URL" \
    --arg smoke_provision "$SMOKE_PROVISION" \
    --arg smoke_runtime_check "$SMOKE_RUNTIME_CHECK" \
    --arg smoke_require_agent_report "$SMOKE_REQUIRE_AGENT_REPORT" \
    --arg smoke_cleanup "$SMOKE_CLEANUP" \
    --arg smoke_cleanup_on_failure "$SMOKE_CLEANUP_ON_FAILURE" \
    --arg smoke_evidence_dir "$SMOKE_EVIDENCE_DIR" \
    --argjson ok_count "$ok_count" \
    --argjson failed_count "$failed_count" \
    --argjson skipped_count "$skipped_count" \
    --argjson selected_count "$selected_count" \
    '
    [inputs
      | split("\t")
      | {
          status: .[0],
          pack_key: .[1],
          base_name: (if .[2] == "-" then null else .[2] end),
          endpoint_host: (if .[3] == "-" then null else .[3] end),
          evidence_file: (if .[4] == "-" then null else .[4] end),
          reason: (if .[5] == "-" then null else .[5] end)
        }
    ] as $results |
    {
      status: (if $failed_count > 0 then "failed" elif $selected_count == 0 then "empty" else "succeeded" end),
      generated_at: $generated_at,
      input: {
        base_url: $base_url,
        node_id: $node_id,
        endpoint_domain: $endpoint_domain,
        certificate_id: (if $certificate_id == "" then null else $certificate_id end),
        matrix_packs: $matrix_packs,
        matrix_exclude_packs: $matrix_exclude_packs,
        fallback_upstream_url: (if $fallback_upstream_url == "" then null else $fallback_upstream_url end),
        smoke_provision: $smoke_provision,
        smoke_runtime_check: $smoke_runtime_check,
        smoke_require_agent_report: $smoke_require_agent_report,
        smoke_cleanup: $smoke_cleanup,
        smoke_cleanup_on_failure: $smoke_cleanup_on_failure,
        smoke_evidence_dir: (if $smoke_evidence_dir == "" then null else $smoke_evidence_dir end)
      },
      totals: {
        ok: $ok_count,
        failed: $failed_count,
        skipped: $skipped_count,
        selected: $selected_count
      },
      results: $results
    }' <"$summary_file" >"$matrix_summary_file"
  echo "[evidence] wrote matrix summary $matrix_summary_file"
}

write_smoke_evidence() {
  local evidence_file="$1"
  local node_id="$2"
  local pack_key="$3"
  local endpoint_host="$4"
  local base_name="$5"
  local client_id="$6"
  local create_file="$7"
  local instance_evidence_file="$8"
  local runtime_evidence_file="$9"
  local provision_file="${10}"
  local accesses_file="${11}"
  local artifacts_file="${12}"
  local generated_at

  [[ -n "$evidence_file" ]] || return 0
  generated_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  mkdir -p "$(dirname "$evidence_file")"
  jq -n \
    --arg generated_at "$generated_at" \
    --arg base_url "$BASE_URL" \
    --arg node_id "$node_id" \
    --arg pack_key "$pack_key" \
    --arg endpoint_host "$endpoint_host" \
    --arg base_name "$base_name" \
    --arg client_id "$client_id" \
    --slurpfile create_doc "$create_file" \
    --slurpfile applied_instances "$instance_evidence_file" \
    --slurpfile runtime_states "$runtime_evidence_file" \
    --slurpfile provision_doc "$provision_file" \
    --slurpfile access_doc "$accesses_file" \
    --slurpfile artifacts_doc "$artifacts_file" \
    --arg smoke_provision "$SMOKE_PROVISION" \
    --arg smoke_runtime_check "$SMOKE_RUNTIME_CHECK" \
    --arg smoke_require_agent_report "$SMOKE_REQUIRE_AGENT_REPORT" \
    --arg smoke_cleanup "$SMOKE_CLEANUP" \
    --arg smoke_cleanup_on_failure "$SMOKE_CLEANUP_ON_FAILURE" \
    '
    ($create_doc[0] // {}) as $create |
    ($provision_doc[0] // {}) as $provision |
    ($access_doc[0] // []) as $accesses |
    ($artifacts_doc[0] // []) as $artifacts |
    {
      status: "succeeded",
      generated_at: $generated_at,
      input: {
        base_url: $base_url,
        node_id: $node_id,
        pack_key: $pack_key,
        endpoint_host: $endpoint_host,
        base_name: $base_name,
        smoke_provision: $smoke_provision,
        smoke_runtime_check: $smoke_runtime_check,
        smoke_require_agent_report: $smoke_require_agent_report,
        smoke_cleanup: $smoke_cleanup,
        smoke_cleanup_on_failure: $smoke_cleanup_on_failure
      },
      created_instances: ($create.created_instances // []),
      runtime_install_jobs: ($create.runtime_install_jobs // []),
      applied_instances: $applied_instances,
      runtime_states: $runtime_states,
      client: (if $client_id == "" then null else {id: $client_id} end),
      provision_result: (if $provision == {} then null else $provision end),
      service_accesses: $accesses,
      artifacts: $artifacts
    }' >"$evidence_file"
  echo "[evidence] wrote $evidence_file"
}

pack_requires_certificate() {
  case "$1" in
    xray_nginx_grpc_edge|xray_nginx_http_edge)
      return 0
      ;;
  esac
  return 1
}

pack_requires_fallback() {
  case "$1" in
    xray_nginx_grpc_edge|xray_nginx_http_edge)
      return 0
      ;;
  esac
  return 1
}

smoke_host_from_value() {
  local value="$1"
  value="${value#*://}"
  value="${value%%/*}"
  value="${value#*@}"
  value="${value#[}"
  value="${value%]}"
  value="${value%%:*}"
  value="${value%.}"
  printf '%s' "$value" | tr '[:upper:]' '[:lower:]'
}

guard_camouflage_fallback_loop() {
  local endpoint_host="$1"
  local endpoint fallback header sni
  endpoint="$(smoke_host_from_value "$endpoint_host")"
  fallback="$(smoke_host_from_value "$FALLBACK_UPSTREAM_URL")"
  header="$(smoke_host_from_value "$FALLBACK_HOST_HEADER")"
  sni="$(smoke_host_from_value "$FALLBACK_SNI")"
  if [[ -n "$endpoint" && -n "$fallback" && "$fallback" == "$endpoint" ]]; then
    echo "fallback_upstream_url must not point back to endpoint_host; choose a separate fallback website" >&2
    return 1
  fi
  if [[ -n "$endpoint" && -n "$header" && "$header" == "$endpoint" ]]; then
    echo "fallback_host_header must not point back to endpoint_host; choose a separate fallback website" >&2
    return 1
  fi
  if [[ -n "$endpoint" && -n "$sni" && "$sni" == "$endpoint" ]]; then
    echo "fallback_sni must not point back to endpoint_host; choose a separate fallback website" >&2
    return 1
  fi
}

pack_list_contains() {
  local list="$1"
  local needle="$2"
  local item
  list="${list//,/ }"
  for item in $list; do
    if [[ "$item" == "$needle" ]]; then
      return 0
    fi
  done
  return 1
}

matrix_pack_selected() {
  local pack_key="$1"
  if [[ -n "$MATRIX_PACKS" ]] && ! pack_list_contains "$MATRIX_PACKS" "$pack_key"; then
    return 1
  fi
  if [[ -n "$MATRIX_EXCLUDE_PACKS" ]] && pack_list_contains "$MATRIX_EXCLUDE_PACKS" "$pack_key"; then
    return 1
  fi
  return 0
}

validate_matrix_pack_filters() {
  local available="$1"
  local item fail=0
  for item in ${MATRIX_PACKS//,/ }; do
    [[ -n "$item" ]] || continue
    if ! pack_list_contains "$available" "$item"; then
      echo "unknown service pack in --packs/MEGAVPN_SMOKE_PACKS: $item" >&2
      fail=1
    fi
  done
  for item in ${MATRIX_EXCLUDE_PACKS//,/ }; do
    [[ -n "$item" ]] || continue
    if ! pack_list_contains "$available" "$item"; then
      echo "unknown service pack in --exclude/MEGAVPN_SMOKE_EXCLUDE_PACKS: $item" >&2
      fail=1
    fi
  done
  [[ "$fail" -eq 0 ]]
}

matrix_skip_reason() {
  local pack_key="$1"
  local certificate_id="$2"
  if ! matrix_pack_selected "$pack_key"; then
    echo "not selected by matrix pack filter"
    return 0
  fi
  if pack_requires_certificate "$pack_key" && [[ -z "$certificate_id" ]]; then
    echo "managed certificate id is required for edge TLS smoke"
    return 0
  fi
  if pack_requires_fallback "$pack_key" && [[ -z "$FALLBACK_UPSTREAM_URL" ]]; then
    echo "MEGAVPN_FALLBACK_UPSTREAM_URL is required for traffic camouflage smoke"
    return 0
  fi
  return 1
}

print_matrix_plan() {
  local packs_json="$1"
  local endpoint_domain="$2"
  local certificate_id="$3"
  local pack_keys="$4"
  local pack_key suffix host components skip_reason ports_tmp selected_count runnable_count
  selected_count=0
  runnable_count=0
  ports_tmp="$(mktemp)"

  echo "matrix plan:"
  echo "base_url: $BASE_URL"
  echo "endpoint_domain: $endpoint_domain"
  echo "certificate_id: ${certificate_id:-none}"
  echo "fallback_upstream_url: ${FALLBACK_UPSTREAM_URL:-none}"
  echo "smoke_runtime_check: $SMOKE_RUNTIME_CHECK"
  echo "smoke_require_agent_report: $SMOKE_REQUIRE_AGENT_REPORT"
  echo "smoke_cleanup: $SMOKE_CLEANUP"
  echo "smoke_cleanup_on_failure: $SMOKE_CLEANUP_ON_FAILURE"
  echo "smoke_evidence_dir: ${SMOKE_EVIDENCE_DIR:-none}"
  echo "matrix_summary_file: ${MATRIX_SUMMARY_FILE:-auto}"
  if [[ -n "$MATRIX_PACKS" ]]; then
    echo "matrix include packs: $MATRIX_PACKS"
  fi
  if [[ -n "$MATRIX_EXCLUDE_PACKS" ]]; then
    echo "matrix exclude packs: $MATRIX_EXCLUDE_PACKS"
  fi
  echo
  printf '%-8s %-30s %-45s %s\n' "ACTION" "PACK" "ENDPOINT_HOST" "COMPONENTS"
  while IFS= read -r pack_key; do
    [[ -n "$pack_key" ]] || continue
    suffix="${pack_key//_/-}"
    host="smoke-${suffix}-<timestamp>.${endpoint_domain}"
    components="$(echo "$packs_json" | jq -r --arg key "$pack_key" '
      [.[] | select(.key == $key) | .components[]? |
        "\(.service_code):\(.endpoint_port // "n/a")"] | join(",")
    ')"
    if skip_reason="$(matrix_skip_reason "$pack_key" "$certificate_id")"; then
      printf '%-8s %-30s %-45s %s\n' "SKIP" "$pack_key" "-" "$skip_reason"
      continue
    fi
    selected_count=$((selected_count + 1))
    runnable_count=$((runnable_count + 1))
    printf '%-8s %-30s %-45s %s\n' "RUN" "$pack_key" "$host" "$components"
    echo "$packs_json" | jq -r --arg key "$pack_key" '
      .[] | select(.key == $key) | .components[]? |
      select(.endpoint_port != null) |
      [.endpoint_port, .service_code, $key] | @tsv
    ' >>"$ports_tmp"
  done <<< "$pack_keys"

  echo
  if [[ -s "$ports_tmp" ]]; then
    local duplicate_ports
    duplicate_ports="$(cut -f1 "$ports_tmp" | sort -n | uniq -d)"
    if [[ -n "$duplicate_ports" ]]; then
      echo "port overlap warning:"
      while IFS= read -r port; do
        [[ -n "$port" ]] || continue
        awk -F '\t' -v p="$port" '$1 == p {print "  port " $1 " -> " $2 " in " $3}' "$ports_tmp"
      done <<< "$duplicate_ports"
      echo "Review whether those packs should run in separate batches on this node."
      echo
    fi
  fi
  rm -f "$ports_tmp"

  echo "matrix plan totals: runnable=$runnable_count"
  if [[ "$selected_count" -eq 0 ]]; then
    echo "matrix selected no runnable service packs; check filters, certificate and fallback settings" >&2
    return 1
  fi
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

json_array_from_args() {
  printf '%s\n' "$@" | jq -R 'select(length > 0)' | jq -s .
}

wait_post_provision_apply_jobs() {
  local provision_json="$1"
  local runtime_evidence_file="${2:-}"
  local job_rows row job_id instance_id service_code label runtime_json
  job_rows="$(echo "$provision_json" | jq -c '.result.instance_apply_jobs[]?')"
  if [[ -z "$job_rows" ]]; then
    echo "[provision-apply] no instance apply jobs queued by client provisioning"
    return 0
  fi
  while IFS= read -r row; do
    [[ -n "$row" ]] || continue
    job_id="$(echo "$row" | jq -r '.job_id // .id // empty')"
    instance_id="$(echo "$row" | jq -r '.instance_id // empty')"
    service_code="$(echo "$row" | jq -r '.service_code // "service"')"
    if [[ -z "$job_id" ]]; then
      echo "[provision-apply] provision result entry is missing job_id: $row" >&2
      return 1
    fi
    label="post-provision-${service_code}-${instance_id:-unknown}"
    echo "[provision-apply] instance=${instance_id:-unknown} service=$service_code job=$job_id"
    poll_job "$job_id" "$label" | jq '{id, type, status, result}'
    if [[ "$SMOKE_RUNTIME_CHECK" == "1" && -n "$instance_id" ]]; then
      runtime_json="$(wait_runtime_state "$instance_id" "$label")"
      echo "$runtime_json"
      if [[ -n "$runtime_evidence_file" ]]; then
        echo "$runtime_json" >>"$runtime_evidence_file"
      fi
    fi
  done <<< "$job_rows"
}

verify_client_provisioning_outputs() {
  local client_id="$1"
  local artifacts_json="$2"
  local accesses_evidence_file="$3"
  shift 3
  local expected_json accesses_json filtered_accesses_json access_report access_ids_json artifact_report
  expected_json="$(json_array_from_args "$@")"
  accesses_json="$(request_json GET "/api/v1/clients/$client_id/accesses")"
  filtered_accesses_json="$(echo "$accesses_json" | jq -c --argjson expected "$expected_json" '
    [.[] | select(.instance_id as $iid | $expected | index($iid)) | {id, instance_id, status, provision_mode}]
  ')"
  if [[ -n "$accesses_evidence_file" ]]; then
    printf '%s\n' "$filtered_accesses_json" >"$accesses_evidence_file"
  fi
  jq -n --argjson expected "$expected_json" --argjson items "$filtered_accesses_json" '{
    count: ($items | length),
    expected_instances: $expected,
    items: $items
  }'

  access_report="$(echo "$accesses_json" | jq -c --argjson expected "$expected_json" '
    def expected_accesses: [.[] | select(.instance_id as $iid | $expected | index($iid))];
    {
      missing_instance_ids: [
        $expected[] as $iid
        | select((expected_accesses | map(select(.instance_id == $iid)) | length) == 0)
        | $iid
      ],
      bad_accesses: [
        expected_accesses[]
        | select((.status // "") != "active")
        | {id, instance_id, status}
      ]
    }
  ')"
  if ! echo "$access_report" | jq -e '(.missing_instance_ids | length) == 0 and (.bad_accesses | length) == 0' >/dev/null; then
    echo "client provisioning did not activate every selected service access" >&2
    echo "$access_report" | jq . >&2
    return 1
  fi

  access_ids_json="$(echo "$accesses_json" | jq -c --argjson expected "$expected_json" '[.[] | select(.instance_id as $iid | $expected | index($iid)) | .id]')"
  artifact_report="$(echo "$artifacts_json" | jq -c --argjson access_ids "$access_ids_json" '
    {
      missing_ready_artifact_access_ids: [
        $access_ids[] as $aid
        | select(([.[] | select(.service_access_id == $aid and .status == "ready" and (.size_bytes // 0) > 0)] | length) == 0)
        | $aid
      ],
      ready_artifacts: [
        .[] | select(.service_access_id as $aid | $access_ids | index($aid))
        | select(.status == "ready")
        | {id, artifact_type, service_access_id, size_bytes}
      ]
    }
  ')"
  if ! echo "$artifact_report" | jq -e '(.missing_ready_artifact_access_ids | length) == 0' >/dev/null; then
    echo "client provisioning did not create a ready artifact for every selected service access" >&2
    echo "$artifact_report" | jq . >&2
    return 1
  fi
  echo "$artifact_report" | jq '{ready_artifacts}'
}

publish_share_link_for_client() {
  local client_id="$1"
  local artifact_id="$2"
  request_json POST "/api/v1/clients/$client_id/share-links" "$(jq -n --arg target_id "$artifact_id" --argjson ttl_hours "$SMOKE_SHARE_LINK_TTL_HOURS" '{target_id: $target_id, ttl_hours: $ttl_hours}')"
}

download_share_link_once() {
  local token="$1"
  local tmp
  tmp="$(mktemp)"
  curl -fsS -o "$tmp" "${BASE_URL%/}/share/$token"
  test -s "$tmp"
  rm -f "$tmp"
}

run_single_pack() {
  local node_id="$1"
  local pack_key="$2"
  local endpoint_host="$3"
  local base_name="${4:-}"
  local certificate_id="${5:-}"
  local create_resp instance_rows install_rows summary_file client_id=""
  local evidence_file="" create_evidence_file instance_evidence_file runtime_evidence_file provision_evidence_file accesses_evidence_file artifacts_evidence_file
  local provision_result_json="{}" artifacts_json="[]"
  local single_pack_failed=1
  local -a provisionable_instance_ids created_instance_ids
  trap cleanup_failed_single_pack ERR

  if [[ -z "$base_name" ]]; then
    base_name="smoke-${pack_key//_/-}-$(date +%H%M%S)"
  fi
  instance_evidence_file="$(mktemp)"
  runtime_evidence_file="$(mktemp)"
  create_evidence_file="$(mktemp)"
  provision_evidence_file="$(mktemp)"
  accesses_evidence_file="$(mktemp)"
  artifacts_evidence_file="$(mktemp)"
  printf '%s\n' '{}' >"$create_evidence_file"
  printf '%s\n' '{}' >"$provision_evidence_file"
  printf '%s\n' '[]' >"$accesses_evidence_file"
  printf '%s\n' '[]' >"$artifacts_evidence_file"
  if evidence_file="$(resolve_evidence_file "$base_name")"; then
    echo "smoke_evidence_file: $evidence_file"
  else
    evidence_file=""
  fi

  echo "base_url: $BASE_URL"
  echo "node_id: $node_id"
  echo "pack_key: $pack_key"
  echo "endpoint_host: $endpoint_host"
  echo "base_name: $base_name"
  echo "smoke_provision: $SMOKE_PROVISION"
  echo "smoke_share_links: $SMOKE_SHARE_LINKS"
  echo "smoke_runtime_check: $SMOKE_RUNTIME_CHECK"
  echo "smoke_require_agent_report: $SMOKE_REQUIRE_AGENT_REPORT"
  echo "smoke_cleanup: $SMOKE_CLEANUP"
  echo "smoke_cleanup_on_failure: $SMOKE_CLEANUP_ON_FAILURE"
  if [[ -n "$certificate_id" ]]; then
    echo "certificate_id: $certificate_id"
  fi
  if pack_requires_fallback "$pack_key"; then
    if [[ -z "$FALLBACK_UPSTREAM_URL" ]]; then
      echo "pack $pack_key requires MEGAVPN_FALLBACK_UPSTREAM_URL for traffic camouflage smoke" >&2
      return 1
    fi
    guard_camouflage_fallback_loop "$endpoint_host"
    echo "fallback_upstream_url: $FALLBACK_UPSTREAM_URL"
    if [[ -n "$CAMOUFLAGE_PATH" ]]; then
      echo "camouflage_path: $CAMOUFLAGE_PATH"
    fi
  fi

  create_resp="$(request_json POST "/api/v1/service-packs/$pack_key/instances" "$(jq -n \
    --arg node_id "$node_id" \
    --arg base_name "$base_name" \
    --arg endpoint_host "$endpoint_host" \
    --arg certificate_id "$certificate_id" \
    --arg camouflage_path "$CAMOUFLAGE_PATH" \
    --arg fallback_upstream_url "$FALLBACK_UPSTREAM_URL" \
    --arg fallback_host_header "$FALLBACK_HOST_HEADER" \
    --arg fallback_sni "$FALLBACK_SNI" \
    '{
      node_id: $node_id,
      base_name: $base_name,
      endpoint_host: $endpoint_host
    }
    + (if $certificate_id != "" then {certificate_id: $certificate_id} else {} end)
    + (if $camouflage_path != "" then {camouflage_path: $camouflage_path} else {} end)
    + (if $fallback_upstream_url != "" then {fallback_upstream_url: $fallback_upstream_url} else {} end)
    + (if $fallback_host_header != "" then {fallback_host_header: $fallback_host_header} else {} end)
    + (if $fallback_sni != "" then {fallback_sni: $fallback_sni} else {} end)')")"
  printf '%s\n' "$create_resp" >"$create_evidence_file"
  echo "$create_resp" | jq .

  instance_rows="$(echo "$create_resp" | jq -c '.created_instances[]')"
  if [[ -z "$instance_rows" ]]; then
    echo "service pack created no instances" >&2
    return 1
  fi
  while IFS= read -r row; do
    local created_instance_id
    [[ -n "$row" ]] || continue
    created_instance_id="$(echo "$row" | jq -r '.id')"
    [[ -n "$created_instance_id" && "$created_instance_id" != "null" ]] && created_instance_ids+=("$created_instance_id")
  done <<< "$instance_rows"

  install_rows="$(echo "$create_resp" | jq -c '.runtime_install_jobs[]?')"
  if [[ -n "$install_rows" ]]; then
    while IFS= read -r row; do
      local install_job_id install_job_type
      [[ -n "$row" ]] || continue
      install_job_id="$(echo "$row" | jq -r '.id')"
      install_job_type="$(echo "$row" | jq -r '.type // "node.capability.install"')"
      echo "[runtime-install] job=$install_job_id type=$install_job_type"
      poll_job "$install_job_id" "runtime-install-$pack_key" | jq '{id, type, status, result}'
    done <<< "$install_rows"
  else
    echo "[runtime-install] no runtime install jobs queued"
  fi

  summary_file="$(mktemp)"
  while IFS= read -r row; do
    local instance_id instance_name service_code job_id instance_json runtime_json
    [[ -n "$row" ]] || continue
    instance_id="$(echo "$row" | jq -r '.id')"
    instance_name="$(echo "$row" | jq -r '.name')"
    service_code="$(echo "$row" | jq -r '.service_code')"
    job_id="$(queue_apply_if_missing "$instance_id")"
    echo "[apply] instance=$instance_name service=$service_code job=$job_id"
    poll_job "$job_id" "$instance_name" >"$summary_file"
    cat "$summary_file" | jq '{id, type, status, result}'
    instance_json="$(request_json GET "/api/v1/instances/$instance_id")"
    echo "$instance_json" | jq '{id, name, service_code, status, enabled, endpoint_host, endpoint_port, systemd_unit}'
    echo "$instance_json" >>"$instance_evidence_file"
    if [[ "$SMOKE_RUNTIME_CHECK" == "1" ]]; then
      runtime_json="$(wait_runtime_state "$instance_id" "$instance_name")"
      echo "$runtime_json"
      echo "$runtime_json" >>"$runtime_evidence_file"
    fi
    if is_provisionable_service "$service_code"; then
      provisionable_instance_ids+=("$instance_id")
    fi
  done <<< "$instance_rows"
  rm -f "$summary_file"

  client_id=""
  if [[ "$SMOKE_PROVISION" == "1" && "${#provisionable_instance_ids[@]}" -gt 0 ]]; then
    local provision_job_id provision_result_file ready_artifact_id share_link_id share_link_json share_link_token share_link_hint
    client_id="$(create_smoke_client "$base_name")"
    echo "[client] created client_id=$client_id"
    provision_job_id="$(provision_client_for_instances "$client_id" "${provisionable_instance_ids[@]}")"
    echo "[provision] client_id=$client_id job=$provision_job_id instances=${#provisionable_instance_ids[@]}"
    provision_result_file="$(mktemp)"
    poll_job "$provision_job_id" "client-provision-$base_name" >"$provision_result_file"
    provision_result_json="$(cat "$provision_result_file")"
    printf '%s\n' "$provision_result_json" >"$provision_evidence_file"
    echo "$provision_result_json" | jq '{id, type, status, result}'
    rm -f "$provision_result_file"
    wait_post_provision_apply_jobs "$provision_result_json" "$runtime_evidence_file"

    artifacts_json="$(request_json GET "/api/v1/clients/$client_id/artifacts")"
    printf '%s\n' "$artifacts_json" >"$artifacts_evidence_file"
    echo "$artifacts_json" | jq '{count: length, items: map({id, artifact_type, status, size_bytes, service_access_id})}'
    verify_client_provisioning_outputs "$client_id" "$artifacts_json" "$accesses_evidence_file" "${provisionable_instance_ids[@]}"
    ready_artifact_id="$(echo "$artifacts_json" | jq -r '[.[] | select(.status == "ready")][0].id // empty')"
    if [[ -z "$ready_artifact_id" ]]; then
      echo "client provisioning succeeded but no ready artifacts were created" >&2
      return 1
    fi

    if [[ "$SMOKE_SHARE_LINKS" == "1" ]]; then
      share_link_json="$(publish_share_link_for_client "$client_id" "$ready_artifact_id")"
      share_link_id="$(echo "$share_link_json" | jq -r '.id')"
      share_link_token="$(echo "$share_link_json" | jq -r '.token // empty')"
      share_link_hint="$(echo "$share_link_json" | jq -r '.token_hint // empty')"
      echo "[share-link] client_id=$client_id artifact_id=$ready_artifact_id share_link_id=$share_link_id token_hint=$share_link_hint"
      if [[ -z "$share_link_token" ]]; then
        echo "share link publish response did not include one-time token" >&2
        return 1
      fi
      download_share_link_once "$share_link_token"
      request_json GET "/api/v1/clients/$client_id/share-links" | jq '{count: length, items: map({id, status, target_type, target_id, token_hint, has_plaintext_token: has("token"), expires_at})}'
    fi
  else
    echo "[provision] skipped"
  fi

  if smoke_truthy "$SMOKE_CLEANUP"; then
    single_pack_failed=0
    if [[ "${#created_instance_ids[@]}" -gt 0 ]]; then
      cleanup_smoke_resources "$client_id" "${created_instance_ids[@]}"
    else
      cleanup_smoke_resources "$client_id"
    fi
  fi

  if [[ -n "$evidence_file" ]]; then
    write_smoke_evidence "$evidence_file" "$node_id" "$pack_key" "$endpoint_host" "$base_name" "$client_id" "$create_evidence_file" "$instance_evidence_file" "$runtime_evidence_file" "$provision_evidence_file" "$accesses_evidence_file" "$artifacts_evidence_file"
  fi
  rm -f "$create_evidence_file" "$instance_evidence_file" "$runtime_evidence_file" "$provision_evidence_file" "$accesses_evidence_file" "$artifacts_evidence_file"
  single_pack_failed=0
  trap - ERR
  echo "service-pack smoke succeeded: $pack_key"
}

run_matrix() {
  local node_id="$1"
  local endpoint_domain="$2"
  local certificate_id="${3:-}"
  local packs_json pack_keys pack_key suffix host base_name stamp status summary_tmp matrix_summary_file evidence_path reason
  local ok_count=0
  local failed_count=0
  local skipped_count=0
  local selected_count=0

  if [[ -z "$node_id" || -z "$endpoint_domain" ]]; then
    usage >&2
    exit 1
  fi

  packs_json="$(request_json GET "/api/v1/service-packs")"
  pack_keys="$(echo "$packs_json" | jq -r '.[].key')"
  validate_matrix_pack_filters "$pack_keys"
  if [[ "$MATRIX_PLAN_ONLY" == "1" ]]; then
    print_matrix_plan "$packs_json" "$endpoint_domain" "$certificate_id" "$pack_keys"
    return
  fi
  summary_tmp="$(mktemp)"
  if [[ -n "$MATRIX_PACKS" ]]; then
    echo "matrix include packs: $MATRIX_PACKS"
  fi
  if [[ -n "$MATRIX_EXCLUDE_PACKS" ]]; then
    echo "matrix exclude packs: $MATRIX_EXCLUDE_PACKS"
  fi
  if matrix_summary_file="$(resolve_matrix_summary_file)"; then
    echo "matrix_summary_file: $matrix_summary_file"
  else
    matrix_summary_file=""
  fi
  while IFS= read -r pack_key; do
    [[ -n "$pack_key" ]] || continue
    if ! matrix_pack_selected "$pack_key"; then
      reason="not selected by matrix pack filter"
      echo "[$pack_key] skipped: $reason"
      printf 'SKIPPED\t%s\t-\t-\t-\t%s\n' "$pack_key" "$reason" >>"$summary_tmp"
      skipped_count=$((skipped_count + 1))
      continue
    fi
    selected_count=$((selected_count + 1))
    if pack_requires_certificate "$pack_key" && [[ -z "$certificate_id" ]]; then
      reason="managed certificate id is required for edge TLS smoke"
      echo "[$pack_key] skipped: $reason" >&2
      printf 'SKIPPED\t%s\t-\t-\t-\t%s\n' "$pack_key" "$reason" >>"$summary_tmp"
      skipped_count=$((skipped_count + 1))
      continue
    fi
    if pack_requires_fallback "$pack_key" && [[ -z "$FALLBACK_UPSTREAM_URL" ]]; then
      reason="MEGAVPN_FALLBACK_UPSTREAM_URL is required for traffic camouflage smoke"
      echo "[$pack_key] skipped: $reason" >&2
      printf 'SKIPPED\t%s\t-\t-\t-\t%s\n' "$pack_key" "$reason" >>"$summary_tmp"
      skipped_count=$((skipped_count + 1))
      continue
    fi
    stamp="$(date +%H%M%S)"
    suffix="${pack_key//_/-}"
    base_name="smoke-${suffix}-${stamp}"
    host="${base_name}.${endpoint_domain}"
    evidence_path="$(matrix_pack_evidence_path "$base_name")"
    if MEGAVPN_SMOKE_PROVISION="$SMOKE_PROVISION" \
      MEGAVPN_SMOKE_SHARE_LINKS="$SMOKE_SHARE_LINKS" \
      MEGAVPN_SMOKE_SHARE_LINK_TTL_HOURS="$SMOKE_SHARE_LINK_TTL_HOURS" \
      MEGAVPN_SMOKE_RUNTIME_CHECK="$SMOKE_RUNTIME_CHECK" \
      MEGAVPN_SMOKE_REQUIRE_AGENT_REPORT="$SMOKE_REQUIRE_AGENT_REPORT" \
      MEGAVPN_SMOKE_CLEANUP="$SMOKE_CLEANUP" \
      MEGAVPN_SMOKE_CLEANUP_ON_FAILURE="$SMOKE_CLEANUP_ON_FAILURE" \
      MEGAVPN_SMOKE_EVIDENCE_DIR="$SMOKE_EVIDENCE_DIR" \
      MEGAVPN_RUNTIME_WAIT_ATTEMPTS="$RUNTIME_WAIT_ATTEMPTS" \
      MEGAVPN_RUNTIME_WAIT_INTERVAL="$RUNTIME_WAIT_INTERVAL" \
      MEGAVPN_CLIENT_EMAIL_DOMAIN="$CLIENT_EMAIL_DOMAIN" \
      MEGAVPN_CAMOUFLAGE_PATH="$CAMOUFLAGE_PATH" \
      MEGAVPN_FALLBACK_UPSTREAM_URL="$FALLBACK_UPSTREAM_URL" \
      MEGAVPN_FALLBACK_HOST_HEADER="$FALLBACK_HOST_HEADER" \
      MEGAVPN_FALLBACK_SNI="$FALLBACK_SNI" \
      "$SCRIPT_PATH" "$node_id" "$pack_key" "$host" "$base_name" "$certificate_id"; then
      status="OK"
      ok_count=$((ok_count + 1))
    else
      status="FAILED"
      failed_count=$((failed_count + 1))
    fi
    printf '%s\t%s\t%s\t%s\t%s\t-\n' "$status" "$pack_key" "$base_name" "$host" "$evidence_path" >>"$summary_tmp"
  done <<< "$pack_keys"

  echo
  echo "matrix summary:"
  printf '%-8s %-30s %-40s %s\n' "STATUS" "PACK" "BASE_NAME" "ENDPOINT_HOST"
  while IFS=$'\t' read -r status pack_key base_name host evidence_path reason; do
    printf '%-8s %-30s %-40s %s\n' "$status" "$pack_key" "$base_name" "$host"
  done <"$summary_tmp"
  if [[ -n "$matrix_summary_file" ]]; then
    write_matrix_summary "$matrix_summary_file" "$summary_tmp" "$node_id" "$endpoint_domain" "$certificate_id" "$ok_count" "$failed_count" "$skipped_count" "$selected_count"
  fi
  rm -f "$summary_tmp"

  echo
  echo "matrix totals: ok=$ok_count failed=$failed_count skipped=$skipped_count"
  if [[ "$selected_count" -eq 0 ]]; then
    echo "matrix selected no service packs; check MEGAVPN_SMOKE_PACKS/--packs filters" >&2
    return 1
  fi
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
