# Security and Release Review: 7.0.1.15

**Release:** `7.0.1.15`

## Scope

- Fix UI/API regression: `cannot insert multiple commands into a prepared
  statement (SQLSTATE 42601)`.
- Remove duplicated service-pack templates from API/UI catalog views.
- Add a release-gate guard against multi-command SQL in production Go runtime
  paths.

## Changes Reviewed

- `EnsureDefaultFirewallPolicies` now executes address-list and policy default
  seeds as separate statements inside one transaction.
- `EnsureDefaultAddressPoolSpaces` now uses one multi-row insert instead of two
  SQL commands in one prepared statement.
- Service-pack list queries now use canonical `distinct on (key)` reads.
- Service-pack default seeding repairs historical duplicate rows and ensures a
  unique key index when old databases do not have one.
- Web core loading deduplicates service-pack lists by `key` and prefers active,
  custom and newer rows.
- Release gate scans production Go runtime code for multi-command SQL patterns.

## Security Assessment

- Availability risk reduced: `/api/v1/firewall` and address-pool catalog loading
  no longer fail in pgx prepared-statement mode due to multi-command SQL.
- Operator integrity improved: Create from pack no longer presents duplicate
  templates that could cause accidental repeated rollout attempts.
- Schema drift resilience improved: older databases with duplicated service-pack
  rows are repaired during default catalog seeding.

## Verification Evidence

- `go test ./internal/infra/postgres ./internal/api/http`: passed.
- Bundled Node.js syntax check for `web/assets/core-loader.js`: passed.
- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...`: passed with
  no vulnerabilities found.
- `MEGAVPN_RELEASE_RUN_RACE=0 MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`:
  passed locally with `10` passed and `7` skipped. Skips were race test
  override, PostgreSQL integration, backup/restore drill, systemd verify, nginx
  verify, API smoke and VPN service matrix because this workstation has no
  disposable DB, systemd/nginx target or test node configured.

## Residual Risk

- Runtime E2E still needs a disposable control plane and node to prove the full
  VPN service matrix, including VLESS ingress-to-egress traffic.
