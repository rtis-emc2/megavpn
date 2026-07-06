# RTIS MegaVPN Roadmap and Technical Specification

**Release:** `7.1.0.3`

**Analysis date:** 2026-07-05
**Code baseline:** RTIS MegaVPN `7.1.0.3`
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

`7.1.0.3` continues the production-hardening line after the firewall,
backhaul, VLESS routing, route-policy preview, traffic-camouflage,
documentation-gate and VLESS provisioning-sync releases. This release makes the
managed firewall model operator-readable and fixes the next development path
around audited user traffic accounting with at least 180-day retention. The
codebase already has a working control-plane foundation:

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
- Route-policy preview and agent-side route-policy apply telemetry for managed
  VLESS/backhaul troubleshooting.
- Managed firewall catalog with explicit protocol presets and controlled
  nftables default-policy enforcement.
- Client provisioning, artifacts, share links, VLESS subscriptions and email
  delivery.
- Backup/restore, deployment scripts, self-test and release gates.
- Shared documentation consistency gate for maintained docs, roadmap, release
  review links, production env templates and Web UI asset cache keys.

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

Node enforcement is managed by `node.route_policy.apply`: client ingress flows
and local Xray/VLESS `sendThrough` flows are marked in nftables and selected by
`ip rule fwmark` into non-main managed backhaul route tables.

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

## 11. Release 7.0.1.13 Closure

The goal of `7.0.1.13` is Web UI typography/layout hardening across the
operator console without changing backend behavior.

Closed in this release:

- The console now uses one UI font stack for visible product text.
- Code, inline `code`, code blocks, textareas and web terminal output remain on
  the monospace stack.
- Legacy local `letter-spacing` overrides were normalized to `0`.
- Buttons, tabs, tags, status pills, cards, modals and table cells now have
  explicit `min-width: 0`, wrapping and overflow behavior.
- Mobile page tabs now render as a responsive grid instead of an offscreen
  horizontal strip.
- Desktop and mobile browser smoke covered Dashboard, Nodes, Instances,
  Firewall, Backhaul, Clients, Jobs, Services and Settings.

No database migration, API contract, agent behavior, runtime apply behavior,
VLESS routing, firewall enforcement or traffic-camouflage rendering changed in
this release.

## 12. Release 7.0.1.14 Closure

The goal of `7.0.1.14` is security and release hardening before the next VPN
feature increment.

Closed in this release:

- Go release baseline now requires patch-level `1.26.4`.
- CI and release gate run `govulncheck@v1.5.0`.
- Control-plane installer compares full Go semver, including patch version.
- NGINX.org repository bootstrap verifies the signing key fingerprint before
  importing node trust material.
- Bootstrap env rendering rejects invalid keys and control characters.
- Node name/address validation rejects control characters at HTTP and store
  boundaries.
- Generic job creation is restricted to an explicit allowlist and all new jobs
  start as `queued`.
- Early-stage installer and smoke-script naming artifacts were removed from the
  active release path.

No database migration or VPN runtime behavior changed in this release.

## 13. Release 7.0.1.21 Closure

The goal of `7.0.1.21` is to close the last visible UI/API regression in the
operator console.

Closed in this release:

- Firewall default catalog seeding no longer sends multiple SQL commands through
  one prepared statement.
- Address-pool default seeding now uses a single multi-row statement instead of
  multiple SQL commands in one runtime query.
- Service-pack catalog reads deduplicate by `key` and prefer active, custom and
  newer rows.
- Service-pack default seeding repairs historical duplicate rows and ensures a
  unique key index when older databases are missing it.
- Web core loading deduplicates service packs by `key`, so Create from pack
  cannot render repeated templates.
- Release gate static scan now blocks multi-command SQL in production Go runtime
  paths.
- Firewall catalog schema drift is repaired by
  `000009_firewall_schema_repair`, covering existing installations where the
  consolidated baseline was already marked as applied before firewall tables
  existed.
- Firewall address-list and policy dialogs no longer expose internal identity
  controls in the manual operator workflow.
- Address-list copy now describes reusable source and destination address
  groups without vendor-specific terminology.
- Service-pack catalog list paths now deduplicate semantic clones, so Create
  from pack renders one effective active template even when historical rows have
  different keys.
- `000010_service_pack_semantic_dedup` archives duplicate default service-pack
  rows left by older installations.
- Nginx capability install now falls back from nginx.org repository installation
  to Ubuntu's distro package when the nginx.org apt path fails before runtime
  verification. This keeps VLESS WebSocket camouflage edge rollout from being
  blocked by stale or unavailable nginx.org repository metadata.
- Ubuntu Nginx fallback now checks the distro package candidate before purging
  any existing Nginx package and can continue from local apt metadata when
  `apt-get update` is degraded but the package candidate is available.

No VPN runtime behavior changed in this release. Database changes are limited to
additive/idempotent catalog repair migrations.

## 14. Release 7.0.1.40 Closure

The goal of `7.0.1.40` is to close the VLESS remote-egress convergence defect
found during live ingress diagnostics: Xray could keep an old
`freedom.sendThrough` address after a standby backhaul transport was promoted,
while route-policy had already stopped using the stale interface.

Closed in this release:

- Backhaul apply, route enable and standby transport promotion now share a
  Control Plane convergence path.
- If an active Xray/VLESS ingress instance references the affected egress node,
  the Control Plane creates a fresh validated Xray revision before route-policy
  refresh.
- The refreshed revision updates instance-level `xray_egress`,
  `xray_default_outbound` and group-level selected-egress outbounds to the
  ingress-side address of the selected live backhaul transport.
- If convergence queues `instance.apply`, route-policy refresh is deferred until
  that apply succeeds, so nft/ip-rule generation uses current Xray metadata.
