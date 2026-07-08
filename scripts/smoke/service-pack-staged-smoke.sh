#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SMOKE_SCRIPT="$ROOT_DIR/scripts/smoke/service-pack-smoke.sh"
REPORT_SCRIPT="$ROOT_DIR/scripts/ci/service-pack-evidence-report.js"
NODE_BIN="${MEGAVPN_SMOKE_NODE_BIN:-${MEGAVPN_RELEASE_NODE_BIN:-node}}"

DEFAULT_BATCHES="remote_access_l3 proxy_access xray_reality xray_nginx_http xray_nginx_grpc legacy_l2tp"
BATCHES="${MEGAVPN_SMOKE_BATCHES:-$DEFAULT_BATCHES}"
EVIDENCE_ROOT="${MEGAVPN_SMOKE_EVIDENCE_ROOT:-tmp/service-pack-evidence}"
STAGED_SUMMARY_FILE="${MEGAVPN_SMOKE_STAGED_SUMMARY_FILE:-}"
PLAN_ONLY="${MEGAVPN_SMOKE_BATCH_PLAN_ONLY:-0}"
SKIP_PLAN="${MEGAVPN_SMOKE_BATCH_SKIP_PLAN:-0}"
KEEP_GOING="${MEGAVPN_SMOKE_BATCH_KEEP_GOING:-0}"
REQUIRE_NO_SKIPS="${MEGAVPN_SMOKE_BATCH_REQUIRE_NO_SKIPS:-1}"
ALLOW_PORT_OVERLAPS="${MEGAVPN_SMOKE_BATCH_ALLOW_PORT_OVERLAPS:-0}"

usage() {
  cat <<'EOF'
Usage:
  scripts/service-pack-staged-smoke.sh --list
  scripts/service-pack-staged-smoke.sh [options] <node-id> <endpoint-domain> [certificate-id]

Options:
  --batches name1,name2  Batch names to run.
  --plan                Print per-batch matrix plans and exit.
  --skip-plan           Do not run the preflight matrix plan before each batch.
  --keep-going          Continue after a failed batch and return failure at the end.
  --evidence-root dir   Root directory for per-batch evidence.
  --allow-skips         Do not fail evidence report on skipped rows.
  --allow-port-overlaps Allow known endpoint-port overlaps without cleanup.
  --cleanup             Delete smoke clients and instances after each successful pack.
  --list                Print available batches and exit.
  --help                Show this help.

Environment:
  MEGAVPN_PUBLIC_BASE_URL / MEGAVPN_API        API base URL for service-pack-smoke.sh.
  MEGAVPN_AUTH_TOKEN                           Bearer token for authenticated API calls.
  MEGAVPN_SMOKE_BATCHES                        Comma/space-separated batch names.
  MEGAVPN_SMOKE_EVIDENCE_ROOT                  Root evidence dir (default: tmp/service-pack-evidence).
  MEGAVPN_SMOKE_STAGED_SUMMARY_FILE            Optional path for the staged run summary JSON.
  MEGAVPN_SMOKE_BATCH_PLAN_ONLY                Print plans and exit.
  MEGAVPN_SMOKE_BATCH_SKIP_PLAN                Skip preflight plan before real runs.
  MEGAVPN_SMOKE_BATCH_KEEP_GOING               Continue through failed batches.
  MEGAVPN_SMOKE_BATCH_REQUIRE_NO_SKIPS         Fail evidence report on skipped rows (default: 1).
  MEGAVPN_SMOKE_BATCH_ALLOW_PORT_OVERLAPS      Allow known endpoint-port overlaps without cleanup.
  MEGAVPN_SMOKE_CLEANUP                        Delete smoke resources after each successful pack.
  MEGAVPN_FALLBACK_UPSTREAM_URL                Required for Nginx camouflage packs.

Examples:
  scripts/service-pack-staged-smoke.sh --plan node-uuid smoke.example.com cert-uuid
  MEGAVPN_FALLBACK_UPSTREAM_URL=https://target.example.com scripts/service-pack-staged-smoke.sh --cleanup node-uuid smoke.example.com cert-uuid
  scripts/service-pack-staged-smoke.sh --batches remote_access_l3,proxy_access node-uuid smoke.example.com
EOF
}

