#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

fail_if_matches() {
  local description="$1"
  shift
  local output rc
  set +e
  output="$("$@" 2>/dev/null)"
  rc=$?
  set -e
  if [[ "$rc" -eq 0 && -n "$output" ]]; then
    printf '%s\n%s\n' "$description" "$output" >&2
    exit 1
  fi
  if [[ "$rc" -ne 0 && "$rc" -ne 1 ]]; then
    printf 'guard command failed for %s with exit=%d\n' "$description" "$rc" >&2
    exit "$rc"
  fi
}

command -v rg >/dev/null 2>&1 || {
  printf 'missing command: rg\n' >&2
  exit 1
}

fail_if_matches \
  'raw /api/v1 usage is allowed only in the shared API module and tests' \
  rg -n '/api/v1' frontend/src \
    --glob '!frontend/src/shared/api/**' \
    --glob '!**/*.test.*'

fail_if_matches \
  'direct fetch usage is allowed only in the same-origin API client' \
  rg -n '\bfetch\(' frontend/src \
    --glob '!frontend/src/shared/api/client.ts' \
    --glob '!**/*.test.*'

fail_if_matches \
  'authentication and secret material must not use browser storage' \
  rg -n '(localStorage|sessionStorage).*(auth|bearer|password|session|token)|(auth|bearer|password|session|token).*(localStorage|sessionStorage)' frontend/src \
    --glob '!**/*.test.*'

fail_if_matches \
  'unreviewed HTML injection is forbidden' \
  rg -n 'dangerouslySetInnerHTML' frontend/src \
    --glob '!**/*.test.*'

fail_if_matches \
  'runtime inline styles are forbidden by the production CSP' \
  rg -n '\bstyle=|\.style\.' frontend/src \
    --glob '!**/*.test.*'

fail_if_matches \
  'production frontend must not log API responses or secrets' \
  rg -n 'console\.(log|debug|info|warn|error)' frontend/src \
    --glob '!**/*.test.*'

fail_if_matches \
  'legacy frontend references are forbidden' \
  rg -n '(/legacy/|Legacy UI|legacy frontend)' frontend/src \
    --glob '!**/*.test.*'

printf 'frontend static guards ok\n'
