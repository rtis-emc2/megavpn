#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="${1:-/opt/megavpn}"
cd "$ROOT_DIR"
./scripts/build.sh
install -m 0644 deploy/systemd/megavpn-api.service /etc/systemd/system/megavpn-api.service
install -m 0644 deploy/systemd/megavpn-worker.service /etc/systemd/system/megavpn-worker.service
install -m 0644 deploy/systemd/megavpn-agent.service /etc/systemd/system/megavpn-agent.service
systemctl daemon-reload
./bin/megavpn-migrate
systemctl enable --now megavpn-api.service megavpn-worker.service
