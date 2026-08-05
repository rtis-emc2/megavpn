# Security and Release Review: 7.1.0.16

**Release:** `7.1.0.16`

## Scope

This review covers the Traffic Accounting operator-view simplification after
`7.1.0.15`:

- replaced storage-maintenance summary fields with operator-facing traffic,
  client, node, collector and retention counters;
- compacted report filters and disabled CSV download when the visible dataset
  is empty;
- added an explicit no-data state that explains missing agent accounting
  samples and collector streams;
- removed the technical collection-model diagram from the primary operator
  page;
- Web UI asset cache-key bump to `7.1.0.16`.

## Security Notes

- No backend authorization, retention, ingest, export or agent runtime command
  behavior changed in this release.
- The CSV path remains the existing authenticated `traffic.read` endpoint.
- The no-data state exposes only aggregate operational status and does not add
  URLs, payloads, request bodies or per-flow details.
- Removing the collection-model diagram from the main page reduces accidental
  disclosure of internal implementation wording to day-to-day operators.

## Validation

- `node --check web/assets/traffic-page.js`
- `node --check web/assets/app.js`
- `scripts/docs-consistency.sh`
- `go test ./...`
- `test -z "$(gofmt -l cmd internal)"`
- static multi-command SQL scan for production Go runtime paths
- `git diff --check`

## Residual Risk

- Live traffic visibility still depends on deployed agents reporting
  accounting samples after counters advance. The first agent read establishes a
  baseline and later reads submit deltas.
