#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-/tmp/megavpn-go-cache}"
export GOTMPDIR="${GOTMPDIR:-/tmp/megavpn-go-tmp}"
mkdir -p "$GOCACHE" "$GOTMPDIR"

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
NGINX_CONFIG="${MEGAVPN_SELF_TEST_NGINX_CONFIG:-}"

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
	go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate
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
	local file
	while IFS= read -r file; do
		bash -n "$file"
	done < <(find scripts -type f -name '*.sh' -print | sort)
}

require_frontend_js_syntax() {
  local file
  if ! command -v node >/dev/null 2>&1; then
    skip_check "node is unavailable; web/assets/*.js syntax check was not run"
    return 77
  fi
  for file in web/assets/*.js; do
    node --check "$file"
  done
}

require_static_security_patterns() {
  local found=0
  require_command rg || return 1
  if rg -n "/bin/sh -c|StrictHostKeyChecking=accept-new" cmd internal scripts deploy --glob '!**/*_test.go' --glob '!scripts/release-gate.sh' --glob '!scripts/self-test.sh'; then
    found=1
  fi
  if rg -n "curl .*\\|.*bash|bash -c .*curl" cmd internal scripts deploy --glob '!**/*_test.go' --glob '!scripts/release-gate.sh' --glob '!scripts/self-test.sh'; then
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
  local file
  for file in \
    docs/RELEASE_GATES.md \
    docs/SELF_TESTING.md \
    docs/THREAT_MODEL.md \
    docs/RBAC_MATRIX.md \
    docs/OPERATIONS_RUNBOOK.md \
    deploy/env/megavpn.production.env.example \
    deploy/env/megavpn-agent.production.env.example; do
    if [[ ! -s "$file" ]]; then
      printf 'missing or empty required release artifact: %s\n' "$file" >&2
      return 1
    fi
  done
}

require_postgres_migrations_and_integration() {
  if [[ -z "$RELEASE_DATABASE_DSN" ]]; then
    skip_check "set MEGAVPN_RELEASE_DATABASE_DSN to a disposable PostgreSQL database"
    return 77
  fi
  MEGAVPN_DATABASE_DSN="$RELEASE_DATABASE_DSN" go run ./cmd/migrate
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
    scripts/backup.sh
  archive="$(ls -t "$backup_dir"/megavpn-backup-*.tar.gz | head -n 1)"
  [[ -f "$archive" ]]
  MEGAVPN_RESTORE_CONFIRM=1 \
    MEGAVPN_DATABASE_DSN="$RELEASE_RESTORE_DATABASE_DSN" \
    MEGAVPN_ARTIFACT_ROOT="$artifact_root.restore" \
    scripts/restore.sh "$archive"
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
  MEGAVPN_PUBLIC_BASE_URL="$RELEASE_BASE_URL" scripts/alpha-smoke.sh
}

require_vpn_service_matrix() {
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
  MEGAVPN_PUBLIC_BASE_URL="$RELEASE_BASE_URL" scripts/service-pack-smoke.sh --matrix "$RELEASE_NODE_ID" "$RELEASE_ENDPOINT_DOMAIN" "$RELEASE_CERTIFICATE_ID"
}

write_report() {
  local i
  {
    printf '# MegaVPN Self-Test Report\n\n'
    printf -- '- Generated UTC: `%s`\n' "$REPORT_TS"
    printf -- '- Workspace: `%s`\n' "$ROOT_DIR"
    printf -- '- GOCACHE: `%s`\n' "$GOCACHE"
    printf -- '- GOTMPDIR: `%s`\n' "$GOTMPDIR"
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
run_check "frontend-js-syntax" "Static Web UI JavaScript parses under node --check" require_frontend_js_syntax
run_check "static-security-patterns" "No banned production command patterns are present" require_static_security_patterns
run_check "smoke-auth-coverage" "Smoke scripts that call protected API endpoints support bearer auth" require_smoke_auth_coverage
run_check "migration-sequence" "SQL migration numbers are unique and gap-free" require_migration_sequence
run_check "release-docs" "Release, security, RBAC, operations and env-template documents exist" require_release_docs
run_check "postgres-migrations-and-integration" "Migrations and PostgreSQL integration tests pass on disposable DB" require_postgres_migrations_and_integration
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
