# Security and Release Review: 7.1.1.14

**Release:** `7.1.1.14`

## Decision

This patch closes an L2TP lifecycle integrity gap without broadening process
control. No unresolved critical or high-severity finding was identified in the
changed surface.

## Reviewed Surface

- External L2TP/IPsec apply takeover.
- External L2TP/IPsec cleanup ordering.
- Managed PID-file parsing and process ownership verification.
- Process termination bounds and context cancellation.
- UDP/1701 listener evidence.
- Cleanup failure and retry behavior.

## Security Properties

- A saved PID is never signaled unless `/proc/<pid>/exe` resolves to
  `xl2tpd`.
- A listener recovered without a PID file is signaled only when `ss` supplies
  its PID, `/proc/<pid>/exe` resolves to `xl2tpd` and
  `/proc/<pid>/cmdline` references an allowlisted MegaVPN L2TP path.
- PID reuse is treated as stale ownership evidence; the unrelated process is
  not terminated.
- Cleanup does not use `pkill`, `killall` or another global name-based process
  operation.
- Failure to inspect or stop the managed process preserves its PID file and
  runtime directory for a safe retry.
- TERM and KILL waits are bounded and honor job context cancellation.
- An unknown UDP/1701 listener blocks apply and is reported to the operator
  rather than being stopped automatically.
- Job evidence contains process and listener metadata, not provider secrets.

## Residual Risk

The agent trusts the root-owned managed runtime directory and PID file it
created. A root-level compromise of the provider node can replace that evidence
and is outside the control-plane trust boundary. Runtime ownership remains
protected by root-only managed paths and the executable identity check.

An unrelated service that legitimately binds UDP/1701 must still be moved or
removed by an operator before a managed L2TP/IPsec deployment can start.

## Verification

Unit and regression tests cover successful orphan termination, PID reuse
without signaling, cleanup-before-delete ordering and listener owner
extraction. Go tests, agent race tests, documentation consistency and the full
release self-test pass for this release.
