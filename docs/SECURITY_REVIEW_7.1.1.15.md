# Security and Release Review: 7.1.1.15

**Release:** `7.1.1.15`

## Decision

This patch closes the package-managed XL2TPD lifecycle gap without adding a
global process-name kill. No unresolved critical or high-severity finding was
identified in the changed surface.

## Reviewed Surface

- L2TP/IPsec Apply takeover and deployment Cleanup.
- Emergency L2TP runtime cleanup.
- UDP/1701 owner extraction.
- `/proc/<pid>/exe`, command-line and cgroup ownership checks.
- PID reuse and ownership-change behavior.
- Process termination bounds.

## Security Properties

- A listener is considered package-managed only after an explicit attempt to
  disable and stop `xl2tpd.service`.
- The listener executable must resolve to `xl2tpd`.
- `/proc/<pid>/cgroup` must contain an exact path component named
  `xl2tpd.service`; suffixes, prefixes and unrelated units are rejected.
- Executable and ownership evidence are revalidated immediately before
  `SIGTERM`.
- MegaVPN deployment runtimes still require an allowlisted managed
  configuration or runtime path in the process command line.
- Cleanup does not use `pkill`, `killall` or another global process-name
  operation.
- An unknown listener remains active and blocks Apply fail-closed.
- Job evidence contains process metadata and does not expose provider secrets.

## Residual Risk

The node agent runs with root privileges and trusts root-owned `/proc` and
systemd state. A root-level node compromise is outside the control-plane trust
boundary. A custom operator service legitimately using UDP/1701 must be moved
or stopped explicitly; it is not treated as managed runtime.

## Verification

Unit tests cover package-service termination, exact cgroup matching, unrelated
service rejection, managed command-line recovery and PID revalidation. Go
tests, the agent race detector, `go vet`, documentation consistency and the
full release self-test pass for this release.
