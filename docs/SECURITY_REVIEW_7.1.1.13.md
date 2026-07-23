# Security and Release Review: 7.1.1.13

**Release:** `7.1.1.13`

## Decision

This is a presentation and operator-safety patch. It does not change
authentication, authorization, encrypted secret storage, provider profile
parsing, agent job payloads or runtime networking. No unresolved critical or
high-severity finding was identified in the changed surface.

## Reviewed Surface

- External egress profile and deployment rendering.
- Tab state and action-handler rebinding.
- Responsive table-to-record conversion.
- Escaping of provider, node, runtime and error values.
- Browser asset cache invalidation.

## Security Properties

- Profiles and deployments still use the same authenticated API and RBAC
  checks.
- Provider credentials remain represented only by secret-purpose labels; the
  list does not expose plaintext secret values.
- Dynamic provider, node, runtime and error fields continue to pass through the
  existing HTML escaping helper.
- The selected tab is ephemeral browser state and is not persisted to storage
  or encoded in a URL.
- Splitting the views does not enable an action that was previously disabled by
  profile state, runtime support or operator permission.

## Residual Risk

The UI reports the latest stored deployment observation. Operators must still
use `Probe` and the documented node diagnostics to verify a provider data
plane. Runtime correctness remains covered by staged deployment checks rather
than presentation tests.

## Verification

JavaScript syntax, frontend bootstrap, service-pack regression, static security
patterns, Go tests, race tests, vet, production binary builds and documentation
consistency passed. Browser checks found no document-level horizontal overflow,
clipped content or overlapping actions at the tested desktop, tablet and mobile
viewports.
