# Security and Release Review: 7.0.1.13

**Release:** `7.0.1.13`

## Scope

- Operator console CSS hardening for typography consistency and text overflow.
- Responsive page-tab behavior on mobile viewports.
- Release baseline and web asset cache-busting updated to `7.0.1.13`.
- No backend API, database migration, RBAC, authentication, agent transport,
  VLESS routing, firewall enforcement or traffic-camouflage rendering behavior
  was changed in this release.

## Changes Reviewed

- Introduced explicit `--font-ui` and `--font-code` stacks and normalized the
  visible application shell to the UI stack.
- Preserved monospace only for `code`, `pre`, config/code textareas, code
  blocks and the web terminal.
- Normalized legacy `letter-spacing` values to `0`.
- Added shared wrapping, `min-width: 0` and overflow rules for buttons, tabs,
  tags, status pills, cards, modals, tables and common text surfaces.
- Changed mobile page tabs from an offscreen horizontal strip to a responsive
  grid.

## Security Assessment

- Attack surface change: none. The release only changes static CSS and asset
  version metadata.
- Data exposure risk: unchanged. No new fields, logs, secrets or generated
  configuration payloads are rendered.
- Authorization risk: unchanged. No RBAC, auth flow or privileged action path
  changed.
- Availability risk reduced for operators: long labels, identifiers and status
  text are less likely to hide actions or push content off screen during
  incident response.

## Verification Evidence

- `go test ./...`: passed before release tagging.
- Frontend JavaScript syntax check with bundled Node.js: passed.
- CSS brace balance check: passed.
- In-app browser desktop smoke at `1440x1000`: Dashboard, Nodes, Instances,
  Firewall, Backhaul, Clients, Jobs, Services and Settings had `0` horizontal
  overflow, `0` offscreen text offenders and `0` console errors.
- In-app browser mobile smoke at `390x844`: the same page set had `0`
  horizontal overflow, `0` offscreen text offenders and `0` console errors
  after responsive page-tab hardening.
- `scripts/self-test.sh`: final tagged run passed with `16` passed, `0` failed
  and `6` skipped on this workstation.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`: final tagged run
  passed with `10` passed, `0` failed and `6` skipped on this workstation.

## Residual Risk

- Browser smoke used a local mock API with representative long labels. Final
  acceptance still needs a pass against a live control plane with real node,
  job and firewall data.
- This release does not address remaining functional hardening for VLESS
  ingress-to-egress traffic, Nginx config preview/diff, live `nginx -t`
  evidence or fallback-site smoke.
