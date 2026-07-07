#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-/tmp/megavpn-go-cache}"
export GOTMPDIR="${GOTMPDIR:-/tmp/megavpn-go-tmp}"
RELEASE_GATE_TEMP_GOMODCACHE=""
if [[ -n "${GOMODCACHE:-}" ]]; then
  export GOMODCACHE
else
  RELEASE_GATE_TEMP_GOMODCACHE="$(mktemp -d "${TMPDIR:-/tmp}/megavpn-go-modcache.XXXXXX")"
  export GOMODCACHE="$RELEASE_GATE_TEMP_GOMODCACHE"
fi
cleanup_release_gate_tmp() {
  if [[ -n "$RELEASE_GATE_TEMP_GOMODCACHE" && -d "$RELEASE_GATE_TEMP_GOMODCACHE" ]]; then
    chmod -R u+w "$RELEASE_GATE_TEMP_GOMODCACHE" 2>/dev/null || true
    rm -rf "$RELEASE_GATE_TEMP_GOMODCACHE"
  fi
}
trap cleanup_release_gate_tmp EXIT
mkdir -p "$GOCACHE" "$GOTMPDIR" "$GOMODCACHE"

RELEASE_DATABASE_DSN="${MEGAVPN_RELEASE_DATABASE_DSN:-${MEGAVPN_TEST_DATABASE_DSN:-}}"
RELEASE_RESTORE_DATABASE_DSN="${MEGAVPN_RELEASE_RESTORE_DATABASE_DSN:-}"
RELEASE_BASE_URL="${MEGAVPN_RELEASE_BASE_URL:-${MEGAVPN_PUBLIC_BASE_URL:-}}"
RELEASE_NODE_ID="${MEGAVPN_RELEASE_NODE_ID:-}"
RELEASE_ENDPOINT_DOMAIN="${MEGAVPN_RELEASE_ENDPOINT_DOMAIN:-}"
RELEASE_CERTIFICATE_ID="${MEGAVPN_RELEASE_CERTIFICATE_ID:-}"
RUN_RACE="${MEGAVPN_RELEASE_RUN_RACE:-1}"
RUN_SERVICE_MATRIX="${MEGAVPN_RELEASE_RUN_SERVICE_MATRIX:-0}"
REQUIRE_AGENT_REPORT="${MEGAVPN_RELEASE_REQUIRE_AGENT_REPORT:-1}"
SERVICE_MATRIX_REQUIRED_PACKS="${MEGAVPN_RELEASE_SERVICE_MATRIX_REQUIRED_PACKS:-}"
SERVICE_MATRIX_REQUIRE_NO_SKIPS="${MEGAVPN_RELEASE_SERVICE_MATRIX_REQUIRE_NO_SKIPS:-0}"
ALLOW_SKIPS="${MEGAVPN_RELEASE_ALLOW_SKIPS:-0}"
GOVULNCHECK_VERSION="${MEGAVPN_RELEASE_GOVULNCHECK_VERSION:-v1.5.0}"
NODE_BIN="${MEGAVPN_RELEASE_NODE_BIN:-node}"

passed=0
skipped=0

log() {
  printf '[release-gate] %s\n' "$*"
}

run_gate() {
  local name="$1"
  shift
  log "RUN $name"
  "$@"
  passed=$((passed + 1))
  log "PASS $name"
}

skip_gate() {
  local name="$1"
  local reason="$2"
  skipped=$((skipped + 1))
  log "SKIP $name: $reason"
}

require_clean_static_scan() {
  if rg -n "/bin/sh -c|StrictHostKeyChecking=accept-new" cmd internal scripts deploy --glob '!**/*_test.go' --glob '!scripts/release-gate.sh' --glob '!scripts/self-test.sh'; then
    log "unsafe production pattern found"
    return 1
  fi
  if rg -n "curl .*\\|.*(bash|sh|gpg|apt-key)|bash\", \"-c\", \"curl|sh\", \"-c\", \"curl|apt-key" cmd internal scripts deploy --glob '!**/*_test.go' --glob '!scripts/release-gate.sh' --glob '!scripts/self-test.sh'; then
    log "unpinned curl pipe or apt key production pattern found"
    return 1
  fi
  if rg -U -n ";[[:space:]]*\\n[[:space:]]*(insert|update|delete|create|alter|drop)\\b" cmd internal --glob '!**/*_test.go' --glob '!cmd/migrate/main.go'; then
    log "multi-command SQL found in production Go runtime path"
    return 1
  fi
}

