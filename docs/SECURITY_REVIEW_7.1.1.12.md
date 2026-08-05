# Security and Release Review: 7.1.1.12

**Release:** `7.1.1.12`

## Decision

This is a presentation and operator-safety patch. It does not change
authentication, authorization, secret storage, provider configuration parsing,
agent jobs or runtime networking. No unresolved critical or high-severity
finding was identified in the changed surface.

## Reviewed Surface

- External egress profile rendering and protocol-specific form state.
- Shared modal size selection.
- Responsive deployment-list rendering.
- Provider configuration file and textarea handling.
- Browser asset cache invalidation.

## Security Properties

- Provider configuration, credentials, certificates and private keys remain
  submitted through the existing authenticated API and encrypted secret
  storage path.
- New UI state is held only in the active browser form and is not persisted to
  local storage or exposed in page URLs.
- Dynamic values continue to pass through the existing HTML escaping helper.
- File input contents are not rendered into HTML.
- The patch does not weaken RBAC checks or enable actions that were previously
  unavailable.

## Residual Risk

The browser necessarily holds provider secrets while an operator edits or
validates a profile. Operators must use trusted administrative workstations and
close the form when work is complete. Runtime protocol correctness still
requires the staged node and data-plane checks documented in the external
egress runbook.

## Verification

JavaScript syntax, frontend bootstrap, service-pack regression, static security
patterns, Go tests, race tests, vet, production binary builds and documentation
consistency passed. Responsive browser checks found no horizontal overflow,
overlapping controls or console errors at the tested desktop and mobile
viewports.
