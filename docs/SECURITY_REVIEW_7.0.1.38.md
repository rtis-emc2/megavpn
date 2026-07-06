# Security and Release Review: 7.0.1.38

**Release:** `7.0.1.38`

## Scope

- Agent-side route-policy nftables command rendering.
- Agent-side service netpolicy and NAT nftables command rendering.
- Route-policy runtime behavior when a selected managed backhaul interface is
  not present or not ready on the ingress node.
- Operator-facing route-policy job error details.

## Changes

- Changed nftables `comment` rendering in route-policy, service firewall rules
  and NAT rules to pass a real nft string literal, for example
  `comment '"megavpn:route-policy:..."'`, instead of relying on shell quoting.
- Route-policy enforcement now installs the nft mark rule and `ip rule` only
  after at least one candidate route has been successfully installed for that
  policy rule.
- If no managed backhaul candidate is ready, route-policy fails closed with a
  clear error such as `route policy rule ... has no ready managed backhaul
  candidate`.
- Failed route-policy jobs now include the first useful execution line in the
  top-level error, so the UI/API does not hide the actionable failure behind a
  generic `route policy kernel enforcement failed` message.

## Security Assessment

- The nft comment fix removes a parser failure that prevented route-policy and
  related netpolicy rules from being applied on Linux nodes.
- The fail-closed route-policy behavior prevents traffic from being marked into
  a policy table when the backing `mgbh*` interface or peer candidate is absent.
- The change does not widen privileged job permissions and does not add new
  shell-interpolated untrusted values. Comments are shell-quoted and nft-quoted.
- The operational failure mode is now explicit: if the selected transport is
  stale or down, operators must promote/reapply a healthy backhaul transport
  before VLESS remote egress is considered enforced.

## Verification Evidence

- `go test ./cmd/agent` passed.
- `go test ./internal/infra/postgres` passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- `scripts/docs-consistency.sh` passed for release `7.0.1.38`.
- `git diff --check` passed.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh` passed
  (`passed=12 skipped=6`), including version/tag consistency, unit tests,
  race tests, vulnerability scan, binary version checks, shell syntax checks,
  docs consistency, installer validation and static security-pattern scan.

## Residual Risk

- Runtime verification still requires a real Linux ingress node with `nft`,
  `iproute2`, systemd and at least one active managed backhaul interface.
- If Xray was already applied with `sendThrough` pointing at a stale backhaul
  source address, operators must promote the healthy transport and re-apply the
  Xray instance so the generated outbound source address matches the live
  `mgbh*` interface.
