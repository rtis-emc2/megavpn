#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-/tmp/megavpn-go-cache}"
export GOTMPDIR="${GOTMPDIR:-/tmp/megavpn-go-tmp}"
mkdir -p "$GOCACHE" "$GOTMPDIR"

RELEASE_DATABASE_DSN="${MEGAVPN_RELEASE_DATABASE_DSN:-${MEGAVPN_TEST_DATABASE_DSN:-}}"
RELEASE_RESTORE_DATABASE_DSN="${MEGAVPN_RELEASE_RESTORE_DATABASE_DSN:-}"
RELEASE_BASE_URL="${MEGAVPN_RELEASE_BASE_URL:-${MEGAVPN_PUBLIC_BASE_URL:-}}"
RELEASE_NODE_ID="${MEGAVPN_RELEASE_NODE_ID:-}"
RELEASE_ENDPOINT_DOMAIN="${MEGAVPN_RELEASE_ENDPOINT_DOMAIN:-}"
RELEASE_CERTIFICATE_ID="${MEGAVPN_RELEASE_CERTIFICATE_ID:-}"
RUN_RACE="${MEGAVPN_RELEASE_RUN_RACE:-1}"
RUN_SERVICE_MATRIX="${MEGAVPN_RELEASE_RUN_SERVICE_MATRIX:-0}"
ALLOW_SKIPS="${MEGAVPN_RELEASE_ALLOW_SKIPS:-0}"

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
  if rg -n "curl .*\\|.*bash|bash -c .*curl" cmd internal scripts deploy --glob '!**/*_test.go' --glob '!scripts/release-gate.sh' --glob '!scripts/self-test.sh'; then
    log "unpinned curl-to-shell production pattern found"
    return 1
  fi
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
  MEGAVPN_PUBLIC_BASE_URL="$RELEASE_BASE_URL" scripts/alpha-smoke.sh
}

run_service_matrix() {
  MEGAVPN_PUBLIC_BASE_URL="$RELEASE_BASE_URL" scripts/service-pack-smoke.sh --matrix "$RELEASE_NODE_ID" "$RELEASE_ENDPOINT_DOMAIN" "$RELEASE_CERTIFICATE_ID"
}

run_gate "gofmt-clean" require_gofmt_clean
run_gate "version-tag-consistency" require_version_tag_consistency
run_gate "go-test" go test ./...
if [[ "$RUN_RACE" == "1" || "$RUN_RACE" == "true" ]]; then
  run_gate "go-test-race" go test -race ./...
else
  skip_gate "go-test-race" "MEGAVPN_RELEASE_RUN_RACE=$RUN_RACE"
fi
run_gate "go-build" go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate
run_gate "shell-syntax" require_shell_syntax
run_gate "smoke-auth-coverage" require_smoke_auth_coverage
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
