# Self-Testing

**Release:** `7.1.1.8`

`scripts/ci/self-test.sh` is the broad diagnostic entrypoint for release readiness. It differs from `scripts/ci/release-gate.sh`: the release gate is fail-fast, while self-test keeps running independent gates and writes a report that separates working, failing and not-tested areas.

## Local Command

```bash
scripts/ci/self-test.sh
```

The report is written under `tmp/self-test/` by default:

```bash
MEGAVPN_SELF_TEST_REPORT_DIR=/tmp/megavpn-self-test scripts/ci/self-test.sh
```

Local gates:

- `gofmt-clean`
- `version-tag-consistency`
- `go-test`
- `go-test-race`
- `go-vet`
- `go-build`
- `binary-version-commands`
- `shell-syntax`
- `actions-pinning`
- `docs-consistency`
- `control-plane-install-validation`
- `frontend-js-syntax`, when `node` is installed
- `frontend-bootstrap-smoke`, when `node` is installed
- `service-pack-smoke-regression`, when `node` is installed
- `static-security-patterns`
- `smoke-auth-coverage`
- `migration-sequence`

The `docs-consistency` gate delegates to `scripts/ci/docs-consistency.sh`. It
verifies that the corporate documentation baseline is present: English and
Russian README files, documentation indexes, documentation reviews, user
guides, threat model, RBAC matrix, operations runbook, release gates,
self-testing guide, roadmap/next-step pairs, current security review, Web UI
asset cache keys and production environment templates. Each required artifact
must declare the current code release near the top of the file, using
`internal/platform/version.Version` as the source of truth.

The `control-plane-install-validation` gate runs the Control Plane installer in
validate-only mode with non-interactive clean-install inputs. It verifies that
the installer accepts a production-shaped configuration without requiring root
writes, systemd changes, package installation or network access.

The `service-pack-smoke-regression` gate runs `scripts/smoke/service-pack-smoke.sh`
against a local mock API. It verifies matrix `--plan` filters, unknown pack
fail-fast behavior, runtime-install polling, post-provision `instance.apply`
polling, per-access artifact validation, success/failure cleanup paths, staged
batch planning and the offline evidence report validator without touching a
real control plane or node.

`FAIL` means a gate ran and found a product or repository problem. `SKIP` means the host or environment did not provide enough release evidence; skipped gates are not acceptable for a production tag.

## Disposable Database Gates

Use disposable PostgreSQL databases. Never point these variables at production.

```bash
MEGAVPN_RELEASE_DATABASE_DSN='postgres://megavpn:megavpn@127.0.0.1:5432/megavpn_selftest?sslmode=disable' \
MEGAVPN_RELEASE_RESTORE_DATABASE_DSN='postgres://megavpn:megavpn@127.0.0.1:5432/megavpn_restore_selftest?sslmode=disable' \
scripts/ci/self-test.sh
```

This enables:

- `postgres-migrations-and-integration`: `scripts/ci/postgres-migration-drill.sh`
  applies every migration from an empty disposable database, applies them again
  to prove idempotent skip behavior, checks critical tables, required indexes,
  token-storage columns, VLESS group templates and firewall seed groups, then
  runs the PostgreSQL integration suite.
- `backup-restore-drill`: creates a backup archive from the migrated disposable
  database and restores it into the separate restore database.

`MEGAVPN_RELEASE_DATABASE_DSN` must be empty for release evidence. Use
`MEGAVPN_MIGRATION_DRILL_ALLOW_EXISTING=1` only for local diagnostics when you
explicitly accept that the run is not a zero-database migration proof.

## Live Runtime Gates

Use a disposable control plane and disposable node.

```bash
MEGAVPN_RELEASE_BASE_URL=https://control.example.com:58765 \
MEGAVPN_AUTH_TOKEN=... \
MEGAVPN_RELEASE_NODE_ID=... \
MEGAVPN_RELEASE_ENDPOINT_DOMAIN=smoke.example.com \
MEGAVPN_SELF_TEST_RUN_SERVICE_MATRIX=1 \
scripts/ci/self-test.sh
```

This enables:

- `api-smoke`
- `vpn-service-smoke-matrix`

The matrix covers OpenVPN, WireGuard, Xray, HTTP Proxy, MTProto, Shadowsocks and IPsec/L2TP through `scripts/smoke/service-pack-smoke.sh`.
When `MEGAVPN_SMOKE_EVIDENCE_DIR` or `MEGAVPN_SMOKE_MATRIX_SUMMARY_FILE` is set,
the live matrix gate also runs `scripts/ci/service-pack-evidence-report.js`. Use
`MEGAVPN_SELF_TEST_SERVICE_MATRIX_REQUIRED_PACKS` and
`MEGAVPN_SELF_TEST_SERVICE_MATRIX_REQUIRE_NO_SKIPS=1` to make a diagnostic run
fail-closed for a staged protocol batch.
For manual staged runs, `scripts/smoke/service-pack-staged-smoke.sh` wraps the matrix
into protocol batches, writes one evidence directory per batch and writes a
top-level `_staged-summary.json` for the whole operator run. It refuses real
multi-batch runs with known endpoint-port overlaps unless cleanup is enabled,
so diagnostics do not accidentally create conflicting 443 listeners on one
node.

Smoke helpers under `scripts/` honor `MEGAVPN_AUTH_TOKEN` and send it as a bearer token. Keep that token scoped to the release-test operator role and rotate it after every shared test environment run.

## Host Infra Gates

Run on a Linux host that has the target dependencies installed:

- `systemd-verify` requires `systemd-analyze`.
- `nginx-t` requires `nginx`.

To validate a specific nginx config:

```bash
MEGAVPN_SELF_TEST_NGINX_CONFIG=/etc/nginx/nginx.conf scripts/ci/self-test.sh
```

## Required Output For Release Review

Attach the generated self-test Markdown report to the release review. Production release approval requires:

- zero `FAIL` gates;
- zero unexplained `SKIP` gates for runtime, infra, database, backup/restore and service matrix;
- linked logs for any rerun after remediation;
- explicit waiver only for gates that are intentionally out of scope for the release.
