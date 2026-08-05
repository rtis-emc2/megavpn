# Security and Release Review: 7.1.0.26

**Release:** `7.1.0.26`

## Scope

This release fixes VLESS credential continuity during ingress/node replacement:

- Adds `client_service_identities` as the durable client-level service identity
  registry.
- Backfills existing Xray/VLESS UUIDs from active and pending
  `service_accesses`.
- Reuses the client-level VLESS UUID when a client is provisioned onto an
  additional or replacement Xray/VLESS ingress.
- Keeps explicit Xray UUID rotation as the controlled path for issuing a new
  client credential.
- Updates operator documentation for node replacement, stable endpoints and
  VLESS subscription delivery.

## Security Notes

- The issued VLESS UUID remains scoped to one client account and one identity
  profile key. It is no longer lost when an instance or service access row is
  deleted.
- Deleting a client cascades the identity row through the existing
  `client_accounts` ownership boundary. Deleting one service access does not
  revoke the reusable client identity.
- The migration chooses a deterministic backfill source when historical access
  rows contain multiple UUIDs: active rows win over pending rows, then the most
  recently updated row wins.
- Subscription tokens are unchanged. They continue to be stored as hashes and
  only deliver profiles for current active VLESS service accesses.
- Endpoint continuity still depends on DNS/SNI/path/port. A static imported
  profile cannot discover a different hostname by itself; operators should keep
  the public endpoint stable or deliver VLESS subscriptions.

## Validation

- `go test ./internal/infra/postgres`
- `go test ./...`
- `scripts/docs-consistency.sh`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/clients-page.js`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/app-router.js`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/app-state.js`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/app.js`
- `git diff --check`

## Residual Risk

- Existing deployments that already issued divergent UUIDs for the same client
  and identity profile are normalized to one durable UUID during migration. The
  deterministic backfill rule avoids nondeterminism, but operators should
  reprovision/apply affected replacement instances after upgrade.
- Full no-touch client continuity requires the public VLESS endpoint to remain
  stable. If hostname, path or port changes, use DNS cutover or VLESS
  subscription delivery.
