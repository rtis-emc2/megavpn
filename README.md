# RTIS MegaVPN

**Release:** `7.0.1.25`

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

`7.0.1.25` is a hardening baseline for release stabilization. The codebase has
moved from feature expansion to controlled production-readiness work. The
current focus is:

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
- firewall schema repair for upgraded installations and simplified address-list
  workflows without internal identity fields;
- semantic service-pack deduplication in API/UI plus database repair for
  historical duplicate default pack rows;
- resilient Nginx capability install for camouflage ingress with nginx.org to
  Ubuntu repository fallback on repository/package-stage failures;
- safer Ubuntu Nginx fallback that preserves existing packages until a distro
  candidate is confirmed and reports precise apt failure commands;
- selective service-pack creation: operators can create only chosen components,
  override per-component listen ports and choose OpenVPN CA material without
  installing every service in a template;
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
