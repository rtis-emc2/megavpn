# Security and Release Review: 7.1.0.11

**Release:** `7.1.0.11`

## Scope

This review covers the Traffic Accounting expected-collector coverage increment
after `7.1.0.10`:

- extended collector status with expected, observed and missing instance counts;
- derived expected collector streams from active/enabled managed
  Xray/WireGuard/OpenVPN instances whose current revision enables traffic
  accounting;
- full-joined expected streams with retained aggregate samples by node, source
  and protocol;
- added `missing` and partial-stream degradation semantics for collector status;
- exposed expected/observed/missing coverage in the Traffic Accounting UI;
- kept the existing agent ingest contract, PostgreSQL schema, retention model
  and aggregate-only privacy boundary.

## Security Posture

No new traffic content is collected or stored. The change derives expected
collector coverage from existing instance metadata and aggregate accounting rows
that are already protected by `traffic.read`. The platform still does not store
packet payloads, URLs, HTTP headers, DNS queries, TLS session contents or
per-destination browsing history.

Expected collector coverage is operational inventory and telemetry: it can show
which managed services should be reporting accounting samples and which streams
are absent. That information remains behind the same authenticated operator API
and `traffic.read` permission as the Traffic Accounting overview and CSV export.

The expected-side query is intentionally disabled for `client_id` filters. A
per-client retained dataset can prove that samples exist for a client, but it
cannot prove that an entire service-instance collector is missing. Avoiding
expected coverage in that mode prevents false missing alerts and misleading
operator action.

## Residual Risk

- Expected collector coverage reveals service inventory health and should be
  treated as sensitive operations data.
- Expected streams are derived from current instance revisions. Live nodes still
  require re-apply after upgrades so runtime configs enable the relevant Xray,
  WireGuard or OpenVPN collector support.
- Current agents emit accounting rows when counters advance. A missing retained
  sample stream can also mean no observed client traffic yet; live-node
  collector heartbeat remains a future hardening item.
- Deployments with custom report intervals should validate the active/degraded
  freshness thresholds before wiring them to alerting.
- PostgreSQL integration with a disposable database remains recommended to
  validate query behavior against real migrations and planner choices.

## Verification

Required gates:

- `go test ./internal/api/http`
- `go test ./internal/infra/postgres`
- `go test ./...`
- `go vet ./...`
- JavaScript syntax checks for changed frontend modules
- `scripts/docs-consistency.sh`
- `scripts/release-gate.sh`

Live acceptance evidence should additionally confirm that an active managed
Xray, WireGuard or OpenVPN instance with traffic accounting enabled transitions
from `missing` to `active` after the agent submits its first retained sample.
