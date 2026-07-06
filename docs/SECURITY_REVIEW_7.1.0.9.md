# Security and Release Review: 7.1.0.9

**Release:** `7.1.0.9`

## Scope

This review covers the Traffic Accounting server-side filtering increment after
`7.1.0.8`:

- changed `/api/v1/traffic/accounting` to accept the same retained-dataset
  filters as CSV export: `limit`, `from`, `to`, `client_id`, `node_id` and
  `protocol`;
- moved overview summary and recent-row filtering to PostgreSQL instead of
  relying on browser-side filtering of already loaded rows;
- shared one validated SQL predicate builder between overview and export reads;
- clamped oversized overview limits to the server maximum instead of falling
  back to a smaller implicit value;
- made the UI reload Traffic Accounting data from the server when filters
  change and kept CSV export on the same selected filter set;
- kept the existing retention cutoff, export cap and `traffic.read` permission
  model.

## Security Posture

No new traffic data is collected and no new write path is introduced. The
platform still stores aggregate byte, packet and flow counters plus attribution
references and small collector metadata. It does not store packet payloads,
URLs, HTTP headers, DNS queries, TLS session contents or per-destination
browsing history.

The overview endpoint now accepts more read filters, but those filters are
bounded and validated before they reach SQL:

- `client_id` and `node_id` must be UUID-shaped before being bound as typed
  PostgreSQL UUID parameters;
- `protocol` is normalized through the existing accounting-token normalizer;
- date values are parsed by the shared request parser and inverted ranges are
  rejected;
- SQL is assembled from fixed predicate fragments with positional bind
  parameters, not interpolated user values;
- retention cutoff remains mandatory on overview and CSV export reads.

## Residual Risk

- CSV artifacts can contain client and node identifiers, so exported files must
  still be handled as sensitive operational audit evidence.
- Browser session storage keeps the latest operator filter values for
  convenience. It stores filters only, not traffic rows.
- Large installations still need live query-plan validation under real
  cardinality before deciding whether table partitioning or cold archive tables
  are required.

## Verification

Required gates:

- `go test ./internal/api/http`
- `go test ./internal/infra/postgres`
- `go test ./...`
- `go vet ./...`
- JavaScript syntax checks for changed frontend modules
- `scripts/docs-consistency.sh`
- `scripts/release-gate.sh`

PostgreSQL integration with `MEGAVPN_TEST_DATABASE_DSN` remains recommended for
full migration and query-plan proof on disposable databases.
