# RTIS MegaVPN - рабочий baseline для дальнейших задач

**Дата анализа:** 2026-07-08  
**Проект:** RTIS MegaVPN  
**Релизная линия в коде/документации:** `7.1.1.0`  
**Статус документа:** рабочий инженерный контекст для последующих задач, не финальный release audit.

## 1. Назначение

Этот документ фиксирует, как я буду работать с проектом RTIS MegaVPN в следующих задачах:

- сохранять production-grade архитектуру;
- не ломать существующие security boundaries;
- учитывать backend-функционал целиком, а не только изменяемый файл;
- держать изменения обезличенными: без реальных клиентов, доменов, IP, пользователей, секретов, инфраструктурных имен;
- явно отделять подтвержденные факты из кода от предположений и будущих review-зон.

## 2. Ограничение покрытия

Проведен локальный parent-agent обзор репозитория: архитектура, backend route surface, security-critical code paths, system deployment model, документация и локальные проверки.

Это **не** sealed exhaustive multi-agent Codex Security scan:

- `codex-security` preflight вернул `incomplete` по неизвестной multi-agent capacity;
- явного запроса на subagents/parallel agents не было;
- не выполнялись release-evidence проверки, требующие disposable PostgreSQL, systemd/nginx host, live node или service matrix.

Следовательно, этот документ является рабочим baseline. Для релиза или аудита нужен полный release gate и, при необходимости, отдельный exhaustive security scan.

## 3. Использованные источники

Код:

- `cmd/api`, `cmd/worker`, `cmd/agent`, `cmd/migrate`, `cmd/admin`;
- `internal/api/http`, `internal/infra/postgres`, `internal/domain`;
- `internal/auth`, `internal/agentauth`, `internal/secrets`, `internal/pki`;
- `internal/backhaul`, `internal/binaryrepo`, `internal/service/driver`;
- `migrations/*.sql`;
- `deploy/systemd`, `deploy/nginx`, `deploy/env`;
- `scripts/ci`, `scripts/ops`, `scripts/smoke`.

Документация:

- `README.md`;
- `docs/THREAT_MODEL.md`;
- `docs/RBAC_MATRIX.md`;
- `docs/OPERATIONS_RUNBOOK.md`;
- `docs/RELEASE_GATES.md`;
- `docs/BACKHAUL.md`;
- `docs/FIREWALL.md`.

## 4. Проверки, выполненные локально

| Проверка | Результат |
| --- | --- |
| `go test ./...` | PASS |
| `go test -race ./...` | PASS |
| `go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate` | PASS |
| `gofmt -l cmd internal` | clean |
| `go vet ./...` | PASS |
| Static production scan: `/bin/sh -c`, `StrictHostKeyChecking=accept-new` | no matches |
| Static production scan: curl-to-shell/gpg/apt-key patterns | no matches |
| Static production scan: multi-command SQL in Go runtime path | no matches |

Не выполнялось в этой итерации:

- `govulncheck`;
- полный `scripts/ci/release-gate.sh`;
- PostgreSQL migration drill на disposable DB;
- backup/restore drill;
- systemd/nginx verification на целевом Linux host;
- live API/agent/service-pack smoke matrix.

## 5. Обезличивание

В дальнейшей работе использовать только обезличенные значения:

| Тип данных | Допустимо |
| --- | --- |
| Домены | `control.example.invalid`, `edge.example.invalid` |
| IP | RFC 5737/3849 examples: `192.0.2.10`, `198.51.100.10`, `203.0.113.10`, `2001:db8::10` |
| Пользователи | `operator-a`, `admin-a`, `client-a` |
| Ноды | `node-a`, `ingress-a`, `egress-a` |
| UUID | `00000000-0000-0000-0000-000000000001` |
| Секреты | `REDACTED`, `replace-with-secret`, `token-hint-only` |

Запрещено добавлять в код, тесты, документацию или ответы:

- реальные customer names;
- реальные production domain/IP;
- реальные токены, ключи, пароли, приватные ключи;
- локальные абсолютные пути пользователя как часть документации;
- данные, позволяющие восстановить инфраструктуру конкретного окружения.