batch_packs() {
  case "$1" in
    remote_access_l3)
      printf '%s\n' 'openvpn_tcp_11994,openvpn_udp_1194,wireguard_roadwarrior'
      ;;
    proxy_access)
      printf '%s\n' 'http_proxy_authenticated,mtproto_telegram_443,shadowsocks_chacha'
      ;;
    xray_reality)
      printf '%s\n' 'xray_vless_reality'
      ;;
    xray_nginx_http)
      printf '%s\n' 'xray_nginx_http_edge'
      ;;
    xray_nginx_grpc)
      printf '%s\n' 'xray_nginx_grpc_edge'
      ;;
    legacy_l2tp)
      printf '%s\n' 'ipsec_xl2tpd_access'
      ;;
    *)
      return 1
      ;;
  esac
}

batch_label() {
  case "$1" in
    remote_access_l3) printf '%s\n' 'OpenVPN TCP/UDP and WireGuard remote access' ;;
    proxy_access) printf '%s\n' 'HTTP proxy, MTProto and Shadowsocks' ;;
    xray_reality) printf '%s\n' 'Xray VLESS / Reality direct endpoint' ;;
    xray_nginx_http) printf '%s\n' 'Xray WebSocket behind Nginx camouflage edge' ;;
    xray_nginx_grpc) printf '%s\n' 'Xray gRPC behind Nginx camouflage edge' ;;
    legacy_l2tp) printf '%s\n' 'IPsec/L2TP legacy access' ;;
    *) printf '%s\n' "$1" ;;
  esac
}

list_batches() {
  local batch packs
  printf '%-18s %-62s %s\n' "BATCH" "PACKS" "DESCRIPTION"
  for batch in $DEFAULT_BATCHES; do
    packs="$(batch_packs "$batch")"
    printf '%-18s %-62s %s\n' "$batch" "$packs" "$(batch_label "$batch")"
  done
}

normalize_batch_list() {
  printf '%s\n' "$1" | tr ',' ' '
}

validate_batches() {
  local batch fail=0
  for batch in $(normalize_batch_list "$BATCHES"); do
    [[ -n "$batch" ]] || continue
    if ! batch_packs "$batch" >/dev/null; then
      printf 'unknown batch: %s\n' "$batch" >&2
      fail=1
    fi
  done
  [[ "$fail" -eq 0 ]]
}

batch_ports() {
  case "$1" in
    remote_access_l3)
      printf '%s\n' '11994/tcp openvpn_tcp_11994'
      printf '%s\n' '1194/udp openvpn_udp_1194'
      printf '%s\n' '51820/udp wireguard_roadwarrior'
      ;;
    proxy_access)
      printf '%s\n' '3128/tcp http_proxy_authenticated'
      printf '%s\n' '443/tcp mtproto_telegram_443'
      printf '%s\n' '8388/tcp+udp shadowsocks_chacha'
      ;;
    xray_reality)
      printf '%s\n' '443/tcp xray_vless_reality'
      ;;
    xray_nginx_http)
      printf '%s\n' '443/tcp xray_nginx_http_edge'
      ;;
    xray_nginx_grpc)
      printf '%s\n' '443/tcp xray_nginx_grpc_edge'
      ;;
    legacy_l2tp)
      printf '%s\n' '1701/udp ipsec_xl2tpd_access'
      ;;
  esac
}

cleanup_enabled() {
  case "${MEGAVPN_SMOKE_CLEANUP:-0}" in
    1|true|TRUE|yes|YES|on|ON)
      return 0
      ;;
  esac
  return 1
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g; s/\r/\\r/g'
}

json_string() {
  printf '"%s"' "$(json_escape "$1")"
}

json_bool() {
  case "$1" in
    1|true|TRUE|yes|YES|on|ON)
      printf 'true'
      ;;
    *)
      printf 'false'
      ;;
  esac
}

