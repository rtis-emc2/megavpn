#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

require_rg() {
  command -v rg >/dev/null 2>&1 || {
    printf 'missing command: rg\n' >&2
    exit 1
  }
}

fail_if_matches() {
  local description="$1"
  shift
  local output
  if output="$("$@" 2>/dev/null)"; then
    if [[ -n "$output" ]]; then
      printf '%s\n%s\n' "$description" "$output" >&2
      exit 1
    fi
  else
    local rc=$?
    if [[ "$rc" -ne 1 ]]; then
      printf 'guard command failed for %s with exit=%d\n' "$description" "$rc" >&2
      exit "$rc"
    fi
  fi
}

require_rg

fail_if_matches \
  'raw /api/v1 usage is allowed only in frontend/src/shared/api and tests' \
  rg -n '/api/v1' frontend/src \
    --glob '!frontend/src/shared/api/**' \
    --glob '!**/*.test.*' \
    --glob '!test/**'

fail_if_matches \
  'auth/session/token material must not be stored in browser storage' \
  rg -n '(localStorage|sessionStorage).*(auth|bearer|password|session|token)|(auth|bearer|password|session|token).*(localStorage|sessionStorage)' frontend/src \
    --glob '!**/*.test.*'

fail_if_matches \
  'dangerouslySetInnerHTML requires a reviewed allowlist component; no usage is currently approved' \
  rg -n 'dangerouslySetInnerHTML' frontend/src \
    --glob '!**/*.test.*'

fail_if_matches \
  'production frontend code must not log API responses or secrets to browser console' \
  rg -n 'console\\.(log|debug|info|warn|error)' frontend/src \
    --glob '!**/*.test.*' \
    --glob '!test/**'

printf 'frontend static guards ok\n'
