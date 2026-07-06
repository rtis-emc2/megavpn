# Дорожная карта и техническая спецификация RTIS MegaVPN

**Релиз:** `7.0.1.37`

Дата анализа: 2026-07-05
Базовая версия кода: RTIS MegaVPN 7.0.1.37
Базовые документы: Decision Sheet v1, ERD Finalization v1, megavpn_full_spec_v1
Канонический репозиторий: `github.com/rtis-emc2/megavpn`
Английская версия: [`ROADMAP_V1_AND_TZ.md`](ROADMAP_V1_AND_TZ.md)

## 1. Назначение документа

Документ фиксирует фактическое состояние репозитория RTIS MegaVPN и целевой план доведения платформы до production-ready релиза v1.0.

Decision Sheet v1 считается базовым продуктово-архитектурным решением. Текущий код уже реализует существенную часть foundation-слоя, но v1.0 требует закрыть несколько критичных enterprise-разрывов: безопасный agent transport, полноценный desired-state/revision workflow, production UI, тестовую базу, observability, deployment hardening и формальные release gates.

## 2. Executive Summary

RTIS MegaVPN сейчас находится не в состоянии "прототипа", а в состоянии hardening baseline: есть PostgreSQL-backed API, RBAC, session auth, node enrollment, agent pull loop, job queue, locks, node inventory, service discovery, capability install, bootstrap через SSH/manual bundle, static operational UI, secrets storage, client provisioning, artifacts/share links, managed firewall, managed backhaul, VLESS route-policy preview и route-policy apply telemetry.

Главный архитектурный риск: текущая реализация быстрее растет как вертикальная монолитная codebase, чем как формализованная driver/revision/platform архитектура. Для v1.0 нужно стабилизировать контракты и границы:

- Public API contract для UI.
- Internal Agent API contract.
- Driver interface и payload schemas.
- Desired state -> validated revision -> locked job -> agent apply -> runtime state.
- Security baseline: TLS/mTLS или эквивалентная agent identity model, signed jobs, hardened secrets, audit completeness.
- Test and release pipeline.

## 2.1 Release Hardening Scope

`7.0.1.12` является boundary-релизом между feature expansion и production hardening. Цель этой ветки - не добавлять хаотично новые сервисы, а стабилизировать три слоя:

- **Topology model:** node location map, ingress/egress/runtime role visualization, managed links, route status and failed-hop diagnostics.
- **Access model:** service packs, manual instances, client inbound selection, VLESS subscription export and explicit access groups.
- **Runtime model:** pinned runtime artifacts, node capability install/verify/apply, systemd/nginx validation, node cleanup, rollback, managed firewall default-policy enforcement and install-on-clean-host evidence.

Release blockers:

- clean install on a new Ubuntu host from `scripts/control-plane-install.sh` or documented manual steps
- migrations green on disposable PostgreSQL database
- API, worker and agent binaries build from a clean checkout
- `go test -race ./...` green
- OpenVPN, WireGuard, Xray/VLESS, Shadowsocks, Nginx and Backhaul smoke matrix documented with pass/fail evidence
- no unsigned agent job/runtime-target responses accepted by default
- no generic privileged job API for apply, cleanup, capability install or route policy mutation
- security review report with explicit coverage limits

## 2.2 Product Roadmap After 7.0.1.12

| Area | Goal | Implementation Direction | Release Risk |
| --- | --- | --- | --- |
| Node map | Show node location, role, health, public/private addresses and workload density | Add topology API projection and UI map/table hybrid; store optional coordinates/region/provider labels on nodes | Medium: needs stable node metadata schema |
| Node links | Show managed backhaul and route-policy paths between nodes | Reuse Backhaul links plus runtime probe history; render ingress->egress graph with active/degraded/failed edges | Low/medium: data exists, UI and diagnostics need polish |
| VLESS subscription | Export per-client VLESS subscription containing allowed inbound services | Per-client token registry, rotation, revocation, public no-store feed and active-access filtering are implemented; remaining work is QR/text export polish and live E2E evidence | Medium: requires careful token lifecycle and cache headers |
| Traffic camouflage | Formalize public edge profiles for VLESS/WebSocket/gRPC and fallback websites | Move Nginx/Xray camouflage options into reusable templates with validation and preview | High: TLS/SNI/fallback mistakes can break public edge |
| Nginx edge | Make Nginx profile management first-class | Add edge profile catalog, nginx -t preview/apply, cert binding, rollback and config diff | Medium: systemd/nginx apply must remain atomic |
| Runtime artifacts | Reduce manual binary upload friction | Add preset fetchers, SHA-256 calculation, artifact status, signed download URL and install logs | Low: foundation exists |
| Security gate | Make release security evidence repeatable | Keep threat model, RBAC matrix, release gate, self-test and scan artifacts in docs | Medium: exhaustive scan requires delegated workers |

Дополнительный программный контекст на текущую фазу:

- История репозитория перезапущена как clean import в `rtis-emc2/megavpn`.
- Продуктовое имя `MegaVPN` сохраняется как часть бренда `RTIS MegaVPN`.
- Operator-facing branding должен быть переведен с legacy-упоминаний на RTIS без изменения внутренних технических префиксов в один шаг.
- Ребрендинг должен идти после стабилизации CI и публичной документации, чтобы не смешивать инфраструктурные и визуальные изменения в одном неконтролируемом релизе.

## 3. Фактически Реализовано

### 3.1 Backend / Control Plane

Реализовано:

- Go API server: `cmd/api`.
- PostgreSQL store: `internal/infra/postgres`.
- Migration runner: `cmd/migrate`.
- Dashboard counters.
- Health/ready/version endpoints.
- Static web asset serving.
- Structured logging через `slog`.
- Context-aware DB/API paths в основных слоях.

