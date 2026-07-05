# Security and Release Review: 7.0.1.17

**Release:** `7.0.1.17`

## Scope

- Fix managed backhaul fallback UX when the active transport fails but an
  OpenVPN UDP standby transport is healthy.
- Add a controlled API/UI operation to promote a healthy standby transport to
  the active ingress-to-egress route path.
- Keep documentation and release metadata aligned with the new patch release.

## Changes Reviewed

- Added `POST /api/v1/backhaul-links/{id}/promote` with `transport_id`.
- Added `PromoteBackhaulTransport` in the PostgreSQL store. The operation only
  promotes an existing transport on the same link when it is route-capable,
  `active`, and applied on both ingress and egress.
- Promotion updates `selected_transport_id`, `desired_driver`, link status and
  metadata, then queues an ingress `node.route_policy.apply` refresh.
- Backhaul Manage UI now labels non-selected healthy transports as
  `standby ready` instead of showing them as a second selected active path.
- Backhaul summary shows a standby-available notice when the selected active
  transport has a health issue.
- Added UI action `Promote to active` for promotable standby transports.
- Added PostgreSQL integration coverage for promoting OpenVPN UDP standby after
  WireGuard apply failure and verifying route-policy projection uses the
  promoted transport.

## Security Assessment

- No automatic failover was introduced. Production route projection changes only
  after an authenticated operator with `node.write` explicitly promotes a
  standby transport.
- Promotion is scoped to an existing transport under the same backhaul link; the
  API does not accept arbitrary interface names, node IDs or generated paths.
- Materialize-only and non-route-capable proxy transports cannot be promoted
  into kernel route projection.
- Disabled/deleted links are not promotable.
- The operation creates audit evidence through `backhaul.transport.promote` and
  schedules route-policy refresh instead of mutating node routes directly.

## Verification Evidence

- Bundled Node.js syntax check for `web/assets/backhaul-page.js`: passed.
- `go test ./internal/infra/postgres ./internal/api/http`: passed.
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

- Live node verification is still required on real ingress/egress nodes:
  promote OpenVPN UDP standby, run Backhaul Test, then verify VLESS remote
  egress traffic exits through the promoted OpenVPN path.
- Automatic failover remains intentionally out of scope until policy,
  hysteresis, audit and rollback semantics are designed.
