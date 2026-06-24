# RTIS MegaVPN

**RTIS MegaVPN 0.6.10.20-alpha**

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

Release: `0.6.10.20-alpha`

Current branch status: expanded service-pack matrix for standalone services, provisioning/artifact/share-link smoke runner for the test server, stricter revision/apply lifecycle with validation markers, rollback baseline and revision diff visibility, certificate lifecycle hardening, job payload validation, deployment baseline stabilization, a top-tabbed node console, actionable node lifecycle/onboarding guidance, visible agent public URL runtime config, automatic SSH bootstrap handoff into the agent-managed channel, HTTPS-only control plane edge guidance, interactive Control Plane installer with secure defaults, sudo/snap Go path handling and correct systemd oneshot migration result handling, UI-backed Control Plane TLS profile settings, worker-driven nginx TLS edge apply, unified service driver contract with typed operation and health-check interfaces, operation-aware agent runtime handlers, service-specific agent validation registry, driver-backed instance runtime health/drift projection with retained observation history and UI-visible health/drift reasons, bidirectional HMAC-signed agent transport for agent requests plus job/runtime-target responses with replay-window verification, route-policy enforceability and ingress egress/output projection for L3/L4 candidates vs observe-only routes, conservative kernel policy routing for IPv4 L3/L4 allow candidates through managed backhaul, compact modular Instances/Revisions/Services/Ops/Clients/Artifacts/Backhaul/Nodes/Settings/Certificates UI pages, safe instance soft-delete flow, a fixed modular UI dependency export that prevents the login screen from rendering as a blank page, an offline admin password reset utility for operational lockout recovery, local-file certificate import with RSA PKCS#1/PKCS#8 key support plus pre-import certificate preview, hardened SSH bootstrap known_hosts/key handling inside the worker sandbox, installer preservation of already-applied managed Control Plane TLS certificates, private-domain-free Xray WebSocket camouflage service-pack defaults with Nginx website fallback, UI-managed client egress/output route policy plus selective client provisioning and authenticated client config preview/download for OVPN/VLESS/WireGuard and related artifacts, managed ingress-egress backhaul links with explicit internal transport profile selection, WireGuard/OpenVPN install-on-apply auto-activation, WireGuard `/30` tunnel address rendering for deterministic peer routes, egress NAT bootstrap with nftables dependency enforcement, unique tunnel CIDR per L3 transport profile, materialize-only IPsec/Xray fallback profiles, stale job lease recovery for backhaul cleanup/delete operations, UI-visible agent version drift with per-node and bulk SSH-bootstrap agent update actions, guarded agent update buttons that surface missing `node.bootstrap` or SSH access preconditions before queueing jobs, corrected backhaul apply semantics that apply every selected profile while keeping profile-only drivers out of false `active` state, separated backhaul service readiness from delayed connectivity probes, preserved backhaul probe diagnostics with peer route lookup, packet-loss, latency and reason visibility across Jobs and Backhaul UI, explicit partial-apply visibility for ingress/egress backhaul sides, root-cause-first backhaul readiness failure messages, idempotent backhaul re-apply/cleanup that stops managed units and removes target `mgbh*` interfaces before recreating runtime state, quoted nft backhaul NAT comments plus stale sibling manifest/interface cleanup for reliable WireGuard re-apply, OpenVPN static-key backhaul compatibility without version-sensitive compression directives, UI-visible first-line systemd diagnostics for failed backhaul jobs, and recent deleted Backhaul visibility with cleanup summaries after successful node cleanup.

Immediate program priorities:

- stabilize clean GitHub baseline under `rtis-emc2/megavpn`
- complete `0.6.10.20-alpha` CI/build/deploy verification
- verify `MEGAVPN_PUBLIC_BASE_URL` with custom ports in Settings and Agent channel diagnostics
- verify the control plane public edge terminates TLS on the same host/port used by `MEGAVPN_PUBLIC_BASE_URL`
- verify Settings -> Control Plane TLS can select imported commercial certificates or self-signed fallback profile and apply nginx TLS edge through worker job
- verify SSH bootstrap success changes node setup method to agent-managed while waiting for first `/agent/register` and heartbeat
- verify the redesigned node bootstrap console on the remote test control plane without horizontal scrolling in access methods, bootstrap runs and enrollment tokens
- production-smoke OpenVPN / WireGuard / HTTP Proxy / MTProto paths on the test server
- run PostgreSQL integration tests with `MEGAVPN_TEST_DATABASE_DSN` against a disposable database/schema before production deploy
- continue agent transport hardening, mTLS decision and release gates

For repeatable service-pack and operational-truth smoke on the test server, use:

```bash
scripts/service-pack-smoke.sh --list
scripts/service-pack-smoke.sh --matrix <node-id> <endpoint-domain> [certificate-id]
scripts/service-pack-smoke.sh <node-id> <pack-key> <endpoint-host> [base-name] [certificate-id]
scripts/service-pack-smoke.sh --with-share-links <node-id> <pack-key> <endpoint-host> [base-name] [certificate-id]
```

For control-plane admin lockout recovery on the server, use:

```bash
cd /opt/megavpn
sudo ./scripts/control-plane-reset-admin-password.sh
sudo cat /root/megavpn-control-plane-admin.txt
```

The reset wrapper reads `/etc/megavpn/megavpn.env`, generates a new random password for `superadmin`, stores it root-only and updates the existing local platform user through the same PBKDF2 password hashing path as the API.

Certificate import accepts local operator-selected PEM files in the browser for certificate, private key and optional CA chain. The API validates RSA PKCS#1, RSA/ECDSA/Ed25519 PKCS#8 and EC private keys, checks that the certificate and key match, and returns a preview with CN, SANs, issuer, expiry and chain count before storing material in secret refs.

SSH bootstrap stores first-seen remote host keys in `/var/lib/megavpn/ssh/known_hosts` instead of `/root/.ssh/known_hosts`, so the worker can run under `ProtectHome=true`. Private keys pasted with literal `\n` escapes are normalized before writing the temporary key file, and accidental public-key paste is rejected with a clear validation error.

The Control Plane installer treats `self-signed-nginx` as bootstrap fallback only. If managed TLS material exists at `/etc/megavpn/control-plane-tls/fullchain.pem` plus `/etc/megavpn/control-plane-tls/privkey.pem`, rerunning the installer keeps or restores nginx to that commercial certificate instead of downgrading the public edge back to the installer self-signed certificate. Use `MEGAVPN_CP_FORCE_SELF_SIGNED_NGINX=1` only for an intentional rollback to installer-generated self-signed TLS.

Managed backhaul links connect ingress and egress nodes through `/api/v1/backhaul-links` and the Backhaul UI page. The selected internal transport profiles are applied to both nodes by `node.backhaul.apply` agent jobs. WireGuard and OpenVPN UDP/TCP profiles now verify/install their runtime packages on apply, write config, install systemd units, start services and run local readiness checks only: systemd `active` plus tunnel interface presence. WireGuard endpoints render the local tunnel address with the `/30` prefix so Linux installs a connected route to the peer tunnel IP even with `Table=off`. OpenVPN static-key P2P configs avoid version-sensitive compression directives while keeping compression disabled by omission. Egress jobs also enforce `iproute2`/`nftables` availability before enabling managed NAT. Before activation, re-apply uses the previous managed manifest to stop/disable obsolete units, remove obsolete generated files and delete previous/current managed `mgbh*` interfaces, so stale `/32` or previous `/30` runtime state does not survive a desired-state change. IPsec/IKEv2/L2TP and Xray/VLESS profiles are written as explicit config/profile files and intentionally remain `materialized` rather than `active` until routing-loop, edge-TLS and strongSwan profile validation are finalized. Operators can run bidirectional `node.backhaul.probe` checks only after the selected transport is active and both ingress/egress sides are applied; probes wait for local readiness, peer route lookup and ping retries before returning failed/degraded. Failed apply jobs now persist per-side health/error details and prioritize root cause such as `systemd unit is not active` or `interface is not present` instead of leaving the peer side as `unknown` or showing only a generic readiness failure; Jobs and Backhaul modal summaries include the unit name, active state and first useful systemd/OpenVPN status line. Failed L3 probes preserve `degraded` health, peer route lookup, peer address, packet loss, latency and the exact agent reason instead of a generic failed message. Managed delete queues `node.backhaul.cleanup` jobs to stop generated units and remove generated files/interfaces from both nodes before the link leaves the active Backhaul list. Recent deleted links are still returned for a short operator-visibility window and displayed in a separate Backhaul cleanup history table with per-profile ingress/egress cleanup summaries. Stale `running` cleanup jobs are recovered back to `retrying` when their lease expires, so a dead agent request no longer blocks deletion forever. See `docs/BACKHAUL.md`.

