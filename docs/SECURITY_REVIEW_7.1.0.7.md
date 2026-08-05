# Security and Release Review: 7.1.0.7

**Release:** `7.1.0.7`

## Scope

This review covers the traffic-accounting retention observability increment
after `7.1.0.6`:

- added retention metadata to the read-only traffic-accounting overview summary:
  `retention_cutoff`, `expired_sample_count`, `prune_batch_size`,
  `prune_batches_per_ingest` and `max_prune_per_ingest`;
- reused one retention-cutoff helper for overview, export and physical prune
  paths;
- added UI summary cards for cutoff, expired cleanup backlog and per-ingest
  prune budget;
- hardened summary-card text wrapping for long cutoff values;
- updated traffic-accounting documentation and roadmap/release evidence;
- added regression tests for cutoff and prune-budget calculations.

## Security Posture

The release does not add packet capture, destination logging or new agent-side
collection. The privacy boundary is unchanged: stored accounting data remains
aggregate byte, packet and flow counters plus attribution references and small
collector metadata.

No unauthenticated endpoint changed. The new fields are returned only through
the existing `/api/v1/traffic/accounting` read path, which already requires the
`traffic.read` permission. The metadata describes retention mechanics and row
counts; it does not expose payloads, URLs, HTTP headers, DNS queries, TLS
session contents or per-destination browsing history.

The shared cutoff helper reduces consistency risk between overview, export and
prune behavior. Expired rows waiting for physical cleanup remain excluded from
overview/export by SQL cutoff enforcement.

## Residual Risk

- `expired_sample_count` is an aggregate operational count. It should be
  visible to operators with `traffic.read`, but not to lower-privilege roles.
- Large installations still need disposable-database query-plan evidence under
  real cardinality.
- Live-node validation is still required for collector attribution and counter
  reset behavior.

## Verification

Required gates:

- `go test ./internal/infra/postgres`
- `go test ./internal/api/http`
- `go test ./...`
- `go vet ./...`
- `scripts/docs-consistency.sh`
- `scripts/release-gate.sh`

PostgreSQL integration with `MEGAVPN_TEST_DATABASE_DSN` remains recommended for
query-plan and migration proof on disposable databases.
