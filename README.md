# RTIS MegaVPN

**Release:** `7.1.1.0`

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
- [Client access groups](docs/ACCESS_GROUPS.md)
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

Current release: `7.1.1.0`.

The release notes and stabilization baseline are maintained in
[docs/releases/7.1.1.0.md](docs/releases/7.1.1.0.md). Release readiness gates
are documented in [docs/RELEASE_GATES.md](docs/RELEASE_GATES.md).

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
go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin
scripts/ci/self-test.sh
```

Release evidence is stricter than local diagnostics:

```bash
scripts/ci/release-gate.sh
```

`scripts/ci/release-gate.sh` is fail-closed. It fails when production evidence is
missing, for example disposable PostgreSQL, backup/restore drill, systemd,
nginx, deployed API or service smoke matrix.

Historical `scripts/*.sh` entrypoints remain as compatibility wrappers. New
automation should prefer `scripts/ci`, `scripts/smoke`, `scripts/ops` and
`scripts/lib`.

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
