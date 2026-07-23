#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
TARGET_DIR="${1:-/opt/megavpn/web}"

if [[ ! -d "$SRC_DIR/web" ]]; then
  echo "Web source directory does not exist: $SRC_DIR/web" >&2
  exit 1
fi

if [[ "$TARGET_DIR" != /* ||
      "$TARGET_DIR" == "/" ||
      "$TARGET_DIR" == *"/./"* ||
      "$TARGET_DIR" == */. ||
      "$TARGET_DIR" == *"/../"* ||
      "$TARGET_DIR" == */.. ]]; then
  echo "Web target must be a safe absolute directory: $TARGET_DIR" >&2
  exit 1
fi

case "$TARGET_DIR" in
  /bin|/boot|/dev|/etc|/home|/lib|/lib64|/opt|/proc|/root|/run|/sbin|/srv|/sys|/tmp|/usr|/var|/Applications|/Library|/System|/Users|/private)
    echo "Web target cannot be a protected system directory: $TARGET_DIR" >&2
    exit 1
    ;;
esac

install -d "$TARGET_DIR"

SRC_REAL="$(cd "$SRC_DIR/web" && pwd -P)"
TARGET_REAL="$(cd "$TARGET_DIR" && pwd -P)"

if [[ "$SRC_REAL" == "$TARGET_REAL" ]]; then
  echo "RTIS MegaVPN Web UI already served from $TARGET_REAL; copy skipped."
  exit 0
fi

case "$SRC_REAL/" in
  "$TARGET_REAL/"*)
    echo "Web target cannot contain the source tree: $TARGET_REAL" >&2
    exit 1
    ;;
esac
case "$TARGET_REAL/" in
  "$SRC_REAL/"*)
    echo "Web target cannot be inside the source tree: $TARGET_REAL" >&2
    exit 1
    ;;
esac

rsync -a --delete "$SRC_DIR/web/" "$TARGET_DIR/"

echo "RTIS MegaVPN Web UI installed to $TARGET_DIR"
echo "Serve it with Nginx or any static web server. API must be reachable through same-origin /api or configured in UI Settings."
