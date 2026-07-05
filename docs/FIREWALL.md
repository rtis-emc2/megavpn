# Firewall Policy Catalog

**Release:** `7.0.1.7`

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

## UI Workflow

Open `Firewall` from the control menu.

- `Overview`: counters and posture.
- `Policies`: policy cards, default chain metadata and quick apply.
- `Rules`: global ordered rule view.
- `Address lists`: list and entry management.
- `Node state`: last apply state per node.

The top workflow buttons jump directly to the required stage. The rule editor
contains presets for common cases such as SSH management, HTTPS control,
WireGuard UDP and invalid-packet drop.

## Security Model

- `firewall.read` allows inspection.
- `firewall.manage` allows policy, rule and address-list changes.
- `firewall.apply` allows queueing node apply jobs.
- All create/update/delete/apply actions produce audit events.
- Rules are stored as catalog data and rendered by the worker into managed node
  firewall payloads.

## Current Enforcement Boundary

This release applies explicit allow/drop/reject rules into managed nftables
chains. Default chain policy fields are stored as rollout metadata; strict
default-policy enforcement must be enabled only after a controlled migration
plan exists, otherwise operators can lock themselves out.

Address-list entries with DNS type are stored for catalog context only in this
release. Node-side nftables apply renders CIDR, single IP address and IP range
entries; a DNS-only list cannot be used as an active rule matcher.

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