json_list() {
  local value="$1"
  local first=1
  local item
  printf '['
  for item in $(printf '%s\n' "$value" | tr ',' ' '); do
    [[ -n "$item" ]] || continue
    if [[ "$first" -eq 0 ]]; then
      printf ', '
    fi
    json_string "$item"
    first=0
  done
  printf ']'
}

append_staged_record() {
  local records_file="$1"
  local status="$2"
  local batch="$3"
  local label="$4"
  local packs="$5"
  local evidence_dir="$6"
  local matrix_summary_file="$7"
  local report_status="$8"
  local reason="$9"

  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$status" \
    "$batch" \
    "$label" \
    "$packs" \
    "$evidence_dir" \
    "$matrix_summary_file" \
    "$report_status" \
    "$reason" >>"$records_file"
}

write_staged_summary() {
  local records_file="$1"
  local summary_file="$2"
  local run_id="$3"
  local status="$4"
  local node_id="$5"
  local endpoint_domain="$6"
  local certificate_id="$7"
  local generated_at
  local cleanup_value
  local first
  local total=0
  local ok=0
  local failed=0
  local planned=0
  local skipped=0
  local row_status batch label packs evidence_dir matrix_summary_file report_status reason

  generated_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  cleanup_value="0"
  if cleanup_enabled; then
    cleanup_value="1"
  fi

  while IFS=$'\t' read -r row_status batch label packs evidence_dir matrix_summary_file report_status reason; do
    [[ -n "$batch" ]] || continue
    total=$((total + 1))
    case "$row_status" in
      OK) ok=$((ok + 1)) ;;
      FAILED) failed=$((failed + 1)) ;;
      PLANNED) planned=$((planned + 1)) ;;
      SKIPPED) skipped=$((skipped + 1)) ;;
    esac
  done <"$records_file"

  mkdir -p "$(dirname "$summary_file")"
  {
    printf '{\n'
    printf '  "schema_version": 1,\n'
    printf '  "status": '; json_string "$status"; printf ',\n'
    printf '  "generated_at": '; json_string "$generated_at"; printf ',\n'
    printf '  "run_id": '; json_string "$run_id"; printf ',\n'
    printf '  "node_id": '; json_string "$node_id"; printf ',\n'
    printf '  "endpoint_domain": '; json_string "$endpoint_domain"; printf ',\n'
    if [[ -n "$certificate_id" ]]; then
      printf '  "certificate_id": '; json_string "$certificate_id"; printf ',\n'
    else
      printf '  "certificate_id": null,\n'
    fi
    printf '  "batches_requested": '; json_list "$BATCHES"; printf ',\n'
    printf '  "options": {\n'
    printf '    "plan_only": '; json_bool "$PLAN_ONLY"; printf ',\n'
    printf '    "skip_plan": '; json_bool "$SKIP_PLAN"; printf ',\n'
    printf '    "keep_going": '; json_bool "$KEEP_GOING"; printf ',\n'
    printf '    "require_no_skips": '; json_bool "$REQUIRE_NO_SKIPS"; printf ',\n'
    printf '    "allow_port_overlaps": '; json_bool "$ALLOW_PORT_OVERLAPS"; printf ',\n'
    printf '    "cleanup": '; json_bool "$cleanup_value"; printf '\n'
    printf '  },\n'
    printf '  "totals": {\n'
    printf '    "total": %s,\n' "$total"
    printf '    "ok": %s,\n' "$ok"
    printf '    "failed": %s,\n' "$failed"
    printf '    "planned": %s,\n' "$planned"
    printf '    "skipped": %s\n' "$skipped"
    printf '  },\n'
    printf '  "results": [\n'
    first=1
    while IFS=$'\t' read -r row_status batch label packs evidence_dir matrix_summary_file report_status reason; do
      [[ -n "$batch" ]] || continue
      if [[ "$first" -eq 0 ]]; then
        printf ',\n'
      fi
      printf '    {\n'
      printf '      "status": '; json_string "$row_status"; printf ',\n'
      printf '      "batch": '; json_string "$batch"; printf ',\n'
      printf '      "description": '; json_string "$label"; printf ',\n'
      printf '      "packs": '; json_list "$packs"; printf ',\n'
      printf '      "evidence_dir": '; json_string "$evidence_dir"; printf ',\n'
      printf '      "matrix_summary_file": '; json_string "$matrix_summary_file"; printf ',\n'
      printf '      "report_status": '; json_string "$report_status"; printf ',\n'
      printf '      "reason": '; json_string "$reason"; printf '\n'
      printf '    }'
      first=0
    done <"$records_file"
    printf '\n'
    printf '  ]\n'
    printf '}\n'
  } >"$summary_file"
}

