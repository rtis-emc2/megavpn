#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-/tmp/megavpn-go-cache}"
export GOTMPDIR="${GOTMPDIR:-/tmp/megavpn-go-tmp}"
SELF_TEST_TEMP_GOMODCACHE=""
if [[ -n "${GOMODCACHE:-}" ]]; then
  export GOMODCACHE
else
  SELF_TEST_TEMP_GOMODCACHE="$(mktemp -d "${TMPDIR:-/tmp}/megavpn-go-modcache.XXXXXX")"
  export GOMODCACHE="$SELF_TEST_TEMP_GOMODCACHE"
fi
cleanup_self_test_tmp() {
  if [[ -n "$SELF_TEST_TEMP_GOMODCACHE" && -d "$SELF_TEST_TEMP_GOMODCACHE" ]]; then
    chmod -R u+w "$SELF_TEST_TEMP_GOMODCACHE" 2>/dev/null || true
    rm -rf "$SELF_TEST_TEMP_GOMODCACHE"
  fi
}
trap cleanup_self_test_tmp EXIT
mkdir -p "$GOCACHE" "$GOTMPDIR" "$GOMODCACHE"

REPORT_DIR="${MEGAVPN_SELF_TEST_REPORT_DIR:-tmp/self-test}"
REPORT_TS="$(date -u '+%Y%m%dT%H%M%SZ')"
REPORT_FILE="$REPORT_DIR/self-test-$REPORT_TS.md"
mkdir -p "$REPORT_DIR"

RELEASE_DATABASE_DSN="${MEGAVPN_RELEASE_DATABASE_DSN:-${MEGAVPN_TEST_DATABASE_DSN:-}}"
RELEASE_RESTORE_DATABASE_DSN="${MEGAVPN_RELEASE_RESTORE_DATABASE_DSN:-}"
RELEASE_BASE_URL="${MEGAVPN_RELEASE_BASE_URL:-${MEGAVPN_PUBLIC_BASE_URL:-}}"
RELEASE_NODE_ID="${MEGAVPN_RELEASE_NODE_ID:-}"
RELEASE_ENDPOINT_DOMAIN="${MEGAVPN_RELEASE_ENDPOINT_DOMAIN:-}"
RELEASE_CERTIFICATE_ID="${MEGAVPN_RELEASE_CERTIFICATE_ID:-}"
RUN_RACE="${MEGAVPN_SELF_TEST_RUN_RACE:-${MEGAVPN_RELEASE_RUN_RACE:-1}}"
RUN_SERVICE_MATRIX="${MEGAVPN_SELF_TEST_RUN_SERVICE_MATRIX:-${MEGAVPN_RELEASE_RUN_SERVICE_MATRIX:-0}}"
REQUIRE_AGENT_REPORT="${MEGAVPN_SELF_TEST_REQUIRE_AGENT_REPORT:-${MEGAVPN_RELEASE_REQUIRE_AGENT_REPORT:-1}}"
SERVICE_MATRIX_REQUIRED_PACKS="${MEGAVPN_SELF_TEST_SERVICE_MATRIX_REQUIRED_PACKS:-${MEGAVPN_RELEASE_SERVICE_MATRIX_REQUIRED_PACKS:-}}"
SERVICE_MATRIX_REQUIRE_NO_SKIPS="${MEGAVPN_SELF_TEST_SERVICE_MATRIX_REQUIRE_NO_SKIPS:-${MEGAVPN_RELEASE_SERVICE_MATRIX_REQUIRE_NO_SKIPS:-0}}"
NGINX_CONFIG="${MEGAVPN_SELF_TEST_NGINX_CONFIG:-}"
NODE_BIN="${MEGAVPN_SELF_TEST_NODE_BIN:-${MEGAVPN_RELEASE_NODE_BIN:-node}}"

passed=0
failed=0
skipped=0
gate_names=()
gate_statuses=()
gate_evidence=()

log() {
  printf '[self-test] %s\n' "$*"
}

escape_md() {
  printf '%s' "$1" | sed 's/|/\\|/g'
}