Фактические API domains:

- Auth: login/logout/me/change-password.
- Admin users and sessions.
- Mail settings and invite flow.
- Nodes, diagnostics, access methods, bootstrap runs.
- Enrollment tokens and agent identity rotation/revoke.
- Node inventory, capabilities, capability install/verify events.
- Service discovery and discovery import.
- Services catalog and installer catalog.
- Instances, revisions, spec replacement, lifecycle actions.
- Clients, service accesses, provisioning/revoke/rotate.
- Artifacts and share links.
- Jobs, job logs, cancellation.
- Audit.
- Agent endpoints: register, heartbeat, inventory, runtime targets/reports, next job, job result.

### 3.2 Database / Domain Model

Database changes применяются через ordered migration set для текущего релиза.
Baseline schema хранится в первой migration, а последующие post-baseline
migrations добавляют расширения без ручного изменения database.

Есть таблицы или расширения для:

- `nodes`, `node_agents`.
- `service_definitions`.
- `instances`, `instance_revisions`.
- `client_accounts`, `service_accesses`.
- `artifacts`, `share_links`.
- `jobs`, `job_logs`, `resource_locks`.
- `audit_events`.
- `node_enrollment_tokens`.
- `node_inventory_snapshots`, `node_capabilities`.
- `node_service_discoveries`, `node_service_discovery_events`.
- `node_capability_install_events`.
- `platform_users`, `roles`, `permissions`, `role_permissions`, `platform_user_roles`, `user_sessions`.
- `secret_refs`, `agent_trust_roots`, `node_agent_certificates`.
- `node_access_methods`, `node_bootstrap_runs`.
- `platform_mail_settings`, `platform_user_invites`, `client_email_deliveries`.
- Agent communication diagnostics fields.
- `instance_runtime_states` для нормализованной runtime projection, health и drift статусов instance, включая agent-observed systemd state, config hash, listening ports и report timestamp.
- `instance_runtime_observations` для retained history job-derived и agent-derived runtime snapshots с retention cleanup.
- Typed driver operation interface for OpenVPN, Xray, WireGuard, IPsec, HTTP Proxy, Shadowsocks, MTProto and Nginx lifecycle operations.
- Typed driver health-check definitions for systemd active state, rendered config observation and endpoint listening-port runtime signals.
- Agent runtime execution split into operation-aware dispatch, apply/systemd execution, render/file materialization and validation modules.
- Service-specific agent validation registry for OpenVPN, Xray, WireGuard, IPsec, HTTP Proxy, Shadowsocks, MTProto and Nginx.
- Driver-backed runtime health checks, health reasons and drift reasons are derived for runtime state APIs and the Instances UI without adding a new storage table.
- Client access route-policy payloads classify routes as L3/L4 enforcement candidates or observe-only routes with explicit reasons, project ingress egress/output decisions, and agent route-policy results state that the current apply stage is snapshot-only.
- Interactive Control Plane installer captures public URL/domain, TLS mode, PostgreSQL connectivity, secret master key, artifact storage, bootstrap admin and systemd/nginx setup with repeatable `MEGAVPN_CP_*` overrides, including sudo/snap Go PATH normalization and systemd oneshot migration result handling for Ubuntu deployments.

Частично отсутствует по ERD v1:

- `credentials` / `credential_materials` как отдельный слой.
- `presets`, `strategies`, `strategy_rules`.
- `virtual_endpoints`, `endpoint_backends`.
- `instance_ports`, `instance_networks`, `instance_health_checks`.
- `share_link_events`.
- `telemetry_sources`, `session_snapshots`.
- `backup_snapshots`, `import_runs`, `import_conflicts`.
- `platform_settings` как универсальный settings слой.

### 3.3 Identity, Auth, RBAC

Реализовано:

- Platform users.
- Roles and permissions.
- Session tokens with hash storage.
- Cookie and bearer auth.
- Session revoke.
- Invite flow with one-time accept link.
- Password hashing via PBKDF2-SHA256.
- Audit for auth/admin operations.
- Basic in-process rate limiting for login, invite accept, public share download and agent registration.
- Trusted proxy header mode via `MEGAVPN_TRUST_PROXY_HEADERS`.
- API request body size limit via `MEGAVPN_API_MAX_REQUEST_BYTES`.
- Strict JSON decoding for API request bodies.
- CSRF guard for cookie-authenticated mutating API requests.
- Bundled Web UI uses HttpOnly session cookie + CSRF header and clears legacy bearer tokens from `localStorage`.
- Shared `MEGAVPN_AGENT_TOKEN` fallback for node/job agent endpoints is limited to explicit auto-register mode.
- Agent-to-server runtime requests and server-to-agent job/runtime-target responses are HMAC-signed with per-node persistent tokens and verified with timestamp/body-hash/nonce replay-window checks; strict request enforcement is controlled by `MEGAVPN_AGENT_SIGNATURE_ENFORCE`.
- Failed operator login attempts are recorded as audit events.
- Platform-scoped OpenVPN CA root model via `platform_service_pki_roots`; OpenVPN instances get server certificates from the shared `openvpn/default` CA by default.
- OpenVPN config paths are slug-scoped to match `openvpn-server@<slug>` systemd units.
- Settings UI exposes a `Platform CA Center`, and `GET /api/v1/platform/pki-roots` exposes platform PKI root inventory without exposing CA private key secret references.
- `ACME / Let's Encrypt` intentionally remains paused at this stage; the UI keeps the operator-facing slot visible, but automated issuance is not part of the active delivery scope until a canonical challenge strategy is approved.

