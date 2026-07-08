#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
TARGET_DIR="${1:-/opt/megavpn/web}"

if [[ ! -d "$SRC_DIR/web" ]]; then
  echo "Web source directory does not exist: $SRC_DIR/web" >&2
  exit 1
fi

install -d "$TARGET_DIR"

SRC_REAL="$(cd "$SRC_DIR/web" && pwd -P)"
TARGET_REAL="$(cd "$TARGET_DIR" && pwd -P)"

if [[ "$SRC_REAL" == "$TARGET_REAL" ]]; then
  echo "RTIS MegaVPN Web UI already served from $TARGET_REAL; copy skipped."
  exit 0
fi

rsync -a --delete "$SRC_DIR/web/" "$TARGET_DIR/"

echo "RTIS MegaVPN Web UI installed to $TARGET_DIR"
echo "Serve it with Nginx or any static web server. API must be reachable through same-origin /api or configured in UI Settings."
