# Security and Release Review: 7.1.1.0

**Release:** `7.1.1.0`

## Scope

- Reviews the merge-ready hardening set for VLESS group membership, managed
  firewall safety, release gates and migration invariants.
- Adds typed VLESS group membership endpoints protected by client and access
  group permissions.
- Moves VLESS membership management to client-owned access groups with
  pagination, all-filtered selection, pasted identity refs and dry-run preview
  before bounded apply.
- Keeps VLESS identity at client scope while allowing runtime instance
  membership to be re-applied after node or ingress replacement.
- Hardens strict firewall apply with source-scoped SSH/bootstrap checks,
  renderable-address-list validation and matching preview hash enforcement.
- Adds configured firewall management CIDRs from
  `MEGAVPN_CP_FIREWALL_SOURCE_CIDRS`,
  `MEGAVPN_CP_SSH_BOOTSTRAP_SOURCE_CIDRS` and `Settings -> Firewall safety`.
- Aligns persisted firewall node state with `pending_disable`, `disabled` and
  `stale` lifecycle statuses used by API, UI and agent.
- Extends CI/release gates with action pinning checks, frontend syntax checks,
  migration invariants, shell syntax coverage and binary version checks.

## Security Notes

- The VLESS membership API intentionally requires multiple permissions for
  mutation: `instance.write` and `client.provision`. Read APIs require both
  `instance.read` and `client.read`.
- Multiple Xray instances remain valid when their endpoint, path, host, purpose
  or rollout lifecycle is intentionally separate. The duplicate-prevention rule
  remains semantic in service-pack creation rather than a broad database
  uniqueness constraint.
- Strict firewall apply remains fail-closed when a referenced active address
  group has no IP/CIDR/range entries. DNS entries are catalog context only and
  are not silently rendered into nftables sets.
- Strict firewall input never auto-opens SSH from `0.0.0.0/0` or `::/0`.
  Strict forward/output apply is blocked when it would isolate active
  VPN/backhaul forwarding or agent control-plane egress.
- Firewall disable affects only the managed `inet megavpn_firewall` table and
  does not remove route-policy, backhaul or service runtime state.
- Existing secrets, generated client artifacts and client-level VLESS UUIDs are
  preserved.

## Validation

- `make build`
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `scripts/ci/docs-consistency.sh`
- `scripts/ci/actions-pinning-check.sh`
- `scripts/ci/self-test.sh`
- `scripts/ci/release-gate.sh`
- `git diff --check`

## Residual Risk

- Live PostgreSQL migration drill, backup/restore drill, `nginx -t`,
  `systemd-analyze`, API smoke and VPN/service matrix require disposable/live
  infrastructure and remain release-environment gates.
- Strict firewall safety depends on operators populating semantic source groups
  such as `trusted_control_plane` or `trusted_operators`; a generic group named
  `whitelist` is intentionally not treated as bootstrap safety.
- VLESS group membership, policy, scope and status changes queue runtime apply
  jobs, so data-plane enforcement depends on the node agent completing those
  jobs successfully.