Текущие ограничения:

- Password policy усилена до 12 символов, но для v1.0 нужен enterprise policy profile.
- Нет MFA/2FA.
- Нет account lockout.
- Нет external proxy/WAF-level rate limiting profile.
- Нет CIDR-scoped trusted proxy allowlist; текущий режим доверия proxy headers включается целиком через env и должен использоваться только за доверенным reverse proxy.
- CSRF защита есть для cookie-auth mutating API, но v1.0 еще требует финальный frontend/API auth mode decision: bearer-only, CSRF token rotation или hybrid.

### 3.4 Secret Management

Реализовано:

- `secret_refs`.
- AES-GCM encryption service.
- External master key file через `MEGAVPN_MASTER_KEY_PATH`.
- Secret-backed provisioning для чувствительных материалов.
- UI/API создание secret refs для bootstrap.

Ограничения:

- Это single KEK file model, не envelope encryption с key hierarchy.
- Нет rotation workflow для KEK.
- Нет Vault/KMS abstraction.
- Нет per-secret access audit на каждое раскрытие.
- Если master key path не настроен, часть secret-backed provisioning отключается.

### 3.5 Node Enrollment / Bootstrap / Agent

Реализовано:

- One-time enrollment tokens.
- Server-side token hashing and token hints.
- Persistent agent identity/token.
- Agent state file.
- Agent bootstrap env cleanup после enroll.
- Agent heartbeat.
- Pull-based job polling.
- Job result submission.
- Agent communication diagnostics.
- Agent token rotation and identity revoke.
- Node access methods.
- SSH bootstrap job.
- Manual bootstrap bundle generation.
- Inventory sync and service discovery sync.

Ограничения:

- Transport сейчас REST over configured URL; mTLS из Decision Sheet еще не завершен как обязательный runtime contract.
- Mandatory mTLS is not completed yet; current HMAC layer protects agent-to-server requests and server-to-agent job/runtime-target responses.
- Agent API authentication основана на bearer-like token model, а не полноценной mTLS identity per node.
- Agent выполняет allowlist job types, что правильно, но payload schema validation нужно формализовать.

### 3.6 Jobs / Locks / Execution

Реализовано:

- PostgreSQL-backed jobs.
- Worker job claim.
- Agent job claim for node-scoped jobs.
- Job logs.
- Resource locks для mutating jobs.
- Locked by / locked until.
- Capability install/verify jobs.
- Node bootstrap jobs.
- Instance lifecycle jobs.
- Client provisioning jobs.
- Remediation actions for stuck jobs/channel probe/stale token rotation.

Ограничения:

- Нет retry policy на уровне job definition.
- Нет dead-letter queue.
- Нет эксплицитной idempotency key model.
- Нет typed job payload schemas.
- Нет full cancellation propagation до agent runtime process.
- `artifact.build` объявлен, но worker явно возвращает not implemented.

### 3.7 Service Catalog and Capabilities

Реализовано:

- Service catalog seeded.
- Installer catalog API.
- Capability install/verify через agent.
- Поддержанные install/verify capabilities:
  - nginx.
  - xray-core.
  - openvpn.
  - wireguard.
  - ipsec / strongSwan.
  - http proxy / squid.
  - xl2tpd.
  - shadowsocks-libev.
- Источники:
  - nginx.org repo или Ubuntu repo.
  - XTLS/Xray-install flow.
  - Ubuntu repo для OpenVPN/IPsec/xl2tpd/Shadowsocks.
- Node capability matrix в UI.

Ограничения:

- WireGuard install/provision/apply path реализован, но ему еще нужны integration tests, runtime hardening и production smoke.
- HTTP Proxy install/verify реализован, но полноценный instance/provision/apply path еще не реализован.
- MTProto пока profile/overlay/future.
- Capability drift сейчас минимальный и hardcoded под nginx/xray.

### 3.8 Instances / Revisions / Desired State

Реализовано:

- CRUD для instances.
- Instance revisions.
- Spec replacement.
- Revision list.
- Rollback flow from revision history.
- Lifecycle actions: apply/restart/start/stop/enable/disable.
- Instance soft delete with service-access guard.
- Agent-side materialization of managed config files.
- Validation:
  - `nginx -t`.
  - `xray run -test` / `xray -test`.
- Default systemd units and config paths.
- Renderers для Xray, Nginx, OpenVPN, IPsec, xl2tpd, Shadowsocks на store/worker side.
- Kernel-level route-policy enforcement через managed nftables marks и policy
  routing: `node.route_policy.apply` materializes signed policy snapshot,
  маркирует client/system egress flows и выбирает managed backhaul route table
  через `ip rule fwmark`.

Ограничения:

- Revision workflow пока не доведен до strict candidate -> validated -> applied model.
- Нет обязательного diff before apply.
- Нет persisted `instance_health_checks`/`instance_ports` таблиц; текущий health/drift detail слой вычисляется из `instance_runtime_states`, observation history и driver contract.
- Нет port/subnet conflict engine.
- Нет full schema validation для raw/structured configs.
- Нет session listing.

### 3.9 Client Provisioning / Artifacts / Share Links

Реализовано:

- Client accounts.
- Service accesses.
- Client provisioning job.
- Provisioning/access rotation для OpenVPN, Xray, IPsec, Shadowsocks.
- Artifact generation:
  - OpenVPN `.ovpn`.
  - Xray URI/text artifacts.
  - Shadowsocks artifacts.
  - IPsec/L2TP bundle artifacts.
  - ZIP bundle.
