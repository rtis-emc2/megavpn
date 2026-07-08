#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/smoke.sh
source "$SCRIPT_DIR/../lib/smoke.sh"

BASE_URL="${MEGAVPN_PUBLIC_BASE_URL:-${MEGAVPN_API_URL:-}}"
GROUP_ID="${MEGAVPN_VLESS_SMOKE_GROUP_ID:-}"
CLIENT_REF="${MEGAVPN_VLESS_SMOKE_CLIENT_REF:-}"
APPLY="${MEGAVPN_VLESS_SMOKE_APPLY:-0}"

smoke_require curl
smoke_require jq

if [[ -z "$BASE_URL" ]]; then
  echo "SKIP: set MEGAVPN_PUBLIC_BASE_URL or MEGAVPN_API_URL for a disposable/local API." >&2
  exit 0
fi

if [[ -z "${MEGAVPN_AUTH_TOKEN:-}" ]]; then
  echo "SKIP: set MEGAVPN_AUTH_TOKEN for protected client access group endpoints." >&2
  exit 0
fi

echo "checking API readiness"
smoke_curl "$BASE_URL/health" | jq .
smoke_curl "$BASE_URL/api/v1/ready" | jq .

echo "checking VLESS service capability"
smoke_curl "$BASE_URL/api/v1/client-access-services" | jq -e '.[] | select(.service_code=="vless" and .supports_groups==true and .supports_membership==true and .supports_materialization==true)' >/dev/null

echo "listing VLESS groups"
smoke_curl "$BASE_URL/api/v1/client-access-groups?service_code=vless" | jq .

if [[ -z "$GROUP_ID" ]]; then
  echo "SKIP: set MEGAVPN_VLESS_SMOKE_GROUP_ID to exercise group members/scope/sync endpoints." >&2
  exit 0
fi

echo "reading group members, scope and sync state for $GROUP_ID"
smoke_curl "$BASE_URL/api/v1/client-access-groups/$GROUP_ID/members?limit=25" | jq .
smoke_curl "$BASE_URL/api/v1/client-access-groups/available-clients?group_id=$GROUP_ID&service_code=vless&assignment=all&limit=25" | jq .
smoke_curl "$BASE_URL/api/v1/client-access-groups/$GROUP_ID/scope" | jq .
smoke_curl "$BASE_URL/api/v1/client-access-groups/$GROUP_ID/sync-state" | jq .

echo "running sync preview"
smoke_json_request POST "$BASE_URL/api/v1/client-access-groups/$GROUP_ID/sync:preview" '{}' | jq .

if [[ -z "$CLIENT_REF" ]]; then
  echo "SKIP: set MEGAVPN_VLESS_SMOKE_CLIENT_REF to run membership preview/apply against disposable data." >&2
  exit 0
fi

preview_payload="$(jq -cn --arg ref "$CLIENT_REF" '{client_refs:[$ref],mode:"add_only",queue_apply:true,dry_run:true}')"
echo "running membership preview for client ref"
smoke_json_request POST "$BASE_URL/api/v1/client-access-groups/$GROUP_ID/members:preview" "$preview_payload" | jq .

if [[ "$APPLY" != "1" ]]; then
  echo "SKIP: set MEGAVPN_VLESS_SMOKE_APPLY=1 to apply previewed membership changes in disposable data." >&2
  exit 0
fi

apply_payload="$(jq -cn --arg ref "$CLIENT_REF" '{client_refs:[$ref],mode:"add_only",queue_apply:true,dry_run:false}')"
echo "applying membership changes"
smoke_json_request POST "$BASE_URL/api/v1/client-access-groups/$GROUP_ID/members:bulk-apply" "$apply_payload" | jq .
