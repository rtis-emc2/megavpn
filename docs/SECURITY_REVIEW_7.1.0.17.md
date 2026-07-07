# Security and Release Review: 7.1.0.17

**Release:** `7.1.0.17`

## Scope

This review covers the Traffic Accounting top-tab restoration after
`7.1.0.16`:

- restored top-level tabs for `Overview`, `Clients`, `Collectors`, `Samples`
  and `Export`;
- moved report filters and CSV download into the `Export` tab instead of
  showing the form on the primary overview screen;
- kept operator-facing summary counters and no-data diagnostics from
  `7.1.0.16`;
- persisted the selected traffic tab in browser local storage;
- Web UI asset cache-key bump to `7.1.0.17`.

## Security Notes

- No backend authorization, agent ingest, retention, export or runtime behavior
  changed in this release.
- CSV export remains protected by `traffic.read` and uses the existing
  authenticated endpoint.
- The tab state is local UI preference only and does not grant access to data.
- The primary page still exposes aggregate counters only; URLs, payloads,
  request bodies and per-destination history are not collected.

## Validation

- `node --check web/assets/traffic-page.js`
- `node --check web/assets/app.js`
- `scripts/docs-consistency.sh`
- `go test ./...`
- `test -z "$(gofmt -l cmd internal)"`
- static multi-command SQL scan for production Go runtime paths
- `git diff --check`

## Residual Risk

- Final acceptance still needs visual verification in the deployed browser to
  confirm that the restored tabs fit the production viewport and cached assets
  are refreshed.
