# Security and Release Review: 7.0.1.23

**Release:** `7.0.1.23`

## Scope

- Client VLESS camouflage provisioning UX.
- Client access endpoint projection for Xray behind Nginx.
- Hard client delete cleanup verification for PostgreSQL-backed installations.

## Changes

- Fixed the client provisioning result modal so the queued state replaces the
  full modal body instead of rendering inside the previous form grid.
- Made provisioning result cards responsive for long job IDs, endpoints and
  service labels.
- Added public-client-endpoint projection for Xray instances with explicit
  `public_*` profile fields. Existing service access rows now show the
  `portal.example.com:443` client endpoint and, when relevant, the backend
  `portal.example.com:7080` service endpoint separately.
- Persisted public endpoint metadata for new Xray provisioning records so API
  consumers receive `client_endpoint`, `endpoint_kind=public` and
  `backend_endpoint` without reverse-engineering instance specs.
- Updated managed baseline route display from the misleading
  `auto / requires explicit ingress output` state to `service default`.
- Added PostgreSQL integration coverage for hard client deletion across
  `client_accounts`, `service_accesses`, `client_access_routes`, `artifacts`,
  `share_links`, `client_subscriptions`, `client_email_deliveries`,
  `service_access` secret refs and managed artifact files.
- Bumped release metadata and web asset cache keys to `7.0.1.23`.

## Security Assessment

- Public VLESS camouflage endpoint display no longer exposes the backend Xray
  listen port as the primary client endpoint, reducing operator error during
  client config issuance.
- Backend endpoint remains visible as diagnostic metadata when it differs from
  the public endpoint, preserving operability without confusing it with the
  client-facing address.
- Hard delete behavior is now backed by explicit database/file cleanup test
  coverage. Audit/job history remains outside the delete cascade by design.
- The public endpoint projection only activates on explicit `public_*` Xray
  profile fields, avoiding accidental reclassification of standalone Xray
  instances.

## Verification Evidence

- `go test ./internal/infra/postgres -run 'TestApplyXrayPublicClientEndpointMetadata|TestClientVLESSSubscriptionProfileUsesCamouflagePublicEndpoint|TestStoreArtifactRoot'`:
  passed.
- `go test ./internal/infra/postgres -run TestPostgresIntegrationDeleteClientRemovesProvisioningRows`:
  passed as a compile/package check on this workstation; the live PostgreSQL
  branch is skipped unless `MEGAVPN_TEST_DATABASE_DSN` is set.
- `go test ./...`: passed.
- `node --check web/assets/clients-page.js` with bundled Node.js: passed.
- `MEGAVPN_RELEASE_RUN_RACE=0 MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`:
  passed locally with `10` passed and `7` skipped. Skips were race test
  override, PostgreSQL release database integration, backup/restore drill,
  systemd verify, nginx verify, API smoke and VPN service matrix because this
  workstation has no disposable release DB, systemd/nginx target or live test
  node configured.

## Residual Risk

- Existing service-access rows get corrected in the UI through instance spec
  projection. Their stored JSON metadata is not rewritten until the access is
  reprovisioned or otherwise updated.
- The PostgreSQL hard-delete integration test requires a disposable test
  database DSN for live execution in CI or release lab environments.
