#!/usr/bin/env bash

smoke_require() {
  command -v "$1" >/dev/null 2>&1 || {
    printf 'missing command: %s\n' "$1" >&2
    return 1
  }
}

smoke_auth_args=()
if [[ -n "${MEGAVPN_AUTH_TOKEN:-}" ]]; then
  smoke_auth_args=(-H "Authorization: Bearer ${MEGAVPN_AUTH_TOKEN}")
fi

smoke_curl() {
  curl -fsS "${smoke_auth_args[@]}" "$@"
}

smoke_json_request() {
  local method="$1"
  local url="$2"
  local body="${3:-}"
  if [[ -n "$body" ]]; then
    smoke_curl -X "$method" -H 'Content-Type: application/json' -d "$body" "$url"
  else
    smoke_curl -X "$method" "$url"
  fi
}
