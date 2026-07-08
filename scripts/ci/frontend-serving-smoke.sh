#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

go test ./internal/api/http -run 'TestStaticServingRoutes|TestShouldServeFrontendFallback' -count=1

[[ -f web/index.html ]] || {
  printf 'missing new frontend index: web/index.html\n' >&2
  exit 1
}
[[ -f web/legacy/index.html ]] || {
  printf 'missing legacy frontend index: web/legacy/index.html\n' >&2
  exit 1
}
find web/assets -maxdepth 1 -type f -name '*.js' -print -quit | grep -q . || {
  printf 'missing built frontend javascript asset in web/assets\n' >&2
  exit 1
}
find web/assets -maxdepth 1 -type f -name '*.css' -print -quit | grep -q . || {
  printf 'missing built frontend stylesheet asset in web/assets\n' >&2
  exit 1
}
find web/legacy/assets -maxdepth 1 -type f -name '*.js' -print -quit | grep -q . || {
  printf 'missing legacy frontend javascript asset in web/legacy/assets\n' >&2
  exit 1
}

printf 'frontend serving smoke ok\n'