Nodes now expose `agent_version`, protocol version and agent timestamps directly in `/api/v1/nodes`. The Nodes UI compares every remote agent to `/api/v1/version.agent_target_version` and shows per-node `Update` plus top-level `Update all agents` actions when a newer agent build is available. Updates are queued through the audited SSH bootstrap reinstall path and require an enabled SSH access method on the node.

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
- xray-core installer via official XTLS/Xray-install flow with required `MEGAVPN_XRAY_INSTALL_SCRIPT_SHA256` pin
- openvpn installer via Ubuntu repository
- wireguard installer via Ubuntu repository
- strongSwan / IPsec installer via Ubuntu repository
- squid HTTP proxy installer via Ubuntu repository
- xl2tpd installer via Ubuntu repository
- shadowsocks-libev installer via Ubuntu repository
- wireguard instance driver with managed peer generation and client config export
- squid HTTP proxy instance driver with managed credentials and client bundle export
- mtproto runtime driver with managed secrets and Telegram proxy URL export
- shared service driver contract for OpenVPN, Xray, WireGuard, IPsec, XL2TPD, HTTP Proxy, Shadowsocks, MTProto and Nginx
- typed driver operation interface for render, validate, apply, restart, start, stop, enable and disable lifecycle operations
- typed driver health-check interface for systemd active state, rendered config observation and endpoint listening-port signals
- operation-aware agent runtime handlers split into dispatch, apply/systemd execution, render/file materialization and validation modules
- service-specific agent validation registry for OpenVPN, Xray, WireGuard, IPsec, HTTP Proxy, Shadowsocks, MTProto and Nginx
- instance runtime state projection with runtime status, health status and drift status derived from instance jobs and agent-observed systemd/config/port reports
- driver-backed runtime health checks, health reasons and drift reasons exposed through runtime state APIs and the Instances UI
- HMAC-signed agent requests for heartbeat, inventory, runtime reports, runtime targets, job polling and job results, with timestamp/body-hash/nonce verification and replay-window protection
- HMAC-signed server-to-agent job and runtime-target responses, verified by the agent before JSON decode and protected by an in-memory replay window
- route-policy payloads classify each client access route as `l3_l4_candidate` or `observe_only` with explicit reasons, project the required egress output and apply conservative kernel policy routing for IPv4 L3/L4 allow candidates
- Xray WebSocket camouflage service pack defaults to hidden path `/assets/rtis-sync` on the public endpoint and a neutral Nginx fallback website placeholder for normal browser traffic
- Client Access route creation UI supports explicit `local_breakout` and remote `egress_node` output policy with backhaul next-hop/interface/table fields
- Managed Backhaul API/UI supports ingress-to-egress links, driver catalog discovery, generated secret refs, dual-node apply jobs, bidirectional RTT/packet-loss probes, node cleanup jobs, route-policy projection through active managed backhaul interfaces and agent-side file/path safety checks
- Client Access UI supports selective provisioning of chosen service instances and exposes generated client configs with authenticated preview/download endpoints for `.ovpn`, VLESS URL, WireGuard config, MTProto, HTTP proxy, Shadowsocks and IPsec/L2TP bundles
- interactive Control Plane installer with prompts for public URL/domain, TLS mode, PostgreSQL DSN/fields, secret master key, artifact path, bootstrap admin and systemd/nginx setup
- retained instance runtime observation history for job-derived and agent-derived health/drift snapshots
- gated PostgreSQL integration test coverage for jobs, resource locks, provisioning and client access routes
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

