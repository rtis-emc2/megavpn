#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

if [[ ! -d .github/workflows ]]; then
  printf 'github actions pinning ok: no workflows directory\n'
  exit 0
fi

failed=0
while IFS= read -r -d '' file; do
  line_no=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_no=$((line_no + 1))
    [[ "$line" =~ ^[[:space:]]*uses:[[:space:]]*([^[:space:]#]+) ]] || continue
    target="${BASH_REMATCH[1]}"
    target="${target%\"}"
    target="${target#\"}"
    target="${target%\'}"
    target="${target#\'}"
    [[ "$target" == ./* ]] && continue
    if [[ "$target" != *@* ]]; then
      printf '%s:%s uses action without an explicit ref: %s\n' "$file" "$line_no" "$target" >&2
      failed=1
      continue
    fi
    ref="${target##*@}"
    if [[ ! "$ref" =~ ^[0-9a-fA-F]{40}$ ]]; then
      printf '%s:%s uses action ref that is not a pinned commit SHA: %s\n' "$file" "$line_no" "$target" >&2
      failed=1
    fi
  done <"$file"
done < <(find .github/workflows -type f \( -name '*.yml' -o -name '*.yaml' \) -print0 | sort -z)

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi

printf 'github actions pinning ok\n'