## 6. Архитектурная модель

```text
Operator
  -> HTTPS edge / Nginx
  -> Control Plane API + Web UI backend
  -> PostgreSQL
  -> Worker queue
  -> Remote node agents
  -> VPN/proxy/backhaul/firewall/runtime services
```

| Компонент | Назначение | Security boundary |
| --- | --- | --- |
| `cmd/api` | HTTP API, Web UI backend, auth/RBAC, public share/subscription endpoints, agent endpoints | Browser/API, public token endpoints, agent signed channel |
| `cmd/worker` | Асинхронные control-plane jobs: bootstrap, client provisioning, artifacts, TLS apply | DB/job queue, SSH bootstrap, artifact/secret material |
| `cmd/agent` | Root-running node agent: inventory, jobs, runtime apply, firewall/backhaul/route policy | Signed control-plane channel, host OS boundary |
| `cmd/migrate` | SQL migration runner | DB schema authority |
| `cmd/admin` | Operational admin helper | Operator workstation/control-plane maintenance |
| PostgreSQL | Source of truth: users, sessions, nodes, jobs, artifacts, secrets metadata | Transactions, constraints, locks, token hashes |
| Nginx | Public TLS edge and WebSocket proxy | TLS termination, public exposure boundary |
| systemd units | Process lifecycle and host hardening | Root privileges bounded by service sandboxing |

## 7. Backend functionality map

### 7.1 Auth, sessions, RBAC

- Local platform users, roles, permissions.
- Session token generation, SHA-256 token hash storage, DB-backed session resolution.
- HttpOnly cookies with `SameSite=Lax`; unsafe cookie-auth requests require `X-MegaVPN-CSRF`.
- Login/invite/change-password/logout flows.
- Admin user/session management.
- Superadmin bypass is explicit in permission evaluation.

Critical files:

- `internal/api/http/auth.go`;
- `internal/auth/password.go`;
- `internal/auth/session.go`;
- `internal/infra/postgres/auth.go`;
- `docs/RBAC_MATRIX.md`.

### 7.2 Operator API surface

Main protected API groups:

- dashboard;
- platform users, invites, sessions;
- mail settings and mail delivery;
- control-plane TLS settings and apply;
- platform certificates and service PKI roots;
- service catalog, service packs, VLESS group templates;
- client access services/groups/routes;
- binary repository;
- nodes, access methods, bootstrap, capabilities, inventory, diagnostics;
- instances, revisions, runtime state, lifecycle actions;
- clients, provisioning, access rotation, artifacts, share links, subscriptions;
- backhaul links and transport promotion/probe/delete;
- firewall catalog, preview/apply/disable;
- traffic accounting;
- jobs and job logs;
- audit log.

All new protected routes must be registered through `protected`, `protectedAll`, or `authenticated` wrappers in `internal/api/http/server.go`.

### 7.3 Agent API surface

Public agent endpoints:

- `POST /agent/register`;
- `POST /agent/heartbeat`;
- `POST /agent/inventory`;
- `GET /agent/runtime/instances`;
- `POST /agent/runtime/instances`;
- `POST /agent/traffic/accounting`;
- `GET /agent/jobs/next`;
- `POST /agent/jobs/{id}/result`;
- `GET /agent/binary-artifacts/{artifact_id}/download`.

Security invariants:

- after enrollment, use per-node agent token, not shared bootstrap token;
- HMAC request signatures bind method, request URI, timestamp, nonce and body hash;
- API rejects replayed nonces per scope;
- API signs agent responses, including empty `204` job polls;
- agent rejects unsigned responses after enrollment;
- job results require current non-expired lease owner.

### 7.4 Jobs and orchestration

Core behavior:

- worker claims non-agent jobs;
- agent claims node/runtime jobs;
- PostgreSQL uses `FOR UPDATE SKIP LOCKED`;
- resource locks prevent unsafe concurrent operations;
- stale running leases recover to `retrying`;
- direct generic job creation is constrained by job type policy;
- privileged jobs must go through typed endpoints.