Production deployments must expose the Control Plane through HTTPS only. Bind the Go API to loopback and put a TLS reverse proxy in front of it:

```bash
export MEGAVPN_API_LISTEN_ADDR=127.0.0.1:8080
export MEGAVPN_PUBLIC_BASE_URL=https://control.example.com:58765
export MEGAVPN_PRODUCTION_MODE=true
export MEGAVPN_TRUST_PROXY_HEADERS=true
```

The public host and port in `MEGAVPN_PUBLIC_BASE_URL` must terminate TLS directly. Remote agents use that exact URL for `/agent/register`, `/agent/heartbeat`, `/agent/jobs/next`, `/agent/jobs/{id}/result` and `/agent/inventory`.

`MEGAVPN_PRODUCTION_MODE=true` makes `/api/v1/ready` fail closed on runtime preflight findings instead of reporting ready after a database ping only. In production mode, HTTP or local `MEGAVPN_PUBLIC_BASE_URL`, missing artifact storage, shared-token agent auto-registration, disabled request body limits and trusted proxy headers on a non-loopback API listener are readiness blockers. The normal production topology is API bound to `127.0.0.1`, HTTPS on the public edge and `MEGAVPN_TRUST_PROXY_HEADERS=true` only behind that controlled reverse proxy.

The operator-facing source of truth for this edge is `Settings -> Control Plane TLS`. It stores the HTTPS-only public URL, server name, listen port, loopback upstream and selected managed certificate from the platform Certificate Manager. Commercial certificates should be imported through `Certificates -> Add certificate`; when no commercial certificate is available, use the self-signed fallback profile and later replace it with an imported or CA-issued leaf certificate. `Apply edge` queues `platform.control_plane_tls.apply`, materializes certificate/key files under `/etc/megavpn/control-plane-tls`, writes a TLS-only `/etc/nginx/conf.d/megavpn-control-plane.conf` server block without an HTTP redirect listener, validates with `nginx -t`, and reloads nginx.

If the API is exposed only through a trusted reverse proxy, set:

```bash
export MEGAVPN_TRUST_PROXY_HEADERS=true
```

This makes audit/session records and in-process rate limits use `X-Forwarded-For` / `X-Real-IP`. Keep it disabled when clients can connect to the Go API directly.

Request bodies are capped by `MEGAVPN_API_MAX_REQUEST_BYTES` and default to `16777216` bytes. JSON request bodies are decoded strictly: unknown fields, malformed JSON and trailing JSON documents are rejected. Browser cookie-authenticated mutating requests require the `X-MegaVPN-CSRF: 1` header; the bundled Web UI sends it automatically and relies on the HttpOnly session cookie instead of storing bearer tokens in `localStorage`.

Agent runtime authorization should use per-node enrollment and persistent agent tokens. The legacy shared `MEGAVPN_AGENT_TOKEN` fallback is accepted for node/job agent endpoints only when `MEGAVPN_AGENT_ALLOW_AUTO_REGISTER=true`; keep that disabled in production.

Agent requests are HMAC-signed by updated agents with the persistent per-node token. Job/runtime-target responses from the API are also HMAC-signed and verified by the agent before JSON decode, including empty `204 No Content` polls. The signature covers method or `RESPONSE`, request URI, timestamp, nonce and SHA-256 body hash. The API verifies signed requests and rejects invalid signatures even before enforcement is enabled. Fresh installs enforce signed agent traffic by default. Temporary compatibility rollback for legacy agents:

```bash
# compatibility mode only while old agents are still being upgraded
export MEGAVPN_AGENT_SIGNATURE_ENFORCE=false
export MEGAVPN_AGENT_SIGNATURE_WINDOW=5m
```

