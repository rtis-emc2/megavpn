# Security and Release Review: 7.0.1.26

**Release:** `7.0.1.26`

## Scope

- Agent-side route-policy enforcement for managed ingress-to-egress routing.
- Kernel cleanup behavior when route-policy snapshots no longer contain
  enforceable rules.
- Operator documentation for node-side mark and policy-routing verification.

## Changes

- Client route policies now always use managed nftables marking in
  `inet megavpn route_policy_prerouting` and `ip rule fwmark` selection into a
  managed non-main backhaul table.
- Xray/VLESS remote-egress system routes now use managed nftables marking in
  `inet megavpn route_policy_output` and `ip rule fwmark` selection, instead of
  source-only policy rules.
- Protocol-only rules such as TCP, UDP and ICMP are represented in nftables, so
  L4 policy intent is preserved even when no destination port list exists.
- Empty route-policy snapshots still render and apply cleanup for reserved
  MegaVPN priorities and managed nftables chains, preventing stale routes after
  policy deletion or instance cleanup.
- Forwarding is enabled only when client ingress route rules exist; cleanup-only
  snapshots do not change `net.ipv4.ip_forward`.
- Bumped release metadata and web asset cache keys to `7.0.1.26`.

## Security Assessment

- Kernel changes remain scoped to the managed `inet megavpn` table chains and
  reserved policy-rule priorities. The agent does not flush unrelated operator
  nftables chains or routing rules.
- Route-policy enforcement is fail-closed for active routes when `nft` is
  missing, because unmarked traffic would otherwise bypass the selected egress
  interface.
- The selected route table must still be non-main and explicitly projected by
  Control Plane route policy. Main-table fallback is rejected by the renderer.
- Cleanup is idempotent and safe for nodes that have no `nft` binary when there
  are no active rules; stale nft chains are flushed only when `nft` is present.

## Verification Evidence

- `go test ./cmd/agent -run 'TestRenderRoutePolicyKernelScript|TestRoutePolicyPathSafety|TestValidateRoutePolicyPayload' -count=1`: passed.
- `go test ./internal/infra/postgres -run 'TestRoute|TestXrayVLESSSystemRoutes|TestPostgresIntegrationBackhaulRouteToggle|TestPostgresIntegrationBackhaulPromote' -count=1`: passed.
- `go test ./...`: passed.

## Residual Risk

- Local workstation validation does not include a live nftables kernel syntax
  check because `nft` is not installed in the current development environment.
  The release relies on renderer unit tests plus live-node verification during
  deployment.
- Route-policy correctness still depends on the selected backhaul transport
  being active and its managed route table containing the expected `mgbh*`
  interface route.
