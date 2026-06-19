#!/usr/bin/env bash
set -euo pipefail
BASE_URL="${MEGAVPN_PUBLIC_BASE_URL:-http://127.0.0.1:8080}"
AUTH_TOKEN="${MEGAVPN_AUTH_TOKEN:-}"

curl -fsS "$BASE_URL/health" | jq
curl -fsS "$BASE_URL/api/v1/ready" | jq

if [[ -z "$AUTH_TOKEN" ]]; then
  echo "MEGAVPN_AUTH_TOKEN is not set; skipping authenticated API smoke checks." >&2
  exit 0
fi

curl -fsS -H "Authorization: Bearer $AUTH_TOKEN" "$BASE_URL/api/v1/dashboard" | jq
curl -fsS -H "Authorization: Bearer $AUTH_TOKEN" "$BASE_URL/api/v1/services" | jq '.[].code'
curl -fsS -H "Authorization: Bearer $AUTH_TOKEN" "$BASE_URL/api/v1/nodes" | jq
curl -fsS -H "Authorization: Bearer $AUTH_TOKEN" "$BASE_URL/api/v1/platform/pki-roots" | jq
curl -fsS -H "Authorization: Bearer $AUTH_TOKEN" "$BASE_URL/api/v1/jobs" | jq
curl -fsS -H "Authorization: Bearer $AUTH_TOKEN" "$BASE_URL/api/v1/audit" | jq
