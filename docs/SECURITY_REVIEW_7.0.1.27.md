# Security and Release Review: 7.0.1.27

**Release:** `7.0.1.27`

## Scope

- Follow-up route-policy audit after the mark-based enforcement release.
- Removal of legacy source-only route creation from new backhaul ingress start
  scripts.
- Documentation cleanup for VLESS remote-egress diagnostics.

## Changes

- New backhaul ingress start scripts no longer create `ip rule add from ...`
  source-policy rules or default routes in the managed backhaul table.
- Backhaul stop scripts still remove legacy source-policy rules and flush the
  managed table for already-deployed nodes during controlled cleanup.
- VLESS group and backhaul documentation now point operators to
  `inet megavpn route_policy_output` and `ip rule fwmark` instead of the old
  source-only kernel rule.
- Bumped release metadata and web asset cache keys to `7.0.1.27`.

## Security Assessment

- Backhaul activation is again scoped to transport lifecycle and managed NAT.
  Runtime route selection is owned by `node.route_policy.apply`.
- Existing nodes with a previously generated source-policy rule are not left
  without cleanup: the stop script keeps the legacy deletion path.
- Removing the legacy start rule prevents a stale source-only rule from
  reappearing after a transport re-apply and keeps diagnostics aligned with the
  mark-based policy model.

## Verification Evidence

- `go test ./internal/infra/postgres -run 'TestRenderBackhaulIngressStartScriptAddsOutboundSNAT|TestRenderBackhaulIngressStopScriptCleansSourcePolicy|TestRoute|TestXrayVLESSSystemRoutes|TestPostgresIntegrationBackhaulRouteToggle|TestPostgresIntegrationBackhaulPromote' -count=1`: passed.
- `go test ./cmd/agent -run 'TestRenderRoutePolicyKernelScript|TestRoutePolicyPathSafety|TestValidateRoutePolicyPayload' -count=1`: passed.
- `go test ./...`: passed.
- `go test -race ./...`: passed.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`: passed with local
  environment skips only.

## Residual Risk

- Live nftables syntax and kernel packet-flow validation still require an
  Ubuntu node with `nft`, `iproute2`, active backhaul and a real VLESS ingress.
- Existing active systemd units need re-apply or cleanup/restart to replace an
  already-written legacy start script on remote nodes.
