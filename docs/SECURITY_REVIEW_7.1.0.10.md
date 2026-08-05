# Security and Release Review: 7.1.0.10

**Release:** `7.1.0.10`

## Scope

This review covers the Traffic Accounting collector-status observability
increment after `7.1.0.9`:

- extended `/api/v1/traffic/accounting` with a `collectors` array derived from
  retained aggregate samples;
- grouped collector status by node, collector source and protocol;
- exposed active/degraded/inactive freshness, last report time, last bucket,
  sample count, attributed-client count and aggregate byte/flow counters;
- added a Traffic Accounting UI `Collector status` table that uses the same
  server-side filters as overview cards, recent rows and CSV export;
- added unit coverage for collector freshness classification;
- kept the existing agent ingest contract, PostgreSQL schema and
  retained-dataset filter model.

## Security Posture

No new traffic data is collected. Collector status is derived from existing
aggregate accounting rows already protected by `traffic.read`. The platform
still does not store packet payloads, URLs, HTTP headers, DNS queries, TLS
session contents or per-destination browsing history.

The new `collectors` response helps operators validate live-node accounting
coverage without adding a new write path. It reports operational freshness and
aggregate counters only. The existing retained-dataset filters, UUID validation,
date-range validation and retention cutoff continue to apply before collector
status is computed.

## Residual Risk

- Collector status can reveal which nodes and protocols are actively reporting
  traffic aggregates, so it remains operational audit data and requires
  `traffic.read`.
- Freshness thresholds are deterministic server-side constants. Large
  deployments should validate them against real agent report intervals before
  treating degraded/inactive as an alerting SLA.
- Live-node validation is still required for Xray, WireGuard and OpenVPN after
  instance re-apply, reconnect and restart scenarios.

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