- Artifact storage in local filesystem.
- Share links with token and TTL.
- Public share download endpoint.
- Client email delivery with attachments/share links.

Ограничения:

- Отдельные credential entities из ERD отсутствуют.
- Share link events/download audit не реализованы как отдельная таблица.
- Нет max downloads policy.
- Нет object storage abstraction.
- Нет QR generation.
- Нет client self-service portal.

### 3.10 Web UI

Реализовано:

- Static HTML/CSS/JavaScript UI.
- Operational views:
  - Dashboard.
  - Nodes.
  - Instances.
  - Clients.
  - Jobs.
  - Artifacts.
  - Share links.
  - Services.
  - Revisions.
  - Audit.
  - Settings/Auth/Mail.
- Forms/modals for core operations.
- Live calls to API.
- `app.js` gradually split into focused static modules:
  - API client.
  - Domain UI helpers.
  - Node UI helpers.
  - Instances page.
  - Revisions page.
  - Services page.
  - Audit/Telemetry pages.
  - Clients page.
  - Artifacts/Share links page.
- Compact Instances/Revisions operational pages with safe action flows.

Расхождение с Decision Sheet:

- Decision Sheet требует React + TypeScript + TanStack Query/Router + AG Grid + MUI + Zustand + Monaco + zod.
- Текущий UI является production-useful admin UI, но не целевым frontend stack для v1.0.

### 3.11 Deployment / Infrastructure

Реализовано:

- Build scripts.
- Local run script.
- Install scripts.
- systemd units for API, agent, worker.
- nginx reverse proxy example.
- FHS layout documented:
  - `/opt/megavpn`.
  - `/etc/megavpn`.
  - `/var/lib/megavpn`.
- Smoke scripts for enrollment, inventory, discovery, capabilities, failures.

Ограничения:

- systemd services run as root. Agent может требовать root для service management, но API/worker должны быть hardenable under dedicated user.
- nginx example HTTP-only; TLS production profile не готов.
- Нет Docker/Kubernetes packaging.
- Нет backup/restore scripts.
- Нет migration rollback policy.

### 3.12 Verification Status

В текущей среде `go` недоступен, поэтому `go test`, `go vet` и `go build` не были выполнены. В репозитории отсутствуют `*_test.go`.

Это означает:

- Кодовая база имеет smoke scripts, но не имеет достаточного automated regression coverage.
- Для v1.0 testing foundation является release blocker.

## 4. Gap Analysis Against Decision Sheet v1

| Area | Decision Sheet Target | Current State | v1.0 Gap |
|---|---|---|---|
| Control plane / execution separation | Required | Mostly implemented | Formalize contracts and payload schemas |
| PostgreSQL source of truth | Required | Implemented | Add missing v1 entities |
| Desired state -> revision -> job -> apply | Required | Partially implemented | Strict revision state machine, validation, rollback |
| No direct shell from API | Required | API queues jobs | Keep; audit all mutating endpoints |
| Agent pull model | Required | Implemented | Harden auth, mTLS/signatures |
| mTLS identity per node | Required | DB tables exist, runtime incomplete | Implement or explicitly defer with accepted risk |
| Job signature validation | Required | HMAC-signed job responses implemented | Decide whether mTLS is mandatory before v1.0 |
| Service drivers | Required | Practical render/apply exists | Extract typed driver contracts |
| OpenVPN | Tier A | Partial provisioning/apply | Complete lifecycle, revoke, health, tests |
| Xray | Tier A | Partial provisioning/apply | Complete Reality/XHTTP/fallback support and validation |
| Nginx | Tier A | Install/render/apply partial | Complete managed sites/stream/fallback model |
| IPsec | Tier A | IPsec/xl2tpd partial | Define supported subset, harden configs |
| WireGuard | Tier B | Not implemented | Decide v1.0 inclusion or v1.1 deferral |
| Shadowsocks | Tier B | Partial | Complete lifecycle or mark operational support |
| Artifacts | Required | Implemented local | Add events, QR, policy |
| RBAC | Required | Implemented foundation | Permission review, sensitive action gates |
| Secrets | Encrypted PG + external KEK | Implemented partially | KEK rotation, reveal audit, policy |
| UI stack | React/TS/MUI/etc. | Static JS UI | Rebuild or accept scope change |
| Observability | Node/instance/jobs/errors/sessions/audit | Partial | Metrics/logs/runtime/session snapshots |
| Retention | Required | Not enforced | Cleanup jobs and policy settings |

## 5. Целевое ТЗ v1.0

### 5.1 Назначение системы

RTIS MegaVPN Platform v1.0 является self-hosted distributed control plane для управления VPN/proxy/edge-инфраструктурой через web UI и agent-managed nodes.

Система должна:

- Централизованно управлять nodes, service capabilities, instances, clients, credentials, artifacts and jobs.
- Работать по модели desired state.
- Выполнять runtime изменения только через jobs.
- Обеспечивать audit trail для security-sensitive операций.
- Поддерживать Ubuntu 24.04 LTS amd64 как primary target и arm64 как secondary target.
- Быть пригодной для enterprise deployment 24/7.

### 5.2 Пользовательские роли

Роли v1.0:

- `superadmin`: полный доступ, нельзя удалить последнего superadmin.
- `admin`: полный operational доступ без нарушения superadmin invariants.
- `engineer`: nodes/services/clients/jobs/artifacts без platform/security settings.
- `readonly`: read-only доступ.

Sensitive actions должны требовать отдельные permissions:

- Secret reveal/resolve.
- Artifact export.
- Share link publish/revoke.
- Config edit.
- Instance apply/restart/stop.
- Destructive delete/revoke.
- Node bootstrap/agent identity rotation.

