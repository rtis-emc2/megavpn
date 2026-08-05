#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/smoke.sh
source "$SCRIPT_DIR/../lib/smoke.sh"

name="${1:-}"
address="${2:-127.0.0.1}"
api="${MEGAVPN_API_URL:-http://127.0.0.1:8080}"
config_dir="/etc/megavpn"
bootstrap_file="$config_dir/agent-bootstrap.env"
agent_file="$config_dir/agent.env"
state_file="/var/lib/megavpn/agent/state.json"

if [[ -z "$name" ]]; then
  echo "usage: $0 <node-name> [address]" >&2
  exit 2
fi

smoke_require curl
smoke_require jq

nodes_json="$(smoke_curl "$api/api/v1/nodes")"
node_id="$(jq -r --arg name "$name" '.[] | select(.name==$name and .status!="retired") | .id' <<<"$nodes_json" | head -1)"

if [[ -z "$node_id" || "$node_id" == "null" ]]; then
  node_payload="$(jq -cn --arg name "$name" --arg address "$address" '{name:$name,kind:"remote",status:"draft",address:$address,execution_mode:"agent_managed"}')"
  node_json="$(smoke_json_request POST "$api/api/v1/nodes" "$node_payload")"
  node_id="$(jq -r '.id' <<<"$node_json")"
fi

if [[ -z "$node_id" || "$node_id" == "null" ]]; then
  echo "cannot create or find node: $name" >&2
  exit 1
fi

token_json="$(smoke_json_request POST "$api/api/v1/nodes/$node_id/enrollment-token")"
token="$(jq -r '.token' <<<"$token_json")"
if [[ -z "$token" || "$token" == "null" ]]; then
  echo "control plane did not return enrollment token" >&2
  echo "$token_json" >&2
  exit 1
fi

install -d -m 0750 "$config_dir"
install -d -m 0700 /var/lib/megavpn/agent

cat >"$agent_file" <<AGENT
MEGAVPN_AGENT_CONTROL_PLANE_URL=$api
MEGAVPN_AGENT_STATE_PATH=$state_file
MEGAVPN_AGENT_BOOTSTRAP_PATH=$bootstrap_file
MEGAVPN_AGENT_POLL_INTERVAL=10s
AGENT
chmod 0640 "$agent_file"

cat >"$bootstrap_file" <<BOOTSTRAP
MEGAVPN_AGENT_NODE_ID=$node_id
MEGAVPN_AGENT_NODE_NAME=$name
MEGAVPN_AGENT_NODE_ADDRESS=$address
MEGAVPN_AGENT_CONTROL_PLANE_URL=$api
MEGAVPN_AGENT_ENROLLMENT_TOKEN=$token
BOOTSTRAP
chmod 0600 "$bootstrap_file"

cat <<OUT
node_id=$node_id
node_name=$name
api=$api
agent_config=$agent_file
bootstrap_file=$bootstrap_file
state_file=$state_file

enrollment_token_hint=$(jq -r '.token_hint' <<<"$token_json")

Next:
  systemctl restart megavpn-agent

Expected after successful enrollment:
  bootstrap file is removed automatically
  state file is created at $state_file
OUT
