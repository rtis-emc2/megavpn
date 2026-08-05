# Security and Release Review: 7.0.1.9

**Release:** `7.0.1.9`

## Scope

- Delta review for Firewall UI hardening after `7.0.1.8`.
- Policy posture cards, default-policy visualization and node-state observed
  result details.
- Local rule/address-list filtering and grouped protocol rule presets.
- Apply dialog split into explicit `Rules only` and `Strict defaults` modes.

This is a targeted UI/release review. It does not replace an independent
repository-wide security scan before a stable production claim.

## Security Model

| Area | Control |
| --- | --- |
| Strict apply intent | UI uses explicit radio modes; strict default enforcement is not inferred from selected policy metadata |
| Operator visibility | Policy cards show strict/output-guard posture before apply |
| Rule discovery | Local filters reduce accidental edits in large catalogs without changing backend query semantics |
| Protocol presets | Presets populate editable forms only; they do not directly apply firewall state |
| Control-plane egress | Rule preset includes explicit output TCP egress for strict output rollout preparation |
| Node state | UI displays observed enforcement mode, explicit rule count and system safety rule count returned by the agent |

## Changes Reviewed

- Firewall overview now includes enforcement posture counters for default
  accept, strict candidates, output-guarded policies and strict policies without
  active rules.
- Policy cards now show posture, default input/forward/output actions and
  action-colored rule previews.
- Rules table now has local policy, chain, action and text filters. Empty
  states distinguish an empty catalog from filter misses.
- Address-list view now has local search across list metadata and entry values.
- Rule presets are grouped by management, VPN, proxy/edge and hygiene, with an
  added `Control-plane egress` preset for strict output rollouts.
- Apply modal now shows the selected policy summary and exposes two explicit
  apply modes: `Rules only` and `Strict defaults`.
- Node state rows now display observed default-policy enforcement mode, rule
  count and system safety rule count.

## Automated Evidence

```bash
/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/firewall-page.js
find web/assets -maxdepth 1 -name "*.js" -print0 | xargs -0 -n1 /Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check
go test ./...
PATH=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH scripts/self-test.sh
MEGAVPN_RELEASE_ALLOW_SKIPS=1 PATH=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH scripts/release-gate.sh
```

Result:

- Targeted frontend syntax checks: passed.
- `go test ./...`: passed.
- `scripts/self-test.sh`: `16 passed`, `0 failed`, `6 skipped`;
  report `tmp/self-test/self-test-20260705T150654Z.md`.
- `scripts/release-gate.sh` with local skips allowed: `10 passed`,
  `0 failed`, `6 skipped`.

Browser harness evidence:

- Desktop mock Firewall screen rendered with 5 tabs, 3 policy cards and 4
  posture cards, with no horizontal overflow.
- Rule modal rendered 4 preset groups and 14 preset buttons, with no modal
  overflow.
- Apply modal rendered policy details, 3 default-policy pills and 2 apply
  modes, with no modal overflow.
- Mobile viewport `390x844` rendered without horizontal overflow.

## Residual Risks

1. UI harness uses representative mock data; live API data should still be
   checked on a staging control plane with a large firewall catalog.
2. Rule filters are client-side only for the current inventory payload.
3. Full independent security scan remains required before a stable production
   release claim.
