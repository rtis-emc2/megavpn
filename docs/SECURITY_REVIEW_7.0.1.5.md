# Security and Release Review: 7.0.1.5

**Release:** `7.0.1.5`

Date: 2026-07-05

Scope:

- Delta review for VLESS access-group catalog sync performance optimization.
- PostgreSQL sync query path for Xray/VLESS instances and latest specs.
- Catalog snapshot merge behavior for active, disabled, deleted and custom
  groups.
- Remote egress resolution reuse inside one Xray/VLESS instance spec.

## Result

No new P0/P1 security defect was found in the reviewed delta paths.

This is a targeted delta review on top of the `7.0.1.4` VLESS group sync
release. It does not replace a full independent repository security scan.

## Reviewed Controls

| Control | Result |
| --- | --- |
| Catalog snapshot scope | Active and full catalog templates are loaded once per sync run |
| Xray instance selection | Sync targets only non-deleted `xray-core` instances |
| Latest spec loading | Xray instances are loaded with latest revision spec in one query |
| Deleted managed group cleanup | Inactive/deleted managed catalog keys are removed from synced specs |
| Custom group preservation | Non-managed instance-local groups remain intact |
| Remote egress lookup reuse | Groups that point to the same egress node reuse one resolved backhaul projection per instance |
| Apply path | Existing revision validation and `instance.apply` queue path are preserved |

## Automated Checks

Passed:

```bash
go test -count=1 ./...
node --check web/assets/instances-page.js
PATH=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH scripts/self-test.sh
MEGAVPN_RELEASE_ALLOW_SKIPS=1 PATH=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH scripts/release-gate.sh
```

Self-test result: `16` passed, `0` failed, `6` skipped for host/live evidence.
Release gate result: `10` passed, `0` failed, `6` skipped with
`MEGAVPN_RELEASE_ALLOW_SKIPS=1`.

## Remaining Release Blockers

1. Run full production release gates on a release host with PostgreSQL,
   backup/restore, `systemd`, Nginx and service smoke evidence enabled.
2. Run live VLESS catalog sync smoke against multiple Xray/VLESS instances on
   remote nodes and confirm only changed instances receive apply jobs.
3. Complete delegated repository-wide security scan before any stable release
   claim.
