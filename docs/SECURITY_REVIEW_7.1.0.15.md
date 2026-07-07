# Security and Release Review: 7.1.0.15

**Release:** `7.1.0.15`

## Scope

This review covers the Traffic Accounting UI polish increment after `7.1.0.14`:

- compact filter action layout for the Accounting filters form;
- dedicated Traffic Accounting collection-model diagram classes instead of
  reusing firewall flow markup;
- responsive behavior for desktop, tablet and narrow viewports;
- Web UI asset cache-key bump to `7.1.0.15`.

## Security Notes

- No backend authorization, data retention, traffic export or agent ingest
  semantics changed in this release.
- The CSV export button still uses the existing authenticated
  `traffic.read`-protected export endpoint.
- The collection model is presentational only; it does not expose additional
  node, client or sample metadata.
- Advancing `web/index.html` asset cache keys reduces stale-asset risk after
  deploys and avoids partial frontend refresh behavior.

## Validation

- `node --check web/assets/traffic-page.js`
- `node --check web/assets/app.js`

## Residual Risk

- Final visual acceptance still depends on checking the deployed UI in the
  production browser after the new assets are served.