Never add a privileged job type without updating:

- route permission;
- job schema/payload validation;
- worker/agent whitelist;
- lease/retry behavior;
- audit and redaction;
- tests.

### 7.5 Node lifecycle and bootstrap

Functions:

- node CRUD and maintenance;
- enrollment tokens;
- per-node agent identity and token rotation;
- SSH host-key scan;
- SSH bootstrap;
- manual bootstrap bundle;
- web SSH terminal;
- capability install/verify;
- inventory and service discovery.

Security controls:

- SSH bootstrap requires pinned `ssh_host_key_sha256`;
- SSH user/host are regex/IP parser constrained;
- command execution uses direct argv, `--` target separator and strict host key checking;
- private key material is temp-file mode `0600`;
- bootstrap env values reject control characters;
- manual bundles go through encrypted secret refs.

High-risk review zones:

- web SSH terminal;
- SSH bootstrap;
- password-based SSH through `sshpass`;
- node emergency cleanup;
- capability installation.

### 7.6 Runtime instances and service drivers

Supported runtime areas include:

- OpenVPN;
- WireGuard;
- Xray/VLESS/Reality/WebSocket/gRPC profiles;
- Shadowsocks;
- HTTP Proxy;
- MTProto;
- IPsec/L2TP;
- Nginx edge profiles.

Agent-side apply writes only managed files under service-specific allowlisted roots and selected exact paths. systemd units are allowlisted by service and `Exec*` directives are parsed against exact expected command fields.

Changes to service rendering must preserve:

- managed path allowlists;
- symlink rejection;
- rollback snapshots;
- service validation before apply success;
- job result redaction.

### 7.7 Client provisioning, artifacts, share links, subscriptions

Functions:

- client CRUD and status;
- per-service access provisioning;
- service access rotation/revocation;
- generated client artifacts;
- artifact preview/download;
- public share links;
- VLESS subscription tokens;
- email delivery.

Security controls:

- share/subscription tokens are bearer credentials;
- DB stores token hashes plus hints, not reusable plaintext tokens;
- public share and subscription responses use `Cache-Control: no-store`;
- artifact path resolution validates artifact root and symlink realpath containment;
- previewable artifact types are allowlisted and size-limited.

### 7.8 Binary repository

Functions:

- upload/import runtime artifacts;
- URL import;
- binary download tickets;
- agent-side verified download and install.

Security controls:

- storage path must be relative and root-contained;
- expected SHA-256 is required for URL import;
- remote URL import is HTTPS-only;
- localhost/private/unsafe IPs are rejected during dial;
- redirects are bounded and revalidated;
- agent download requires agent auth, ticket, response signature and body hash;
- install paths are service-specific allowlists;
- zip extraction selects safe members and enforces size limits.

### 7.9 Backhaul and route policy

Functions:

- ingress-to-egress managed links;
- selected active transport plus optional standby transports;
- apply/probe/cleanup;
- route-policy projection for client and VLESS/Xray egress.

Security controls:

- backhaul secrets are generated server-side and stored as secret refs;
- jobs are agent-only;
- cleanup is scoped to managed units/files/interfaces;
- route policy owns reserved nftables chains and `ip rule` priorities only;
- promotion is explicit, no silent failover.

### 7.10 Firewall

Functions:

- firewall address groups;
- policies and ordered rules;
- preview/apply/disable;
- node state tracking.

Security controls:

- catalog changes do not affect nodes until apply;
- strict mode is fail-closed around SSH bootstrap, control-plane egress and forwarding safety;
- DNS entries are catalog metadata only for nft rendering;
- broad SSH-from-any is not generated automatically;
- preview hash gates strict apply.

### 7.11 Secrets and PKI

Functions:

- encrypted secret refs;
- platform certificates;
- managed CAs;
- service PKI roots;
- OpenVPN/WireGuard/Xray and client material generation.

Security controls:

- AES-GCM with 32-byte master key;
- master key stored outside DB backups;
- version metadata exists for key rotation workflows;
- secret-backed operations fail closed when master key is unavailable.

