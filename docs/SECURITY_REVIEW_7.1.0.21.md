# Security and Release Review: 7.1.0.21

**Release:** `7.1.0.21`

## Scope

This hotfix covers lost-node lifecycle recovery:

- New `POST /api/v1/nodes/{id}/force-retire` endpoint protected by
  `node.bootstrap`.
- Web UI delete dialog now separates normal idle-node retire from destructive
  lost-node force retire.
- Force retire requires exact node-name confirmation.
- The backend cancels pending/running jobs that reference the node, node-local
  instances, affected backhaul links or client provisioning/artifact jobs for
  the affected instances.
- The backend removes client service access rows, client access routes,
  generated artifacts, share links, service-access secrets, instance runtime
  states and node-scoped resource locks before marking the node retired.
- Affected backhaul links are marked `deleted`, firewall apply state is removed
  for the retired node and the agent identity is revoked.
- Web UI asset cache-key bump to `7.1.0.21`.

## Security Notes

- The normal `DELETE /nodes/{id}` path remains conservative and still blocks
  active/deleting instances. Force retire is an explicit privileged operation
  for permanently unavailable nodes.
- Force retire does not claim to clean the lost host. If the host returns, it
  must be wiped or manually audited before re-enrollment.
- Client access artifacts and service-access secrets tied to deleted instances
  are removed from PostgreSQL and managed artifact storage to avoid stale
  usable client material after node retirement.
- The operation is audited with cleanup counts and cancellation reason.

## Validation

- `go test ./internal/api/http ./internal/infra/postgres`
- `go test ./...`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/node-workflows.js`
- static multi-command SQL scan for production Go runtime paths
- `scripts/docs-consistency.sh`
- `git diff --check`

## Residual Risk

- Force retire cannot remove systemd units, config files, nftables state or VPN
  interfaces from an unreachable host. Physical or provider-console cleanup is
  required before reusing that machine.
