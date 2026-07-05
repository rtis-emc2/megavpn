# Security and Release Review: 7.0.1.31

**Release:** `7.0.1.31`

## Scope

- Documentation consistency hardening.
- Release-gate/self-test alignment for maintained documentation artifacts.
- Route-policy apply telemetry for node-side troubleshooting.

## Changes

- Added `scripts/docs-consistency.sh` as a shared documentation gate for
  release and self-test workflows.
- `scripts/release-gate.sh` now fails on missing/stale maintained docs,
  outdated release banners, broken primary documentation links, stale Web UI
  asset cache keys and vendor-specific firewall wording in current operator
  docs.
- `scripts/self-test.sh` now delegates its `docs-consistency` gate to the same
  documentation consistency script, eliminating duplicate file lists.
- Synchronized roadmap, documentation indexes, release docs and production env
  templates to `7.0.1.31`.
- `node.route_policy.apply` agent results now include route-policy telemetry:
  systemd unit/timer active checks, `ip rule show`, and managed nftables
  `route_policy_output` / `route_policy_prerouting` chain snapshots.

## Security Assessment

- The documentation gate reduces release drift risk for security-sensitive
  operational procedures, env templates and current security review links.
- Route-policy telemetry is collected after managed apply using read-only
  system commands and is returned in the existing signed job-result path. It
  does not change kernel routing behavior and does not expose route-policy
  secrets.
- Telemetry output is truncated before persistence to avoid unbounded job
  result growth.
- The telemetry can include local route table and nftables state. That is
  appropriate for `node.route_policy.apply` job viewers and should remain under
  existing job/node read permissions.

## Verification Evidence

- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`: passed
  (`passed=12 skipped=6`).
- `govulncheck ./...`: no vulnerabilities found.
- `go test ./...`: passed.
- `go test -race ./...`: passed.
- `go vet ./...`: passed.
- `scripts/docs-consistency.sh`: passed.
- `web/assets/node-workflows.js` syntax check with bundled Node.js: passed.
- `git diff --check`: passed.

## Residual Risk

- Live route-policy telemetry still requires a real Linux node with `systemd`,
  `ip` and `nft`; workstation release-gate runs can only validate syntax and
  unit tests.
- The documentation gate validates current maintained docs and primary links,
  but it intentionally does not rewrite or fail historical security-review
  artifacts for older releases.