## 8. System security baseline

### Deployment model

Production baseline:

- API listens on loopback, not public interface;
- Nginx terminates public TLS;
- `MEGAVPN_PUBLIC_BASE_URL` is HTTPS and externally reachable by agents;
- PostgreSQL uses a trusted host or managed service, preferably TLS verify mode;
- artifact root is persistent storage;
- master key is file-backed and protected outside backups;
- worker is stopped before API during controlled upgrades.

### systemd hardening

Control-plane units include meaningful hardening:

- `NoNewPrivileges=true`;
- `PrivateTmp=true`;
- `ProtectHome=true`;
- `ProtectSystem=full` for API/worker/migrate;
- `MemoryDenyWriteExecute=true` for API/worker/migrate;
- `RestrictSUIDSGID=true`;
- `SystemCallArchitectures=native`;
- `UMask=0077`.

Residual risk:

- API and worker run as root in current service files.
- Agent must run as root by design to manage host networking, systemd, nftables and service configs.
- Root-running processes require strict typed APIs, allowlists, auditability and host-level isolation.

### Scaling strategy

Current architecture scales primarily through:

- stateless API handlers backed by PostgreSQL;
- DB-backed sessions;
- horizontally scalable workers via job leases and row locking;
- per-node agents polling outbound;
- service runtime distribution across nodes.

Scaling constraints to preserve or address:

- rate limiter is in-memory per API process;
- agent request replay cache is in-memory per API process;
- web terminal session tickets are in-memory per API process;
- artifact root must be shared storage if API/worker are split across hosts;
- PostgreSQL remains the consistency bottleneck for jobs, locks, inventory and audit.

For multi-API deployment, use sticky routing or replace in-memory limiter/replay/ticket state with shared storage.

## 9. Observability and audit model

Required evidence paths:

- structured logs through `slog`;
- audit events for auth, settings, bootstrap, jobs, capabilities, certificates, share links, traffic export;
- job logs with sensitive payload redaction;
- runtime state and observations from agents;
- backhaul health JSON per side;
- firewall node state and preview/apply hashes;
- traffic accounting samples and export audit event.

Any new privileged or externally visible workflow must add:

- audit event;
- job result stage/error details;
- redaction tests for sensitive result fields;
- operator-visible status or reason.

## 10. Backup and recovery model

Baseline:

- backup PostgreSQL and artifact root together;
- do not include master key by default;
- store master-key checksum and sealed copy separately;
- test restore into disposable DB, never production;
- rollback must consider schema compatibility, binaries, web assets and artifacts.

Failure scenarios that future changes must handle:

- API starts before migrations;
- worker crashes after claiming a job;
- agent dies after claim before result;
- node token rotation partially completes;
- binary artifact ticket expires or is replayed;
- artifact file missing while DB row exists;
- strict firewall isolates SSH or control-plane egress;
- backhaul one side applies and the other fails;
- master key missing during secret-backed operation;
- service apply fails after writing files and needs rollback.

## 11. Security invariants for future changes

### HTTP/API

- Use strict JSON decoding with unknown-field rejection.
- Bound request body size.
- Use route-level RBAC.
- Cookie-auth unsafe methods must keep CSRF protection.
- Public bearer-token endpoints must be rate-limited and `no-store`.
- Do not expose token hashes, secret refs with material, private keys, passwords or bootstrap env plaintext in normal responses.

### PostgreSQL

- Use parameterized queries only.
- Use transactions for multi-row state transitions.
- Preserve idempotency for retries.
- Add constraints and indexes with migrations.
- Keep token columns hash-only wherever possible.
- Use row locks/resource locks for concurrent job-sensitive operations.

### Agent and worker jobs

- New privileged job types must be typed, whitelisted and permission-scoped.
- Agent-only jobs must not be executable by worker.
- Worker-only jobs must not be accepted by agent.
- Job completion from agent must require matching lease owner and valid lease.
- Result payloads must be redacted before storage and logs.

### Filesystem

