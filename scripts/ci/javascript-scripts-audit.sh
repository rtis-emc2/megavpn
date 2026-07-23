#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NODE_BIN="${MEGAVPN_RELEASE_NODE_BIN:-node}"
cd "$ROOT_DIR"

command -v "$NODE_BIN" >/dev/null 2>&1 || {
  printf 'node executable is unavailable: %s\n' "$NODE_BIN" >&2
  exit 1
}

checked=0

while IFS= read -r -d '' file; do
  "$NODE_BIN" --check "$file"
  checked=$((checked + 1))
done < <(
  {
    find scripts deploy -type f -name '*.js' -print0
    find web/assets -maxdepth 1 -type f -name '*.js' -print0
  } | sort -z
)

while IFS= read -r -d '' wrapper; do
  target="$(sed -n "s|^require('./\\(ci/[^']*\\)');.*|scripts/\\1|p" "$wrapper")"
  [[ -n "$target" ]] || continue
  if [[ ! -f "$target" || ! -x "$target" ]]; then
    printf '%s: wrapper target is missing or not executable: %s\n' "$wrapper" "$target" >&2
    exit 1
  fi
done < <(find scripts -maxdepth 1 -type f -name '*.js' -print0 | sort -z)

printf 'javascript scripts audit passed: %d files\n' "$checked"
