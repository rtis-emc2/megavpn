# Security and Release Review: 7.1.1.17

**Release:** `7.1.1.17`

## Decision

The patch removes a false-fatal Apply condition without weakening the
external-egress fail-closed routing boundary. No unresolved critical or
high-severity finding was identified in the changed surface.

## Reviewed Surface

- External-egress policy-routing table initialization.
- Fail-closed unreachable route installation.
- Deployment-specific fwmark rule installation.
- Missing-table and permission-failure classification.
- Apply and Cleanup idempotency.

## Security Properties

- Only known missing-table diagnostics are accepted as an empty initial state.
- The managed table receives an `unreachable default` before the fwmark rule is
  added.
- The provider default route is installed later by the managed runtime only
  after its interface becomes available.
- Permission errors, malformed command output and unknown failures remain
  fatal.
- The node main routing table is not modified.
- Routing table number, fwmark and rule priority remain constrained to the
  deployment-managed ranges validated by job decoding.

## Residual Risk

The agent relies on the Linux `ip` command diagnostics to distinguish an absent
table from other failures. Known IPv4, IPv6 and generic missing-table messages
are recognized; an unknown platform-specific message fails closed and is
reported to the operator.

## Verification

Unit tests cover missing-table initialization, ordered fail-closed guard
creation, permission failure rejection and Cleanup behavior. Full Go tests,
race tests, static security checks, script audits, documentation consistency
and release self-test are required before tag publication.
