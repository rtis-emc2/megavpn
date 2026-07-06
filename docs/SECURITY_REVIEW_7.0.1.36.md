# Security and Release Review: 7.0.1.36

**Release:** `7.0.1.36`

## Scope

- Xray/VLESS client provisioning identity reuse.
- Pending service-access materialization into Xray managed clients.
- Xray UUID rotation behavior for explicit operator rotation.
- Operator documentation for VLESS client identity across ingress instances.

## Changes

- Added an atomic `EnsureXrayServiceAccessUUID` Store helper that locks a
  service access row before assigning or reusing a VLESS UUID.
- Changed client provisioning metadata to reuse an existing non-rotating
  Xray/VLESS UUID for the same client when provisioning additional ingress
  instances.
- Updated the worker Xray provisioning path to use the Store helper instead of
  generating UUIDs ad hoc.
- Hardened Xray managed-client rendering so legacy pending accesses without a
  UUID are materialized before the instance spec is rebuilt.
- Marked explicit Xray UUID rotation with `rotate_credentials`, so rotation
  issues a new UUID instead of reusing the current client UUID.
- Added PostgreSQL regression coverage for UUID reuse across Xray instances and
  explicit rotation.

## Security Assessment

- The change narrows credential drift: a client provisioned onto a new VLESS
  ingress receives the same existing UUID, so the new server accepts the
  credential that was already issued to that client.
- UUID assignment is performed under a row lock, reducing concurrent
  provisioning races for the same service access.
- Reuse is scoped to the same client account and non-revoked Xray/VLESS service
  accesses only; deleted instances and rotation-marked accesses are excluded.
- Explicit rotation remains fail-closed for stale credentials: the rotation flag
  forces a new UUID and is cleared after the new UUID is stored.
- No authentication, RBAC, public subscription-token hashing, share-link token
  handling, agent authentication or firewall enforcement behavior changed.

## Verification Evidence

- `go test ./internal/infra/postgres -run 'TestPostgresIntegrationXrayProvisioningReusesClientUUIDAcrossInstances|TestApplyXrayPublicClientEndpointMetadata|TestClientVLESSSubscriptionProfileUsesCamouflagePublicEndpoint|TestResolveXrayVLESSGroupEgressWithResolver' -count=1`
  passed.
- `go test ./cmd/worker -run 'TestBuildXrayArtifacts|TestSelectArtifactFiles' -count=1`
  passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.
- `scripts/docs-consistency.sh` passed for release `7.0.1.36`.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh` passed
  (`passed=12 skipped=6`), including `go test -race ./...`,
  `govulncheck ./...`, binary version checks, shell syntax checks, docs
  consistency, installer validation and static security-pattern scan.

## Residual Risk

- The PostgreSQL integration test is skipped unless `MEGAVPN_TEST_DATABASE_DSN`
  is configured; live release validation should include provisioning one client
  onto two Xray/Nginx camouflage ingress instances and confirming both agents
  apply the same client UUID.
- Existing deployments with previously divergent per-instance VLESS UUIDs will
  converge only when the operator reprovisions or rotates affected accesses.
