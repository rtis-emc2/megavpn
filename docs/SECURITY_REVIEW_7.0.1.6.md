# Security and Release Review: 7.0.1.6

**Release:** `7.0.1.6`

Date: 2026-07-05

Scope:

- Delta review for VLESS ingress-to-egress routing through managed backhaul.
- `node.route_policy.apply` payload extension with Xray/VLESS system routes.
- Agent-side kernel source-route rendering for Xray `sendThrough` addresses.
- Unified web console UI layer across tabs, cards, forms, tables and auth.

## Result

No new P0/P1 security defect was found in the reviewed delta paths.

This is a targeted delta review on top of the `7.0.1.5` VLESS catalog sync
release. It does not replace a full independent repository security scan.

## Reviewed Controls

| Control | Result |
| --- | --- |
| Xray route source | `sendThrough` is normalized to an IPv4 `/32` source route |
| Kernel routing table | System routes require a non-main managed backhaul table |
| Backhaul interface | System routes require an explicit managed backhaul interface |
| Agent idempotency | Existing system-route priority range is cleared before re-apply |
| Client route isolation | Client L3/L4 rules keep the existing `22000-22999` priority range |
| Payload schema | `system_routes` is normalized and rejects non-object entries |
| Route-policy snapshot | Revision hash includes both client routes and system routes |
| UI consistency | Console surfaces now share one light enterprise theme layer |

## Automated Checks

Passed:

```bash
go test -count=1 ./...
go test -count=1 ./cmd/agent
go test -count=1 ./internal/jobschema
go test -count=1 ./internal/infra/postgres -run 'Test(Route|XrayVLESSSystem|Render|Normalize|BuildXray|Postgres)'
env PATH=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH sh -c 'find web/assets -maxdepth 1 -name "*.js" -print0 | xargs -0 -n1 node --check'
PATH=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH scripts/self-test.sh
MEGAVPN_RELEASE_ALLOW_SKIPS=1 PATH=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH scripts/release-gate.sh
```

Self-test result: `16` passed, `0` failed, `6` skipped for host/live evidence.
Release gate result: `10` passed, `0` failed, `6` skipped with
`MEGAVPN_RELEASE_ALLOW_SKIPS=1`.

Static UI smoke:

- Local static web server loaded `web/index.html` without browser console
  errors.
- Auth shell verified with unified light surface, `8px` card/control radius and
  no radial background layer.

## Remaining Release Blockers

1. Run full production release gates on a release host with PostgreSQL,
   backup/restore, `systemd`, Nginx and service smoke evidence enabled.
2. Run live VLESS ingress-to-egress smoke through an active managed backhaul and
   confirm `ip rule from <sendThrough>/32 table <backhaul_table>` exists on the
   ingress node after `node.route_policy.apply`.
3. Complete delegated repository-wide security scan before any stable release
   claim.
