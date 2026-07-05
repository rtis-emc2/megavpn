# Security and Release Review: 7.0.1.18

**Release:** `7.0.1.18`

## Scope

- Repair firewall catalog schema drift on installations where the consolidated
  baseline migration was applied before firewall tables were introduced.
- Remove vendor-specific wording from the Firewall address-list workflow.
- Hide internal identity controls from manual Firewall policy and address-list
  forms; keys remain generated and preserved server-side.

## Changes

- Added `migrations/000009_firewall_schema_repair.up.sql` as an idempotent
  additive repair migration for firewall tables, indexes, RBAC permissions,
  default address lists and baseline policies.
- Updated Firewall UI copy from vendor-oriented wording to functional address
  group language.
- Simplified Firewall create/edit dialogs so manual operators configure names,
  scope, status and policy/rule behavior instead of internal keys.
- Bumped release metadata, documentation banners and web asset cache keys to
  `7.0.1.18`.

## Security Assessment

- The schema repair is additive and idempotent: it does not drop tables, alter
  existing rows destructively or weaken foreign-key relationships.
- `firewall.manage` remains required for policy, rule and address-list writes;
  `firewall.apply` remains required for node apply jobs.
- Address-list keys and policy keys are still normalized and validated in the
  PostgreSQL repository layer before insert/update.
- No new secret material is stored by this change.

## Operational Notes

- Existing deployments must run migrations through
  `000009_firewall_schema_repair` before creating Firewall address lists.
- A clean install remains compatible: the baseline already contains the firewall
  catalog, and this repair migration no-ops where objects already exist.
- The previous runtime error `relation "firewall_address_lists" does not exist`
  indicates the database schema is behind the release and should be remediated
  by `megavpn-migrate.service`.

## Verification Evidence

- Bundled Node.js syntax check for `web/assets/firewall-page.js`: passed.
- `go test ./internal/infra/postgres ./internal/api/http`: passed.
- `scripts/self-test.sh`: passed all runnable code, docs, build and static
  gates before tagging; the only failure was the expected pre-tag
  `version-tag-consistency` gate for `v7.0.1.18`.
- `go vet ./...`: passed.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...`: passed with no
  vulnerabilities found.
- `git diff --check`: passed.
- `MEGAVPN_RELEASE_RUN_RACE=0 MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`:
  passed locally with `10` passed and `7` skipped. Skips were race test
  override, PostgreSQL integration, backup/restore drill, systemd verify, nginx
  verify, API smoke and VPN service matrix because this workstation has no
  disposable DB, systemd/nginx target or test node configured.

## Residual Risk

- PostgreSQL migrations must be executed against each deployed control-plane
  database before operators use the Firewall catalog.
- DNS entries in address lists remain catalog-only in this release and are not
  rendered into nftables matchers.
