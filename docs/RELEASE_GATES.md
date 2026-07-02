# Release Gates

**Release:** `7.0.1.2`

This file defines the minimum evidence required before tagging a production release.

For `7.0.1.2`, these gates are used as release promotion evidence. A release can be published with documented product gaps, but not with unknown install, migration, agent-channel, node-cleanup or runtime-apply behavior.

## 1. Release Gate

Required command:

```bash
scripts/release-gate.sh
```

The release gate is fail-closed: any skipped gate makes the command fail because skipped checks are not production release evidence. For local diagnostics on a workstation that does not have PostgreSQL, systemd, nginx or a disposable test node, use `scripts/self-test.sh` or explicitly allow skips:

```bash
MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh
```

Diagnostic command with full PASS/FAIL/SKIP report:

```bash
scripts/self-test.sh
```

The baseline gate must pass:

- `gofmt -l cmd internal` returns no files.
- `go test ./...` passes.
- `go test -race ./...` passes. Disabling it with `MEGAVPN_RELEASE_RUN_RACE=0` is a local diagnostic shortcut and counts as a skipped release gate.
- `go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin` passes.
- Operational binaries print the same release through `--version`.
- Shell scripts under `scripts/` pass `bash -n`.
- The Control Plane installer accepts non-interactive clean-install inputs in validate-only mode.
- Smoke scripts that call `/api/v1` support `MEGAVPN_AUTH_TOKEN`.
- Static production scan finds no `/bin/sh -c`, `StrictHostKeyChecking=accept-new`, or curl-to-shell pattern outside tests.

## 2. Security Gate

Required release evidence:

- Share-link tokens are stored as `token_hash` plus `token_hint`; plaintext token is returned only once at publish time.
- Agent transport rejects unsigned responses by default, including empty `204` polls.
- SSH bootstrap requires pinned `ssh_host_key_sha256`.
- Xray remote installer is fail-closed unless `MEGAVPN_XRAY_INSTALL_SCRIPT_SHA256` pins the downloaded script.
- Audit events exist for bootstrap, instance apply, capability install/verify, share-link publish/revoke, login, settings and certificate changes.
- Secret rotation runbook in `docs/OPERATIONS_RUNBOOK.md` is reviewed for this release.

## 3. Runtime Gate

Use a disposable PostgreSQL database:

```bash
MEGAVPN_RELEASE_DATABASE_DSN='postgres://...' scripts/release-gate.sh
```

This runs migrations and PostgreSQL integration tests, including job lifecycle and stale lease recovery.

For API/worker/agent E2E, run against a disposable control plane and test node:

```bash
MEGAVPN_RELEASE_BASE_URL=https://control.example.com:58765 \
MEGAVPN_AUTH_TOKEN=... \
scripts/release-gate.sh
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
- `scripts/control-plane-install.sh` validates non-interactive clean-install inputs through `MEGAVPN_CP_VALIDATE_ONLY=1`.
- `systemd-analyze verify deploy/systemd/*.service` passes on a Linux host with systemd.
- `nginx -t` passes on the target host after managed control-plane TLS apply.
- `scripts/backup.sh` produces an archive from the disposable DB.
- `scripts/restore.sh` restores the archive into a separate disposable DB.
- Rollback plan is documented for the exact release version.
- Rewritten-history deployment, when used, has an explicit maintenance window and a documented server-side checkout recovery procedure.

Run backup/restore drill:

```bash
MEGAVPN_RELEASE_DATABASE_DSN='postgres://source...' \
MEGAVPN_RELEASE_RESTORE_DATABASE_DSN='postgres://target...' \
scripts/release-gate.sh
```

## 5. VPN / Service Gate

Run the service-pack smoke matrix on a disposable node:

```bash
MEGAVPN_RELEASE_RUN_SERVICE_MATRIX=1 \
MEGAVPN_RELEASE_BASE_URL=https://control.example.com:58765 \
MEGAVPN_AUTH_TOKEN=... \
MEGAVPN_RELEASE_NODE_ID=... \
MEGAVPN_RELEASE_ENDPOINT_DOMAIN=smoke.example.com \
scripts/release-gate.sh
```

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

Drivers that are intentionally materialize-only must be recorded as such, not counted as active runtime success.

## 5.1 Topology And Access Gate

Required evidence before promoting topology/access features beyond the hardening baseline:

- Node map API projection resolves approximate country/city/provider metadata from public node IPs, skips private addresses, and does not expose unnecessary secrets.
- Managed backhaul graph shows active/degraded/failed links and the last probe reason.
- Client provisioning clearly shows which inbound services were selected for a client.
- VLESS access groups are managed centrally under `Instances -> VLESS groups`;
  default, local-breakout, selected-egress, target-only and blocked behavior is
  covered by provisioning/apply evidence.
- VLESS subscription endpoint uses per-client bearer token rotation, audit events,
  `Cache-Control: no-store`, active-access filtering and one-time plaintext URL
  display.
- Traffic camouflage profiles validate Nginx and Xray config before apply and expose rollback on failed `nginx -t` or runtime validation.
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
