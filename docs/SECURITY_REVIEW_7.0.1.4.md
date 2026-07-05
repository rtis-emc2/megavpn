# Security and Release Review: 7.0.1.4

**Release:** `7.0.1.4`

Date: 2026-07-05

Scope:

- Delta review for centralized VLESS access-group catalog propagation.
- Control Plane API mutation path for VLESS group save, enable, disable and delete.
- Xray/VLESS instance revision sync, remote node `instance.apply` queueing and
  VLESS routing fallback behavior.
- Operator UI and documentation updates for group sync visibility.

## Result

No new P0/P1 security defect was found in the reviewed delta paths.

This is a targeted delta review on top of the `7.0.1.3` hardening baseline. It
does not replace a full independent repository security scan.

## Reviewed Controls

| Control | Result |
| --- | --- |
| VLESS group mutation authorization | Existing `settings.manage` permission still gates save/status/delete endpoints |
| Catalog propagation path | Group mutation triggers server-side sync into existing Xray/VLESS instance revisions |
| Apply queue path | Active Xray/VLESS instances receive normal `instance.apply` jobs addressed to their node |
| Disabled/draft instance behavior | Disabled, draft and deleting instances receive synced revisions but are not auto-applied |
| Revision validation | Sync pre-validates generated revisions before replacing current instance spec |
| Remote egress resolution | `egress_node` groups are resolved through managed backhaul before apply payload generation |
| Unknown group fallback | Client bindings referencing deleted or unknown groups fall back to the instance default group |
| Operator visibility | API/UI report sync failures with instance, stage and error details |

## Automated Checks

Passed before tagging:

```bash
go test -count=1 ./...
node --check web/assets/instances-page.js
PATH=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH scripts/self-test.sh
```

The pre-tag self-test passed all non-tag gates: `15` passed, `6` skipped for
host/live evidence, and `version-tag-consistency` failed because `v7.0.1.4` had
not yet been created.

Passed after tagging:

```bash
PATH=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH scripts/self-test.sh
MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh
```

Post-tag self-test result: `16` passed, `0` failed, `6` skipped for host/live
evidence. Release gate result: `10` passed, `0` failed, `6` skipped with
`MEGAVPN_RELEASE_ALLOW_SKIPS=1`.

## Remaining Release Blockers

1. Run full production release gates on a release host with PostgreSQL,
   backup/restore, `systemd`, Nginx and service smoke evidence enabled.
2. Run a live remote-egress smoke: VLESS ingress node, active managed backhaul,
   selected egress node group, generated routing rule and successful remote
   `instance.apply` convergence.
3. Complete delegated repository-wide security scan before any stable release
   claim.
