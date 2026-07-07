# Security and Release Review: 7.1.0.22

**Release:** `7.1.0.22`

## Scope

This hotfix covers single-instance recovery for nodes that are permanently
unavailable:

- New `POST /api/v1/instances/{id}/force-delete` endpoint protected by
  `instance.write`.
- Web UI instance delete dialog now separates normal managed cleanup from
  explicit lost-node force delete.
- Force delete requires exact instance-name confirmation.
- The backend cancels pending/running/retrying jobs that reference the instance,
  including instance jobs and client provisioning/artifact jobs carrying the
  instance id in their payload.
- The backend removes client service access rows, client access routes,
  generated artifacts, share links, service-access secrets, instance-scoped
  secrets, instance runtime state and instance-scoped resource locks before
  marking the instance `deleted`.
- Node force-retire cleanup now reuses the same instance control-plane cleanup
  helper, including instance-scoped secret cleanup.
- Web UI asset cache-key bump to `7.1.0.22`.

## Security Notes

- The normal `DELETE /instances/{id}` path remains conservative and still
  queues agent-side cleanup. Force delete is an explicit recovery operation for
  lost nodes where agent convergence is impossible.
- Force delete does not claim to clean the remote host. If the host returns, it
  must be wiped or manually audited before reuse.
- Client access artifacts and service-access secrets tied to the deleted
  instance are removed from PostgreSQL and managed artifact storage to avoid
  stale usable client material after recovery.
- The operation is audited with cleanup counts and cancellation reason.

## Validation

- `go test ./internal/api/http ./internal/infra/postgres`
- `go test ./...`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/instances-page.js`
- static multi-command SQL scan for production Go runtime paths
- `scripts/docs-consistency.sh`
- `git diff --check`

## Residual Risk

- Force delete cannot remove systemd units, config files, nftables state or VPN
  interfaces from an unreachable host. Physical or provider-console cleanup is
  required before reusing that machine.
