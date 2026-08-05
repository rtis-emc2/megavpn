# Security and Release Review: 7.0.1.28

**Release:** `7.0.1.28`

## Scope

- Default managed firewall baseline for node rollout.
- nftables ownership hardening for firewall apply.
- IPv6 ICMP support in the firewall rule model.

## Changes

- Added migration `000011_default_firewall_baseline` to seed the `Default node
  firewall` policy, `vpn_client_sources` address list and minimal baseline
  rules.
- `node_base` now defaults to `input=drop`, `forward=drop`, `output=accept`
  when strict enforcement is selected.
- Seeded active baseline rules for invalid packet drops, IPv4/IPv6 ICMP,
  public HTTP/HTTPS edge entrypoints and private/CGNAT/ULA VPN client
  forwarding ranges.
- Seeded SSH management as a disabled rule tied to `trusted_operators`, so it
  must be deliberately enabled after operator source networks are populated.
- Firewall apply now owns `inet megavpn_firewall` instead of deleting the shared
  `inet megavpn` table used by route-policy and service-policy chains.
- Agent cleanup removes legacy `firewall_*` chains from the shared table without
  deleting unrelated route-policy/service-policy chains.
- UI rule editor and backend validation now accept `icmpv6`.
- Baseline seed detection uses stable metadata and rule comments, and the UI
  preserves rule metadata on edit to avoid duplicate seeded rules.
- Bumped release metadata and web asset cache keys to `7.0.1.28`.

## Security Assessment

- The default baseline follows least-privilege host-firewall behavior for
  strict rollout without opening every supported VPN protocol port by default.
- Output remains `accept` in the default baseline to avoid isolating agents from
  package repositories, DNS, telemetry and the control plane during the first
  production firewall rollout.
- SSH is intentionally not active by default because the platform cannot infer
  trusted operator public networks safely.
- Separating `inet megavpn_firewall` prevents firewall apply from destroying
  route-policy marking chains, VLESS remote-egress steering or service-specific
  listener rules.

## Verification Evidence

- `go test ./cmd/agent -run 'TestRenderNodeFirewallPlan' -count=1`: passed.
- `go test ./internal/infra/postgres -run 'TestPostgresIntegrationDefaultFirewallBaseline|Test.*Firewall' -count=1`: passed locally with PostgreSQL integration skipped because `MEGAVPN_TEST_DATABASE_DSN` is unset.
- `go test ./internal/infra/postgres -run 'Test.*Firewall|Test.*Route|TestRenderBackhaulIngressStartScriptAddsOutboundSNAT|TestRenderBackhaulIngressStopScriptCleansSourcePolicy' -count=1`: passed.
- `go test ./...`: passed.
- `go test -race ./...`: passed.
- `go vet ./...`: passed.
- Bundled Node.js `node --check web/assets/firewall-page.js`: passed.
- `git diff --check`: passed.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`: passed with 11
  checks passed and 6 local environment skips.

## Residual Risk

- Live PostgreSQL migration execution was not run locally because no disposable
  PostgreSQL DSN or `psql` client is configured in this workspace.
- Live nftables validation still requires an Ubuntu node with `nft`, existing
  route-policy chains and at least one managed service listener.
- Operators must populate `trusted_operators` and enable the SSH baseline rule
  before strict apply if SSH access must remain available.
