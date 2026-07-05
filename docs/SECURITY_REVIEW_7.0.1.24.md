# Security and Release Review: 7.0.1.24

**Release:** `7.0.1.24`

## Scope

- Instance deletion cleanup for dependent client service access.
- Manual stale service-access removal from the Client Access modal.
- PostgreSQL cleanup coverage for service-access scoped artifacts and secrets.

## Changes

- Added `DELETE /api/v1/clients/{id}/accesses/{access_id}` to remove one client
  service binding without deleting the client account.
- Added a Client Access UI action, `Remove access`, beside rotation actions.
  It deletes the selected service access, managed route rows, generated config
  artifacts, delivery links for those artifacts and service-access scoped
  secrets.
- Changed instance deletion behavior so active client service access no longer
  blocks queuing managed cleanup. After the agent confirms `instance.delete`
  succeeded, the Control Plane removes dependent service access, routes,
  generated artifacts, delivery links and service-access secrets.
- Node emergency cleanup now uses the same service-access cleanup path instead
  of leaving failed dangling client access rows.
- Added PostgreSQL integration tests for manual service-access deletion and
  instance-delete cleanup of dependent client access rows/files.
- Bumped release metadata and web asset cache keys to `7.0.1.24`.

## Security Assessment

- Deleting a service instance no longer leaves stale client credentials,
  generated config artifacts or delivery links attached to a non-existent
  runtime service.
- Manual service-access deletion is scoped to one client/access pair and does
  not delete the client account or unrelated service access.
- Artifact file removal still validates that paths are under the managed
  artifact root before deleting from disk.
- Audit/job history remains retained for traceability; only live access/config
  material is removed.

## Verification Evidence

- `go test ./internal/infra/postgres -run 'TestPostgresIntegrationDeleteClientServiceAccessRemovesRows|TestPostgresIntegrationInstanceDeleteRemovesClientServiceAccessRows|TestPostgresIntegrationDeleteClientRemovesProvisioningRows|TestApplyXrayPublicClientEndpointMetadata'`:
  passed as a compile/package check on this workstation; the live PostgreSQL
  branches are skipped unless `MEGAVPN_TEST_DATABASE_DSN` is set.
- `go test ./...`: passed.
- `node --check web/assets/clients-page.js` with bundled Node.js: passed.
- `node --check web/assets/instances-page.js` with bundled Node.js: passed.
- `git diff --check`: passed.
- `MEGAVPN_RELEASE_RUN_RACE=0 MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`:
  passed locally with `10` passed and `7` skipped. Skips were race test
  override, PostgreSQL release database integration, backup/restore drill,
  systemd verify, nginx verify, API smoke and VPN service matrix because this
  workstation has no disposable release DB, systemd/nginx target or live test
  node configured.

## Residual Risk

- If an agent-side instance delete job fails, dependent client access remains
  until the operator retries cleanup or manually removes the access from the
  Client Access modal.
- Live PostgreSQL cleanup assertions require a disposable test database DSN in
  CI or release lab environments.
