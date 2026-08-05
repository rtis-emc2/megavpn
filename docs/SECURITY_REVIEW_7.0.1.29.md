# Security and Release Review: 7.0.1.29

**Release:** `7.0.1.29`

## Scope

- Corrective audit pass after `7.0.1.28` default firewall baseline.
- Preservation of operator-disabled firewall address lists during automatic
  catalog seeding.

## Changes

- `EnsureDefaultFirewallPolicies` now updates default firewall address-list
  labels/descriptions without changing an existing list status.
- Added migration `000012_firewall_seed_status_preserve` to mark the release
  boundary for upgraded installations.
- Added PostgreSQL integration coverage that disables `vpn_client_sources` and
  verifies a subsequent `FirewallInventory()` call does not re-enable it.
- Bumped release metadata and web asset cache keys to `7.0.1.29`.

## Security Assessment

- The default firewall baseline remains available, but operator intent wins for
  address-list enablement. This prevents a disabled `trusted_operators` or
  `vpn_client_sources` list from becoming active again during a normal UI/API
  refresh.
- Existing strict-apply safety behavior is unchanged: operators still need to
  keep explicit allow rules and source lists consistent before enforcing drop
  defaults.

## Verification Evidence

- `go test ./...`: passed.
- `go test -race ./...`: passed.
- `go vet ./...`: passed.
- Bundled Node.js `node --check web/assets/firewall-page.js`: passed.
- `git diff --check`: passed.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`: passed with 11
  checks passed and 6 local environment skips.

## Residual Risk

- Live PostgreSQL migration execution still requires a configured disposable
  database DSN.
- Live nftables validation still requires an Ubuntu node with `nft` and managed
  firewall apply privileges.
