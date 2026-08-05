# Security and Release Review: 7.0.1.37

**Release:** `7.0.1.37`

## Scope

- VLESS access-group preservation during client reprovisioning.
- Xray UUID rotation behavior for clients bound to non-default VLESS groups.
- Removal of the legacy implicit `route` fallback from generic route-value
  selection.
- Client provisioning result modal layout hardening.

## Changes

- Changed Xray client provisioning metadata selection to distinguish explicit
  operator group input from implicit reprovision/rotation paths.
- Reprovisioning and `Rotate Xray UUID` now preserve the existing
  `service_accesses` VLESS group when it is still present in the active
  instance/catalog group set.
- Explicit invalid VLESS group selections remain fail-closed and return the
  available active group keys.
- Stale implicit group/default metadata now falls back to an active catalog
  group instead of failing with a synthetic `route` value.
- Removed the hidden `"route"` fallback from `firstNonEmptyRouteValue`; callers
  that need a default must pass it explicitly.
- Changed Xray UUID materialization to use empty-string semantics when no UUID
  exists, ensuring a new UUID is generated instead of accepting a legacy helper
  fallback.
- Tightened the client provisioning queued modal so status, workflow steps and
  action buttons do not collapse into unreadable narrow columns.

## Security Assessment

- The change reduces unintended policy drift: rotating a VLESS UUID no longer
  silently moves a client from a selected egress group to an instance default.
- Explicit operator input is still strict. A typo or stale manually selected
  group is rejected instead of being remapped silently.
- Removing the synthetic `route` fallback eliminates a legacy value that could
  leak into credential, interface, table or route metadata paths that expected
  true empty-string behavior.
- Xray UUID assignment remains scoped to service access metadata and still uses
  the row-locking helper introduced in `7.0.1.36`.
- No authentication, RBAC, public subscription-token hashing, share-link token
  handling, agent authentication or firewall enforcement behavior changed.

## Verification Evidence

- `go test ./internal/infra/postgres` passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- Bundled Node `--check web/assets/clients-page.js` passed.
- `git diff --check` passed.
- `scripts/docs-consistency.sh` passed for release `7.0.1.37`.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh` passed
  (`passed=12 skipped=6`), including unit tests, race tests, vulnerability
  scan, binary version checks, shell syntax checks, docs consistency, installer
  validation and static security-pattern scan.

## Residual Risk

- PostgreSQL integration coverage still depends on
  `MEGAVPN_TEST_DATABASE_DSN`; when it is not configured, live validation should
  rotate one client bound to a non-default VLESS group and verify that both the
  new UUID and the original group are applied on the ingress instance.
- Existing records that already contain stale group metadata will only be
  normalized on reprovision, rotation or another metadata refresh path.