### 5.3 Functional Requirements

#### Platform and Auth

- Bootstrap первого superadmin из env только если нет пользователей.
- Login/logout/session management.
- Invite operator flow.
- Password policy:
  - минимум 12 символов для production profile;
  - запрет common passwords;
  - audit password changes/resets.
- Session policy:
  - configurable TTL;
  - revoke own session/all user sessions;
  - secure cookie production mode.
- Optional v1.0 hardening: TOTP MFA for admin/superadmin.

#### Nodes

- Create node in draft state.
- Configure access methods.
- Generate/rotate enrollment token.
- SSH bootstrap.
- Manual bundle bootstrap.
- Agent enrollment.
- Agent heartbeat.
- Agent diagnostics.
- Agent token rotation.
- Agent identity revoke.
- Maintenance mode.
- Retire node only when no active instances remain.
- Inventory sync and service discovery.
- Import discovered services as managed/unmanaged instances.

#### Services and Capabilities

- Service catalog must expose supported service definitions with tier, capability and account/artifact support flags.
- Agent must support install/verify jobs for v1.0 supported runtimes.
- Capability status must be derived from inventory and explicit verification.
- Capability drift must compare required runtime capabilities for active instances.

v1.0 service support:

- Tier A release-critical:
  - OpenVPN.
  - Xray-core.
  - Nginx.
  - IPsec/L2TP scoped to explicitly documented modes.
- Tier B operational:
  - Shadowsocks.
- Explicit decision required:
  - WireGuard in v1.0 or v1.1.

#### Instances

- Create instance only on node with required capability or explicit override.
- Maintain structured spec and rendered files.
- Create revision for each spec change.
- Validate revision before apply.
- Show diff before apply in UI.
- Enqueue one mutating job per instance at a time.
- Apply/restart/start/stop/enable/disable through agent.
- Store apply result, active state, error summary and last applied revision.
- Rollback to previous applied revision.

#### Client Provisioning

- Create/suspend/activate/delete client account.
- Assign client to selected instances or strategy-selected instances.
- Create service access records.
- Generate credentials and artifacts.
- Revoke access.
- Rotate access per service.
- Send artifacts by email.
- Publish/revoke share links.
- Audit all provisioning, revoke, rotate, export, download operations.

#### Artifacts

- Store artifacts locally in v1.0.
- Support:
  - `.ovpn`.
  - Xray URI/text.
  - Shadowsocks URI/text.
  - IPsec/L2TP profile bundle.
  - ZIP bundle.
  - QR where protocol supports URI.
- Enforce TTL and status.
- Record download/share events.
- Support cleanup of expired/revoked artifacts and links.

#### Jobs

- Job states:
  - queued.
  - running.
  - succeeded.
  - failed.
  - canceled.
  - expired.
- Job lease and lock semantics.
- Retry policy per job type.
- Dead-letter visibility for repeated failures.
- Typed payload schemas.
- Idempotency keys for externally triggered mutating operations.
- Job logs with structured payload.

#### Audit

Audit must include:

- actor type and id;
- action;
- resource type/id;
- IP/user-agent for UI operations where available;
- success/failure;
- sensitive metadata without leaking secrets;
- timestamp.

Audit retention target: at least 1 year.

### 5.4 Non-Functional Requirements

Reliability:

- API graceful shutdown.
- Worker graceful shutdown.
- Agent cancellation-aware loop.
- DB transaction boundaries for every state mutation.
- Idempotent bootstrap/enrollment/provisioning operations where feasible.

Security:

- HTTPS-only production deployment.
- Agent identity per node.
- mTLS or signed job/result payloads as mandatory control.
- No secret in logs.
- Secrets encrypted at rest.
- RBAC for every protected API route.
- Rate limiting for auth and public share endpoints.
- Strict input validation.
- Secure headers.

Performance targets:

- API p95 < 300 ms for common list/detail endpoints at target scale.
- Job enqueue < 1 s.
- Provisioning < 30-90 s excluding package installation.
- Heartbeat interval 15-30 s.
- Offline detection < 2 min.

Capacity targets:

- Nodes: 20.
- Instances: 200.
- Clients: 10,000.
- Concurrent jobs: 50.
- Operators: 100.

Observability:

- Dashboard for node/instance/client/job health.
- Metrics endpoint or external collector integration.
- Structured logs.
- Job and agent diagnostics.
- Session snapshots for supported services where possible.
- Backup and restore status.

Deployment:

- Production systemd deployment.
- Hardened service users for API/worker.
- Agent root privileges minimized and justified.
- Nginx TLS reverse proxy profile.
- DB migration documented.
- Backup/restore documented.

## 6. Roadmap to v1.0

### Phase 0 - Stabilization Baseline

Goal: make the current state reproducible and measurable.

Deliverables:

- Install Go toolchain in CI/dev environment and verify `go test`, `go vet`, `go build`.
- Add CI pipeline for build/vet/test.
- Replace outdated `docs/NEXT_STEPS.md` with current roadmap reference or mark as historical.
- Add API route inventory document.
- Add migration inventory document.
- Add local development quickstart.
- Define release versioning policy.

Exit criteria:

- Clean build for API/agent/worker/migrate.
- CI runs on every PR.
- Smoke scripts are documented and runnable.

### Phase 1 - Architecture Contracts

Goal: stop implicit behavior from spreading.

Deliverables:

- Define driver interface:
  - ValidateSpec.
  - Render.
  - ApplyPayload.
  - Provision.
  - Revoke.
  - Rotate.
  - ExportArtifacts.
  - Health.
