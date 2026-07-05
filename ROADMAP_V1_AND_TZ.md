# RTIS MegaVPN Roadmap and Technical Specification

**Release:** `7.0.1.6`

**Analysis date:** 2026-07-01
**Code baseline:** RTIS MegaVPN `7.0.1.6`
**Canonical repository:** `github.com/rtis-emc2/megavpn`

This document is the English roadmap and technical specification for the
current release baseline. The Russian companion is
[`ROADMAP_V1_AND_TZ_RU.md`](ROADMAP_V1_AND_TZ_RU.md).

## 1. Purpose

RTIS MegaVPN is a self-hosted control plane for managed VPN, proxy and edge
service infrastructure. The platform coordinates remote nodes, agents, service
instances, clients, route policy, runtime artifacts, certificates, jobs and
audit events.

The purpose of this roadmap is to keep product direction, engineering scope and
release evidence aligned. It is not a changelog. Operational procedures live in
the runbook and user guides.

## 2. Current Baseline

`7.0.1.6` is the boundary between feature expansion and production hardening.
The codebase already has a working control-plane foundation:

- Go API, worker, agent, migration and admin binaries.
- PostgreSQL-backed persistence and ordered migrations.
- Web UI served by the API.
- Session auth, roles, permissions and audit events.
- Node enrollment, heartbeat, inventory and runtime capability reporting.
- Signed agent transport, typed privileged job flows and job leases.
- Service catalog, service packs, manual instance creation and revisions.
- Runtime binary repository and node capability install/verify jobs.
- OpenVPN, WireGuard, Xray/VLESS, Shadowsocks, HTTP Proxy, MTProto, IPsec/L2TP
  and Nginx service-driver foundation.
- Managed ingress-to-egress backhaul.
- Client provisioning, artifacts, share links, VLESS subscriptions and email
  delivery.
- Backup/restore, deployment scripts, self-test and release gates.

This is not a stable production release yet. It is a hardening baseline that
must produce repeatable evidence for install, migration, agent transport,
runtime apply, route policy and recovery scenarios.

## 3. Release Blockers

The following blockers must be closed before promotion beyond the hardening line:

| Area | Required outcome |
| --- | --- |
| Clean install | Fresh Ubuntu host can install API, worker, migrations, Web UI, Nginx edge and systemd units from documented steps. |
| Database | Migrations apply on a disposable PostgreSQL database and an existing upgrade database. |
| Build and tests | `go test ./...`, `go vet ./...`, binary build and optional race gate are green. |
| Agent channel | Unsigned job/runtime responses are rejected by default; signed empty responses are handled explicitly. |
| Privileged jobs | Apply, cleanup, route policy and capability installation are typed and permission-scoped. |
| Node policy | Agent file writes stay inside canonical managed roots with symlink-safe validation and strict systemd allowlists. |
| Runtime install | Xray and Shadowsocks can be installed from pinned control-plane artifacts or verified OS repositories. |
| Instance apply | Service packs and manual instances converge without false early failure while jobs are queued or running. |
| Backhaul | Ingress-to-egress links apply, probe and clean up without breaking unrelated managed state. |
| Clients | Provisioning requires explicit inbound-service selection and produces verifiable artifacts. |
| Observability | Jobs, runtime logs, health, drift and failure reasons are visible in the UI. |
| Security | Threat model, RBAC matrix, release gates and self-test evidence are updated with each release. |

## 4. Architecture Direction

The platform should continue toward explicit contracts and stable boundaries:

1. Public API contract for the Web UI and external automation.
2. Internal agent API contract for jobs, heartbeat, runtime targets and reports.
3. Driver interfaces for render, validate, apply, stop, cleanup and probe.
4. Desired state -> candidate revision -> validation -> apply-ready revision.
5. Locked job -> current lease owner -> agent execution -> signed result.
6. Runtime observation -> health/drift projection -> operator-visible action.
7. Audit event for every bootstrap, apply, cleanup, capability and share-link
   operation.

This architecture keeps the control plane deterministic, debuggable and safe to
operate across multiple nodes.

## 5. Product Roadmap

