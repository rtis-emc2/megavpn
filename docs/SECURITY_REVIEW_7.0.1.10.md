# Security and Release Review: 7.0.1.10

**Release:** `7.0.1.10`

## Scope

- Frontend-only layout regression fix after the `7.0.1.9` Firewall UI release.
- Reviewed pages: Dashboard, Nodes, Node Map, Instances, Clients, Jobs, Backhaul, Address pools, Artifacts, Share Links, Firewall, Certificates, Audit, Services, Telemetry, Revisions, Settings.
- No backend API, agent, firewall enforcement, VLESS routing, database migration, RBAC, authentication, or secret-handling behavior was changed in this release.

## Changes Reviewed

- Backhaul rows now use shrink-safe grid tracks and bounded row containers.
- Instances list toolbar now fits the available desktop content width without clipping right-side actions.
- Node Map pending GeoIP rows now shrink inside the right detail column instead of clipping the reason text.
- Revisions page now uses bounded card/content widths on mobile and no longer expands the document body horizontally.
- Web asset cache-busting and documented release baseline were updated to `7.0.1.10`.

## Security Assessment

- Attack surface change: none. The delta is CSS layout only.
- Data exposure risk: unchanged. No new rendered data fields, API calls, or client-side storage keys were introduced.
- Authorization risk: unchanged. RBAC checks and action availability remain controlled by existing page modules.
- Operational risk reduced: operators can now inspect all primary pages without clipped controls on desktop and without body-level horizontal overflow on mobile.

## Verification Evidence

- CSS brace balance check: passed.
- Frontend JavaScript syntax check for all `web/assets/*.js`: passed.
- Browser desktop smoke with mock API data, default viewport: 17/17 primary pages passed with no body horizontal overflow, no off-viewport controls, no clipped controls, and no console errors.
- Browser mobile smoke with mock API data, `390x844` viewport: 17/17 primary pages passed with no body horizontal overflow, no off-viewport controls outside intentional scroll containers, no clipped controls, and no console errors.
- Intended horizontal scrolling remains limited to table wrappers and tab strips where the UI contract requires more columns/options than the viewport can display.

## Residual Risk

- Browser smoke used deterministic mock data, not live production inventory. Very long real-world labels should remain bounded by the same `min-width: 0`, `max-width`, ellipsis, and `overflow-wrap` rules, but live visual QA is still recommended before a customer-facing deployment.
- This release does not address transport behavior, VLESS ingress-to-egress routing, or firewall policy semantics; those remain separate implementation and verification tracks.
