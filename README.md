# RTIS MegaVPN

**RTIS MegaVPN 0.6.7.2-alpha**

RTIS MegaVPN — self-hosted distributed VPN orchestration platform.

Canonical repository: `github.com/rtis-emc2/megavpn`

Проект предназначен для централизованного управления VPN, proxy и edge-инфраструктурой через единую Control Plane с удаленными Agent-managed nodes.

Основная идея платформы:

```text
Control Plane
    ↓
Node Agents
    ↓
Ingress / Egress / Runtime Nodes
    ↓
VPN / Proxy Services
```

Поддерживаемые сервисы (roadmap):

- OpenVPN
- Xray (VLESS / Reality / xHTTP)
- WireGuard
- IPsec / L2TP / IKEv2
- Shadowsocks
- MTProto
- HTTP Proxy
- Nginx Edge Runtime

---

# Current Status

Release: `0.6.7.2-alpha`

Current branch status: WireGuard runtime lifecycle, HTTP Proxy / Squid runtime lifecycle, MTProto direction via Xray runtime, secure bootstrap hardening and deployment baseline stabilization.

Immediate program priorities:

- stabilize clean GitHub baseline under `rtis-emc2/megavpn`
- complete `0.6.7.2-alpha` CI/build/deploy verification
- production-smoke OpenVPN / WireGuard / HTTP Proxy / MTProto paths on the test server
- continue driver hardening, integration tests and release gates

Реализовано:

- API Control Plane
- PostgreSQL backend
- Agent enrollment model
- One-time enrollment tokens
- Persistent agent identity
- Heartbeat transport
- Job polling transport
- Node lifecycle
- Services catalog
- Audit events
- Worker runtime
- Systemd integration
- Runtime migrations
- Node inventory collection
- Service discovery
- Capability installation jobs
- nginx installer via official nginx.org repository
- xray-core installer via official XTLS/Xray-install flow
- openvpn installer via Ubuntu repository
- wireguard installer via Ubuntu repository
- strongSwan / IPsec installer via Ubuntu repository
- squid HTTP proxy installer via Ubuntu repository
- xl2tpd installer via Ubuntu repository
- shadowsocks-libev installer via Ubuntu repository
- wireguard instance driver with managed peer generation and client config export
- squid HTTP proxy instance driver with managed credentials and client bundle export
- mtproto runtime driver with managed secrets and Telegram proxy URL export
- live Services WUI with node capability matrix and install/verify actions
- PostgreSQL-backed SMTP settings
- Operator invite flow with one-time password setup link
- Client access email delivery with attachments/share links
- Public share download endpoint for artifact links
- Basic in-process rate limiting for login, invite accept, public share download and agent registration
- Secure-by-default bootstrap admin flow: no built-in default password; operator credentials must be provided explicitly

---

# Architecture

## Core Components

### Control Plane

Компонент централизованного управления.

Отвечает за:

- nodes
- services
- instances
- revisions
- jobs
- audit
- orchestration
- API
- authentication
- state management

Запуск:

```text
cmd/api
```

---

### Worker

Background runtime.

Отвечает за:

- queue processing
- async tasks
- cleanup
- orchestration jobs
- future schedulers

Запуск:

```text
cmd/worker
```

---

### Agent

Удаленный runtime-agent.

Устанавливается на ingress/egress nodes.

Отвечает за:

- enrollment
- heartbeat
- job polling
- runtime execution
- config apply
- artifacts
- service lifecycle

Запуск:

```text
cmd/agent
```

---

### Migrate

Database migrations runner.

Запуск:

```text
cmd/migrate
```

---

# Node Enrollment Model

RTIS MegaVPN uses a production-oriented enrollment model.

## Enrollment Flow

```text
1. Control Plane creates node
2. Control Plane issues one-time enrollment token
3. Agent performs enrollment
4. Agent receives permanent identity
5. Agent stores local state
6. Enrollment token becomes invalid
7. Agent uses persistent auth afterwards
```

---

## Security Model

Enrollment token:

```text
single-use
time-limited
server-side hashed
revocable
```

Agent identity:

```text
persistent
stored locally
authenticated on every request
```

---

# Operator Bootstrap

RTIS MegaVPN does not create an operator with a built-in default password. Bootstrap credentials create the first operator only when the `platform_users` table is empty.

For the first deployment, set explicit bootstrap credentials before starting `megavpn-api`:

```bash
export MEGAVPN_BOOTSTRAP_ADMIN_USERNAME=superadmin
export MEGAVPN_BOOTSTRAP_ADMIN_EMAIL=superadmin@rtis.local
export MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD='replace-with-a-long-random-password'
```

Password policy currently requires at least 12 characters. After the first operator exists, remove the bootstrap password from the runtime environment and manage users through the platform UI/API. Existing users are not reactivated and their passwords are not reset from bootstrap env on API restart.

Successful and failed operator login attempts are recorded in the audit log. Failed login responses remain intentionally generic.

