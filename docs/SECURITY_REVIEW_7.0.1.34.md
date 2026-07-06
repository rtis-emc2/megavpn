# Security and Release Review: 7.0.1.34

**Release:** `7.0.1.34`

## Scope

- VLESS client provisioning access-group validation.
- On-demand synchronization of active VLESS group catalog entries into selected
  Xray service instances.
- Selected-egress group materialization before provisioning accepts a client
  binding.

## Changes

- Client provisioning now prepares the selected Xray instance with the active
  VLESS group catalog before validating the requested group key.
- The provisioning path now uses the same group egress-resolution logic as
  catalog sync. `egress_node` groups are materialized into resolved egress
  metadata, Xray outbound config and `sendThrough` source-route metadata before
  the revision is accepted.
- Invalid group selection errors now include the available group keys after
  catalog sync.
- Regression coverage verifies that a selected-egress group such as `route`
  becomes an `egress-route` outbound with concrete `sendThrough` metadata before
  apply.

## Security Assessment

- The change is fail-closed: provisioning still rejects unknown, deleted or
  disabled group keys after catalog synchronization.
- The group key continues to be normalized through the existing VLESS group key
  sanitizer before it is stored in service-access metadata.
- Selected-egress groups are not accepted as labels only. They must resolve
  through the managed backhaul model before provisioning stores the binding.
- The API does not trust frontend state. It reloads the current instance spec
  and active catalog server-side before accepting the client binding.
- No secret material is added to client-access metadata by this change.

## Verification Evidence

- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh` passed
  (`passed=12 skipped=6`).
- `govulncheck ./...` completed with no vulnerabilities found.
- `go test ./...` passed.
- `go test -race ./...` passed.
- `go vet ./...` passed during pre-release verification.
- `go test ./internal/infra/postgres -count=1` passed after the provisioning
  path change.
- `scripts/docs-consistency.sh` passed for release `7.0.1.34`.
- `git diff --check` passed during pre-release verification.

## Residual Risk

- Live verification still requires real ingress/egress nodes with active
  backhaul to confirm route-policy apply evidence, Xray `sendThrough` behavior
  and client traffic egress.
- If an operator creates an active selected-egress group with an invalid or
  unavailable egress node, provisioning correctly fails until the backhaul
  topology is fixed or the group is disabled.
