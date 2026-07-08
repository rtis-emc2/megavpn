#!/usr/bin/env bash
set -euo pipefail

KEY_PATH="${MEGAVPN_MASTER_KEY_PATH:-/etc/megavpn/master.key}"
FORCE="${MEGAVPN_MASTER_KEY_FORCE:-0}"

die() {
  printf '[master-key] ERROR: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

is_enabled() {
  case "${1,,}" in
    1|true|yes|y|on)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

require_command openssl

if [[ -e "$KEY_PATH" ]] && ! is_enabled "$FORCE"; then
  die "master key already exists: $KEY_PATH (set MEGAVPN_MASTER_KEY_FORCE=1 to overwrite)"
fi

install -d -m 0750 "$(dirname "$KEY_PATH")"
tmp="${KEY_PATH}.tmp.$$"
trap 'rm -f "$tmp"' EXIT

umask 0077
openssl rand -hex 32 >"$tmp"
chmod 0600 "$tmp"
mv "$tmp" "$KEY_PATH"
trap - EXIT

printf '[master-key] generated %s\n' "$KEY_PATH"
printf 'MEGAVPN_MASTER_KEY_PATH=%s\n' "$KEY_PATH"
printf 'MEGAVPN_MASTER_KEY_VERSION=%s\n' "${MEGAVPN_MASTER_KEY_VERSION:-v1}"
