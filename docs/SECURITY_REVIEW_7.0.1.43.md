# Security and Release Review: 7.0.1.43

**Release:** `7.0.1.43`

## Scope

- Agent-side executable discovery for Nginx and other node runtime binaries.
- Nginx capability verification and `instance.apply` preflight recovery.
- Control Plane capability-state side effect when Nginx apply recovery succeeds.
- Release documentation, Web UI asset cache keys and version metadata.

## Changes

- Added canonical fallback executable paths to the agent resolver for Nginx,
  systemd, Xray, OpenVPN, WireGuard, Shadowsocks, Squid and network tooling.
- Inventory collection now uses the shared resolver, so runtimes installed under
  `/usr/sbin` are reported even when the agent systemd unit has a narrow PATH.
- Nginx capability verification uses the resolved binary path for `nginx -v`
  and `nginx -t`, and includes `binary_path` in job results.
- Nginx `instance.apply` now attempts managed runtime recovery through the
  existing `installNginx` path when the preflight cannot find the Nginx binary.
- Apply continues to rendered-config validation when recovery makes the binary
  available but installer verification still fails against old Nginx config.
- Successful apply recovery marks the node capability `available` with source
  `instance_apply_recovery` inside the existing instance job completion
  transaction.

## Security Assessment

- No public API endpoint, unauthenticated path or new job type was added.
- Nginx recovery is only triggered inside an already authorized `instance.apply`
  job that the node agent has claimed through the existing signed job channel.
- The recovery path reuses the existing Nginx installer, including nginx.org
  repository validation, Ubuntu fallback checks, apt sandbox fallback and tracked
  command telemetry.
- The recovery is scoped to Nginx only. Other service runtime preflights remain
  fail-closed and continue to require their normal capability install flow.
- Continuing after a failed `nginx -t` is allowed only when the Nginx binary is
  present after recovery; final rendered-config validation still runs before the
  shared `nginx` unit is restarted.
- The Control Plane capability update trusts only agent job results for the
  same instance job completion transaction and only when
  `binary_available_after_install=true` is present in `runtime_preflight`.

## Verification Evidence

- `go test ./cmd/agent ./internal/infra/postgres` passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `scripts/docs-consistency.sh` passed.
- Regression coverage added:
  - Nginx apply preflight recovers a missing binary and continues when the
    binary is present after installer recovery.
  - Non-Nginx missing runtime preflights do not call the Nginx installer.
- Full release gate evidence is tracked by `scripts/release-gate.sh` for the
  tagged release commit.
