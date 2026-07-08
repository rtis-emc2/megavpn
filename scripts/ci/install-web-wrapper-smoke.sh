#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TARGET_DIR="$(mktemp -d "${TMPDIR:-/tmp}/megavpn-web-install.XXXXXX")"

cleanup() {
  rm -rf "$TARGET_DIR"
}
trap cleanup EXIT

cd "$ROOT_DIR"

scripts/install-web.sh "$TARGET_DIR"

for required in index.html assets/app.js; do
  if [[ ! -f "$TARGET_DIR/$required" ]]; then
    echo "installed Web UI is missing $required" >&2
    exit 1
  fi
done