require_govulncheck() {
  go run "golang.org/x/vuln/cmd/govulncheck@$GOVULNCHECK_VERSION" ./...
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
		log "unable to read internal/platform/version.Version"
		return 1
	fi
	head_tag="$(git tag --points-at HEAD | sort -V | tail -n 1)"
	if [[ -z "$head_tag" ]]; then
		log "HEAD is not tagged; code version is $code_version"
		return 0
	fi
	if [[ "$head_tag" != "v$code_version" ]]; then
		log "HEAD tag $head_tag does not match code version v$code_version"
		return 1
	fi
}

require_shell_syntax() {
  local file
  while IFS= read -r file; do
    bash -n "$file"
  done < <(find scripts -type f -name '*.sh' -print | sort)
}

require_frontend_js_syntax() {
  find web/assets -maxdepth 1 -name '*.js' -print0 | xargs -0 -n1 "$NODE_BIN" --check
}

run_frontend_bootstrap_smoke() {
  "$NODE_BIN" scripts/frontend-bootstrap-smoke.js
}

run_service_pack_smoke_regression() {
  "$NODE_BIN" scripts/service-pack-smoke-regression.js
}

require_binary_version_commands() {
	local code_version tmp bin out
	code_version="$(sed -nE 's/^const Version = "([^"]+)"/\1/p' internal/platform/version/version.go)"
	if [[ -z "$code_version" ]]; then
		log "unable to read internal/platform/version.Version"
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

require_docs_consistency() {
  scripts/docs-consistency.sh
}

require_control_plane_install_validation() {
  MEGAVPN_CP_VALIDATE_ONLY=1 \
    MEGAVPN_CP_ASSUME_YES=1 \
    MEGAVPN_CP_TLS_MODE=self-signed-nginx \
    MEGAVPN_CP_DOMAIN=control.example.com \
    MEGAVPN_CP_PUBLIC_BASE_URL=https://control.example.com \
    MEGAVPN_CP_DATABASE_DSN='postgres://megavpn:password@127.0.0.1:5432/megavpn?sslmode=disable' \
    MEGAVPN_CP_ADMIN_PASSWORD='release-gate-bootstrap-password' \
    MEGAVPN_CP_INSTALL_PACKAGES=0 \
    scripts/control-plane-install.sh
}

require_smoke_auth_coverage() {
  local file missing=0
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

run_migrations_and_integration() {
  MEGAVPN_DATABASE_DSN="$RELEASE_DATABASE_DSN" go run ./cmd/migrate
  MEGAVPN_TEST_DATABASE_DSN="$RELEASE_DATABASE_DSN" go test ./internal/infra/postgres -run 'TestPostgresIntegration' -count=1
}

run_backup_restore_drill() {
  local backup_dir artifact_root archive
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

run_systemd_verify() {
  systemd-analyze verify deploy/systemd/megavpn-api.service deploy/systemd/megavpn-worker.service deploy/systemd/megavpn-agent.service deploy/systemd/megavpn-migrate.service
}

run_nginx_verify() {
  nginx -t
}

run_api_smoke() {
  MEGAVPN_PUBLIC_BASE_URL="$RELEASE_BASE_URL" scripts/api-smoke.sh
}

run_service_matrix() {
  local summary_file
  summary_file="${MEGAVPN_SMOKE_MATRIX_SUMMARY_FILE:-}"
  if [[ -z "$summary_file" && -n "${MEGAVPN_SMOKE_EVIDENCE_DIR:-}" ]]; then
    summary_file="${MEGAVPN_SMOKE_EVIDENCE_DIR%/}/_matrix-summary.json"
  fi
  MEGAVPN_PUBLIC_BASE_URL="$RELEASE_BASE_URL" \
    MEGAVPN_SMOKE_REQUIRE_AGENT_REPORT="$REQUIRE_AGENT_REPORT" \
    scripts/service-pack-smoke.sh --matrix "$RELEASE_NODE_ID" "$RELEASE_ENDPOINT_DOMAIN" "$RELEASE_CERTIFICATE_ID"
  if [[ -n "$summary_file" ]]; then
    [[ -f "$summary_file" ]] || {
      log "service matrix summary file was not written: $summary_file"
      return 1
    }
    command -v "$NODE_BIN" >/dev/null 2>&1 || {
      log "node is unavailable; cannot validate service matrix evidence report"
      return 1
    }
    if [[ -n "$SERVICE_MATRIX_REQUIRED_PACKS" && ( "$SERVICE_MATRIX_REQUIRE_NO_SKIPS" == "1" || "$SERVICE_MATRIX_REQUIRE_NO_SKIPS" == "true" ) ]]; then
      "$NODE_BIN" scripts/service-pack-evidence-report.js --require-pack "$SERVICE_MATRIX_REQUIRED_PACKS" --require-no-skips "$summary_file"
    elif [[ -n "$SERVICE_MATRIX_REQUIRED_PACKS" ]]; then
      "$NODE_BIN" scripts/service-pack-evidence-report.js --require-pack "$SERVICE_MATRIX_REQUIRED_PACKS" "$summary_file"
    elif [[ "$SERVICE_MATRIX_REQUIRE_NO_SKIPS" == "1" || "$SERVICE_MATRIX_REQUIRE_NO_SKIPS" == "true" ]]; then
      "$NODE_BIN" scripts/service-pack-evidence-report.js --require-no-skips "$summary_file"
    else
      "$NODE_BIN" scripts/service-pack-evidence-report.js "$summary_file"
    fi
  fi
}

run_gate "gofmt-clean" require_gofmt_clean
run_gate "version-tag-consistency" require_version_tag_consistency
run_gate "go-test" go test ./...
run_gate "govulncheck" require_govulncheck
if [[ "$RUN_RACE" == "1" || "$RUN_RACE" == "true" ]]; then
  run_gate "go-test-race" go test -race ./...
else
  skip_gate "go-test-race" "MEGAVPN_RELEASE_RUN_RACE=$RUN_RACE"
fi
run_gate "go-build" go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin
run_gate "binary-version-commands" require_binary_version_commands
run_gate "shell-syntax" require_shell_syntax
run_gate "docs-consistency" require_docs_consistency
run_gate "control-plane-install-validation" require_control_plane_install_validation
run_gate "smoke-auth-coverage" require_smoke_auth_coverage
if command -v "$NODE_BIN" >/dev/null 2>&1; then
  run_gate "frontend-js-syntax" require_frontend_js_syntax
  run_gate "frontend-bootstrap-smoke" run_frontend_bootstrap_smoke
  run_gate "service-pack-smoke-regression" run_service_pack_smoke_regression
else
  skip_gate "frontend-js-syntax" "set MEGAVPN_RELEASE_NODE_BIN or install node"
  skip_gate "frontend-bootstrap-smoke" "set MEGAVPN_RELEASE_NODE_BIN or install node"
  skip_gate "service-pack-smoke-regression" "set MEGAVPN_RELEASE_NODE_BIN or install node"
fi
run_gate "static-security-patterns" require_clean_static_scan

if [[ -n "$RELEASE_DATABASE_DSN" ]]; then
  run_gate "postgres-migrations-and-integration" run_migrations_and_integration
else
  skip_gate "postgres-migrations-and-integration" "set MEGAVPN_RELEASE_DATABASE_DSN to a disposable database"
fi

if [[ -n "$RELEASE_DATABASE_DSN" && -n "$RELEASE_RESTORE_DATABASE_DSN" ]]; then
  run_gate "backup-restore-drill" run_backup_restore_drill
else
  skip_gate "backup-restore-drill" "set MEGAVPN_RELEASE_DATABASE_DSN and MEGAVPN_RELEASE_RESTORE_DATABASE_DSN"
fi

if command -v systemd-analyze >/dev/null 2>&1; then
  run_gate "systemd-verify" run_systemd_verify
else
  skip_gate "systemd-verify" "systemd-analyze is unavailable"
fi

if command -v nginx >/dev/null 2>&1; then
  run_gate "nginx-t" run_nginx_verify
else
  skip_gate "nginx-t" "nginx is unavailable"
fi

if [[ -n "$RELEASE_BASE_URL" ]]; then
  run_gate "api-smoke" run_api_smoke
else
  skip_gate "api-smoke" "set MEGAVPN_RELEASE_BASE_URL or MEGAVPN_PUBLIC_BASE_URL"
fi

if [[ "$RUN_SERVICE_MATRIX" == "1" || "$RUN_SERVICE_MATRIX" == "true" ]]; then
  if [[ -n "$RELEASE_BASE_URL" && -n "$RELEASE_NODE_ID" && -n "$RELEASE_ENDPOINT_DOMAIN" ]]; then
    run_gate "vpn-service-smoke-matrix" run_service_matrix
  else
    skip_gate "vpn-service-smoke-matrix" "set MEGAVPN_RELEASE_BASE_URL, MEGAVPN_RELEASE_NODE_ID, MEGAVPN_RELEASE_ENDPOINT_DOMAIN"
  fi
else
  skip_gate "vpn-service-smoke-matrix" "set MEGAVPN_RELEASE_RUN_SERVICE_MATRIX=1"
fi

log "completed: passed=$passed skipped=$skipped"
if [[ "$skipped" -gt 0 && "$ALLOW_SKIPS" != "1" && "$ALLOW_SKIPS" != "true" ]]; then
  log "failed: skipped release gates are not production release evidence; set MEGAVPN_RELEASE_ALLOW_SKIPS=1 only for local diagnostics"
  exit 1
fi
