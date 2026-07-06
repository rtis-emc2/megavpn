# Security and Release Review: 7.0.1.35

**Release:** `7.0.1.35`

## Scope

- Client provisioning modal action-row layout.
- Queued-provisioning success view action-row layout.
- Web asset cache-key update for the patched UI.

## Changes

- Replaced generic `inline-actions` usage in the `Provision client` modal footer
  with a scoped `client-provision-actions` class.
- Added compact, capped-width action-button styles for the provisioning form.
- Applied the same scoped action-row styling to the queued-provisioning success
  actions.
- Added responsive overrides so provisioning buttons remain compact on smaller
  screens instead of inheriting full-width modal button behavior.

## Security Assessment

- No API, authentication, authorization, routing, VPN runtime, secret handling or
  database behavior changed.
- The change only affects Web UI markup and CSS for client provisioning action
  rows.
- Button identifiers and event bindings remain unchanged, so the existing
  provisioning workflow and permission checks are preserved.
- The UI continues to submit the same `POST /api/v1/clients/{id}/provision`
  payload as before.

## Verification Evidence

- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/clients-page.js`
  passed.
- `go test ./...` passed.
- `go test -race ./...` passed.
- `go vet ./...` passed during pre-release verification.
- `govulncheck ./...` completed with no vulnerabilities found.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh` passed
  (`passed=12 skipped=6`).
- `scripts/docs-consistency.sh` passed for release `7.0.1.35`.
- `git diff --check` passed.

## Residual Risk

- Visual QA should still be repeated in the browser across desktop and narrow
  modal widths, because CSS-only UI changes can be affected by viewport-specific
  overrides outside automated syntax checks.
