#!/usr/bin/env bash
set -euo pipefail
systemctl stop megavpn-agent 2>/dev/null || true
rm -f /var/lib/megavpn/agent/state.json
rm -f /var/lib/megavpn/agent/state.json.tmp
rm -f /etc/megavpn/agent-bootstrap.env
cat <<OUT
Agent local identity and bootstrap file removed.
Server-side node and agent records are not deleted.

Create a new one-time enrollment token and bootstrap file before starting agent:
  /opt/megavpn/scripts/create-node-enrollment.sh <node-name> <node-address>
  systemctl restart megavpn-agent
OUT
