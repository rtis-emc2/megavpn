#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
BUILD_DIR="$SRC_DIR/frontend/dist"
TARGET_DIR="${1:-/opt/megavpn/web}"
DEFAULT_TARGET_DIR="/opt/megavpn/web"

if [[ ! -f "$BUILD_DIR/index.html" || ! -d "$BUILD_DIR/assets" ]]; then
  echo "Frontend build is missing: $BUILD_DIR (run the verified frontend build before deployment)" >&2
  exit 1
fi

if [[ "$TARGET_DIR" != /* ||
      "$TARGET_DIR" == "/" ||
      "$TARGET_DIR" == *"/./"* ||
      "$TARGET_DIR" == */. ||
      "$TARGET_DIR" == *"/../"* ||
      "$TARGET_DIR" == */.. ]]; then
  echo "Frontend target must be a safe absolute directory: $TARGET_DIR" >&2
  exit 1
fi

is_protected_target() {
  case "$1" in
    /bin|/bin/*|/boot|/boot/*|/dev|/dev/*|/etc|/etc/*|/home|/home/*|/lib|/lib/*|/lib64|/lib64/*|/proc|/proc/*|/root|/root/*|/run|/run/*|/sbin|/sbin/*|/srv|/srv/*|/sys|/sys/*|/usr|/usr/*|/var|/var/*|/Applications|/Applications/*|/Library|/Library/*|/System|/System/*|/Users|/Users/*)
      return 0
      ;;
  esac
  return 1
}

if is_protected_target "$TARGET_DIR"; then
  echo "Frontend target cannot be a protected system directory: $TARGET_DIR" >&2
  exit 1
fi

if [[ "$TARGET_DIR" != "$DEFAULT_TARGET_DIR" && "${MEGAVPN_FRONTEND_ALLOW_CUSTOM_TARGET:-0}" != "1" ]]; then
  echo "Custom frontend target requires MEGAVPN_FRONTEND_ALLOW_CUSTOM_TARGET=1: $TARGET_DIR" >&2
  exit 1
fi

case "$TARGET_DIR" in
  /opt/megavpn/web|/opt/megavpn/web/*|/tmp/*|/private/tmp/*)
    ;;
  *)
    if [[ "${MEGAVPN_FRONTEND_ALLOW_CUSTOM_TARGET:-0}" != "1" ]]; then
      echo "Frontend target is outside the managed runtime root: $TARGET_DIR" >&2
      exit 1
    fi
    ;;
esac

if [[ -L "$TARGET_DIR" ]]; then
  echo "Frontend target must not be a symbolic link: $TARGET_DIR" >&2
  exit 1
fi

install -d "$TARGET_DIR"
BUILD_REAL="$(cd "$BUILD_DIR" && pwd -P)"
TARGET_REAL="$(cd "$TARGET_DIR" && pwd -P)"

if is_protected_target "$TARGET_REAL"; then
  echo "Resolved frontend target cannot be a protected system directory: $TARGET_REAL" >&2
  exit 1
fi
if [[ "$TARGET_DIR" == "$DEFAULT_TARGET_DIR" && "$TARGET_REAL" != "$DEFAULT_TARGET_DIR" ]]; then
  echo "Managed frontend target resolves outside $DEFAULT_TARGET_DIR: $TARGET_REAL" >&2
  exit 1
fi
if [[ -L "$TARGET_REAL" ]]; then
  echo "Resolved frontend target must not be a symbolic link: $TARGET_REAL" >&2
  exit 1
fi

case "$TARGET_REAL" in
  /opt/megavpn/web|/opt/megavpn/web/*|/tmp/*|/private/tmp/*)
    ;;
  *)
    if [[ "${MEGAVPN_FRONTEND_ALLOW_CUSTOM_TARGET:-0}" != "1" ]]; then
      echo "Resolved frontend target is outside the managed runtime root: $TARGET_REAL" >&2
      exit 1
    fi
    ;;
esac

if [[ "$BUILD_REAL" == "$TARGET_REAL" ]]; then
  echo "RTIS MegaVPN frontend already served from $TARGET_REAL; copy skipped."
  exit 0
fi
case "$BUILD_REAL/" in
  "$TARGET_REAL/"*)
    echo "Frontend target cannot contain the build tree: $TARGET_REAL" >&2
    exit 1
    ;;
esac
case "$TARGET_REAL/" in
  "$BUILD_REAL/"*)
    echo "Frontend target cannot be inside the build tree: $TARGET_REAL" >&2
    exit 1
    ;;
esac

rsync -a --delete "$BUILD_REAL/" "$TARGET_REAL/"

echo "RTIS MegaVPN frontend installed to $TARGET_REAL"
echo "The control-plane API and frontend must use the same origin."