- Re-running promote/enable for an already active selected backhaul transport
  now triggers the same convergence path, giving operators a managed repair
  action for stale Xray revisions created before this release.
- Regression coverage now checks default Xray egress refresh and standby
  OpenVPN promotion after a failed selected WireGuard transport, including the
  idempotent promote repair path.

No database migration is required. The change is a Control Plane desired-state
convergence fix plus documentation and release evidence.

## 15. Release 7.0.1.41 Closure

The goal of `7.0.1.41` is to harden node recovery after agent reinstall and to
make edge reboot/redirect operations explicit managed workflows instead of
manual host-side interventions.

Closed in this release:

- SSH bootstrap/reinstall completion now queues node runtime reconcile:
  inventory sync, service discovery, active instance apply, managed backhaul
  apply, route-policy apply and existing firewall policy apply.
- Operators can run the same runtime reconcile from Node diagnostics without
  reinstalling the agent.
- Node reboot is now a typed privileged job executed by the enrolled agent. The
  agent schedules the reboot after submitting the job result, so the Control
  Plane receives auditable success/failure evidence.
- Nginx generated configs now expose an explicit `http_to_https_redirect` spec
  flag and optional redirect `server_name`, including wildcard DNS names.
- Agent/worker job whitelists now consistently route route-policy cleanup,
  firewall jobs, backhaul jobs, emergency cleanup and reboot to the node agent.

No database migration is required. The change is a Control Plane, agent and UI
hardening release with renderer/job-schema regression coverage.

## 16. Release 7.0.1.42 Closure

The goal of `7.0.1.42` is to close the operator UX defect in service-pack
creation: after a successful create, the page looked unchanged, the selected
node could visually move after refresh, and the operator could submit the same
pack again.

Closed in this release:

- Create-from-pack now persists the submitted form draft through the post-create
  refresh, including selected node, endpoint, routing, camouflage and
  per-component settings.
- A successful service-pack create renders a prominent completion banner above
  the form with created instance count and queued apply/runtime-install jobs.
- The submitted form and service-pack picker are locked after success, so the
  same payload cannot be accidentally submitted again.
- The result action path is explicit: operators can open instances or choose
  "Create another" to intentionally reset the page for a new rollout.
- The component selection state is restored after validation failures and after
  successful creation, including per-component port and OpenVPN CA overrides.

No database migration, API contract or node runtime behavior changed in this
release. The change is a Web UI state-management and operator-safety fix with
asset cache-busting and documentation evidence.

## 17. Release 7.0.1.43 Closure

The goal of `7.0.1.43` is to harden Nginx runtime capability recovery after
agent reinstall, package removal or systemd PATH differences where `/usr/sbin`
is not visible to the agent process.

Closed in this release:

- Agent executable resolution now checks canonical system runtime paths for
  Nginx, systemd and VPN/proxy binaries in addition to `PATH`.
- Inventory collection uses the same resolver, so `/usr/sbin/nginx` is reported
  even when the agent service environment has a reduced PATH.
- Nginx capability verification runs the resolved binary path for `nginx -v`
  and `nginx -t`, and reports `binary_path` in the result payload.
- `instance.apply` for Nginx now attempts managed Nginx recovery through the
  existing nginx.org-to-Ubuntu fallback installer when preflight cannot find the
  binary.
- If the installer makes the Nginx binary available but `nginx -t` still fails
  against old config, instance apply continues to rendered-config validation so
  the new managed config can repair the shared Nginx state.
- Successful Nginx apply recovery updates `node_capabilities` immediately with
  source `instance_apply_recovery`, avoiding stale UI `missing` state until the
  next inventory heartbeat.

No database migration or public API contract changed. The change is an
agent/runtime recovery hardening release with Control Plane capability-state
side effects and regression coverage.

## 18. Release 7.1.0.3 Closure

The goal of `7.1.0.3` is to make managed firewall configuration understandable
before broader production rollout. The runtime enforcement model already exists;
the operator issue was that the UI and docs did not show how address lists,
rules, policies, apply jobs and node state relate to each other.

Closed in this release:

- Firewall Overview now shows the catalog-to-apply pipeline:
  address lists -> rules -> policy -> apply job -> node state.
- Firewall Rules now shows a compact rule-anatomy guide:
  priority -> chain -> match -> action -> apply mode.
- Firewall documentation now includes a Mermaid model diagram and an explicit
  default baseline table for the seeded production node rules.
- The documented development path puts user traffic accounting with at least
  180-day retention first, before production traffic collection is enabled.
- Web asset cache keys, release banners and release review artifacts were
  advanced to `7.1.0.3`.

No database migration, public API contract or node nftables runtime behavior
changed. The default firewall seed and agent renderer remain covered by the
existing integration and agent tests.

## 19. Immediate Next Actions

1. Design user traffic accounting with at least 180-day retention: event schema,
   aggregation, partitioning, privacy boundary, RBAC and export audit.
2. Run the clean-install procedure on a fresh Ubuntu host and record evidence.
3. Run disposable PostgreSQL migrations and integration tests.
4. Verify runtime artifact upload/fetch/install for Xray and Shadowsocks.
5. Validate service-pack creation, apply, runtime logs and cleanup on real nodes.
6. Validate OpenVPN client config generation and customizable templates.
7. Validate VLESS ingress with managed egress route policy, route-policy preview,
   route-policy telemetry, explicit cleanup, on-demand access-group catalog sync
   and Nginx HTTP-to-HTTPS redirect on real ingress/egress nodes.
8. Validate VLESS subscription rotation, public feed import and revocation on a
   real client profile.
9. Continue UI consistency review for remaining modal form action rows.
10. Keep release banners and English/Russian documentation pairs synchronized.
