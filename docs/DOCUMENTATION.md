# Documentation Index

**Release:** `7.0.1.30`

This document is the English entry point for RTIS MegaVPN documentation. It keeps
the documentation map explicit so operators and engineers can quickly find the
correct source of truth.

Russian entry point: [DOCUMENTATION_RU.md](DOCUMENTATION_RU.md).

## Recommended Reading Order

| Order | Document | Purpose |
| --- | --- | --- |
| 1 | [README](../README.md) | Product overview and component model |
| 2 | [User guide](USER_GUIDE_EN.md) | End-to-end operator guide |
| 3 | [Operations runbook](OPERATIONS_RUNBOOK.md) | Production operations, backup, restore, upgrade and rollback |
| 4 | [Release gates](RELEASE_GATES.md) | Release acceptance evidence |
| 5 | [Self-testing](SELF_TESTING.md) | Local and live diagnostic gates |
| 6 | [Threat model](THREAT_MODEL.md) | Security model and residual risks |
| 7 | [RBAC matrix](RBAC_MATRIX.md) | Roles, permissions and privileged job rules |
| 8 | [Managed backhaul](BACKHAUL.md) | Ingress-to-egress transport model |
| 9 | [Node map](NODE_MAP.md) | GeoIP node placement and backhaul topology overlay |
| 10 | [Firewall policy catalog](FIREWALL.md) | Managed firewall policies, address lists, rules and node apply state |
| 11 | [VLESS access groups](VLESS_GROUPS.md) | Reusable VLESS client routing groups |
| 12 | [VLESS subscriptions](VLESS_SUBSCRIPTIONS.md) | Per-client VLESS subscription tokens and delivery workflow |
| 13 | [Roadmap](../ROADMAP_V1_AND_TZ.md) | Product roadmap and technical specification |
| 14 | [Next steps](NEXT_STEPS.md) | Engineering backlog checkpoint |
| 15 | [Security review](SECURITY_REVIEW_7.0.1.30.md) | Security and release review artifact |
| 16 | [Russian roadmap](../ROADMAP_V1_AND_TZ_RU.md) | Russian roadmap companion |

## Documentation Ownership

| Area | Source of truth | Notes |
| --- | --- | --- |
| Product overview | `README.md`, `README_RU.md` | Keep concise. Do not move long operational procedures into README. |
| Operator usage | `docs/USER_GUIDE_EN.md`, `docs/USER_GUIDE_RU.md` | English and Russian guides must stay aligned. |
| Operations | `docs/OPERATIONS_RUNBOOK.md` | Production procedures and controlled maintenance. |
| Release readiness | `docs/RELEASE_GATES.md`, `docs/SELF_TESTING.md` | Release evidence, self-test and smoke gates. |
| Security | `docs/THREAT_MODEL.md`, `docs/RBAC_MATRIX.md` | Threat model, roles, permissions and privileged jobs. |
| Backhaul/routing | `docs/BACKHAUL.md` | Managed links, route projection and troubleshooting. |
| Topology | `docs/NODE_MAP.md`, `docs/NODE_MAP_RU.md` | GeoIP node placement, local static map, node owner metadata and backhaul overlay. |
| Firewall | `docs/FIREWALL.md`, `docs/FIREWALL_RU.md` | Managed firewall catalog, address lists, rules and node apply state. |
| VLESS client routing | `docs/VLESS_GROUPS.md`, `docs/VLESS_GROUPS_RU.md` | Reusable access groups, default VLESS group selection and provisioning behavior. |
| VLESS subscriptions | `docs/VLESS_SUBSCRIPTIONS.md`, `docs/VLESS_SUBSCRIPTIONS_RU.md` | Per-client bearer-token rotation, public feed behavior and operator delivery workflow. |
| Roadmap | `ROADMAP_V1_AND_TZ.md`, `ROADMAP_V1_AND_TZ_RU.md`, `docs/NEXT_STEPS.md`, `docs/NEXT_STEPS_RU.md` | Strategic and tactical product planning. |

## Language Policy

- `README.md` is English-only.
- `README_RU.md` is Russian-only.
- English docs use the default filename.
- Russian paired docs use `_RU.md`.
- Historical appendix files may keep domain terminology, but supported entry
  points must stay language-specific.
- Every user-facing workflow must have Russian and English coverage before it is
  treated as production-ready.

## Corporate Documentation Rules

- README must remain a product entry point, not a changelog dump.
- Operational documents must state prerequisites, commands, expected result and
  rollback/failure behavior.
- Security-sensitive procedures must state the trust boundary and audit evidence.
- Release documents must separate `PASS`, `FAIL`, `SKIP` and explicit waiver.
- Examples must use neutral placeholder domains such as `control.example.com`,
  `edge.example.com` or `vpn.example.com`.
- Avoid documenting manual node-side changes as the primary path when the Control
  Plane has a managed workflow.
- Each maintained documentation file must show the current release banner:
  `7.0.1.30`.

## Current Documentation Gaps

These items must be closed before a stable release claim:

- Full OpenAPI/public API contract.
- Internal agent API contract.
- Environment-specific installation appendices for external TLS/LB, managed
  PostgreSQL and offline installs.
- Bilingual troubleshooting matrix for every supported service.
- Service-specific client configuration examples.