This is an HTTP-message integrity and replay-window layer, not a replacement for HTTPS. The remaining v1.0 security decision is whether mTLS becomes mandatory in addition to signed HTTP messages.

Generated client artifacts are stored under `MEGAVPN_ARTIFACT_ROOT`, defaulting to `/var/lib/megavpn/artifacts`. The worker creates this tree on first artifact build; the API later serves files from the recorded storage paths for authenticated downloads, public share links and email attachments. Keep this path on persistent storage and include it in backups:

```bash
export MEGAVPN_ARTIFACT_ROOT=/var/lib/megavpn/artifacts
```

Create the secret master key before enabling secret-backed bootstrap, token rotation, platform certificates or service PKI:

```bash
sudo MEGAVPN_MASTER_KEY_PATH=/etc/megavpn/master.key scripts/generate-master-key.sh
```

The key encrypts `secret_refs`. Losing it makes encrypted bootstrap bundles, certificates, private keys and service secrets unrecoverable from the database backup.

## Backup And Restore

Backups are operator-driven and intentionally explicit. A normal backup contains:

- PostgreSQL custom-format dump: `db.dump`
- generated client artifacts from `MEGAVPN_ARTIFACT_ROOT`, when present
- manifest with metadata and master-key checksum, not the master key material

Run:

```bash
sudo MEGAVPN_DATABASE_DSN='postgres://...' \
  MEGAVPN_BACKUP_DIR=/var/backups/megavpn \
  MEGAVPN_ARTIFACT_ROOT=/var/lib/megavpn/artifacts \
  MEGAVPN_MASTER_KEY_PATH=/etc/megavpn/master.key \
  scripts/backup.sh
```

By default, `scripts/backup.sh` does not include `/etc/megavpn` or the master key. Store `/etc/megavpn/master.key` separately in an offline/sealed location. If your operational policy requires a fully self-contained emergency archive, opt in explicitly:

```bash
MEGAVPN_BACKUP_INCLUDE_MASTER_KEY=1 MEGAVPN_BACKUP_INCLUDE_CONFIG=1 scripts/backup.sh
```

Restore is destructive and must be run with API/worker stopped:

```bash
sudo systemctl stop megavpn-api.service megavpn-worker.service
sudo MEGAVPN_RESTORE_CONFIRM=1 \
  MEGAVPN_DATABASE_DSN='postgres://...' \
  MEGAVPN_ARTIFACT_ROOT=/var/lib/megavpn/artifacts \
  scripts/restore.sh /var/backups/megavpn/megavpn-backup-YYYYmmdd-HHMMSS.tar.gz
sudo systemctl start megavpn-api.service megavpn-worker.service
```

If a backup intentionally includes `master.key`, restore it only when building a replacement environment or when the target key is known lost:

```bash
MEGAVPN_RESTORE_MASTER_KEY=1 MEGAVPN_MASTER_KEY_PATH=/etc/megavpn/master.key scripts/restore.sh <backup.tar.gz>
```

OpenVPN instances use a platform-scoped PKI root by default. The first provisioning/apply path creates one active `openvpn/default` CA in `platform_service_pki_roots`, stores CA material in encrypted `secret_refs`, signs per-instance server certificates from that CA, and signs client certificates from the same root. Instance config paths are slug-scoped (`/etc/openvpn/server/<slug>.conf`) to match `openvpn-server@<slug>`. Moving an OpenVPN instance to another ingress node therefore requires reapplying the instance/server config, not replacing every client CA.

The current PKI roots are visible to operators with `instance.read` through the `Platform CA Center` block in Settings and the `GET /api/v1/platform/pki-roots` endpoint. The API response intentionally exposes the public CA certificate secret reference only; the CA private key reference remains server-side.

The control plane also keeps a managed `Certificates` inventory for leaf certificates and internal certificate authorities:

- imported commercial certificates
- self-signed certificates
- managed certificate authorities
- certificates issued from a managed CA