- All operator/agent-provided paths must be canonicalized.
- Reject `..`, NUL, control and whitespace path hazards where applicable.
- Validate realpath containment under allowed roots.
- Reject symlink parents and symlink targets for managed writes.
- Use atomic temp-write and rename where practical.
- Scope cleanup to managed manifests, unit prefixes and allowlisted directories.

### Process execution

- Prefer direct `exec.CommandContext` argv.
- Avoid shell unless a controlled remote SSH script is intentionally built and fully quoted.
- Use timeouts.
- Validate executable names, args, target host/user and paths.
- Preserve `--` target separator for SSH.
- Preserve strict SSH host-key checking and pinned fingerprint validation.

### Remote fetch and supply chain

- Use HTTPS.
- Reject localhost/private/unsafe addresses for server-side URL imports.
- Bound redirects and revalidate each redirect.
- Require SHA-256 pinning for imported artifacts and remote installer scripts.
- Preserve signed agent artifact downloads.

### Secrets

- Store encrypted secret material in `secret_refs`.
- Keep plaintext only in one-time responses or short-lived local variables/files.
- Do not log plaintext secrets.
- Treat bootstrap bundles, private keys, tokens and generated client configs as secret-bearing.

## 12. Current high-risk areas

These areas are not automatically broken, but any task touching them requires focused threat review and tests:

| Area | Why high-risk |
| --- | --- |
| Web SSH terminal | Interactive privileged bridge through API process |
| SSH bootstrap | Remote command execution and secret material transfer |
| Agent root runtime | Writes system files, systemd, nftables, route policy |
| Binary repository install | Supply-chain and root binary replacement path |
| Firewall strict mode | Can isolate nodes or control-plane egress |
| Backhaul/route policy | Kernel routing/nftables correctness and traffic egress control |
| Public share/subscription endpoints | Bearer URL exposure |
| Artifact preview/download | Filesystem containment and sensitive client configs |
| GeoIP lookup | External metadata disclosure and SSRF-like fetch class |
| Multi-API deployment | In-memory rate/replay/terminal state is not shared |

## 13. Working process for next tasks

For every future task I will use this checklist:

1. Identify touched backend surface and trust boundaries.
2. Read the current implementation before designing changes.
3. Preserve existing route permissions, job ownership, redaction and audit semantics.
4. Prefer minimal scoped changes over broad refactors.
5. Add or update tests proportional to risk.
6. Run at least targeted tests for touched packages.
7. For security-sensitive changes, run `go test ./...`, `go test -race ./...`, `go vet ./...` when feasible.
8. Update docs/env/systemd/nginx/scripts when behavior or deployment assumptions change.
9. Explicitly report any verification that could not be run.

## 14. Task-specific review matrix

| If task touches | Required review |
| --- | --- |
| API handler | RBAC, CSRF, decode, request size, audit, tests |
| Auth/session | token storage, cookie flags, login rate limit, session revocation |
| Agent endpoint | HMAC signature, replay, token scope, signed response |
| Job type | permission, schema, queue owner, lease, redaction, recovery |
| SQL/migration | constraints, indexes, transactions, rollback impact |
| File/artifact path | canonicalization, root containment, symlink rejection |
| SSH/process execution | argv construction, timeout, host/user validation, audit |
| Runtime driver | allowed paths, systemd allowlist, rollback, validation |
| Firewall/route/backhaul | management reachability, cleanup, idempotency, node safety |
| Public token endpoint | entropy, hash storage, expiry, revocation, no-store |
| Secret handling | encrypted refs, one-time plaintext, no logs |
| Deployment config | production defaults, systemd hardening, nginx/TLS behavior |

## 15. Engineering posture

Default decision rules:

- security and predictability over shortcut;
- fail closed when secrets, signatures, hash pins or host-key pins are absent;
- keep privileged operations typed and auditable;
- keep generated runtime state reversible;
- make retries idempotent;
- do not introduce global mutable state unless lifecycle and concurrency are explicit;
- do not weaken release gates to make local diagnostics easier;
- do not add demo-only code paths.

This document is the baseline context I will apply when you provide the next task.
