#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

code_version="$(sed -nE 's/^const Version = "([^"]+)"/\1/p' internal/platform/version/version.go)"
if [[ -z "$code_version" ]]; then
  printf 'unable to read internal/platform/version.Version\n' >&2
  exit 1
fi

required_release_artifacts=(
  README.md
  README_RU.md
  ROADMAP_V1_AND_TZ.md
  ROADMAP_V1_AND_TZ_RU.md
  docs/DOCUMENTATION.md
  docs/DOCUMENTATION_RU.md
  docs/DOCUMENTATION_REVIEW.md
  docs/DOCUMENTATION_REVIEW_RU.md
  docs/USER_GUIDE_EN.md
  docs/USER_GUIDE_RU.md
  docs/NEXT_STEPS.md
  docs/NEXT_STEPS_RU.md
  docs/RELEASE_GATES.md
  docs/SELF_TESTING.md
  docs/THREAT_MODEL.md
  docs/RBAC_MATRIX.md
  docs/OPERATIONS_RUNBOOK.md
  docs/BACKHAUL.md
  docs/FIREWALL.md
  docs/FIREWALL_RU.md
  docs/NODE_MAP.md
  docs/NODE_MAP_RU.md
  docs/VLESS_GROUPS.md
  docs/VLESS_GROUPS_RU.md
  docs/VLESS_SUBSCRIPTIONS.md
  docs/VLESS_SUBSCRIPTIONS_RU.md
  "docs/SECURITY_REVIEW_${code_version}.md"
  deploy/env/megavpn.production.env.example
  deploy/env/megavpn-agent.production.env.example
)

fail=0

require_file_release_banner() {
  local file="$1"
  if [[ ! -s "$file" ]]; then
    printf 'missing or empty required release artifact: %s\n' "$file" >&2
    fail=1
    return
  fi
  if ! head -n 8 "$file" | grep -Fq "$code_version"; then
    printf 'required release artifact does not declare release %s near top: %s\n' "$code_version" "$file" >&2
    fail=1
  fi
}

require_link_target() {
  local source="$1"
  local target="$2"
  local resolved
  case "$target" in
    http://*|https://*|mailto:*|\#*|"")
      return
      ;;
  esac
  target="${target%%#*}"
  case "$target" in
    /*)
      resolved="${target#/}"
      ;;
    *)
      resolved="$(dirname "$source")/$target"
      ;;
  esac
  if [[ ! -e "$resolved" ]]; then
    printf 'broken documentation link in %s: %s -> %s\n' "$source" "$target" "$resolved" >&2
    fail=1
  fi
}

for file in "${required_release_artifacts[@]}"; do
  require_file_release_banner "$file"
done

for file in README.md README_RU.md docs/DOCUMENTATION.md docs/DOCUMENTATION_RU.md; do
  while IFS= read -r target; do
    require_link_target "$file" "$target"
  done < <(sed -nE 's/.*\[[^]]+\]\(([^)]+)\).*/\1/p' "$file")
done

if ! grep -Fq "SECURITY_REVIEW_${code_version}.md" docs/DOCUMENTATION.md; then
  printf 'docs/DOCUMENTATION.md does not link current security review SECURITY_REVIEW_%s.md\n' "$code_version" >&2
  fail=1
fi
if ! grep -Fq "SECURITY_REVIEW_${code_version}.md" docs/DOCUMENTATION_RU.md; then
  printf 'docs/DOCUMENTATION_RU.md does not link current security review SECURITY_REVIEW_%s.md\n' "$code_version" >&2
  fail=1
fi

if rg -n 'v=7\.0\.1\.' web/index.html | grep -Fv "v=${code_version}" >&2; then
  printf 'web/index.html contains asset cache keys that do not match release %s\n' "$code_version" >&2
  fail=1
fi

if rg -n 'MikroTik|микротик' README.md README_RU.md docs/*.md ROADMAP_V1_AND_TZ.md ROADMAP_V1_AND_TZ_RU.md --glob '!docs/SECURITY_REVIEW_*.md' >&2; then
  printf 'current documentation contains vendor-specific firewall wording that should stay out of generic operator copy\n' >&2
  fail=1
fi

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi

printf 'documentation consistency ok for release %s\n' "$code_version"
