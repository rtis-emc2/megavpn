# Security and Release Review: 7.0.1.11

**Release:** `7.0.1.11`

## Scope

- Backhaul create/details UI clarity fix for the ingress-to-egress transport
  selection model.
- Documentation update for the difference between `Primary transport driver`
  and `Transport profiles to create`.
- Release baseline, web asset cache-busting and required release artifacts were
  updated to `7.0.1.11`.
- No backend API, database migration, agent apply/probe/cleanup, firewall
  enforcement, VLESS routing, authentication, RBAC or secret-handling behavior
  was changed in this release.

## Changes Reviewed

- The Backhaul create modal now labels `desired_driver` as the primary
  transport driver.
- The selected primary driver is always checked in the transport profile list,
  visibly marked as `primary`, and serialized first in the `drivers` payload.
- Additional checked drivers are presented as standby profiles; unchecked
  drivers remain available options.
- The Backhaul details modal now labels the selected driver as
  `Primary transport`.
- `docs/BACKHAUL.md`, English user guide and Russian user guide now explain
  that these controls configure internal node-to-node backhaul transports, not
  client-facing VPN protocols.

## Security Assessment

- Attack surface change: none. The release does not add endpoints, privileged
  job types, node commands, persistence fields or public UI data sources.
- Authorization risk: unchanged. Backhaul create/apply/probe/delete remains
  controlled by the existing API and RBAC path.
- Data exposure risk: unchanged. The UI renders existing driver metadata only;
  no secret refs or generated config contents are exposed.
- Operational risk reduced: operators are less likely to misconfigure a link by
  treating the primary backhaul driver and standby transport profiles as
  unrelated protocol choices.

## Verification Evidence

- `go test ./...`: passed.
- Frontend JavaScript syntax check with bundled Node.js: passed.
- CSS brace balance check: passed.
- Browser smoke against a local mock API: desktop and `390x844` mobile Backhaul
  create modal rendered without body/modal horizontal overflow; primary,
  standby and available badges matched checkbox state; changing the primary
  driver kept it checked and placed it first in the computed `drivers` payload.
- `scripts/self-test.sh`: final tagged run passed with `15` passed, `0` failed
  and `7` skipped on this workstation. The `frontend-js-syntax` gate is skipped
  by the script because system `node` is not on `PATH`; the same check passed
  manually with the bundled Node.js runtime.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`: final tagged run
  passed with `10` passed, `0` failed and `6` skipped on this workstation.

## Residual Risk

- Browser/live-node smoke was not run against a production inventory in this
  review. The change is bounded to static modal labels, selection state and
  payload ordering, but customer-facing deployments should still validate the
  Backhaul create flow with real ingress/egress nodes.
- This release does not implement new backhaul drivers, VLESS traffic
  camouflage routing or firewall enforcement changes; those remain separate
  implementation tracks.
