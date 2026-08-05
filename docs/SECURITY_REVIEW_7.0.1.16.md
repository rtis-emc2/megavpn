# Security and Release Review: 7.0.1.16

**Release:** `7.0.1.16`

## Scope

- Clarify Backhaul create UX so operators can distinguish the single active
  ingress-to-egress transport from optional standby transport profiles.
- Remove the implicit OpenVPN UDP standby selection during backhaul creation.
- Keep release metadata, asset cache-busting and documentation aligned with
  the new patch release.

## Changes Reviewed

- `Active backhaul transport` now names the selected path used by apply, probe,
  route projection and ingress-to-egress traffic.
- `Optional standby transports` now starts with no extra selections. Standby
  profiles are generated only when the operator explicitly checks them.
- The active transport checkbox is locked as always included, and changing the
  active transport clears the previous active-only selection instead of silently
  converting it into standby.
- Backhaul status badges now use `active`, `standby selected` and
  `not created` states.
- Backhaul documentation and user guides now use the same terminology as the UI.

## Security Assessment

- Operator-error risk reduced: the UI no longer makes it look like unrelated
  client VPN protocols are being selected for one backhaul link.
- Change scope is frontend state/labeling plus documentation. The backend
  `desired_driver` and `drivers` API contract remains unchanged.
- Default runtime blast radius is smaller: create requests now include only the
  active transport unless the operator deliberately selects standby profiles.
- No new secret exposure, filesystem write path, SQL query path, authentication
  path or agent job executor path was introduced.

## Verification Evidence

- Bundled Node.js syntax check for `web/assets/backhaul-page.js`: passed.
- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...`: passed with
  no vulnerabilities found.
- `MEGAVPN_RELEASE_RUN_RACE=0 MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`:
  passed locally with `10` passed and `7` skipped. Skips were race test
  override, PostgreSQL integration, backup/restore drill, systemd verify, nginx
  verify, API smoke and VPN service matrix because this workstation has no
  disposable DB, systemd/nginx target or test node configured.

## Residual Risk

- Browser-level visual verification on the live authenticated Control Plane
  should be repeated against the deployed UI after publish.
- Runtime E2E still needs a disposable control plane and ingress/egress nodes to
  prove VLESS traffic over the selected managed backhaul path.
