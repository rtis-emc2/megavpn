# Security and Release Review: 7.0.1.32

**Release:** `7.0.1.32`

## Scope

- Explicit node route-policy cleanup lifecycle.
- Stale route-policy destination cleanup from previous on-node snapshots.
- Typed API/UI workflow for route-policy rollback.

## Changes

- Added `node.route_policy.cleanup` as a typed privileged job with
  `node.write` authorization.
- Added `POST /api/v1/nodes/{id}/routes/cleanup` and a `Clean route policy`
  action in node diagnostics.
- Agent cleanup stops/disables the route-policy timer/unit, deletes reserved
  route-policy `ip rule` priorities, flushes managed nftables route-policy
  chains and removes managed route-policy files.
- Agent apply now reads the previous route-policy snapshot before writing a new
  one and includes stale destination/table cleanup in the generated kernel
  script.
- Job schema and permission tests were extended for the cleanup job type.

## Security Assessment

- The cleanup path is typed-only and cannot be queued through the generic job
  API. It uses the same node-scoped authorization boundary as
  `node.route_policy.apply`.
- Cleanup is scoped to MegaVPN-owned route-policy files, managed nftables
  chains and reserved policy-rule priorities `21900..21949` and
  `22000..22999`.
- Previous snapshot data is treated as untrusted input for shell rendering:
  destination and table values are normalized and checked with existing network
  token validation before they can be emitted into cleanup scripts.
- The operation intentionally does not remove unrelated nftables tables,
  backhaul runtime, service runtime or arbitrary route-table contents.

## Verification Evidence

- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`: passed
  (`passed=12 skipped=6`).
- `govulncheck ./...`: no vulnerabilities found.
- `go test ./...`: passed.
- `go test -race ./...`: passed.
- `go vet ./...`: passed.
- `scripts/docs-consistency.sh`: passed.
- `web/assets/node-workflows.js` syntax check with bundled Node.js: passed.
- `git diff --check`: passed.

## Residual Risk

- Live cleanup behavior still requires validation on real Linux nodes with
  `systemd`, `iproute2` and `nftables`.
- Cleanup removes policy routing and marking state. Operators must run
  `Sync route policy` again after cleanup when the node remains an active
  ingress node.
