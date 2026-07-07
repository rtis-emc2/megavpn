# Security and Release Review: 7.1.0.20

**Release:** `7.1.0.20`

## Scope

This hotfix covers the interaction between node firewall enforcement and SSH
bootstrap based agent updates:

- SSH bootstrap and reinstall now preflight the node firewall state before
  queueing `node.bootstrap`.
- If a node has an applied enforced firewall without an active input accept rule
  for the configured SSH port, bootstrap fails before job creation with an
  explicit remediation message.
- The preflight understands custom SSH ports, port lists and ranges, source
  address groups with active renderable entries, and broad input SSH allow
  rules.
- Web UI asset cache-key bump to `7.1.0.20`.

## Security Notes

- The guard avoids an operator-triggered lockout pattern where strict input
  policy drops SSH and `Update all agents` silently queues doomed SSH bootstrap
  jobs.
- The guard is intentionally conservative: if the current applied firewall
  policy does not prove SSH reachability, the operator must disable the managed
  firewall or add/apply a scoped SSH allow rule first.
- The guard does not weaken firewall enforcement and does not auto-open SSH.
  Rollback still uses the typed `node.firewall.disable` path when the agent
  channel is available.

## Validation

- `node --check web/assets/firewall-page.js`
- `node --check web/assets/app-runtime.js`
- `node --check web/assets/app.js`
- `node --check web/assets/nodes-page.js`
- `node --check web/assets/node-workflows.js`
- `go test ./internal/infra/postgres -run 'Test(PostgresIntegrationBootstrapBlockedByEnforcedFirewallWithoutSSHAllow|ShouldHandoff|FirewallPortListContains|PostgresIntegrationDefaultFirewallBaseline|PostgresIntegrationFirewallApplyCreatesRevisionJobAndNodeState)'`
- `go test ./...`
- `test -z "$(gofmt -l cmd internal)"`
- static multi-command SQL scan for production Go runtime paths
- `scripts/docs-consistency.sh`
- `git diff --check`

## Residual Risk

- If both SSH and agent egress are already blocked on a node, remediation still
  requires out-of-band console access to delete `inet megavpn_firewall`.
