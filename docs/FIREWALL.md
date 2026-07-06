# Firewall Policy Catalog

**Release:** `7.0.1.41`

Firewall is the managed policy workspace for node and control-plane boundaries.
It is intentionally modeled as a catalog before apply: operators prepare address
lists, ordered rules and policies, then queue an apply job for a selected node.

Russian companion: [FIREWALL_RU.md](FIREWALL_RU.md).

## Operating Model

The workflow is:

1. Create reusable address lists for operators, trusted networks, VPN pools or
   blocked destinations.
2. Add entries to address lists. Leave type on auto-detect for CIDR, single IP
   or IP range entries.
3. Create ordered rules inside a policy. Lower priority is evaluated first.
4. Apply the policy to a node and watch the node firewall state.

This keeps editing separate from rollout. A catalog change does not alter a node
until an apply job is queued and completed.

Existing installations upgraded from an earlier `7.0.1` build must run database
migrations through `000009_firewall_schema_repair` before creating address
lists. If migrations are behind, the API can return
`relation "firewall_address_lists" does not exist`.

## UI Workflow

Open `Firewall` from the control menu.

- `Overview`: counters and posture.
- `Policies`: policy cards, default chain metadata and quick apply.
- `Rules`: global ordered rule view.
- `Address lists`: list and entry management.
- `Node state`: last apply state per node.

The top workflow buttons jump directly to the required stage. The rule editor
contains presets for SSH management, HTTPS control, WireGuard, OpenVPN
TCP/UDP, IPsec IKE/NAT-T, L2TP, Shadowsocks TCP/UDP, HTTP proxy, MTProto,
Nginx edge HTTP(S) and invalid-packet drop.

The `Policies` view shows each policy posture, default input/forward/output
actions and a short rule preview. The `Rules` view includes local filters for
policy, chain, action and text search across CIDR/list/port/comment fields.
The `Address lists` view includes local search across list metadata and entry
values.

The built-in `Default node firewall` policy is the recommended minimal
baseline for production nodes. In strict mode it denies unsolicited input and
forwarded traffic, keeps node output at `accept`, allows IPv4/IPv6 diagnostics,
allows public HTTP/HTTPS edge entrypoints and permits forwarding for the seeded
private/CGNAT/ULA client source ranges in `vpn_client_sources`.

The SSH rule is present but disabled until `trusted_operators` is populated and
the operator deliberately enables it. Protocol listener ports beyond HTTP/HTTPS
should be added only for services that are actually installed, using rule
presets or service-specific policy.

The apply dialog is split into two explicit modes:

- `Rules only`: base chains stay at `accept`; explicit catalog rules are
  installed.
- `Strict defaults`: default input/forward/output policies are enforced by the
  agent.

`Node state` shows the last observed enforcement mode, explicit rule count and
system safety rule count returned by the agent.

## Security Model

- `firewall.read` allows inspection.
- `firewall.manage` allows policy, rule and address-list changes.
- `firewall.apply` allows queueing node apply jobs.
- All create/update/delete/apply actions produce audit events.
- Rules are stored as catalog data and rendered by the worker into managed node
  firewall payloads.

## Enforcement Boundary

By default, apply jobs install explicit allow/drop/reject rules into managed
nftables chains while keeping base chain policy at `accept`. This is the safe
staging mode for first rollout and catalog validation.

Strict default-policy enforcement is available per apply job through the
`enforce_default_policy` flag in the API/UI. In strict mode the agent replaces
the managed `inet megavpn_firewall` table atomically with `nft -f`, recreates
input, forward and output base chains and applies the policy defaults:

- `accept` is rendered as base chain policy `accept`.
- `drop` is rendered as base chain policy `drop`.
- `reject` is rendered as base chain policy `drop` plus a terminal `reject`
  rule, because nftables base chain policy does not support `reject`.

The agent also adds system safety rules for established/related traffic and
loopback before catalog rules. If output default policy is `drop` or `reject`,
the agent must preserve control-plane egress. It does this by either:

- generating a TCP egress allow rule when the agent control-plane URL host is
  an IP address; or
- accepting an explicit active catalog `output accept` rule for the
  control-plane TCP port when the control-plane URL host is DNS.

If neither condition is true, render fails before touching nftables. This keeps
strict output rollout from silently isolating a node.

Address-list entries with DNS type are stored for catalog context only in this
release. Node-side nftables apply renders CIDR, single IP address and IP range
entries; a DNS-only list cannot be used as an active rule matcher. The rule
protocol model supports `any`, `tcp`, `udp`, `icmp` and `icmpv6`.

The managed table is owned by MegaVPN. Do not place hand-written rules in
`inet megavpn_firewall`; strict apply replaces that table as a single managed
unit. Route-policy and service-policy chains continue to use `inet megavpn`;
firewall apply cleans legacy `firewall_*` chains from that shared table without
deleting the table.

## Failure Handling

If apply fails:

1. Open `Firewall -> Node state`.
2. Find the failed node and last policy.
3. Open `Jobs` for the corresponding `node.firewall.apply` job.
4. Check agent logs and rendered payload details.
5. Fix the catalog rule and queue apply again.

Do not make persistent node-side firewall changes outside the managed catalog.
Temporary emergency access changes must be documented and then converted into a
managed policy rule.
