#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

go test ./internal/api/http -run 'TestStaticServingRoutes|TestShouldServeFrontendFallback|TestResolveWebAssetRejectsTraversalAndSymlinkEscape' -count=1

[[ -f frontend/dist/index.html ]] || {
  printf 'missing frontend build index: frontend/dist/index.html\n' >&2
  exit 1
}
find frontend/dist/assets -maxdepth 1 -type f -name '*.js' -print -quit | grep -q . || {
  printf 'missing frontend JavaScript bundle\n' >&2
  exit 1
}
find frontend/dist/assets -maxdepth 1 -type f -name '*.css' -print -quit | grep -q . || {
  printf 'missing frontend stylesheet bundle\n' >&2
  exit 1
}
[[ ! -e frontend/dist/legacy ]] || {
  printf 'frontend build contains forbidden legacy assets\n' >&2
  exit 1
}

printf 'frontend serving smoke ok\n'
