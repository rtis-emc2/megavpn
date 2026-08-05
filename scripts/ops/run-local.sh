#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
set -a
[ -f /etc/megavpn/megavpn.env ] && source /etc/megavpn/megavpn.env
set +a
if [[ -z "${MEGAVPN_BOOTSTRAP_ADMIN_USERNAME:-}" || -z "${MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD:-}" ]]; then
  cat >&2 <<'WARN'
WARNING: bootstrap admin credentials are not fully configured.
Set MEGAVPN_BOOTSTRAP_ADMIN_USERNAME and MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD
before first start, otherwise no initial operator will be created.
WARN
fi
go run ./cmd/migrate
go run ./cmd/api