- Define typed job payload schemas.
- Define internal agent API schemas.
- Define public frontend API contract, preferably OpenAPI.
- Define revision state machine.
- Define error taxonomy and API error format.

Exit criteria:

- New service driver cannot be added without schema and tests.
- Mutating endpoints use typed request validation.

### Phase 2 - Security Hardening

Goal: close v1.0 audit/security blockers.

Deliverables:

- Enforce HTTPS production settings.
- Decide whether mTLS is mandatory on top of signed HTTP messages.
- Sign jobs and verify job signatures on agent is implemented for job/runtime-target responses; continue hardening with key rotation and release gates.
- Sign/verify agent results.
- Extend rate limiting beyond the current in-process baseline:
  - login.
  - invite accept.
  - public share download.
  - agent register.
- Add CIDR-scoped trusted proxy config for client IP.
- Expand password policy into production profile.
- Finalize CSRF/auth mode strategy beyond the current cookie-auth mutating request guard.
- Add secret access audit.
- Add KEK rotation plan or documented v1 limitation.
- Harden systemd units for API/worker.

Exit criteria:

- Security review finds no blocker for exposing UI behind TLS.
- Agent identity is revocable and cryptographically bound to node.

### Phase 3 - Database Model Completion

Goal: align schema with ERD v1 where required for v1.0.

Deliverables:

- Add `credentials` and `credential_materials` or explicitly document metadata-based credential storage as v1.0 deviation.
- Add `share_link_events`.
- Runtime observation history and retention for agent-observed `instance_runtime_states` is implemented; driver-backed health/drift details are API-derived.
- Add persisted `instance_health_checks` only if v1.0 audit queries require check-level retention beyond `instance_runtime_observations`.
- Add `platform_settings`.
- Add `presets` and minimal `strategies` if strategy-based provisioning is v1.0 scope.
- Add `virtual_endpoints` and `endpoint_backends` if endpoint abstraction is v1.0 scope.
- Add retention cleanup jobs for audit/jobs/share links/artifacts.

Exit criteria:

- ERD v1 deviations are intentional and documented.
- Runtime state no longer lives only in job result payloads.

### Phase 4 - Desired State, Revisions and Apply

Goal: make config changes safe, reviewable and reversible.

Deliverables:

- Candidate revision creation.
- Schema validation.
- Render validation.
- Diff preview.
- Approve/apply.
- Mark applied revision.
- Rollback to previous applied revision.
- Prevent apply of invalid/unvalidated revision.
- Port/subnet conflict checks.
- Per-instance lock enforcement tests.

Exit criteria:

- Every instance mutation produces auditable revision and job.
- Operator can see what will change before apply.
- Failed apply preserves enough state to recover.

### Phase 5 - Service Driver Completion

Goal: deliver stable v1.0 service support.

Deliverables by service:

OpenVPN:

- Structured instance spec.
- PKI model: platform shared CA default via `platform_service_pki_roots`; optional explicit per-instance CA remains as compatibility escape hatch.
- Server config render.
- Server cert/key generation from the active OpenVPN PKI root.
- Client cert issuance with CA tracking in service access metadata.
- Revoke flow and CRL update.
- Embedded `.ovpn` export.
- Health check.
- Apply/rollback tests.

Xray:

- Structured VLESS Reality.
- Structured VLESS XHTTP.
- Multi-SNI/fallback via Nginx.
- Raw JSON advanced mode with validation.
- Client UUID provisioning/rotation/revoke.
- URI and QR export.
- Health check.

Nginx:

- Managed reverse proxy configs.
- TLS termination profile.
- Xray fallback profile.
- Stream proxy where required.
- Config validation.
- Reload without downtime where possible.

IPsec/L2TP:

- Explicitly define supported v1 modes.
- strongSwan config render.
- xl2tpd config render.
- PSK/cert/EAP scope decision.
- Client bundle export.
- Revoke/rotate.

Shadowsocks:

- Standalone mode.
- Xray inbound mode decision.
- URI export.
- Revoke/rotate.

WireGuard:

- Product decision required:
  - include in v1.0 with peer provisioning and config export;
  - or move to v1.1 and remove from v1.0 acceptance criteria.

Exit criteria:

- Tier A services pass install, create instance, provision client, apply, rotate, revoke, artifact export and rollback scenarios.

### Phase 6 - Frontend v1.0

Goal: replace or formalize current static UI as production UI.

Decision required:

- Option A: rebuild with Decision Sheet stack: React + TypeScript + TanStack Query/Router + AG Grid + MUI + Zustand + Monaco + react-hook-form + zod.
- Option B: keep static UI for v1.0 and update Decision Sheet formally.

Recommended path: Option A for maintainability.

Deliverables:

- Typed API client.
- Auth/session shell.
- Dashboard.
- Nodes workflow.
- Services/capability matrix.
- Instance editor with schema forms and raw Monaco editor.
- Revision diff/apply/rollback view.
- Clients/provisioning/artifacts/share links.
- Jobs/logs/detail view.
- Audit filters.
- Settings/admin/mail.
- Accessibility and responsive checks.

Exit criteria:

- UI has no stub/future screens for v1.0 scope.
- All sensitive operations require explicit confirmation.
- Validation errors are field-level and actionable.

### Phase 7 - Observability and Operations

Goal: make the platform supportable in production.

Deliverables:

- Metrics endpoint or collector integration.
- Structured log conventions.
- Node communication health model.
- Instance runtime state model.
- Service session snapshots where supported.
- Retention cleanup worker.
- Backup script and restore runbook.
- Operational runbooks:
  - bootstrap failure.
  - agent offline.
  - stuck job.
  - failed apply.
  - expired share link.
  - lost master key.

Exit criteria:

- Operator can diagnose common failures from UI and logs.
- Backup/restore tested on clean environment.

### Phase 8 - Testing and Release Hardening

Goal: make v1.0 releasable without manual heroics.

Deliverables:

- Unit tests for:
  - auth/password/session.
  - secrets.
  - job locking.
  - revision state machine.
  - driver renderers.
  - artifact generation.
- Integration tests with PostgreSQL.
- Agent contract tests.
- End-to-end smoke tests.
- Race detector job for selected packages.
- Static analysis: `go vet`, `gofmt`, optional `staticcheck`.
- Security scan for dependencies and code.
- Release checklist.
- Upgrade test from current migrations to latest.

Exit criteria:

- CI green.
- Critical path E2E green.
- No release-blocking security findings.

## 7. v1.0 Release Criteria

v1.0 can be released only when:

- API, worker, agent and migrate build reproducibly.
- PostgreSQL migrations apply cleanly on empty DB and existing upgrade DB.
- Auth/RBAC protects every non-public route.
- Agent communication is cryptographically hardened.
- OpenVPN, Xray, Nginx and scoped IPsec/L2TP workflows are complete.
- Client provisioning produces valid artifacts and applies runtime state.
- Revoke/rotate flows work for supported services.
- Instance revision apply/rollback is implemented.
- Jobs are locked, observable and recoverable.
- Audit covers sensitive operations.
- UI has no unfinished v1.0 operational screens.
- Backup/restore documented and tested.
- Smoke and integration tests pass.
- Production deployment guide exists.

## 8. Recommended Milestones

| Phase | Focus | Result |
|---|---|---|
| Current release baseline | CI, contracts, current-state docs | Reproducible engineering baseline |
| Security foundation | Security transport, job schemas, secret audit | Hardened control plane and agent channel |
| Safe config lifecycle | Revision/apply/rollback | Auditable desired-state changes |
| Feature-complete backend | Tier A drivers complete | Complete service runtime baseline |
| Release candidate platform | Frontend v1, observability, retention | Operationally complete UI and telemetry |
| Production candidate | Testing, migration, deployment hardening | Final production validation |
| Stable v1 | Production release | Stable v1 platform |

## 9. Main Risks

| Risk | Impact | Mitigation |
|---|---|---|
| Mandatory mTLS decision delayed | Security blocker | Decide before v1.0 release gate |
| UI stack mismatch | Long-term maintainability risk | Decide now: rebuild or amend Decision Sheet |
| No automated tests | Regression risk | Phase 0/8 must be non-negotiable |
| Service drivers remain implicit | Hard to maintain | Extract typed contracts and schemas |
| Secrets model too simple | Audit/key-rotation risk | Define KEK rotation/Vault-compatible future path |
| IPsec scope expands | Schedule risk | Freeze v1 supported modes |
| WireGuard ambiguity | Scope creep | Explicit v1/v1.1 decision |
| Root services | Security audit issue | Harden API/worker users, constrain agent |

## 10. Open Questions

1. WireGuard должен входить в v1.0 или переносим в v1.1?
2. Frontend обязательно переводим на React/TypeScript stack из Decision Sheet или официально меняем Decision Sheet и оставляем static UI на v1.0?
3. Agent security для v1.0: строго mTLS или допускается signed jobs/results поверх HTTPS с per-node key material?
4. IPsec v1.0 scope: только L2TP+PSK или также cert/IKEv2 EAP?
5. Нужен ли self-service portal для клиентов в v1.0 или это post-v1?
6. Нужна ли DNS provider automation в v1.0 или достаточно manual virtual endpoint model?
7. Где должен жить production artifact storage: local filesystem only или сразу закладываем S3-compatible abstraction?
8. Требуется ли MFA для admin/superadmin в v1.0?

## 11. Release 7.0.1.12 Closure

Цель релиза `7.0.1.12`: усилить traffic-camouflage ingress path, чтобы
fallback website не мог случайно указывать обратно на публичный ingress
endpoint и создавать reverse-proxy loop.

Зафиксировано в этом релизе:

- Service-pack API отклоняет `fallback_upstream_url`, `Fallback Host header`
  и `Fallback SNI`, если они совпадают с `endpoint_host` camouflage pack.
- Nginx renderer применяет такой же loop guard для managed
  `ws_camouflage_edge`/`grpc_edge` profiles и explicit
  `traffic_camouflage`/`fallback_loop_guard` specs.
- Web UI показывает оператору правило "fallback website должен быть отдельным
  сайтом" и останавливает очевидный loop-case до API request.
- `scripts/service-pack-smoke.sh` получил preflight guard, чтобы matrix smoke
  не запускал camouflage pack с fallback на тот же endpoint.
- Добавлены regression tests для API normalization и Nginx renderer.
- English/Russian user guides и release gates описывают правило: fallback
  URL/Host/SNI не должны указывать на public ingress host.
- Обновлены release version, web asset cache-busting и release/security review
  artifact `SECURITY_REVIEW_7.0.1.12.md`.

Что сознательно остается после текущего release baseline:

- Следующим functional инкрементом нужно продолжить traffic-camouflage ingress
  case: config preview/diff, `nginx -t` evidence surface и live fallback-site
  smoke.
- Прогнать service-pack smoke matrix на реальной disposable node с реальным
  certificate id и `MEGAVPN_FALLBACK_UPSTREAM_URL`.
- Добавить Nginx config preview/config diff перед apply.
- Добавить live smoke для fallback website и generated VLESS subscription.
- Довести Nginx edge profile catalog до reusable profile definitions и
  operator-visible failed-apply evidence.

## 12. Release 7.0.1.13 Closure

