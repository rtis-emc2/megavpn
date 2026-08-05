# Security and Release Review: 7.1.0.25

**Release:** `7.1.0.25`

## Scope

This hotfix covers the Clients provisioning table layout:

- Rebalanced Clients table column widths so the Actions column has enough
  space for row actions.
- Removed the hard minimum width from the per-client action grid.
- Kept action buttons inside the row cell with a bounded maximum width.
- Web UI asset cache-key bump to `7.1.0.25`.

## Security Notes

- No backend API, RBAC, secret handling or database behavior changed in this
  release.
- The change is limited to operator UI presentation. It reduces the chance of
  accidental wrong-row operation caused by clipped action buttons.
- Existing destructive client actions still require the same server-side
  permissions and confirmation flows as before.

## Validation

- `go test ./...`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/clients-page.js`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/app-router.js`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/app-state.js`
- `scripts/docs-consistency.sh`
- `git diff --check`

## Residual Risk

- Very narrow screens may still require horizontal table scrolling, but row
  actions remain inside the table rather than overflowing past the card edge.