Managed certificates can now be selected directly in the instance editor for `nginx` and `xray` TLS-backed scenarios, and in `Xray + Nginx` service-pack creation. Runtime files are materialized on the node from `certificate_id`; operators no longer need to hand-enter `fullchain.pem` / `privkey.pem` paths for the common path.

`Let's Encrypt / ACME` is exposed in the UI as a paused certificate source. The slot stays visible to operators, but backend issuance is intentionally blocked until the product resumes this track and chooses the canonical challenge strategy.

For operator workflow, the Instances UI now exposes service packs for the canonical production baselines:

- `IPsec + XL2TPD Access`
- `Xray VLESS / Reality`
- `Xray + Nginx gRPC Edge`
- `Xray HTTP/WebSocket Edge`
- `OpenVPN TCP 11994`
- `OpenVPN UDP 1194`

These packs are meant to be the primary entrypoint for standard deployments; the raw single-instance form remains available for advanced/manual cases.

## Operator Access Workflow

The intended MVP flow for real ingress/egress nodes is:

1. Add and enroll `role=ingress` and `role=egress` nodes until both show `online`.
2. In `Instances`, create a service pack on the ingress node, for example `Xray VLESS WebSocket Camouflage`, `OpenVPN TCP 11994` or `OpenVPN UDP 1194`.
3. Apply the created instances so the agent installs/materializes runtime config and host network policy on the node.
4. In `Clients`, create the client account.
5. Press `Provision` on the client and select only the service instances this client may use.
6. Open `Access` for the client, add route policy if the client must leave through a dedicated egress node instead of local breakout.
7. Press `Build configs`, select the already provisioned service accesses and artifact type, then preview or download generated `.ovpn`, VLESS URL, WireGuard config or bundle artifacts.
8. Optionally publish a share link or send the access package by email from the client/artifact actions. Share-link plaintext tokens are shown only once when published; persisted links expose only `token_hint`.

Provisioning is intentionally explicit. The UI no longer silently grants every compatible service instance to a client; operators must choose the exact ingress/service entrypoints per account.

Instance lifecycle operations are exposed from the Instances page:

- `Manage` opens the desired-state editor and revision/apply feedback.
- `Apply`, `Restart`, `Start` and `Stop` queue agent-backed lifecycle jobs.
- `Delete` performs a backend soft-delete and removes the instance from active operational lists.

Deletion is intentionally blocked while active, disabled or pending service accesses still reference the instance. Revoke dependent client/service accesses first, then retry the instance delete.

Revision history failures are intentionally shown to browser operators as a safe generic error. The API writes the real store error to structured backend logs with `instance_id`, `limit`, request path and remote address, so SQL/internal details are available to operators without being exposed in the UI.

## Frontend Structure

The bundled static UI is being split away from the original monolithic `app.js` into focused browser modules:

- `api-client.js`: same-origin API transport, cookie credentials and CSRF header handling.
- `domain-ui.js`: certificate/service-pack/instance form helpers.
- `node-ui.js`: node bootstrap, inventory and diagnostics UI helpers.
- `instances-page.js`: Instances table, lifecycle actions and instance delete flow.
- `revisions-page.js`: compact revision history page, diff summary and rollback flow.
- `services-page.js`: runtime capability matrix plus install/verify job flows.
- `ops-pages.js`: Audit and Telemetry operational views.
- `clients-page.js`: client account table, provisioning, access routes and access email delivery.
- `artifacts-page.js`: artifact export, share-link publish/open/revoke and delivery helper flows.

`app.js` remains the shell/orchestration layer while page modules take ownership of domain-specific rendering and action binding.

## Instance Network Policy

`instance.apply` now carries host network policy to the agent together with rendered service config. The agent writes a persistent systemd oneshot unit:

```text
/usr/local/lib/megavpn/netpolicy/<instance-slug>.sh
/etc/systemd/system/megavpn-netpolicy-<instance-slug>.service
```

The unit is enabled with `systemctl enable --now`, so routing/firewall/sysctl state is re-applied after reboot. The control plane adds safe defaults for common ingress services:

- `firewall_rules` for the instance endpoint port: TCP for Xray/Nginx/HTTP Proxy/MTProto, UDP for WireGuard/IPsec/L2TP, protocol-specific for OpenVPN/Shadowsocks.
- `sysctl.net.ipv4.ip_forward=1` for routed VPN services: OpenVPN, WireGuard, IPsec and XL2TPD.

Operators can extend an instance spec with explicit network policy:

```json
{
  "sysctl": {
    "net.ipv4.ip_forward": "1"
  },
  "firewall_rules": [
    {
      "direction": "input",
      "action": "allow",
      "protocol": "udp",
      "port": 51820,
      "source": "0.0.0.0/0"
    }
  ],
  "routes": [
    {
      "destination": "10.66.0.0/24",
      "dev": "wg0",
      "table": "main"
    }
  ]
}
```

Set `"network_policy_enabled": false` in the instance spec to prevent automatic default firewall/sysctl policy for environments where a separate firewall/orchestrator owns host networking.

## Public Ingress Camouflage

For the first production MVP, use one neutral public hostname for all equivalent ingress nodes, for example `enter.example.com`. Publish multiple `A` records for DNS round-robin:

```text
enter.example.com.  A  <ingress-1-public-ip>
enter.example.com.  A  <ingress-2-public-ip>
```

Each ingress node must expose the same TLS certificate/SNI and equivalent service instances. The recommended first public VLESS profile is the `xray_nginx_http_edge` service pack, now presented as `Xray VLESS WebSocket Camouflage`. It creates:

- Xray backend on `127.0.0.1:7080` with WebSocket path `/assets/rtis-sync`.
- Nginx TLS edge on `443` for `enter.example.com`.
- Nginx `location /assets/rtis-sync` proxying VPN traffic to Xray.
- Nginx `location /` proxying normal browser traffic to a configured fallback website, for example `https://example.com` with matching Host and SNI.

This means a direct browser visit to the public ingress host behaves like the fallback website, while client profiles use only the hidden WebSocket path. Do not run direct Reality and Nginx camouflage on the same `443` listener of one node without an explicit port/SNI plan.

## Client Route Egress Policy

Client access routes are projected into `/etc/megavpn/client-access-routes.json` by the `node.route_policy.apply` job. For IPv4 L3/L4 `allow` routes with an enforceable source identity and explicit remote egress output, the agent also installs an idempotent policy-routing unit using a dedicated route table, `ip rule` and nftables marks for port-specific policies. L7 identities such as VLESS UUID, HTTP proxy usernames and OpenVPN common names remain observe-only until their runtime-specific TUN/proxy enforcement stages exist.

Ingress nodes require an explicit output decision. Without it, the route is marked as blocked for kernel enforcement:

```json
{
  "policy": {
    "egress_mode": "local_breakout"
  }
}
```

Use `local_breakout` when the current ingress node is allowed to be the internet/private-network exit. Use a remote egress node when traffic must leave through a dedicated egress host:

```json
{
  "policy": {
    "egress": {
      "mode": "egress_node",
      "node_id": "<egress-node-id>",
      "next_hop": "10.10.0.2",
      "table": "main"
    }
  }
}
```

Remote egress requires an active `role=egress` node and either `egress_next_hop`/`next_hop` or `egress_interface`/`backhaul_interface`. This makes the ingress-to-egress backhaul explicit and prevents agents from guessing forwarding behavior.

---

# Interactive Control Plane Installer

For a first Control Plane deployment, use the interactive installer:

```bash
cd /opt/megavpn
sudo ./scripts/control-plane-install.sh
```

The installer asks for:

- public domain or IP and public URL
- TLS mode: installer-managed self-signed nginx edge, external HTTPS proxy/LB, or lab-only direct HTTP
- PostgreSQL DSN or database host/port/name/user/password fields
- secret master key path and whether to generate a new 32-byte key
- artifact storage directory and Web UI directory
- bootstrap admin username/email/password
- whether to run Go tests during install

Default-secure behavior:

- Go API binds to `127.0.0.1:8080` unless direct HTTP lab mode is selected.
- `MEGAVPN_AGENT_ALLOW_AUTO_REGISTER=false`.
- `MEGAVPN_AGENT_SIGNATURE_ENFORCE=true` for fresh signed-agent installations.
- `/etc/megavpn/megavpn.env` is written with `0600` permissions and backed up before replacement.
- Existing `/etc/megavpn/master.key` is reused and never overwritten.
- Generated bootstrap admin credentials are stored in `/root/megavpn-control-plane-admin.txt`.
- The installer prepends `/snap/bin` and `/usr/local/go/bin` to `PATH`, so Ubuntu snap Go and official tarball Go are detected even under `sudo`.
- systemd migration oneshot success is checked through unit `Result=success`, not active state, because a successful migration unit normally returns to `inactive (dead)`.

Repeatable non-interactive install is supported through `MEGAVPN_CP_*` variables:

```bash
sudo MEGAVPN_CP_ASSUME_YES=1 \
  MEGAVPN_CP_DOMAIN=control.example.com \
  MEGAVPN_CP_TLS_MODE=self-signed-nginx \
  MEGAVPN_CP_DATABASE_DSN='postgres://megavpn:password@127.0.0.1:5432/megavpn?sslmode=disable' \
  MEGAVPN_CP_ADMIN_USERNAME=superadmin \
  ./scripts/control-plane-install.sh
```

Release hardening material is maintained in:

- `docs/RELEASE_GATES.md`
- `docs/SELF_TESTING.md`
- `docs/THREAT_MODEL.md`
- `docs/RBAC_MATRIX.md`
- `docs/OPERATIONS_RUNBOOK.md`
- `deploy/env/megavpn.production.env.example`
- `deploy/env/megavpn-agent.production.env.example`

Run the local release gate with:

```bash
scripts/release-gate.sh
```

Run the full diagnostic self-test with PASS/FAIL/SKIP reporting:

```bash
scripts/self-test.sh
```

`scripts/release-gate.sh` fails when required release checks are skipped. On a local workstation without PostgreSQL/systemd/nginx/live test-node dependencies, use `scripts/self-test.sh` for diagnostics or set `MEGAVPN_RELEASE_ALLOW_SKIPS=1` explicitly.

`scripts/mvp-control-plane-install.sh` remains available for the older env-driven MVP automation path. `deploy-local.sh` is intended for updating an already installed checkout.

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
MEGAVPN_DEPLOY_ALLOW_HISTORY_REWRITE=0
```

The script refuses to deploy from a dirty worktree by default. Use `MEGAVPN_DEPLOY_ALLOW_DIRTY=1` only for emergency diagnostics on a disposable test host.

Git sync behavior:

- `MEGAVPN_DEPLOY_SYNC_MODE=auto`:
  - fast-forwards when possible
  - if local history diverges from `origin/<branch>`, asks for explicit approval before creating a local backup branch `deploy-backup/<utc>-<sha>` and resetting to the remote branch
  - for non-interactive rollout after an intentional Git history rewrite, set `MEGAVPN_DEPLOY_ALLOW_HISTORY_REWRITE=1`
- `MEGAVPN_DEPLOY_SYNC_MODE=ff-only`:
  - refuses deploy when fast-forward is impossible
- `MEGAVPN_DEPLOY_SYNC_MODE=reset-hard`:
  - always creates a backup branch and hard-resets to `origin/<branch>`

`deploy-local.sh` installs `megavpn-migrate.service` and runs migrations through systemd before starting API/worker. The migration runner resolves migrations from `MEGAVPN_MIGRATIONS_DIR`, `./migrations`, `/opt/megavpn/migrations`, or paths adjacent to the binary. In production set the explicit path:

```bash
MEGAVPN_MIGRATIONS_DIR=/opt/megavpn/migrations
```

`deploy-local.sh` loads `/etc/megavpn/megavpn.env` by default, and the systemd services use the same file.

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
/var/lib/megavpn/artifacts/
```

---

## Logs

```text
journalctl
```

Systemd-first logging model is currently used.
