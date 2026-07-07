# RTIS MegaVPN

**Release:** `7.1.0.16`

- **Russian README:** [README_RU.md](README_RU.md)
- **License:** Apache License 2.0. See [LICENSE](LICENSE).
- **Repository:** `github.com/rtis-emc2/megavpn`

RTIS MegaVPN is a self-hosted distributed VPN and edge orchestration platform.
It provides a central Control Plane for managing remote nodes, VPN/proxy
services, client access, runtime artifacts, route policy and operational audit.

```text
Operator
  -> Control Plane API / Web UI
  -> Worker queue
  -> Remote node agents
  -> Ingress / egress / runtime nodes
  -> VPN, proxy and edge services
```

## Documentation

Start here:

- [Documentation index](docs/DOCUMENTATION.md)
- [Russian documentation index](docs/DOCUMENTATION_RU.md)
- [User guide](docs/USER_GUIDE_EN.md)
- [Russian user guide](docs/USER_GUIDE_RU.md)
- [Operations runbook](docs/OPERATIONS_RUNBOOK.md)
- [Release gates](docs/RELEASE_GATES.md)
- [Threat model](docs/THREAT_MODEL.md)
- [RBAC matrix](docs/RBAC_MATRIX.md)
- [Managed backhaul](docs/BACKHAUL.md)
- [Node map](docs/NODE_MAP.md)
- [Traffic accounting](docs/TRAFFIC_ACCOUNTING.md)
- [VLESS access groups](docs/VLESS_GROUPS.md)
- [Self-testing](docs/SELF_TESTING.md)
- [Roadmap and technical specification](ROADMAP_V1_AND_TZ.md)

## Product Scope

RTIS MegaVPN is intended for enterprise-style operation of distributed access
infrastructure:

- node enrollment and lifecycle management;
- signed agent transport and remote job execution;
- service-pack based instance creation and manual instance editing;
- OpenVPN, WireGuard, Xray/VLESS, Shadowsocks, HTTP Proxy, MTProto, IPsec/L2TP
  and Nginx edge workflows;
- managed ingress-to-egress backhaul and route-policy projection;
- client provisioning, generated configs, artifacts, share links and email
  delivery;
- runtime binary repository for pinned artifacts;
- audit, diagnostics, backup/restore and release gates.

## Current Release Status

`7.1.0.16` simplifies the Traffic Accounting operator UI: storage-maintenance
fields are replaced with traffic/client/node/collector counters, empty datasets
show an actionable diagnostics state, filter actions stay compact, and Web UI
asset cache keys are advanced for reliable browser refresh after deployment.
The current focus is:

- clean install and upgrade path on a new Ubuntu host;
- PostgreSQL migrations on disposable databases;
- signed agent channel with replay-window checks;
- typed privileged job APIs and job-type permission matrix;
- node bootstrap, update and emergency cleanup workflows;
- service-pack/manual instance creation, apply and runtime convergence;
- centralized VLESS access groups for client routing policy;
- clearer managed backhaul UX: one active ingress-to-egress transport, optional
  standby transports only by explicit operator choice and controlled standby
  promotion;
- traffic-camouflage Nginx/Xray ingress with explicit fallback website and
  managed rollback on failed validation/apply;
- managed firewall catalog with explicit protocol presets and controlled
  default-policy enforcement for nftables apply;
- default node firewall baseline with strict input/forward deny, HTTP/HTTPS
  edge allow rules, ICMP/ICMPv6 diagnostics, VPN client forwarding ranges and a
  dedicated `inet megavpn_firewall` table;
- operator-facing firewall model diagram that shows address lists, rules,
  policies, apply jobs and node state as one catalog-to-apply workflow;
- firewall schema repair for upgraded installations and simplified address-list
  workflows without internal identity fields;
