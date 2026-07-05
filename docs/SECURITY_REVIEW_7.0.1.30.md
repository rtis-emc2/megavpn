# Security and Release Review: 7.0.1.30

**Release:** `7.0.1.30`

## Scope

- Route-policy operability and diagnostics hardening.
- Read-only node route-policy preview API and UI before queueing
  `node.route_policy.apply`.
- Documentation update for VLESS/Xray remote-egress inspection.

## Changes

- Added `GET /api/v1/nodes/{id}/routes/preview` with `node.read`
  authorization. The endpoint uses the same route-policy payload builder as
  `node.route_policy.apply`, but returns a safe operational projection instead
  of queueing an agent job.
- Added `Inspect route policy` to the node diagnostics UI. Operators can now
  see client route counts, enforceable/observe-only counts, VLESS/Xray system
  egress routes, blocked reasons and managed nft/ip-rule primitives before
  applying.
- Added source-identity redaction in preview output. Non-L3 credential-like
  identifiers such as VLESS UUIDs are returned as `[redacted]`; L3 identities
  needed for kernel enforcement, such as WireGuard IPv4 source prefixes, remain
  visible.
- Documented the managed route-policy primitives: `inet megavpn`
  `route_policy_prerouting`, `route_policy_output`, system-rule priorities
  `21900..21949` and client-rule priorities `22000..22999`.
- Bumped release metadata and web asset cache keys to `7.0.1.30`.

## Security Assessment

- The new preview endpoint is read-only and does not create jobs, mutate node
  state or expose raw policy/metadata JSON.
- The preview intentionally reuses the production route-policy projection code,
  reducing the chance that operators inspect a different path than the apply
  job will use.
- Source-identity redaction reduces accidental leakage of VLESS UUIDs or future
  L7 credentials in node diagnostics while preserving enough detail to debug
  kernel-routable policies.
- No secret refs or runtime file contents are returned by the endpoint.

## Verification Evidence

- `go test ./...`: passed.
- `go test -race ./...`: passed.
- `go vet ./...`: passed.
- Bundled Node.js `node --check web/assets/node-workflows.js`: passed.
- `git diff --check`: passed.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`: passed with 11
  checks passed and 6 expected local environment skips.

## Residual Risk

- Live verification still requires real ingress/egress nodes to confirm the
  preview projection matches `nft list chain inet megavpn route_policy_output`,
  `ip rule show` and `ip route show table <backhaul_table>` after apply.
- The preview is intentionally summarized. If operators need raw payload
  debugging later, it should be added behind a stricter permission and explicit
  sensitive-field redaction.
