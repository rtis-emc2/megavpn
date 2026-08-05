# Security and Release Review: 7.0.1.8

**Release:** `7.0.1.8`

## Scope

- Delta review for managed firewall strict default-policy enforcement after
  `7.0.1.7`.
- Agent-side nftables rendering, atomic managed table replacement and output
  lockout guards.
- Firewall apply API/UI changes for explicit `enforce_default_policy`.
- Protocol rule presets for OpenVPN, IPsec/L2TP, Shadowsocks, HTTP proxy,
  MTProto and Nginx edge listeners.

This is a targeted release review. It does not replace an independent
repository-wide security scan before a stable production claim.

## Security Model

| Area | Control |
| --- | --- |
| Default enforcement | Strict mode is opt-in per apply job; default apply remains explicit-rules-only with base chain `accept` |
| nftables transaction | Agent renders `delete table inet megavpn` plus full table recreation in one `nft -f` batch |
| `reject` defaults | Rendered as base policy `drop` plus terminal `reject` rule because nftables base chains do not support `reject` |
| Established traffic | Strict mode adds managed established/related safety rules before catalog rules |
| Loopback | Strict input/output defaults add loopback allow rules before catalog rules |
| Output lockout | Strict output `drop`/`reject` requires IP-pinned control-plane egress or explicit catalog output allow for the control-plane TCP port |
| Operator intent | API/UI carries `enforce_default_policy`; strict mode is not silently inferred from policy metadata |
| Protocol exposure | New presets only populate editable catalog rules; operators still choose source/destination scope before apply |

## Changes Reviewed

- `node.firewall.apply` now receives and persists `enforce_default_policy` from
  the HTTP API instead of always setting it to `false`.
- Agent firewall renderer now reports `default_policy_enforcement` as
  `observe_only` or `enforced` and includes `system_rule_count` in preview/apply
  results.
- Managed nftables apply no longer performs a non-transactional pre-flush of
  chains. The rendered script atomically replaces the managed `inet megavpn`
  table.
- Strict output default handling validates the agent control-plane URL. If the
  URL host is an IP address, the renderer adds a pinned TCP egress allow rule.
  If the URL host is DNS, the renderer requires an explicit active catalog
  `output accept` rule for the control-plane TCP port.
- Firewall UI apply dialog exposes strict default enforcement as an explicit
  checkbox with rollout warning copy.
- Firewall rule presets now cover additional protocol listeners: OpenVPN
  TCP/UDP, IPsec IKE/NAT-T, L2TP, Shadowsocks TCP/UDP, HTTP proxy, MTProto and
  Nginx edge HTTP(S).

## Automated Evidence

```bash
go test ./cmd/agent ./internal/api/http ./internal/jobschema ./internal/infra/postgres
go test ./cmd/agent
go test ./...
/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/firewall-page.js
find web/assets -maxdepth 1 -name "*.js" -print0 | xargs -0 -n1 /Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check
PATH=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH scripts/self-test.sh
MEGAVPN_RELEASE_ALLOW_SKIPS=1 PATH=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH scripts/release-gate.sh
```

Result:

- Targeted firewall/API/schema/store checks: passed.
- `go test ./...`: passed.
- Frontend JS syntax check: passed.
- `scripts/self-test.sh`: `16 passed`, `0 failed`, `6 skipped`;
  report `tmp/self-test/self-test-20260705T144514Z.md`.
- `scripts/release-gate.sh` with local skips allowed: `10 passed`,
  `0 failed`, `6 skipped`.

## Residual Risks

1. Strict output policy with DNS control-plane host depends on operator-owned
   catalog allow rules; live rollout should start on a disposable node.
2. The firewall rule model still does not expose interface matching in the UI.
3. Full independent security scan remains required before a stable production
   release claim.
