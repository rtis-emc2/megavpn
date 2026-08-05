# Security and Release Review: 7.0.1.40

**Release:** `7.0.1.40`

## Scope

- Idempotent managed backhaul convergence for already-selected active
  transports.
- Operator recovery path for existing Xray/VLESS instances that were applied
  with stale `freedom.sendThrough` metadata before the convergence fix.

## Changes

- Re-running `Promote to active` for the already selected active transport now
  queues Xray/VLESS egress convergence or route-policy refresh instead of
  returning a silent no-op.
- Re-running route enable on an already active backhaul route now follows the
  same convergence path.
- Backhaul-to-Xray convergence seeds a missing `egress_node_id` from the known
  backhaul link when older Xray revisions only stored `link_id` or
  `transport_id`.
- Regression coverage now restores a stale Xray revision after a successful
  OpenVPN standby promotion and verifies that idempotent promote queues a fresh
  `instance.apply`.

## Security Assessment

- The change provides a managed recovery action for stale Xray source routing
  without requiring manual node-side JSON edits.
- Existing validation remains fail-closed: stale or incomplete Xray metadata is
  accepted only after it resolves to an active managed backhaul and validates as
  an apply-ready revision.
- No new privileged agent command, broad database migration or shell execution
  surface is introduced.

## Verification Evidence

- `go test ./internal/infra/postgres` passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- `scripts/docs-consistency.sh` passed for release `7.0.1.40`.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh` passed
  (`passed=12 skipped=6`), including version/tag consistency, unit tests,
  race tests, vulnerability scan, binary version checks, shell syntax checks,
  docs consistency, installer validation and static security-pattern scan.

## Residual Risk

- Runtime validation still requires a real ingress node with active Xray,
  `nft`, `iproute2`, systemd and at least one healthy managed backhaul
  transport.
- Operators must still let the queued `instance.apply` complete. That successful
  apply is what causes route-policy refresh to be generated from current Xray
  metadata.