validate_port_overlaps() {
  local tmp batch duplicate_ports port count
  if [[ "$ALLOW_PORT_OVERLAPS" == "1" || "$ALLOW_PORT_OVERLAPS" == "true" ]] || cleanup_enabled; then
    return 0
  fi
  tmp="$(mktemp)"
  for batch in $(normalize_batch_list "$BATCHES"); do
    [[ -n "$batch" ]] || continue
    while IFS= read -r port; do
      [[ -n "$port" ]] || continue
      printf '%s %s\n' "$port" "$batch" >>"$tmp"
    done < <(batch_ports "$batch")
  done
  duplicate_ports="$(awk '{print $1}' "$tmp" | sort | uniq -d)"
  if [[ -z "$duplicate_ports" ]]; then
    rm -f "$tmp"
    return 0
  fi
  printf 'selected staged batches contain endpoint-port overlaps and MEGAVPN_SMOKE_CLEANUP is not enabled:\n' >&2
  while IFS= read -r port; do
    [[ -n "$port" ]] || continue
    count="$(awk -v p="$port" '$1 == p {count++} END {print count + 0}' "$tmp")"
    if [[ "$count" -gt 1 ]]; then
      printf '  %s\n' "$port" >&2
      awk -v p="$port" '$1 == p {print "    batch=" $3 " pack=" $2}' "$tmp" >&2
    fi
  done <<< "$duplicate_ports"
  rm -f "$tmp"
  if [[ "$PLAN_ONLY" == "1" || "$PLAN_ONLY" == "true" ]]; then
    printf 'Plan mode continues so operators can inspect every batch; real runs must resolve the overlap first.\n' >&2
    return 0
  fi
  printf 'Run one conflicting batch at a time, enable --cleanup for diagnostic runs, or pass --allow-port-overlaps only when nodes/ports are isolated externally.\n' >&2
  return 1
}

smoke_matrix() {
  local node_id="$1"
  local endpoint_domain="$2"
  local certificate_id="$3"
  local packs="$4"
  local evidence_dir="$5"
  local plan_flag="$6"

  if [[ "$plan_flag" == "1" ]]; then
    if [[ -n "$certificate_id" ]]; then
      MEGAVPN_SMOKE_PACKS="$packs" \
      MEGAVPN_SMOKE_EVIDENCE_DIR="$evidence_dir" \
        "$SMOKE_SCRIPT" --matrix "$node_id" "$endpoint_domain" "$certificate_id" --packs "$packs" --plan
    else
      MEGAVPN_SMOKE_PACKS="$packs" \
      MEGAVPN_SMOKE_EVIDENCE_DIR="$evidence_dir" \
        "$SMOKE_SCRIPT" --matrix "$node_id" "$endpoint_domain" --packs "$packs" --plan
    fi
    return
  fi

  if [[ -n "$certificate_id" ]]; then
    MEGAVPN_SMOKE_PACKS="$packs" \
    MEGAVPN_SMOKE_EVIDENCE_DIR="$evidence_dir" \
      "$SMOKE_SCRIPT" --matrix "$node_id" "$endpoint_domain" "$certificate_id" --packs "$packs"
  else
    MEGAVPN_SMOKE_PACKS="$packs" \
    MEGAVPN_SMOKE_EVIDENCE_DIR="$evidence_dir" \
      "$SMOKE_SCRIPT" --matrix "$node_id" "$endpoint_domain" --packs "$packs"
  fi
}

