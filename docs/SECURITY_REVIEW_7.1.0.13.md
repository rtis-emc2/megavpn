# Security and Release Review: 7.1.0.13

**Release:** `7.1.0.13`

## Scope

This review covers the stabilization increment after `7.1.0.12`:

- single generated client artifact deletion through authenticated API and UI;
- route-policy job evidence enrichment with managed `ip route show table`
  telemetry;
- operator documentation for artifact cleanup and route-policy evidence;
- frontend asset cache-busting for the updated UI.

## Security Notes

- Artifact deletion is scoped by `client_account_id` and `artifact_id`, requires
  the existing `artifact.export` permission and removes only share links pointing
  to the selected artifact.
- Filesystem cleanup reuses the existing artifact-root containment check before
  deleting a stored file. Unmanaged paths are skipped and reported as file
  errors instead of being removed.
- Route-policy telemetry only runs fixed `ip route show table <table>` commands
  for tables derived from route-policy payloads/snapshots after safe-token
  validation, de-duplication and a bounded table limit.
- No VPN data-plane routing semantics changed in this release.

## Validation

- Unit and integration tests cover route-policy telemetry table extraction and
  single-artifact deletion without removing service access or client route rows.
- Full release validation still requires a live disposable-node staged smoke run
  for VLESS camouflage, route-policy kernel state, cleanup lifecycle and
  OpenVPN/WireGuard/Xray accounting counters.

## Residual Risk

- Live-node behavior depends on host `ip`, `nft`, `nginx`, Xray, OpenVPN and
  WireGuard runtime state. The code path is covered locally, but final
  production acceptance must use staged smoke evidence from disposable nodes.
