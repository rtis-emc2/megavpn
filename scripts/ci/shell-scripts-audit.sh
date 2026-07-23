#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

failed=0
checked=0

check_script() {
  local file="$1"
  local first_line
  checked=$((checked + 1))

  if ! bash -n "$file"; then
    failed=1
  fi

  IFS= read -r first_line <"$file" || true
  case "$first_line" in
    '#!/usr/bin/env bash'|'#!/bin/bash'|'#!/usr/bin/bash'|'#!/usr/bin/env sh'|'#!/bin/sh')
      ;;
    *)
      printf '%s: unsupported or missing shell shebang: %s\n' "$file" "$first_line" >&2
      failed=1
      ;;
  esac

  if [[ "$file" != "scripts/lib/smoke.sh" && ! -x "$file" ]]; then
    printf '%s: operational script is not executable\n' "$file" >&2
    failed=1
  fi
}

while IFS= read -r -d '' file; do
  check_script "$file"
done < <(
  {
    find scripts deploy -type f -name '*.sh' -print0
    [[ ! -f deploy-local.sh ]] || printf '%s\0' deploy-local.sh
  } | sort -z
)

while IFS= read -r -d '' wrapper; do
  target="$(sed -n 's|^exec "\$SCRIPT_DIR/\([^" ]*\)".*|scripts/\1|p' "$wrapper")"
  [[ -n "$target" ]] || continue
  if [[ ! -f "$target" || ! -x "$target" ]]; then
    printf '%s: wrapper target is missing or not executable: %s\n' "$wrapper" "$target" >&2
    failed=1
  fi
done < <(find scripts -maxdepth 1 -type f -name '*.sh' -print0 | sort -z)

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi

printf 'shell scripts audit passed: %d files\n' "$checked"
