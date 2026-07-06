# Security and Release Review: 7.0.1.41

**Release:** `7.0.1.41`

## Scope

- Agent-mediated node reboot job and operator UI action.
- Runtime reconcile after agent reinstall/bootstrap and a manual diagnostic
  action for already degraded nodes.
- Managed Nginx HTTP-to-HTTPS redirect option for generated edge configs.
- Agent/worker job routing consistency for node-side runtime operations.

## Changes

- Added typed `node.reboot` API/job schema with node-name confirmation and
  `node.bootstrap` permission.
- The agent schedules reboot through `systemd-run --on-active=5s` and reports
  the job result before the host restarts; `shutdown -r +1` is a fallback.
- Successful SSH bootstrap/reinstall now queues node runtime reconcile:
  inventory, service discovery, active instance apply, backhaul apply,
  route-policy apply and existing firewall policy apply.
- Added operator-triggered runtime reconcile in Node diagnostics for recovery
  without another bootstrap.
- Nginx renderer now supports `http_to_https_redirect=false` and
  `http_redirect_server_name`, while keeping legacy TLS redirect behavior
  enabled by default.

## Security Assessment

- Reboot is not exposed through the generic job API. It is a typed privileged
  endpoint, requires `node.bootstrap`, and requires exact node-name
  confirmation.
- Reboot execution avoids shell interpolation and uses fixed argv commands.
- Runtime reconcile reuses existing desired-state builders and agent job
  schemas instead of introducing manual node-side commands.
- The Nginx redirect server name is validated with the same DNS/wildcard/IP
  validator as the primary `server_name`.
- No database migration, secret material format change or public unauthenticated
  endpoint was introduced.

## Verification Evidence

- `go test ./internal/jobschema ./internal/api/http ./internal/infra/postgres ./cmd/agent` passed.
- `go test ./...` passed.
- `go vet ./...` passed.
- Bundled Node.js syntax check passed for changed web assets:
  `domain-ui.js`, `node-workflows.js`, `instance-catalog.js`.
