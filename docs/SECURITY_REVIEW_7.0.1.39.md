# Security and Release Review: 7.0.1.39

**Release:** `7.0.1.39`

## Scope

- Control Plane convergence between managed backhaul transport selection,
  Xray/VLESS remote-egress outbound source address and route-policy apply.
- Xray instance-level default egress materialization for managed backhaul.
- VLESS group remote-egress refresh for `vless_groups`, `xray_groups` and
  `outbound_groups` aliases.

## Changes

- Added a backhaul-to-Xray convergence path for active backhaul apply, route
  enable and standby transport promotion.
- When an Xray/VLESS instance references the affected egress node, the Control
  Plane now refreshes the instance revision first, replacing stale
  `freedom.sendThrough` values with the ingress-side address of the selected
  active backhaul transport.
- If Xray convergence queues `instance.apply`, route-policy refresh is deferred
  until that apply completes. This prevents route-policy from being generated
  from stale Xray outbound metadata.
- VLESS group catalog sync now also refreshes instance-level default remote
  egress metadata, not only group-level remote egress metadata.
- Added regression coverage for default Xray egress `sendThrough` refresh and
  standby OpenVPN promotion after a failed selected WireGuard transport.

## Security Assessment

- The fix reduces the risk of silent traffic blackholing or accidental local
  breakout caused by stale Xray source-route metadata after backhaul failover.
- The convergence path is fail-closed: if a remote egress cannot be resolved to
  an active managed backhaul, promotion/enable returns an error instead of
  publishing an inconsistent route-policy state.
- No new privileged agent command or shell interpolation surface is introduced.
  The change is limited to validated Control Plane revisions and existing
  typed `instance.apply` / `node.route_policy.apply` jobs.
- Existing apply validation remains in the path before any new Xray revision is
  accepted as current.

## Verification Evidence

- `go test ./internal/infra/postgres` passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- `scripts/docs-consistency.sh` passed for release `7.0.1.39`.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh` passed
  (`passed=12 skipped=6`), including version/tag consistency, unit tests,
  race tests, vulnerability scan, binary version checks, shell syntax checks,
  docs consistency, installer validation and static security-pattern scan.

## Residual Risk

- Runtime verification still requires a real Linux ingress node with systemd,
  Xray, `nft`, `iproute2` and at least one active managed backhaul transport.
- Operators still need to apply the queued Xray job after a promotion. The
  successful Xray apply is what causes route-policy refresh to use the updated
  `sendThrough`.
- If a node was manually edited outside the Control Plane, the next managed
  apply intentionally overwrites that state with the current validated
  revision.
