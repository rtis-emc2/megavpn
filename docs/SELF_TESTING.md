# Self-Testing

**Release:** `7.0.1.20`

`scripts/self-test.sh` is the broad diagnostic entrypoint for release readiness. It differs from `scripts/release-gate.sh`: the release gate is fail-fast, while self-test keeps running independent gates and writes a report that separates working, failing and not-tested areas.

## Local Command

```bash
scripts/self-test.sh
```

The report is written under `tmp/self-test/` by default:

```bash
MEGAVPN_SELF_TEST_REPORT_DIR=/tmp/megavpn-self-test scripts/self-test.sh
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
- `control-plane-install-validation`
- `frontend-js-syntax`, when `node` is installed
- `static-security-patterns`
- `smoke-auth-coverage`
- `migration-sequence`
- `release-docs`

The `release-docs` gate verifies that the corporate documentation baseline is
present: English and Russian README files, documentation indexes, documentation
reviews, user guides, threat model, RBAC matrix, operations runbook, release
gates, self-testing guide, roadmap/next-step pairs and production environment
templates.
Each required artifact must declare the current code release near the top of the
file, using `internal/platform/version.Version` as the source of truth.

The `control-plane-install-validation` gate runs the Control Plane installer in
validate-only mode with non-interactive clean-install inputs. It verifies that
the installer accepts a production-shaped configuration without requiring root
writes, systemd changes, package installation or network access.

`FAIL` means a gate ran and found a product or repository problem. `SKIP` means the host or environment did not provide enough release evidence; skipped gates are not acceptable for a production tag.

## Disposable Database Gates

Use disposable PostgreSQL databases. Never point these variables at production.

```bash
MEGAVPN_RELEASE_DATABASE_DSN='postgres://megavpn:megavpn@127.0.0.1:5432/megavpn_selftest?sslmode=disable' \
MEGAVPN_RELEASE_RESTORE_DATABASE_DSN='postgres://megavpn:megavpn@127.0.0.1:5432/megavpn_restore_selftest?sslmode=disable' \
scripts/self-test.sh
```

This enables:

- `postgres-migrations-and-integration`
- `backup-restore-drill`

## Live Runtime Gates

Use a disposable control plane and disposable node.

```bash
MEGAVPN_RELEASE_BASE_URL=https://control.example.com:58765 \
MEGAVPN_AUTH_TOKEN=... \
MEGAVPN_RELEASE_NODE_ID=... \
MEGAVPN_RELEASE_ENDPOINT_DOMAIN=smoke.example.com \
MEGAVPN_SELF_TEST_RUN_SERVICE_MATRIX=1 \
scripts/self-test.sh
```

This enables:

- `api-smoke`
- `vpn-service-smoke-matrix`

The matrix covers OpenVPN, WireGuard, Xray, HTTP Proxy, MTProto, Shadowsocks and IPsec/L2TP through `scripts/service-pack-smoke.sh`.

Smoke helpers under `scripts/` honor `MEGAVPN_AUTH_TOKEN` and send it as a bearer token. Keep that token scoped to the release-test operator role and rotate it after every shared test environment run.

## Host Infra Gates

Run on a Linux host that has the target dependencies installed:

- `systemd-verify` requires `systemd-analyze`.
- `nginx-t` requires `nginx`.

To validate a specific nginx config:

```bash
MEGAVPN_SELF_TEST_NGINX_CONFIG=/etc/nginx/nginx.conf scripts/self-test.sh
```

## Required Output For Release Review

Attach the generated self-test Markdown report to the release review. Production release approval requires:

- zero `FAIL` gates;
- zero unexplained `SKIP` gates for runtime, infra, database, backup/restore and service matrix;
- linked logs for any rerun after remediation;
- explicit waiver only for gates that are intentionally out of scope for the release.