| Area | Goal | Implementation direction | Risk |
| --- | --- | --- | --- |
| Node map | Show location, role, workload and health for every node. | Add topology projection, optional coordinates, region/provider labels and map/table UI. | Medium |
| Node links | Visualize backhaul and route-policy paths. | Reuse managed backhaul links and runtime probes; render healthy/degraded/failed edges. | Medium |
| VLESS subscriptions | Export per-client subscriptions for selected inbound services. | Implemented per-client token registry, rotation, revocation, public no-store feed and current active-access filtering; remaining work is QR/text export polish and live E2E evidence. | Medium |
| Traffic camouflage | Formalize WebSocket/gRPC/fallback edge profiles. | Move Xray/Nginx camouflage into reusable profiles with validation and preview. | High |
| Nginx edge | Make edge profiles first-class. | Add profile catalog, certificate binding, config diff, `nginx -t`, atomic apply and rollback. | Medium |
| Runtime artifacts | Reduce manual binary handling. | Add preset fetchers, SHA-256 calculation, artifact status, signed download tickets and install logs. | Low |
| Service logs | Make node-side debugging available in UI. | Add scoped log retrieval for managed units with redaction and retention controls. | Medium |
| Address pools | Centralize network allocation. | Keep reusable pools, allocations, edit/delete guardrails and route-between-pools policy. | Medium |
| OpenVPN templates | Allow controlled client config customization. | Add managed client-template profiles with validation and safe variables. | Medium |
| Security evidence | Make release review repeatable. | Keep threat model, RBAC matrix, self-test, release gates and scan artifacts current. | Medium |

## 6. Runtime and Instance Strategy

Service packs and manual instances must use the same backend mechanism:

- A service pack is a predefined set of instance specifications.
- Manual creation is a single instance specification edited in detail.
- Both paths produce revisions.
- Only validated apply-ready revisions can be applied.
- Secrets are generated at revision/apply time and stored as secret references.
- Network pools are allocated by the platform unless the operator explicitly
  overrides them.
- Runtime capability installation is a node-level prerequisite, not a hidden
  side effect of a broken apply.

The UI should group instances by node while still preserving the instance as the
primary entity. Operators need both views:

- Fleet view: all instances with filters, status, issue and actions.
- Node workload view: what is installed on a selected node.

## 7. Routing and Backhaul Strategy

VLESS, OpenVPN, WireGuard and Shadowsocks are ingress services. The exit path is
controlled by route policy and managed backhaul:

1. A client connects to an ingress instance.
2. The service accepts traffic locally.
3. Instance route policy chooses local breakout or managed egress.
4. Backhaul transport carries traffic to the egress node when required.
5. Health and drift projections show whether the desired path is active.

Node cleanup must be scoped to managed state. It must not remove unrelated
interfaces, routes, firewall rules or backhaul state outside the managed
allowlist.

## 8. Documentation Policy

Documentation is split by language:

- Base filenames are English.
- Russian counterparts use the `_RU.md` suffix.
- `README.md` is English.
- `README_RU.md` is Russian.
- `docs/DOCUMENTATION.md` and `docs/DOCUMENTATION_RU.md` are the entry indexes.
- User-facing workflows need both English and Russian instructions before they
  are considered production-ready.

Every maintained release document must declare the current code release near
the top of the file.

## 9. Release Evidence

The release gate is documented in
[`docs/RELEASE_GATES.md`](docs/RELEASE_GATES.md). The local self-test is
documented in [`docs/SELF_TESTING.md`](docs/SELF_TESTING.md).

Required evidence:

- Build and unit-test results.
- Optional race detector result or an explicit waiver.
- Migration result on a disposable database.
- API, worker and agent smoke tests.
- Node enrollment and update flow.
- Runtime capability install/verify.
- Service pack create/apply/delete.
- Client provisioning and artifact generation.
- Backhaul apply/probe/delete.
- Backup/restore drill.
- Security review and threat-model update.

## 10. Open Questions

1. Should strict mTLS become mandatory for the agent channel before stable v1,
   or is HMAC-signed HTTPS sufficient for this hardening line with a documented
   migration path?
2. Should the static Web UI remain the supported production UI for v1, or should
   the project switch to a typed frontend stack before stable?
3. What exact IPsec scope is required for stable: L2TP/PSK only, IKEv2, or both?
4. Should client self-service be included in stable v1 or remain a post-v1
   feature?
5. Which artifact storage backend is required first: local filesystem only or an
   S3-compatible abstraction?
6. Is MFA mandatory for admin/superadmin before stable?

## 11. Immediate Next Actions

1. Run the clean-install procedure on a fresh Ubuntu host and record evidence.
2. Run disposable PostgreSQL migrations and integration tests.
3. Verify runtime artifact upload/fetch/install for Xray and Shadowsocks.
4. Validate service-pack creation, apply, runtime logs and cleanup on real nodes.
5. Validate OpenVPN client config generation and customizable templates.
6. Validate VLESS ingress with managed egress route policy.
7. Validate VLESS subscription rotation, public feed import and revocation on a
   real client profile.
8. Complete topology-map and node-link design before implementing the UI.
9. Keep release banners and English/Russian documentation pairs synchronized.