- traffic-accounting foundation with signed agent ingest API, PostgreSQL
  aggregate storage, 180-day retention cleanup, `traffic.read` RBAC, a
  dedicated operator UI page, managed Xray/WireGuard/OpenVPN byte-counter
  collectors and no-store CSV export for aggregate audit rows;
- bounded batched traffic-accounting retention cleanup and PostgreSQL query
  indexes for overview/export filters by bucket, client, node and protocol;
- traffic-accounting operator visibility: the summary now prioritizes total
  traffic, received/sent bytes, retained samples, clients, nodes, collectors and
  retention instead of backend prune internals;
- traffic-accounting filters for date range, protocol, client, node and row
  limit are now applied by the backend to overview summary, recent rows and
  no-store CSV export with one retained-dataset query model;
- traffic-accounting collector status now shows node/source/protocol freshness,
  active/degraded/inactive streams, last report time, last bucket and aggregate
  client/sample coverage for the selected retained dataset;
- expected collector coverage now compares active traffic-accounting-enabled
  Xray, WireGuard and OpenVPN runtime revisions with observed sample streams and
  marks missing or partial retained sample streams in the Traffic Accounting UI;
- semantic service-pack deduplication in API/UI plus database repair for
  historical duplicate default pack rows;
- VLESS client provisioning now syncs active access-group catalog entries into
  the selected Xray instance before validating the chosen group, and materializes
  selected-egress groups into concrete Xray outbound/source-route metadata;
- VLESS client identity is now stable across Xray/VLESS ingress instances:
  provisioning a client onto a new ingress reuses the existing client UUID and
  queues apply so the new server accepts the already issued client credential;
- Xray UUID rotation now preserves the client's selected VLESS access group
  instead of falling back to a stale implicit `route` value; stale implicit
  group metadata falls back to an active catalog group while explicit invalid
  operator choices still fail closed;
- client provisioning action rows use compact, scoped buttons instead of
  inherited full-width modal bars;
- resilient Nginx capability install for camouflage ingress with nginx.org to
  Ubuntu repository fallback on repository/package-stage failures;
- safer Ubuntu Nginx fallback that preserves existing packages until a distro
  candidate is confirmed and reports precise apt failure commands;
- selective service-pack creation: operators can create only chosen components,
  override per-component listen ports and choose OpenVPN CA material without
  installing every service in a template;
- service-pack creation now has an explicit completed state: the submitted
  form is preserved and locked after success, while the operator gets direct
  actions to open created instances or start a separate new rollout;
- route-policy enforcement hardening: ingress client traffic and local
  Xray/VLESS system egress are marked through managed nftables chains and
  routed by `fwmark` into managed backhaul tables;
- read-only route-policy preview for nodes: operators can inspect projected
  client routes, VLESS/Xray system egress routes, blocked/observe-only reasons
  and managed nft/ip-rule primitives before queueing `node.route_policy.apply`;
- route-policy apply telemetry in agent job results: systemd unit/timer state,
  `ip rule show` and managed nftables route-policy chains are captured after
  apply for VLESS/backhaul troubleshooting;
- route-policy and netpolicy nft comments are rendered as nft string literals,
  and route-policy fails closed instead of marking traffic when no managed
  backhaul candidate is ready;
- Xray/VLESS remote-egress convergence: when an active backhaul transport is
  promoted, enabled or becomes active after apply, the Control Plane refreshes
  affected Xray instance revisions first so `freedom.sendThrough` matches the
  selected live backhaul interface before route-policy is queued;
- idempotent backhaul promote/enable now acts as a managed repair trigger for
  existing Xray instances with stale remote-egress metadata;
- explicit route-policy cleanup: operators can queue a typed node job that
  stops route-policy runtime, removes managed `fwmark` rules/chains and cleans
  stale destinations remembered in the previous node snapshot;
- generated TLS-enabled Nginx edge configs include an HTTP listener that
  redirects plain HTTP traffic to HTTPS before camouflage/fallback routing;