evidence_report() {
  local packs="$1"
  local summary_file="$2"
  if ! command -v "$NODE_BIN" >/dev/null 2>&1; then
    printf 'node is unavailable; cannot validate service-pack evidence report\n' >&2
    return 1
  fi
  if [[ "$REQUIRE_NO_SKIPS" == "1" || "$REQUIRE_NO_SKIPS" == "true" ]]; then
    "$NODE_BIN" "$REPORT_SCRIPT" --require-pack "$packs" --require-no-skips "$summary_file"
  else
    "$NODE_BIN" "$REPORT_SCRIPT" --require-pack "$packs" "$summary_file"
  fi
}

run_batches() {
  local node_id="$1"
  local endpoint_domain="$2"
  local certificate_id="${3:-}"
  local run_id run_evidence_root staged_summary_file records_file batch packs evidence_dir summary_file failed=0
  local label

  validate_batches
  validate_port_overlaps
  run_id="$(date -u '+%Y%m%dT%H%M%SZ')"
  run_evidence_root="${EVIDENCE_ROOT%/}/$run_id"
  staged_summary_file="${STAGED_SUMMARY_FILE:-$run_evidence_root/_staged-summary.json}"
  records_file="$run_evidence_root/.staged-records.tsv"
  mkdir -p "$run_evidence_root"
  : >"$records_file"

  printf 'staged smoke run: %s\n' "$run_id"
  printf 'node_id: %s\n' "$node_id"
  printf 'endpoint_domain: %s\n' "$endpoint_domain"
  printf 'evidence_root: %s\n' "$EVIDENCE_ROOT"
  printf 'staged_summary: %s\n' "$staged_summary_file"
  printf 'batches: %s\n' "$BATCHES"
  write_staged_summary "$records_file" "$staged_summary_file" "$run_id" "running" "$node_id" "$endpoint_domain" "$certificate_id"

  for batch in $(normalize_batch_list "$BATCHES"); do
    [[ -n "$batch" ]] || continue
    packs="$(batch_packs "$batch")"
    label="$(batch_label "$batch")"
    evidence_dir="$run_evidence_root/$batch"
    summary_file="$evidence_dir/_matrix-summary.json"
    mkdir -p "$evidence_dir"

    printf '\n== batch: %s ==\n' "$batch"
    printf 'description: %s\n' "$label"
    printf 'packs: %s\n' "$packs"
    printf 'evidence_dir: %s\n' "$evidence_dir"

    if [[ "$SKIP_PLAN" != "1" && "$SKIP_PLAN" != "true" ]]; then
      if ! smoke_matrix "$node_id" "$endpoint_domain" "$certificate_id" "$packs" "$evidence_dir" "1"; then
        failed=1
        printf 'batch plan failed: %s\n' "$batch" >&2
        append_staged_record "$records_file" "FAILED" "$batch" "$label" "$packs" "$evidence_dir" "$summary_file" "not_run" "batch plan failed"
        write_staged_summary "$records_file" "$staged_summary_file" "$run_id" "failed" "$node_id" "$endpoint_domain" "$certificate_id"
        if [[ "$KEEP_GOING" != "1" && "$KEEP_GOING" != "true" ]]; then
          return 1
        fi
        continue
      fi
    fi
    if [[ "$PLAN_ONLY" == "1" || "$PLAN_ONLY" == "true" ]]; then
      if [[ "$SKIP_PLAN" == "1" || "$SKIP_PLAN" == "true" ]]; then
        append_staged_record "$records_file" "SKIPPED" "$batch" "$label" "$packs" "$evidence_dir" "$summary_file" "not_run" "preflight plan skipped"
      else
        append_staged_record "$records_file" "PLANNED" "$batch" "$label" "$packs" "$evidence_dir" "$summary_file" "not_run" ""
      fi
      write_staged_summary "$records_file" "$staged_summary_file" "$run_id" "running" "$node_id" "$endpoint_domain" "$certificate_id"
      continue
    fi

    if smoke_matrix "$node_id" "$endpoint_domain" "$certificate_id" "$packs" "$evidence_dir" "0"; then
      if ! evidence_report "$packs" "$summary_file"; then
        failed=1
        printf 'batch evidence validation failed: %s\n' "$batch" >&2
        append_staged_record "$records_file" "FAILED" "$batch" "$label" "$packs" "$evidence_dir" "$summary_file" "failed" "batch evidence validation failed"
        write_staged_summary "$records_file" "$staged_summary_file" "$run_id" "failed" "$node_id" "$endpoint_domain" "$certificate_id"
        if [[ "$KEEP_GOING" != "1" && "$KEEP_GOING" != "true" ]]; then
          return 1
        fi
      else
        append_staged_record "$records_file" "OK" "$batch" "$label" "$packs" "$evidence_dir" "$summary_file" "succeeded" ""
        write_staged_summary "$records_file" "$staged_summary_file" "$run_id" "running" "$node_id" "$endpoint_domain" "$certificate_id"
      fi
    else
      failed=1
      printf 'batch failed: %s\n' "$batch" >&2
      if [[ -f "$summary_file" ]]; then
        evidence_report "$packs" "$summary_file" || true
      fi
      append_staged_record "$records_file" "FAILED" "$batch" "$label" "$packs" "$evidence_dir" "$summary_file" "not_run" "batch smoke matrix failed"
      write_staged_summary "$records_file" "$staged_summary_file" "$run_id" "failed" "$node_id" "$endpoint_domain" "$certificate_id"
      if [[ "$KEEP_GOING" != "1" && "$KEEP_GOING" != "true" ]]; then
        return 1
      fi
    fi
  done

  if [[ "$failed" -ne 0 ]]; then
    write_staged_summary "$records_file" "$staged_summary_file" "$run_id" "failed" "$node_id" "$endpoint_domain" "$certificate_id"
    return 1
  fi
  if [[ "$PLAN_ONLY" == "1" || "$PLAN_ONLY" == "true" ]]; then
    write_staged_summary "$records_file" "$staged_summary_file" "$run_id" "planned" "$node_id" "$endpoint_domain" "$certificate_id"
  else
    write_staged_summary "$records_file" "$staged_summary_file" "$run_id" "succeeded" "$node_id" "$endpoint_domain" "$certificate_id"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --list)
      list_batches
      exit 0
      ;;
    --batches)
      if [[ -z "${2:-}" || "${2:-}" == -* ]]; then
        printf '%s\n' '--batches requires a comma-separated batch list' >&2
        usage >&2
        exit 1
      fi
      BATCHES="$2"
      shift 2
      ;;
    --plan|--dry-run)
      PLAN_ONLY=1
      shift
      ;;
    --skip-plan)
      SKIP_PLAN=1
      shift
      ;;
    --keep-going)
      KEEP_GOING=1
      shift
      ;;
    --evidence-root)
      if [[ -z "${2:-}" || "${2:-}" == -* ]]; then
        printf '%s\n' '--evidence-root requires a directory path' >&2
        usage >&2
        exit 1
      fi
      EVIDENCE_ROOT="$2"
      shift 2
      ;;
    --allow-skips)
      REQUIRE_NO_SKIPS=0
      shift
      ;;
    --allow-port-overlaps)
      ALLOW_PORT_OVERLAPS=1
      shift
      ;;
    --cleanup)
      export MEGAVPN_SMOKE_CLEANUP=1
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
      printf 'unknown option: %s\n' "$1" >&2
      usage >&2
      exit 1
      ;;
    *)
      break
      ;;
  esac
done

NODE_ID="${1:-${MEGAVPN_NODE_ID:-}}"
ENDPOINT_DOMAIN="${2:-${MEGAVPN_ENDPOINT_DOMAIN:-}}"
CERTIFICATE_ID="${3:-${MEGAVPN_CERTIFICATE_ID:-}}"

if [[ -z "$NODE_ID" || -z "$ENDPOINT_DOMAIN" ]]; then
  usage >&2
  exit 1
fi

run_batches "$NODE_ID" "$ENDPOINT_DOMAIN" "$CERTIFICATE_ID"
