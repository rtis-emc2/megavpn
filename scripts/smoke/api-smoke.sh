#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/smoke.sh
source "$SCRIPT_DIR/../lib/smoke.sh"
BASE_URL="${MEGAVPN_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
AUTH_TOKEN="${MEGAVPN_AUTH_TOKEN:-}"
smoke_require curl
smoke_require jq

smoke_curl "$BASE_URL/health" | jq
smoke_curl "$BASE_URL/api/v1/ready" | jq

if [[ -z "$AUTH_TOKEN" ]]; then
  echo "MEGAVPN_AUTH_TOKEN is not set; skipping authenticated API smoke checks." >&2
  exit 0
fi

smoke_curl "$BASE_URL/api/v1/dashboard" | jq
smoke_curl "$BASE_URL/api/v1/services" | jq '.[].code'
smoke_curl "$BASE_URL/api/v1/nodes" | jq
smoke_curl "$BASE_URL/api/v1/platform/pki-roots" | jq
smoke_curl "$BASE_URL/api/v1/jobs" | jq
smoke_curl "$BASE_URL/api/v1/audit" | jq
