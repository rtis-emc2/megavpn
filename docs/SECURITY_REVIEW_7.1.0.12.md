# Security and Release Review: 7.1.0.12

**Release:** `7.1.0.12`

## Scope

This review covers the post-release audit fixes for Traffic Accounting expected
collector coverage after `7.1.0.11`:

- expected collector projection now uses the applied runtime revision, falling
  back to the current revision only for legacy rows without applied revision
  metadata;
- expected collectors are derived only from enabled managed instances in
  `active` or `degraded` state;
- draft, provisioning, disabled, deleted and other non-runtime lifecycle states
  no longer create expected accounting streams;
- SQL query construction is covered by regression tests for lifecycle scope,
  applied-revision source and placeholder ordering under client/node/protocol
  filters;
- documentation now matches the runtime model and calls out that `missing`
  means no retained sample stream, not standalone proof of collector failure.

## Security Posture

No new data is collected and no schema, permission or agent ingest contract is
changed. The update narrows expected collector projection so the Traffic
Accounting UI reflects runtime services that should actually be reporting, not
unapplied edits or transient instance states.

The existing `traffic.read` permission remains the boundary for overview,
collector status and CSV export. The platform continues to store aggregate byte
counters only; it does not store packet payloads, URLs, HTTP headers, DNS query
names, TLS session contents or per-destination browsing history.

Using the applied revision is important for operator safety: if a revision is
edited but not applied to a node, the UI must not report a missing collector for
runtime configuration that does not exist yet. Restricting expected rows to
enabled `active`/`degraded` instances similarly avoids false missing signals for
draft, provisioning or disabled services.

## Residual Risk

- Current agents emit accounting rows when counters advance. A missing retained
  sample stream can also mean no observed client traffic yet; explicit collector
  heartbeat remains the next hardening item if health must be independent from
  traffic deltas.
- Live nodes still require re-apply after upgrades so runtime configs enable
  Xray, WireGuard or OpenVPN collector support.
- PostgreSQL integration with a disposable database remains recommended for
  full migration and query-plan proof.

## Verification

Required gates:

- `go test ./internal/infra/postgres`
- `go test ./internal/api/http`
- `go test ./...`
- `go vet ./...`
- JavaScript syntax checks for changed frontend modules
- `scripts/docs-consistency.sh`
- `scripts/release-gate.sh`

Live acceptance should confirm that editing an unapplied revision does not
create a false expected collector row, while an applied enabled `active` or
`degraded` accounting-enabled Xray/WireGuard/OpenVPN instance does appear in the
expected collector coverage.
