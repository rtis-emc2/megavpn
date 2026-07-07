# Security and Release Review: 7.1.0.14

**Release:** `7.1.0.14`

## Scope

This review covers the firewall apply stabilization increment after `7.1.0.13`:

- PostgreSQL lookup hardening for firewall policies and address lists;
- atomic `node.firewall.apply` persistence for revision, job and desired node
  state;
- regression coverage for `node.firewall.apply` revision, job and node-state
  persistence;
- migration `000017_firewall_apply_schema_hardening` for upgraded databases;
- release version bump for deployable backend/runtime assets.

## Security Notes

- Firewall policy and address-list lookups no longer reuse one prepared
  statement parameter as both `uuid` and `text`. The runtime validates UUID
  shape before passing a typed UUID lookup parameter, which avoids PostgreSQL
  type ambiguity and prevents key strings from being cast as UUIDs.
- The firewall apply path still requires `firewall.apply`; catalog mutation
  remains separate under `firewall.manage`.
- The new migration only aligns check constraints with the runtime catalog
  model. It does not weaken rule evaluation, job permissions or node trust.
- Apply jobs continue to write desired state first and only mark a revision as
  applied after the signed agent completes `node.firewall.apply` successfully.
- Revision, job and desired node-state rows are now inserted in a single
  transaction, reducing orphaned apply state after database write failures.

## Validation

- `go test ./internal/jobschema -count=1`
- `go test ./internal/infra/postgres -run 'TestPostgresIntegration(DefaultFirewallBaseline|FirewallApplyCreatesRevisionJobAndNodeState)' -count=1 -v`
  was executed locally; PostgreSQL integration tests were skipped because
  `MEGAVPN_TEST_DATABASE_DSN` is not set in this workspace.

## Residual Risk

- Full verification still requires release-gate or disposable PostgreSQL
  execution with `MEGAVPN_TEST_DATABASE_DSN` so the new firewall apply
  integration test runs against a real database.
- Node-side nftables behavior is unchanged by this release; live firewall
  acceptance still depends on staged node apply/preview evidence.
