# Security and Release Review: 7.1.1.8

**Release:** `7.1.1.8`

## Decision

This release is an agent lifecycle hardening hotfix for partial external egress
deployments. It is suitable for production promotion after the documented
release gates pass.

## Security Changes

- Cleanup recognizes only explicit Linux `ip route` missing-table diagnostics
  as an already-clean state.
- Permission errors, residual policy rules and non-empty routing tables remain
  hard failures.
- Cleanup does not install StrongSwan or execute a missing `ipsec` command.
- A present `ipsec` executable is resolved before execution and is required for
  a successful L2TP/IPsec capability check.
- Emergency cleanup uses the same bounded managed table and mark ranges as
  before; the hotfix does not broaden its ownership boundary.

## Failure Model

An apply may stop before packages, managed files, a systemd unit or a routing
table are created. Cleanup must converge from each of those states. Missing
resources are idempotent success conditions, while ownership, authorization and
kernel mutation failures remain visible to the operator.

## Residual Risk

Package installation still depends on the node's configured Ubuntu repositories
and network access. A failed `runtime_capability` stage must be diagnosed from
the job evidence; cleanup success does not imply that the provider runtime can
subsequently be installed or connected.

## Verification

Regression coverage includes missing FIB table cleanup and rejection of
permission failures. Agent tests, the full Go test suite, documentation
consistency checks and release self-tests form the release evidence.
