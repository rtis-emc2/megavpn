# Security and Release Review: 7.0.1.25

**Release:** `7.0.1.25`

## Scope

- Selective service-pack component creation in API and Web UI.
- Per-component listen-port overrides and OpenVPN CA profile overrides.
- Runtime preflight, TLS certificate preflight and traffic-camouflage validation
  scoped to the selected components.

## Changes

- Added optional `components` selection to
  `POST /api/v1/service-packs/{key}/instances`.
- Preserved backward compatibility: requests without `components` still create
  every component in the pack.
- Added strict selection validation for required index, duplicate index,
  out-of-range index, listen-port range and OpenVPN-only PKI overrides.
- Updated `Instances -> Create from pack` so operators can check only the
  services to create, tune per-component listen ports and choose pack-level or
  component-level OpenVPN CA profile material.
- Disabled unrelated global form sections when their service type is not
  selected, so disabled camouflage/TLS/OpenVPN/VLESS controls are not submitted.
- Bumped release metadata and web asset cache keys to `7.0.1.25`.

## Security Assessment

- The API accepts typed component overrides only. It does not accept arbitrary
  raw `spec` JSON from the service-pack UI path.
- Runtime install jobs are created only for selected component runtime
  capabilities, reducing unnecessary package installation on nodes.
- TLS certificate default lookup and traffic-camouflage fallback validation are
  evaluated against selected components only.
- Component selection is fail-closed: an explicit empty list, missing index,
  duplicate index or invalid port returns `400`.
- OpenVPN PKI override is rejected for non-OpenVPN components.

## Verification Evidence

- `go test ./...`: passed.
- `go test ./internal/api/http -count=1`: passed.
- Bundled Node.js `node --check web/assets/instances-page.js`: passed.
- Bundled Node.js syntax-check for every `web/assets/*.js`: passed.

## Residual Risk

- Visual validation still depends on an operator session with seeded node,
  certificate and service-pack data; local syntax/unit tests do not prove every
  browser viewport state.
- Component selection is scoped to creation time. Existing service packs and
  existing instances are not rewritten by this release.