For HTTPS deployments, `MEGAVPN_AUTH_SESSION_COOKIE_SECURE` is enabled automatically when `MEGAVPN_PUBLIC_BASE_URL` starts with `https://`. It can still be overridden explicitly when needed.

If the API is exposed only through a trusted reverse proxy, set:

```bash
export MEGAVPN_TRUST_PROXY_HEADERS=true
```

This makes audit/session records and in-process rate limits use `X-Forwarded-For` / `X-Real-IP`. Keep it disabled when clients can connect to the Go API directly.

Request bodies are capped by `MEGAVPN_API_MAX_REQUEST_BYTES` and default to `16777216` bytes. JSON request bodies are decoded strictly: unknown fields, malformed JSON and trailing JSON documents are rejected. Browser cookie-authenticated mutating requests require the `X-MegaVPN-CSRF: 1` header; the bundled Web UI sends it automatically and relies on the HttpOnly session cookie instead of storing bearer tokens in `localStorage`.

Agent runtime authorization should use per-node enrollment and persistent agent tokens. The legacy shared `MEGAVPN_AGENT_TOKEN` fallback is accepted for node/job agent endpoints only when `MEGAVPN_AGENT_ALLOW_AUTO_REGISTER=true`; keep that disabled in production.

OpenVPN instances use a platform-scoped PKI root by default. The first provisioning/apply path creates one active `openvpn/default` CA in `platform_service_pki_roots`, stores CA material in encrypted `secret_refs`, signs per-instance server certificates from that CA, and signs client certificates from the same root. Instance config paths are slug-scoped (`/etc/openvpn/server/<slug>.conf`) to match `openvpn-server@<slug>`. Moving an OpenVPN instance to another ingress node therefore requires reapplying the instance/server config, not replacing every client CA.

The current PKI roots are visible to operators with `instance.read` through the Settings UI and the `GET /api/v1/platform/pki-roots` endpoint. The API response intentionally exposes the public CA certificate secret reference only; the CA private key reference remains server-side.

---

# Remote Test Deployment

Typical test-server flow:

```bash
cd /opt/megavpn
sudo ./deploy-local.sh
```

The deploy script is intentionally interactive. It asks for confirmation before updating from GitHub:

```text
Update RTIS MegaVPN in /opt/megavpn from GitHub and deploy it? [y/N]:
```

Behavior:

- `y` / `yes`: fetches the current branch from GitHub, synchronizes the local checkout, downloads Go modules, runs tests, builds binaries, installs Web UI and systemd units, runs migrations, restarts API/worker, restarts agent only if it was active before deploy, then checks `/healthz`.
- `n` / `no` / empty answer: exits without changes.

Required deploy host commands: `git`, `go`, `systemctl`, `curl`, `rsync`.

Useful environment overrides:

```bash
MEGAVPN_DEPLOY_APP_DIR=/opt/megavpn
MEGAVPN_DEPLOY_REMOTE=origin
MEGAVPN_DEPLOY_BRANCH=main
MEGAVPN_DEPLOY_HEALTH_URL=http://127.0.0.1:8080/healthz
MEGAVPN_DEPLOY_ENV_FILE=/etc/megavpn/megavpn.env
MEGAVPN_DEPLOY_RUN_TESTS=1
MEGAVPN_DEPLOY_YES=0
MEGAVPN_DEPLOY_SYNC_MODE=auto
```

The script refuses to deploy from a dirty worktree by default. Use `MEGAVPN_DEPLOY_ALLOW_DIRTY=1` only for emergency diagnostics on a disposable test host.

Git sync behavior:

- `MEGAVPN_DEPLOY_SYNC_MODE=auto`:
  - fast-forwards when possible
  - if local history diverges from `origin/<branch>`, creates a local backup branch `deploy-backup/<utc>-<sha>` and resets to the remote branch automatically
- `MEGAVPN_DEPLOY_SYNC_MODE=ff-only`:
  - refuses deploy when fast-forward is impossible
- `MEGAVPN_DEPLOY_SYNC_MODE=reset-hard`:
  - always creates a backup branch and hard-resets to `origin/<branch>`

`deploy-local.sh` loads `/etc/megavpn/megavpn.env` by default before running migrations, so the same database DSN used by systemd is available to `megavpn-migrate`.

---

# Filesystem Layout

RTIS MegaVPN follows Linux FHS conventions.

## Application

```text
/opt/megavpn/
```

Contains:

- binaries
- scripts
- web assets
- migrations

---

## Configuration

```text
/etc/megavpn/
```

Examples:

```text
/etc/megavpn/megavpn.env
/etc/megavpn/agent.env
/etc/megavpn/agent-bootstrap.env
```

---

## Runtime State

```text
/var/lib/megavpn/
```

Examples:

```text
/var/lib/megavpn/agent/state.json
/var/lib/megavpn/agent/work/
```

---

## Logs

```text
journalctl
```

Systemd-first logging model is currently used.
