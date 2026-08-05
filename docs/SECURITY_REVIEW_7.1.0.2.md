# Security and Release Review: 7.1.0.2

**Release:** `7.1.0.2`

## Scope

This review covers the traffic-accounting foundation added after `7.1.0.1`:

- PostgreSQL aggregate storage for `traffic_accounting_samples`;
- signed agent ingest endpoint `/agent/traffic/accounting`;
- operator read endpoint `/api/v1/traffic/accounting`;
- RBAC permission `traffic.read`;
- Traffic Accounting UI page;
- bilingual operator documentation.

## Security Posture

The change stores aggregate counters only. It does not introduce packet capture,
payload logging, URL logging, HTTP body logging, DNS query logging or TLS
session inspection.

Agent ingest uses the existing node bearer-token and signed-message model. The
store validates node ownership for submitted instances and rejects malformed
UUIDs, negative counters, empty counter samples and oversized buckets before
storage.

## Residual Risk

- Runtime collectors are intentionally not enabled by default in this release.
  Protocol-specific collection must be validated on real nodes before
  production accounting is enabled.
- Future export/report APIs must add explicit audit events and rate limits.
- Long-term high-volume installations may need table partitioning after live
  cardinality measurements.

## Verification

Required gates:

- `go test ./...`
- `scripts/docs-consistency.sh`
- `scripts/release-gate.sh`

PostgreSQL integration with `MEGAVPN_TEST_DATABASE_DSN` remains recommended for
upgrade proof on disposable databases.