Цель релиза `7.0.1.13`: hardening типографики и layout во всех основных
разделах operator console без изменения backend/API/agent behavior.

Зафиксировано в этом релизе:

- Для видимого UI-текста введен единый UI font stack.
- Monospace сохранен только для `code`, code blocks, config textarea и web
  terminal output.
- Старые локальные `letter-spacing` overrides нормализованы в `0`.
- Для buttons, tabs, tags, status pills, cards, modals и table cells добавлены
  явные правила `min-width: 0`, wrapping и overflow behavior.
- Мобильные page tabs теперь рендерятся responsive grid-сеткой, а не offscreen
  горизонтальной лентой.
- Desktop/mobile browser smoke проверил Dashboard, Nodes, Instances, Firewall,
  Backhaul, Clients, Jobs, Services и Settings.

В релизе не менялись database migrations, API contract, agent behavior,
runtime apply behavior, VLESS routing, firewall enforcement и
traffic-camouflage rendering.

## 13. Release 7.0.1.14 Closure

Цель релиза `7.0.1.14`: security и release hardening перед следующим
инкрементом VPN-функциональности.

Зафиксировано в этом релизе:

- Go release baseline теперь требует patch-level `1.26.4`.
- CI и release gate запускают `govulncheck@v1.5.0`.
- Control-plane installer сравнивает полный Go semver, включая patch version.
- NGINX.org repository bootstrap проверяет fingerprint signing key перед
  импортом trust material на ноде.
- Bootstrap env rendering отклоняет invalid keys и control characters.
- Node name/address validation отклоняет control characters на HTTP и store
  boundaries.
- Generic job creation ограничен explicit allowlist, а новые jobs всегда
  стартуют как `queued`.
- Старые early-stage installer/smoke naming артефакты удалены из активного
  release path.

В релизе не менялись database migrations и VPN runtime behavior.

## 14. Release 7.0.1.21 Closure

Цель релиза `7.0.1.21`: закрыть последний видимый UI/API regression в
operator console.

Зафиксировано в этом релизе:

- Firewall default catalog seed больше не отправляет несколько SQL-команд через
  один prepared statement.
- Address-pool default seed использует один multi-row statement вместо
  нескольких SQL-команд в одном runtime query.
- Service-pack catalog reads дедуплицируются по `key` и предпочитают active,
  custom и более новые строки.
- Service-pack default seed чинит исторические duplicate rows и добавляет unique
  key index, если он отсутствует в старой БД.
- Web core loading дедуплицирует service packs по `key`, поэтому Create from
  pack не рендерит повторяющиеся templates.
- Release gate static scan теперь блокирует multi-command SQL в production Go
  runtime paths.
- Firewall catalog schema drift исправлен через
  `000009_firewall_schema_repair`: это покрывает существующие инсталляции, где
  consolidated baseline уже был отмечен примененным до появления firewall
  tables.
- Dialogs для Firewall address lists и policies больше не показывают internal
  identity controls в ручном operator workflow.
- Текст address lists теперь описывает reusable source/destination address
  groups без vendor-specific терминологии.
- Service-pack catalog list paths теперь дедуплицируют semantic clones, поэтому
  Create from pack показывает один effective active template даже при разных
  historical keys.
- `000010_service_pack_semantic_dedup` архивирует duplicate default
  service-pack rows, оставшиеся на старых инсталляциях.
- Nginx capability install теперь делает fallback с nginx.org repository install
  на Ubuntu distro package, если nginx.org apt path падает до runtime
  verification. Это не блокирует rollout VLESS WebSocket camouflage edge из-за
  stale или недоступного nginx.org repository metadata.
- Ubuntu Nginx fallback теперь проверяет distro package candidate до purge
  существующего Nginx package и может продолжить с local apt metadata, если
  `apt-get update` деградировал, но package candidate доступен.

В релизе не менялся VPN runtime behavior. Database-изменения ограничены
additive/idempotent catalog repair migrations.

## 15. Release 7.0.1.37 Closure

Цель релиза `7.0.1.37`: закрыть operator-facing дефект дизайна формы client
provisioning: primary/secondary actions выглядели как слишком широкие
горизонтальные полосы, особенно в full-screen provisioning modal.

Зафиксировано в этом релизе:

- Форма `Provision client` теперь использует scoped footer
  `client-provision-actions` вместо generic `inline-actions`, чтобы не
  наследовать full-width mobile/button behavior.
- Кнопки provisioning actions стали компактными, ограничены по width и
  выровнены как нормальный action row на desktop.
- Success view после queue provisioning использует тот же compact action row
  для `Open jobs`, `Open client access` и `Close`.
- Responsive rules сохраняют компактный вид на узких экранах и не возвращают
  кнопки в full-width bars.

Database migration и VPN runtime behavior не менялись. Изменение ограничено Web
UI markup, CSS и asset cache keys.

## 16. Immediate Next Actions

1. Прогнать clean-install procedure на свежем Ubuntu host и записать evidence.
2. Прогнать disposable PostgreSQL migrations и integration tests.
3. Проверить runtime artifact upload/fetch/install для Xray и Shadowsocks.
4. Validate service-pack create/apply/delete на реальных disposable nodes.
5. Validate VLESS ingress с managed egress route policy, route-policy preview,
   route-policy telemetry, explicit cleanup, on-demand access-group catalog sync
   и Nginx HTTP-to-HTTPS redirect на реальных ingress/egress nodes.
6. Продолжить UI consistency review для оставшихся modal form action rows.
7. Продолжить traffic-camouflage ingress case: config preview/diff,
   `nginx -t` evidence surface и live fallback-site smoke.
