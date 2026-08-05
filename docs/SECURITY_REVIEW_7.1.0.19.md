# Security and Release Review: 7.1.0.19

**Release:** `7.1.0.19`

## Scope

This review covers Firewall operator UX and node firewall lifecycle hardening
after `7.1.0.18`:

- Apply actions are visually marked as high-impact red actions in the Firewall
  workspace.
- Applied node firewall state now has a distinct green status treatment.
- Address group create/add actions use clearer primary/secondary hierarchy.
- A typed `node.firewall.disable` workflow was added for node-scoped firewall
  rollback.
- Web UI button styling was normalized through the shared button classes.
- Web UI asset cache-key bump to `7.1.0.19`.

## Security Notes

- `node.firewall.disable` is a typed privileged job and is blocked from the
  generic job API.
- The disable endpoint requires `firewall.apply`, matching preview/apply
  privileges.
- The agent implementation is idempotent: disabling an already absent managed
  firewall table succeeds and reports `already_disabled`.
- Disable only removes the managed `inet megavpn_firewall` table. It does not
  stop instances, route policy, backhaul, Nginx, Xray, OpenVPN or WireGuard.
- `firewall_node_state` is updated to `pending_disable` while queued and to
  `disabled` only after successful agent completion.
- Disabled firewall state clears policy/revision references so node runtime
  reconcile does not silently re-apply a previously active firewall policy.

## Validation

- `node --check web/assets/firewall-page.js`
- `node --check web/assets/app-runtime.js`
- `node --check web/assets/app.js`
- `go test ./internal/jobschema ./internal/api/http ./cmd/agent ./internal/infra/postgres`
- `go test ./...`
- `test -z "$(gofmt -l cmd internal)"`
- static multi-command SQL scan for production Go runtime paths
- `scripts/docs-consistency.sh`
- `git diff --check`

## Residual Risk

- Final acceptance still needs visual verification in the deployed browser on
  wide desktop and smaller operator screens.
- Live disable should be tested on a disposable node before using it as a
  production rollback procedure.
