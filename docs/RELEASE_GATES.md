# Release Gates

**Release:** `7.1.1.9`

This file defines the minimum evidence required before tagging a production release.

For `7.1.1.9`, these gates are used as release promotion evidence. A release can be published with documented product gaps, but not with unknown install, migration, agent-channel, node-cleanup or runtime-apply behavior.

## 1. Release Gate

Required command:

```bash
scripts/ci/release-gate.sh
```

The release gate is fail-closed: any skipped gate makes the command fail because skipped checks are not production release evidence. For local diagnostics on a workstation that does not have PostgreSQL, systemd, nginx or a disposable test node, use `scripts/ci/self-test.sh` or explicitly allow skips:

```bash
MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/ci/release-gate.sh
```

Diagnostic command with full PASS/FAIL/SKIP report:

```bash
scripts/ci/self-test.sh
```

The baseline gate must pass:

- `gofmt -l cmd internal` returns no files.
- `go test ./...` passes.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...` passes on the release Go patch version.
- `go test -race ./...` passes. Disabling it with `MEGAVPN_RELEASE_RUN_RACE=0` is a local diagnostic shortcut and counts as a skipped release gate.
- `go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin` passes.
- Operational binaries print the same release through `--version`.
- Shell scripts under `scripts/` pass `bash -n`.
- GitHub Actions workflow `uses:` entries are pinned to immutable 40-character
  commit SHA refs by `scripts/ci/actions-pinning-check.sh`.
- `scripts/ci/docs-consistency.sh` passes: maintained docs, roadmap files,
  production env templates, current security review links and Web UI asset
  cache keys match the code release.
- The Control Plane installer accepts non-interactive clean-install inputs in validate-only mode.
- Smoke scripts that call `/api/v1` support `MEGAVPN_AUTH_TOKEN`.
- Static Web UI JavaScript passes `node --check` and
  `scripts/ci/frontend-bootstrap-smoke.js` reaches `__MegaVPNBootReady`.
- `scripts/ci/service-pack-smoke-regression.js` passes against its local mock API,
  covering matrix planning, pack filter validation, post-provision apply waits,
  staged batch planning, per-access artifact checks and cleanup behavior.
- Static production scan finds no `/bin/sh -c`, `StrictHostKeyChecking=accept-new`, curl-to-shell, curl-to-gpg or apt-key trust bootstrap pattern outside tests.

## 2. Security Gate

Required release evidence:

- Share-link tokens are stored as `token_hash` plus `token_hint`; plaintext token is returned only once at publish time.
- Agent transport rejects unsigned responses by default, including empty `204` polls.
- SSH bootstrap requires pinned `ssh_host_key_sha256`.
- Bootstrap env generation rejects control characters in node identity and rendered env values.
- Generic job creation only allows explicitly supported manual job types, and new jobs must start as `queued`.
- NGINX.org repository bootstrap verifies the signing key fingerprint before importing it into the node trust store.
- Xray remote installer is fail-closed unless `MEGAVPN_XRAY_INSTALL_SCRIPT_SHA256` pins the downloaded script.
- Audit events exist for bootstrap, instance apply, capability install/verify, share-link publish/revoke, login, settings and certificate changes.
- Secret rotation runbook in `docs/OPERATIONS_RUNBOOK.md` is reviewed for this release.

## 3. Runtime Gate

Use a disposable PostgreSQL database:

```bash
MEGAVPN_RELEASE_DATABASE_DSN='postgres://...' scripts/ci/release-gate.sh
```

This runs `scripts/ci/postgres-migration-drill.sh` before PostgreSQL integration
tests. The drill is intentionally strict: `MEGAVPN_RELEASE_DATABASE_DSN` must
point at an empty disposable database unless
`MEGAVPN_MIGRATION_DRILL_ALLOW_EXISTING=1` is set for diagnostics only. It
applies all migrations from zero, applies them again to verify runner
idempotency, checks critical schema invariants, required security indexes,
token-storage columns, VLESS group templates and firewall seed groups, then the
integration suite covers job lifecycle, stale lease recovery and client/share
cleanup behavior.

For backup/restore evidence, provide a separate disposable restore target:

```bash
MEGAVPN_RELEASE_DATABASE_DSN='postgres://...' \
MEGAVPN_RELEASE_RESTORE_DATABASE_DSN='postgres://...' \
scripts/ci/release-gate.sh
```

For API/worker/agent E2E, run against a disposable control plane and test node:

```bash
MEGAVPN_RELEASE_BASE_URL=https://control.example.com:58765 \
MEGAVPN_AUTH_TOKEN=... \
scripts/ci/release-gate.sh
```

Minimum runtime evidence:

- `/health` returns success as liveness.
- `/api/v1/ready` returns success with `MEGAVPN_PRODUCTION_MODE=true`; this requires the runtime preflight status to be `ready`, not just a database ping.
- Node enrollment creates a persistent agent token and heartbeat.
- Agent job claim/result lifecycle works.
- Stale running job leases recover after expiry.

## 4. Infra Gate

Required checks:

- A fresh Ubuntu host can install the Control Plane from documented scripts or manual steps without relying on previous local state.
- `scripts/ops/control-plane-install.sh` validates non-interactive clean-install inputs through `MEGAVPN_CP_VALIDATE_ONLY=1`.
- `systemd-analyze verify deploy/systemd/*.service` passes on a Linux host with systemd.
- `nginx -t` passes on the target host after managed control-plane TLS apply.
- `scripts/ops/backup.sh` produces an archive from the disposable DB.
- `scripts/ops/restore.sh` restores the archive into a separate disposable DB.
- Rollback plan is documented for the exact release version.
- Rewritten-history deployment, when used, has an explicit maintenance window and a documented server-side checkout recovery procedure.

Run backup/restore drill:

```bash
MEGAVPN_RELEASE_DATABASE_DSN='postgres://source...' \
MEGAVPN_RELEASE_RESTORE_DATABASE_DSN='postgres://target...' \
scripts/ci/release-gate.sh
```

## 5. VPN / Service Gate

Run the service-pack smoke matrix on a disposable node:

```bash
MEGAVPN_RELEASE_RUN_SERVICE_MATRIX=1 \
MEGAVPN_RELEASE_BASE_URL=https://control.example.com:58765 \
MEGAVPN_AUTH_TOKEN=... \
MEGAVPN_RELEASE_NODE_ID=... \
MEGAVPN_RELEASE_ENDPOINT_DOMAIN=smoke.example.com \
scripts/ci/release-gate.sh
```

For staged protocol validation on a shared disposable node, restrict the matrix
with `MEGAVPN_SMOKE_PACKS` or `MEGAVPN_SMOKE_EXCLUDE_PACKS`. Full production
promotion still requires evidence for every pack in the minimum matrix, but
batched runs reduce port conflicts while diagnosing individual runtimes. Run
`scripts/smoke/service-pack-smoke.sh --matrix ... --plan` first when changing the
batch: the dry run must show the intended packs, required fallback/certificate
inputs and any listen-port overlap before instances are created. For
operator-friendly staged runs, use `scripts/smoke/service-pack-staged-smoke.sh`:
`remote_access_l3`, `proxy_access`, `xray_reality`, `xray_nginx_http`,
`xray_nginx_grpc` and `legacy_l2tp` are separate batch names with per-batch
evidence directories, automatic evidence-report validation and a top-level
`_staged-summary.json` under the staged evidence root. The summary path is
printed as `staged_summary:` and can be overridden with
`MEGAVPN_SMOKE_STAGED_SUMMARY_FILE`. The runner fails before creating
resources when selected batches reuse endpoint ports without
`MEGAVPN_SMOKE_CLEANUP=1`; use cleanup for diagnostic all-batch runs on one
node, or run 443-based batches on isolated nodes for final evidence. For
diagnostic reruns on a shared disposable node, `MEGAVPN_SMOKE_CLEANUP=1` may be
used to delete smoke clients and instances after a successful run; add
`MEGAVPN_SMOKE_CLEANUP_ON_FAILURE=1` only when failed diagnostic runs should
also remove partial resources automatically. Final release evidence should keep
cleanup disabled until the runtime state has been reviewed. Set
`MEGAVPN_SMOKE_EVIDENCE_DIR` for release-candidate runs so every successful
pack writes machine-readable JSON evidence with created instances, runtime
states, provision result and artifacts. Matrix runs must also retain
`_matrix-summary.json` or the file configured by
`MEGAVPN_SMOKE_MATRIX_SUMMARY_FILE` so OK/FAILED/SKIPPED rows remain tied to
their per-pack evidence files. After every matrix run, render and validate the
saved evidence before accepting it:

```bash
scripts/ci/service-pack-evidence-report.js \
  --require-pack openvpn_tcp_11994,openvpn_udp_1194,wireguard_roadwarrior \
  tmp/service-pack-evidence/_matrix-summary.json
```

For final promotion, include every pack from the minimum matrix in
`--require-pack`; the report is fail-closed for failed rows, missing per-pack
evidence, non-ready runtime state, inactive service accesses and missing or
wrong-type client artifacts. When the matrix is run through
`scripts/ci/release-gate.sh`, the same validation runs automatically if
`MEGAVPN_SMOKE_EVIDENCE_DIR` or
`MEGAVPN_SMOKE_MATRIX_SUMMARY_FILE` is set. Use
`MEGAVPN_RELEASE_SERVICE_MATRIX_REQUIRED_PACKS` for the required pack list and
`MEGAVPN_RELEASE_SERVICE_MATRIX_REQUIRE_NO_SKIPS=1` when any skipped row should
block the gate.

Minimum matrix:

- OpenVPN TCP/UDP
- WireGuard
- Xray Reality
- Xray WebSocket/Nginx edge
- VLESS instance-level egress through managed ingress-to-egress backhaul
- HTTP Proxy
- MTProto
- Shadowsocks
- IPsec/L2TP

The matrix must verify created instance runtime projection after apply:
`runtime_status=active`, `health_status=healthy` and `drift_status=in_sync`.
For final release evidence, `scripts/ci/release-gate.sh` passes
`MEGAVPN_SMOKE_REQUIRE_AGENT_REPORT=1` by default so the matrix proves
agent-reported systemd/listening-port state, not only job-derived runtime
state. On clean nodes the matrix must also wait for any `runtime_install_jobs`
created by the service-pack preflight before applying dependent instances.
Client provisioning smoke must then wait for post-provision `instance.apply`
jobs and verify every selected service access is active with a ready per-access
artifact of the expected protocol type, not merely that one artifact exists for
the client.
Override with `MEGAVPN_RELEASE_REQUIRE_AGENT_REPORT=0` only for diagnostics.
Drivers that are intentionally materialize-only must be recorded as such, not
counted as active runtime success.

## 5.1 Topology And Access Gate

Required evidence before promoting topology/access features beyond the hardening baseline:

- Node map API projection resolves approximate country/city/provider metadata from public node IPs, skips private addresses, and does not expose unnecessary secrets.
- Managed backhaul graph shows active/degraded/failed links and the last probe reason.
- Client provisioning clearly shows which inbound services were selected for a client.
- VLESS access groups are managed centrally under `Clients -> Groups`;
  default, local-breakout, selected-egress, target-only and blocked behavior is
  covered by group membership/materialization and provisioning/apply evidence.
- VLESS subscription endpoint uses per-client bearer token rotation, audit events,
  `Cache-Control: no-store`, active-access filtering and one-time plaintext URL
  display.
- Traffic camouflage profiles validate Nginx and Xray config before apply,
  require an explicit HTTP(S) fallback website in API/smoke flows, reject
  fallback URL/Host/SNI values that target the same public ingress endpoint,
  and expose rollback on failed `nginx -t` or runtime validation.
- Agent `instance.apply` snapshots managed files and restores/removes them on failed validation, network-policy or systemd apply.
- Nginx edge profiles bind explicit certificate material and do not silently fall back to an unintended self-signed profile after a managed certificate exists.

## 6. Observability Gate

Required UI/API visibility before release:

- Job result includes failure stage and reason.
- Agent version drift is visible per node.
- Runtime state exposes health/drift reason.
- Backhaul apply/probe surfaces systemd state, interface state and first useful diagnostic line.
- Audit log can answer: who changed settings, who queued bootstrap/apply/capability jobs, who published/revoked share links.

## 7. Documentation Gate

Required documents:

- `README.md`
- `README_RU.md`
- `docs/DOCUMENTATION.md`
- `docs/DOCUMENTATION_RU.md`
- `docs/USER_GUIDE_RU.md`
- `docs/USER_GUIDE_EN.md`
- `ROADMAP_V1_AND_TZ.md`
- `ROADMAP_V1_AND_TZ_RU.md`
- `docs/NEXT_STEPS.md`
- `docs/NEXT_STEPS_RU.md`
- `docs/THREAT_MODEL.md`
- `docs/RBAC_MATRIX.md`
- `docs/OPERATIONS_RUNBOOK.md`
- `docs/RELEASE_GATES.md`
- `docs/SELF_TESTING.md`
- `docs/DOCUMENTATION_REVIEW.md`
- `docs/DOCUMENTATION_REVIEW_RU.md`
- Production env templates under `deploy/env/`

User-facing workflows must have Russian and English documentation before they
can be treated as production-ready.
