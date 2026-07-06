# Security and Release Review: 7.1.0.6

**Release:** `7.1.0.6`

## Scope

This review covers the traffic-accounting retention and query hardening
increment after `7.1.0.5`:

- replaced unbounded expired-sample deletion with bounded batched pruning on
  ingest;
- added PostgreSQL indexes for traffic-accounting recent-sample ordering and
  common export filters by client, node and protocol;
- documented that overview/export queries enforce the 180-day retention cutoff
  independently from physical cleanup progress;
- updated release documentation, next steps and roadmap evidence;
- added regression coverage for the prune query shape and per-ingest cleanup
  budget.

## Security Posture

The privacy boundary is unchanged. The platform still stores aggregate byte,
packet and flow counters plus attribution references and collector metadata. It
does not store packet payloads, URLs, HTTP headers, DNS queries, TLS session
contents or per-destination browsing history.

No public API or agent collector contract changed in this release. The
operator export endpoint remains read-only and protected by the existing
`traffic.read` permission. Agent ingest remains node-scoped and signed.

The retention change reduces operational risk: expired-row cleanup is bounded
per ingest request, which avoids one large delete blocking the request path or
creating excessive database churn. Overview and export queries still apply the
retention cutoff in SQL, so old rows waiting for a later prune batch are not
returned to operators.

The new indexes are non-secret metadata indexes over existing accounting rows.
They improve predictable query behavior for current UI/export paths and do not
introduce new data exposure.

## Residual Risk

- Live-node validation is still required for Xray, WireGuard and OpenVPN
  collector correctness under reconnect, restart and counter-reset scenarios.
- Real-cardinality query plans must be measured on disposable production-like
  databases. Declarative partitioning or archive tables should be added if
  volume exceeds the indexed single-table path.
- CSV exports still contain operational audit identifiers and must be handled
  as sensitive operational evidence.
- Physical cleanup is intentionally bounded; a very large historical backlog
  can require multiple ingest cycles to drain.

## Verification

Required gates:

- `go test ./internal/infra/postgres`
- `go test ./internal/api/http`
- `go test ./...`
- `go vet ./...`
- `scripts/docs-consistency.sh`
- `scripts/release-gate.sh`

PostgreSQL integration with `MEGAVPN_TEST_DATABASE_DSN` remains recommended for
migration and query-plan proof on disposable databases.