add_gate() {
  gate_statuses[${#gate_statuses[@]}]="$1"
  gate_names[${#gate_names[@]}]="$2"
  gate_evidence[${#gate_evidence[@]}]="$3"
}

skip_check() {
  printf '%s\n' "$1"
  return 77
}

run_check() {
  local name="$1"
  local description="$2"
  shift 2
  local slug log_file rc reason
  slug="$(printf '%s' "$name" | tr '[:upper:] ' '[:lower:]-' | tr -cd '[:alnum:]_.-')"
  log_file="$REPORT_DIR/$REPORT_TS-$slug.log"

  log "RUN $name"
  "$@" >"$log_file" 2>&1
  rc=$?

  if [[ "$rc" -eq 0 ]]; then
    passed=$((passed + 1))
    add_gate "PASS" "$name" "$description; log: $log_file"
    log "PASS $name"
    return 0
  fi

  if [[ "$rc" -eq 77 ]]; then
    skipped=$((skipped + 1))
    reason="$(tail -n 1 "$log_file" 2>/dev/null || true)"
    [[ -n "$reason" ]] || reason="$description"
    add_gate "SKIP" "$name" "$reason; log: $log_file"
    log "SKIP $name: $reason"
    return 0
  fi

  failed=$((failed + 1))
  add_gate "FAIL" "$name" "$description; exit=$rc; log: $log_file"
  log "FAIL $name: exit=$rc, log=$log_file"
  return 0
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    printf 'missing command: %s\n' "$1" >&2
    return 1
  }
}

require_gofmt_clean() {
  local unformatted
  unformatted="$(gofmt -l cmd internal)"
  if [[ -n "$unformatted" ]]; then
    printf '%s\n' "$unformatted" >&2
    return 1
  fi
}

require_version_tag_consistency() {
  local code_version head_tag
  code_version="$(sed -nE 's/^const Version = "([^"]+)"/\1/p' internal/platform/version/version.go)"
  if [[ -z "$code_version" ]]; then
    printf 'unable to read internal/platform/version.Version\n' >&2
    return 1
  fi
  head_tag="$(git tag --points-at HEAD | sort -V | tail -n 1)"
  if [[ -z "$head_tag" ]]; then
    printf 'HEAD is not tagged; code version is %s\n' "$code_version"
    return 0
  fi
  if [[ "$head_tag" != "v$code_version" ]]; then
    printf 'HEAD tag %s does not match code version v%s\n' "$head_tag" "$code_version" >&2
    return 1
  fi
}

require_go_test() {
  go test ./...
}

require_go_race() {
  if [[ "$RUN_RACE" != "1" && "$RUN_RACE" != "true" ]]; then
    skip_check "set MEGAVPN_SELF_TEST_RUN_RACE=1 to enable race detector"
    return 77
  fi
  go test -race ./...
}

require_go_vet() {
  go vet ./...
}

require_go_build() {
	go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin
}

require_binary_version_commands() {
	local code_version tmp bin out
	code_version="$(sed -nE 's/^const Version = "([^"]+)"/\1/p' internal/platform/version/version.go)"
	if [[ -z "$code_version" ]]; then
		printf 'unable to read internal/platform/version.Version\n' >&2
		return 1
	fi
	tmp="$(mktemp -d)"
	go build -o "$tmp/megavpn-api" ./cmd/api
	go build -o "$tmp/megavpn-worker" ./cmd/worker
	go build -o "$tmp/megavpn-agent" ./cmd/agent
	go build -o "$tmp/megavpn-migrate" ./cmd/migrate
	go build -o "$tmp/megavpn-admin" ./cmd/admin
	for bin in megavpn-api megavpn-worker megavpn-agent megavpn-migrate megavpn-admin; do
		out="$("$tmp/$bin" --version 2>&1)"
		if [[ "$out" != "$code_version" ]]; then
			printf '%s --version = %q, want %q\n' "$bin" "$out" "$code_version" >&2
			return 1
		fi
	done
}

require_shell_syntax() {
	scripts/ci/shell-scripts-audit.sh
}

require_actions_pinning() {
	scripts/ci/actions-pinning-check.sh
}

require_control_plane_install_validation() {
  MEGAVPN_CP_VALIDATE_ONLY=1 \
    MEGAVPN_CP_ASSUME_YES=1 \
    MEGAVPN_CP_TLS_MODE=self-signed-nginx \
    MEGAVPN_CP_DOMAIN=control.example.com \
    MEGAVPN_CP_PUBLIC_BASE_URL=https://control.example.com \
    MEGAVPN_CP_DATABASE_DSN='postgres://megavpn:password@127.0.0.1:5432/megavpn?sslmode=disable' \
    MEGAVPN_CP_ADMIN_PASSWORD='self-test-bootstrap-password' \
    MEGAVPN_CP_INSTALL_PACKAGES=0 \
    scripts/ops/control-plane-install.sh
}

require_frontend_js_syntax() {
  if ! command -v "$NODE_BIN" >/dev/null 2>&1; then
    skip_check "node is unavailable; JavaScript scripts audit was not run"
    return 77
  fi
  MEGAVPN_RELEASE_NODE_BIN="$NODE_BIN" scripts/ci/javascript-scripts-audit.sh
}

require_frontend_bootstrap_smoke() {
  if ! command -v "$NODE_BIN" >/dev/null 2>&1; then
    skip_check "node is unavailable; frontend bootstrap smoke was not run"
    return 77
  fi
  "$NODE_BIN" scripts/ci/frontend-bootstrap-smoke.js
}

require_install_web_wrapper_smoke() {
  scripts/ci/install-web-wrapper-smoke.sh
}

require_ops_script_safety_smoke() {
  scripts/ci/ops-script-safety-smoke.sh
}

require_service_pack_smoke_regression() {
  if ! command -v "$NODE_BIN" >/dev/null 2>&1; then
    skip_check "node is unavailable; service-pack smoke regression was not run"
    return 77
  fi
  "$NODE_BIN" scripts/ci/service-pack-smoke-regression.js
}

require_frontend_asset_manifest() {
  local ref path missing=0
  while IFS= read -r ref; do
    path="${ref%%\?*}"
    if [[ "$path" != assets/* ]]; then
      continue
    fi
    if [[ ! -f "web/$path" ]]; then
      printf 'missing web/%s\n' "$path"
      missing=1
    fi
  done < <(sed -nE 's/.*(src|href)="\.\/([^"]+)".*/\2/p' web/index.html)
  [[ "$missing" -eq 0 ]]
}

require_frontend_page_module_exports() {
  local module file export missing=0
  while IFS= read -r module; do
    [[ -n "$module" ]] || continue
    file="$(printf '%s\n' "$module" | sed -E 's/^MegaVPN//' | sed -E 's/Page$//' | sed -E 's/([a-z0-9])([A-Z])/\1-\2/g' | tr '[:upper:]' '[:lower:]')-page.js"
    case "$module" in
      MegaVPNOpsPages) file="ops-pages.js" ;;
    esac
    if [[ ! -f "web/assets/$file" ]]; then
      continue
    fi
    export="window.${module} = \\{ create:"
    if ! rg -q "$export" "web/assets/$file"; then
      printf 'web/assets/%s does not export window.%s.create\n' "$file" "$module" >&2
      missing=1
    fi
  done < <(sed -nE 's/.*window\.([A-Za-z0-9]+Page[s]?)\?\.create.*/\1/p' web/assets/app.js | sort -u)
  [[ "$missing" -eq 0 ]]
}

require_static_security_patterns() {
  local found=0
  require_command rg || return 1
  if rg -n "/bin/sh -c|StrictHostKeyChecking=accept-new" cmd internal scripts deploy --glob '!**/*_test.go' --glob '!scripts/release-gate.sh' --glob '!scripts/self-test.sh' --glob '!scripts/ci/release-gate.sh' --glob '!scripts/ci/self-test.sh'; then
    found=1
  fi
  if rg -n "curl .*\\|.*bash|bash -c .*curl" cmd internal scripts deploy --glob '!**/*_test.go' --glob '!scripts/release-gate.sh' --glob '!scripts/self-test.sh' --glob '!scripts/ci/release-gate.sh' --glob '!scripts/ci/self-test.sh'; then
    found=1
  fi
  [[ "$found" -eq 0 ]]
}

require_smoke_auth_coverage() {
  local file missing=0
  require_command rg || return 1
  while IFS= read -r file; do
    if ! rg -q '/api/v1' "$file"; then
      continue
    fi
    if rg -q 'source .*/lib/smoke\.sh|Authorization: Bearer' "$file"; then
      continue
    fi
    printf 'script calls /api/v1 without bearer auth support: %s\n' "$file" >&2
    missing=1
  done < <(find scripts -maxdepth 1 -type f \( -name '*smoke.sh' -o -name 'create-node-enrollment.sh' \) -print | sort)
  [[ "$missing" -eq 0 ]]
}

require_migration_sequence() {
  local numbers duplicates expected number file
  numbers="$(printf '%s\n' migrations/[0-9][0-9][0-9][0-9][0-9][0-9]_*.up.sql | sed -E 's#^migrations/([0-9]{6})_.*#\1#' | sort)"
  duplicates="$(printf '%s\n' "$numbers" | uniq -d)"
  if [[ -n "$duplicates" ]]; then
    printf 'duplicate migration numbers:\n%s\n' "$duplicates" >&2
    return 1
  fi

  expected=1
  while IFS= read -r file; do
    [[ -n "$file" ]] || continue
    number="$(printf '%s' "$file" | sed -E 's#^migrations/([0-9]{6})_.*#\1#')"
    if [[ $((10#$number)) -ne "$expected" ]]; then
      printf 'migration gap: expected %06d, got %s (%s)\n' "$expected" "$number" "$file" >&2
      return 1
    fi
    expected=$((expected + 1))
  done < <(printf '%s\n' migrations/[0-9][0-9][0-9][0-9][0-9][0-9]_*.up.sql | sort)
}

require_release_docs() {
  scripts/ci/docs-consistency.sh
}

require_postgres_migrations_and_integration() {
  if [[ -z "$RELEASE_DATABASE_DSN" ]]; then
    skip_check "set MEGAVPN_RELEASE_DATABASE_DSN to a disposable PostgreSQL database"
    return 77
  fi
  MEGAVPN_DATABASE_DSN="$RELEASE_DATABASE_DSN" \
    MEGAVPN_MIGRATION_DRILL_RUN_BACKUP_RESTORE=0 \
    scripts/ci/postgres-migration-drill.sh
  MEGAVPN_TEST_DATABASE_DSN="$RELEASE_DATABASE_DSN" go test ./internal/infra/postgres -run 'TestPostgresIntegration' -count=1
}

require_backup_restore_drill() {
  local backup_dir artifact_root archive
  if [[ -z "$RELEASE_DATABASE_DSN" || -z "$RELEASE_RESTORE_DATABASE_DSN" ]]; then
    skip_check "set MEGAVPN_RELEASE_DATABASE_DSN and MEGAVPN_RELEASE_RESTORE_DATABASE_DSN"
    return 77
  fi
  backup_dir="$(mktemp -d)"
  artifact_root="$(mktemp -d)"
  MEGAVPN_DATABASE_DSN="$RELEASE_DATABASE_DSN" \
    MEGAVPN_BACKUP_DIR="$backup_dir" \
    MEGAVPN_ARTIFACT_ROOT="$artifact_root" \
    scripts/ops/backup.sh
  archive="$(ls -t "$backup_dir"/megavpn-backup-*.tar.gz | head -n 1)"
  [[ -f "$archive" ]]
  MEGAVPN_RESTORE_CONFIRM=1 \
    MEGAVPN_DATABASE_DSN="$RELEASE_RESTORE_DATABASE_DSN" \
    MEGAVPN_ARTIFACT_ROOT="$artifact_root.restore" \
    scripts/ops/restore.sh "$archive"
}

require_systemd_verify() {
  if ! command -v systemd-analyze >/dev/null 2>&1; then
    skip_check "systemd-analyze is unavailable on this host"
    return 77
  fi
  systemd-analyze verify deploy/systemd/megavpn-api.service deploy/systemd/megavpn-worker.service deploy/systemd/megavpn-agent.service deploy/systemd/megavpn-migrate.service
}

require_nginx_verify() {
  if ! command -v nginx >/dev/null 2>&1; then
    skip_check "nginx is unavailable on this host"
    return 77
  fi
  if [[ -n "$NGINX_CONFIG" ]]; then
    nginx -t -c "$NGINX_CONFIG"
  else
    nginx -t
  fi
}

require_api_smoke() {
  if [[ -z "$RELEASE_BASE_URL" ]]; then
    skip_check "set MEGAVPN_RELEASE_BASE_URL or MEGAVPN_PUBLIC_BASE_URL"
    return 77
  fi
  require_command curl || return 1
  require_command jq || return 1
  MEGAVPN_PUBLIC_BASE_URL="$RELEASE_BASE_URL" scripts/smoke/api-smoke.sh
}

require_vpn_service_matrix() {
  local summary_file
  if [[ "$RUN_SERVICE_MATRIX" != "1" && "$RUN_SERVICE_MATRIX" != "true" ]]; then
    skip_check "set MEGAVPN_SELF_TEST_RUN_SERVICE_MATRIX=1 to run VPN/service matrix"
    return 77
  fi
  if [[ -z "$RELEASE_BASE_URL" || -z "$RELEASE_NODE_ID" || -z "$RELEASE_ENDPOINT_DOMAIN" ]]; then
    skip_check "set MEGAVPN_RELEASE_BASE_URL, MEGAVPN_RELEASE_NODE_ID and MEGAVPN_RELEASE_ENDPOINT_DOMAIN"
    return 77
  fi
  require_command curl || return 1
  require_command jq || return 1
  summary_file="${MEGAVPN_SMOKE_MATRIX_SUMMARY_FILE:-}"
  if [[ -z "$summary_file" && -n "${MEGAVPN_SMOKE_EVIDENCE_DIR:-}" ]]; then
    summary_file="${MEGAVPN_SMOKE_EVIDENCE_DIR%/}/_matrix-summary.json"
  fi
  MEGAVPN_PUBLIC_BASE_URL="$RELEASE_BASE_URL" \
    MEGAVPN_SMOKE_REQUIRE_AGENT_REPORT="$REQUIRE_AGENT_REPORT" \
    scripts/smoke/service-pack-smoke.sh --matrix "$RELEASE_NODE_ID" "$RELEASE_ENDPOINT_DOMAIN" "$RELEASE_CERTIFICATE_ID"
  if [[ -n "$summary_file" ]]; then
    [[ -f "$summary_file" ]] || {
      printf 'service matrix summary file was not written: %s\n' "$summary_file" >&2
      return 1
    }
    if ! command -v "$NODE_BIN" >/dev/null 2>&1; then
      printf 'node is unavailable; cannot validate service matrix evidence report\n' >&2
      return 1
    fi
    if [[ -n "$SERVICE_MATRIX_REQUIRED_PACKS" && ( "$SERVICE_MATRIX_REQUIRE_NO_SKIPS" == "1" || "$SERVICE_MATRIX_REQUIRE_NO_SKIPS" == "true" ) ]]; then
      "$NODE_BIN" scripts/ci/service-pack-evidence-report.js --require-pack "$SERVICE_MATRIX_REQUIRED_PACKS" --require-no-skips "$summary_file"
    elif [[ -n "$SERVICE_MATRIX_REQUIRED_PACKS" ]]; then
      "$NODE_BIN" scripts/ci/service-pack-evidence-report.js --require-pack "$SERVICE_MATRIX_REQUIRED_PACKS" "$summary_file"
    elif [[ "$SERVICE_MATRIX_REQUIRE_NO_SKIPS" == "1" || "$SERVICE_MATRIX_REQUIRE_NO_SKIPS" == "true" ]]; then
      "$NODE_BIN" scripts/ci/service-pack-evidence-report.js --require-no-skips "$summary_file"
    else
      "$NODE_BIN" scripts/ci/service-pack-evidence-report.js "$summary_file"
    fi
  fi
}

write_report() {
  local i
  {
    printf '# MegaVPN Self-Test Report\n\n'
    printf -- '- Generated UTC: `%s`\n' "$REPORT_TS"
    printf -- '- Workspace: `%s`\n' "$ROOT_DIR"
    printf -- '- GOCACHE: `%s`\n' "$GOCACHE"
    printf -- '- GOTMPDIR: `%s`\n' "$GOTMPDIR"
    printf -- '- GOMODCACHE: `%s`\n' "$GOMODCACHE"
    printf '\n'
    printf '## Summary\n\n'
    printf '| Status | Count |\n'
    printf '| --- | ---: |\n'
    printf '| PASS | %d |\n' "$passed"
    printf '| FAIL | %d |\n' "$failed"
    printf '| SKIP | %d |\n' "$skipped"
    printf '\n'
    printf '## Gates\n\n'
    printf '| Status | Gate | Evidence |\n'
    printf '| --- | --- | --- |\n'
    for ((i = 0; i < ${#gate_names[@]}; i++)); do
      printf '| %s | %s | %s |\n' \
        "$(escape_md "${gate_statuses[$i]}")" \
        "$(escape_md "${gate_names[$i]}")" \
        "$(escape_md "${gate_evidence[$i]}")"
    done
    printf '\n'
    if [[ "$skipped" -gt 0 ]]; then
      printf '## Not Tested In This Run\n\n'
      printf 'Skipped gates are not release evidence. Provide the required host tools, disposable databases, live control-plane URL, authenticated token and disposable test node, then rerun this script.\n'
    fi
  } >"$REPORT_FILE"
}

run_check "gofmt-clean" "Go source formatting is clean" require_gofmt_clean
run_check "version-tag-consistency" "HEAD tag matches internal/platform/version.Version when tagged" require_version_tag_consistency
run_check "go-test" "All Go package tests pass" require_go_test
run_check "go-test-race" "All Go package tests pass under race detector" require_go_race
run_check "go-vet" "Go vet reports no issues" require_go_vet
run_check "go-build" "API, worker, agent and migrate binaries build" require_go_build
run_check "binary-version-commands" "All operational binaries print version and exit without runtime startup" require_binary_version_commands
run_check "shell-syntax" "Shell scripts parse under bash -n" require_shell_syntax
run_check "actions-pinning" "GitHub Actions use pinned commit SHA refs" require_actions_pinning
run_check "control-plane-install-validation" "Control Plane installer validates non-interactive clean-install inputs" require_control_plane_install_validation
run_check "frontend-js-syntax" "Web UI and operational JavaScript parse under node --check" require_frontend_js_syntax
run_check "frontend-bootstrap-smoke" "Static Web UI assets bootstrap with browser-like runtime dependencies" require_frontend_bootstrap_smoke
run_check "install-web-wrapper-smoke" "Deploy Web UI wrapper copies static frontend assets from the repository root" require_install_web_wrapper_smoke
run_check "ops-script-safety-smoke" "Destructive ops scripts reject unsafe targets and malicious archives" require_ops_script_safety_smoke
run_check "service-pack-smoke-regression" "Service-pack smoke script handles matrix planning, provision apply, artifacts and cleanup against a mock API" require_service_pack_smoke_regression
run_check "frontend-asset-manifest" "Static Web UI index references only existing assets" require_frontend_asset_manifest
run_check "frontend-page-module-exports" "Static Web UI page modules export the create contract expected by app.js" require_frontend_page_module_exports
run_check "static-security-patterns" "No banned production command patterns are present" require_static_security_patterns
run_check "smoke-auth-coverage" "Smoke scripts that call protected API endpoints support bearer auth" require_smoke_auth_coverage
run_check "migration-sequence" "SQL migration numbers are unique and gap-free" require_migration_sequence
run_check "docs-consistency" "Release, security, RBAC, operations and env-template documents are synchronized" require_release_docs
run_check "postgres-migrations-and-integration" "Zero-database migration drill, idempotent re-run and PostgreSQL integration tests pass on disposable DB" require_postgres_migrations_and_integration
run_check "backup-restore-drill" "Backup archive restores into a separate disposable DB" require_backup_restore_drill
run_check "systemd-verify" "Systemd unit files verify on a systemd host" require_systemd_verify
run_check "nginx-t" "Nginx configuration validates on target host" require_nginx_verify
run_check "api-smoke" "Health, readiness and authenticated API smoke pass" require_api_smoke
run_check "vpn-service-smoke-matrix" "OpenVPN, WireGuard, Xray, HTTP Proxy, MTProto, Shadowsocks and IPsec/L2TP smoke matrix passes" require_vpn_service_matrix

write_report

log "report: $REPORT_FILE"
log "completed: passed=$passed failed=$failed skipped=$skipped"

if [[ "$failed" -gt 0 ]]; then
  exit 1
fi
