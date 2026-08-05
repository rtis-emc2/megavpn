#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_ROOT="${MEGAVPN_TEST_TMPDIR:-/tmp}"
TARGET_DIR="$(mktemp -d "$TMP_ROOT/megavpn-frontend-install.XXXXXX")"
SYMLINK_TARGET="$(mktemp -d "$TMP_ROOT/megavpn-frontend-symlink-target.XXXXXX")"
SYMLINK_PATH="$TMP_ROOT/megavpn-frontend-symlink.$$"

cleanup() {
  rm -rf "$TARGET_DIR" "$SYMLINK_TARGET" "$SYMLINK_PATH"
}
trap cleanup EXIT

cd "$ROOT_DIR"
MEGAVPN_FRONTEND_ALLOW_CUSTOM_TARGET=1 scripts/install-frontend.sh "$TARGET_DIR"

[[ -f "$TARGET_DIR/index.html" ]] || {
  echo "installed frontend is missing index.html" >&2
  exit 1
}
find "$TARGET_DIR/assets" -maxdepth 1 -type f -name '*.js' -print -quit | grep -q . || {
  echo "installed frontend has no JavaScript bundle" >&2
  exit 1
}
find "$TARGET_DIR/assets" -maxdepth 1 -type f -name '*.css' -print -quit | grep -q . || {
  echo "installed frontend has no stylesheet bundle" >&2
  exit 1
}
[[ ! -e "$TARGET_DIR/legacy" ]] || {
  echo "installed frontend unexpectedly contains legacy assets" >&2
  exit 1
}

ln -s "$SYMLINK_TARGET" "$SYMLINK_PATH"
if MEGAVPN_FRONTEND_ALLOW_CUSTOM_TARGET=1 scripts/install-frontend.sh "$SYMLINK_PATH" >/dev/null 2>&1; then
  echo "frontend installer accepted a symbolic-link target" >&2
  exit 1
fi

if scripts/install-frontend.sh "$TARGET_DIR" >/dev/null 2>&1; then
  echo "frontend installer accepted a custom target without explicit opt-in" >&2
  exit 1
fi