- Nginx instance and emergency cleanup now reload shared Nginx when managed
  configs remain and stop it when all MegaVPN-managed edge configs are gone;
- Nginx capability recovery is now agent-side: reduced systemd PATHs still find
  `/usr/sbin/nginx`, and Nginx instance apply can recover a missing binary via
  the managed nginx.org-to-Ubuntu fallback installer;
- client VLESS camouflage UX that separates public client endpoints from local
  Xray backend endpoints and makes pending provisioning state actionable;
- hard client deletion with PostgreSQL cleanup coverage for service access,
  routes, generated artifacts, share links, subscriptions, delivery records and
  service-access scoped secrets;
- instance deletion now cascades client service-access cleanup after managed
  node cleanup succeeds, and operators can remove a single stale service access
  from the Client Access modal;
- operator-grade Firewall UI with posture cards, rule filters, grouped
  protocol presets and explicit apply modes;
- operator console typography and layout hardening with one UI font stack,
  safe text wrapping and mobile tab grids;
- OpenVPN full-tunnel defaults with managed forwarding and NAT policy;
- OpenVPN, WireGuard, Xray/VLESS, Shadowsocks, Nginx and Backhaul smoke matrix;
- client provisioning and route-policy validation;
- operator-visible diagnostics for jobs, runtime capabilities and service logs.

## Component Model

| Component | Purpose |
| --- | --- |
| `cmd/api` | Control Plane API and Web UI backend |
| `cmd/worker` | Asynchronous orchestration worker |
| `cmd/agent` | Remote node runtime agent |
| `cmd/migrate` | Database migration runner |
| PostgreSQL | Persistent state store |
| Nginx | Public HTTPS edge |

## Production Principles

- Public Control Plane access is HTTPS-only.
- The API should bind to loopback behind a trusted reverse proxy.
- Bootstrap credentials are explicit; there is no built-in default password.
- The secret master key is stored outside database backups.
- Agents use per-node identity and signed HTTP messages.
- Privileged operations use typed endpoints for bootstrap, apply, cleanup,
  capability installation and route-policy changes.
- Runtime artifacts are pinned by SHA-256 before node installation.
- Backups and restore drills are part of release evidence.

## Quick Local Commands

```bash
go test ./...
go test -race ./...
go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate
scripts/self-test.sh
```

Release evidence is stricter than local diagnostics:

```bash
scripts/release-gate.sh
```

`scripts/release-gate.sh` is fail-closed. It fails when production evidence is
missing, for example disposable PostgreSQL, backup/restore drill, systemd,
nginx, deployed API or service smoke matrix.

## Minimal Operator Flow

1. Install the Control Plane and run migrations.
2. Configure public HTTPS edge and production environment variables.
3. Create the first operator account with explicit bootstrap credentials.
4. Add nodes and enroll agents.
5. Verify node heartbeat, inventory and runtime capabilities.
6. Add runtime artifacts when a service cannot be installed safely from OS
   repositories.
7. Create managed backhaul between ingress and egress nodes when remote egress is
   required.
8. Create service instances from a pack or manually.
9. Apply instances and wait for runtime convergence.
10. Create clients, choose allowed inbound services and provision access.
11. Build client artifacts, preview/download configs and optionally publish share
    links or send email.
12. Monitor Jobs, Audit, Runtime state and Backhaul health.

See the full [User Guide](docs/USER_GUIDE_EN.md).

## Security Baseline

The security model is documented in [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md).
Important defaults:

- unsigned agent responses are rejected by updated agents;
- empty job-poll `204` responses are signed;
- job completion from agents requires a current non-expired lease;
- agent file writes are restricted by path roots, canonicalization and systemd
  unit allowlists;
- SSH bootstrap uses strict host-key fingerprint validation;
- public share links store token hashes, not plaintext tokens.

## License

Apache License 2.0. See [LICENSE](LICENSE).
