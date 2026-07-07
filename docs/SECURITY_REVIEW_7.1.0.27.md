# Security and Release Review: 7.1.0.27

**Release:** `7.1.0.27`

## Scope

This hotfix covers failed VLESS client provisioning jobs after the
client-level identity registry rollout:

- Explicitly casts the Xray UUID placeholder as `text` inside
  `jsonb_build_object`.
- Prevents PostgreSQL prepared/extended protocol failures with
  `could not determine data type of parameter $5`.
- Keeps the `7.1.0.26` client-level VLESS identity model unchanged.

## Security Notes

- No credential broadening is introduced. The same client-scoped UUID is stored
  in `client_service_identities` and mirrored into service-access metadata.
- The change only removes an SQL type-inference ambiguity. It does not change
  RBAC, job authorization, subscription tokens, route policy or artifact
  delivery.
- Explicit Xray UUID rotation remains the path for invalidating previously
  issued client configs.

## Validation

- `go test ./internal/infra/postgres -count=1`
- `go test ./cmd/migrate ./internal/infra/postgres -count=1`
- `go test ./...`
- `scripts/docs-consistency.sh`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/clients-page.js`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/app-router.js`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/app-state.js`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/app.js`
- `git diff --check`

## Residual Risk

- Failed `client.provision` jobs created before this hotfix need to be retried
  by running the client provisioning/build workflow again after deployment.
