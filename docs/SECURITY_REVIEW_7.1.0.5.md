# Security and Release Review: 7.1.0.5

**Release:** `7.1.0.5`

## Scope

This review covers the traffic-accounting audit export increment after
`7.1.0.4`:

- added `GET /api/v1/traffic/accounting/export`;
- reused the existing `traffic.read` RBAC permission;
- added CSV export filters for `limit`, `from`, `to`, `client_id`, `node_id`
  and `protocol`;
- enforced server-side export caps and the existing 180-day retention cutoff;
- added `Cache-Control: no-store` and attachment disposition to CSV responses;
- added the Traffic Accounting UI `Export CSV` action;
- added regression tests for export time parsing and CSV row serialization.

## Security Posture

The export endpoint is read-only and does not create new agent, node or
operator write behavior. It exposes the same aggregate accounting rows that the
operator can already view through `/api/v1/traffic/accounting`, but in a
machine-readable CSV artifact for audit handoff.

The privacy boundary is unchanged. Exported rows contain aggregate counters,
references, timestamps and collector metadata. They do not contain packet
payloads, URLs, HTTP headers, DNS queries, TLS session contents or
per-destination browsing history.

The handler relies on the existing authenticated UI/API session path and
requires `traffic.read`. Filter values are passed to PostgreSQL as parameters;
user input is not interpolated into SQL. Invalid UUID filters fail closed in
the store layer.

## Residual Risk

- CSV export can contain client identifiers and public key metadata, so the
  artifact must be handled as operational audit data.
- Large exports are capped at 50,000 rows. Long-term retention still needs
  partitioning and operational archive policy under real-cardinality load.
- Live-node validation is still required for collector correctness under
  reconnect, restart and counter-reset scenarios.

## Verification

Required gates:

- `go test ./internal/api/http`
- `go test ./internal/infra/postgres`
- `go test ./...`
- `scripts/docs-consistency.sh`
- `scripts/release-gate.sh`

PostgreSQL integration with `MEGAVPN_TEST_DATABASE_DSN` remains recommended for
query-plan and migration proof on disposable databases.
