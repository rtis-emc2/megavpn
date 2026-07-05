# Documentation Review

**Release:** `7.0.1.25`

**Date:** 2026-07-02

This review records the current documentation structure, the problems found and
the remediation applied without changing the release version.

Russian review: [DOCUMENTATION_REVIEW_RU.md](DOCUMENTATION_REVIEW_RU.md).

## Findings

| Finding | Impact | Remediation |
| --- | --- | --- |
| README mixed overview, status, changelog, operations and roadmap in one long file | Operators could not quickly understand where to start; engineers had no clear ownership boundaries | README was rewritten as a concise English product entry point and `README_RU.md` was added for Russian |
| Russian and English content were mixed inside the same paragraphs without structure | Documentation was hard to maintain and hard to review | User-facing documentation now uses paired English/Russian files |
| There was no single documentation map | New operators had to guess which document was authoritative | Added `docs/DOCUMENTATION.md` and `docs/DOCUMENTATION_RU.md` |
| There was no operator usage guide | UI flows were described indirectly through release notes and status text | Added `docs/USER_GUIDE_EN.md` and `docs/USER_GUIDE_RU.md` |
| The operator guide did not cover a clean-host installation path | Operators still had to infer installer, env, migration, systemd and nginx steps from scripts | Expanded EN/RU guides from installation through first system setup and runtime validation |
| Release gate did not require the new user-facing docs | Documentation could regress silently | Updated `docs/RELEASE_GATES.md` and `scripts/self-test.sh` documentation gate |
| Product roadmap, release evidence and operational procedures were not clearly separated | High risk of stale operational instructions in overview docs | README now links to source-of-truth docs instead of duplicating runbooks |
| Release version was not visible in every primary document | Operators could confuse current release docs with historical roadmap notes | Added a `7.0.1.25` release banner to maintained documentation files |
| Roadmap and next-step notes mixed languages under default filenames | Documentation ownership was unclear for English and Russian operators | Split roadmap and next-step notes into English defaults and `_RU` companions |
| VLESS access groups moved out of instance manage but docs still described the old workflow | Operators could configure client groups in the wrong place or miss the required re-apply step | Added paired VLESS access-group docs and updated the operator guide to use `Instances -> VLESS groups` |

## Applied Structure

| Document | Purpose |
| --- | --- |
| `README.md` | English product overview, component model and starting links |
| `README_RU.md` | Russian product overview, component model and starting links |
| `docs/DOCUMENTATION.md` | English documentation index, ownership and corporate rules |
| `docs/DOCUMENTATION_RU.md` | Russian documentation index, ownership and corporate rules |
| `docs/USER_GUIDE_EN.md` | English operator guide |
| `docs/USER_GUIDE_RU.md` | Russian operator guide |
| `docs/OPERATIONS_RUNBOOK.md` | Production operations, backup, restore, upgrade and rollback |
| `docs/RELEASE_GATES.md` | Release acceptance criteria |
| `docs/SELF_TESTING.md` | Diagnostic and release-test commands |
| `docs/THREAT_MODEL.md` | Threat model and residual risks |
| `docs/RBAC_MATRIX.md` | Roles, permissions and privileged job rules |
| `docs/BACKHAUL.md` | Managed ingress-to-egress transport model |
| `docs/VLESS_GROUPS.md` | English VLESS access-group routing model |
| `docs/VLESS_GROUPS_RU.md` | Russian VLESS access-group routing model |
| `ROADMAP_V1_AND_TZ.md` | Product roadmap and technical specification |
| `ROADMAP_V1_AND_TZ_RU.md` | Russian roadmap and technical specification |
| `docs/NEXT_STEPS.md` | English tactical next-step checkpoint |
| `docs/NEXT_STEPS_RU.md` | Russian tactical next-step checkpoint |

## Corporate Documentation Rules

- README is the English entry point; README_RU is the Russian entry point.
- Long procedures live in runbooks or guides.
- User-facing workflows require Russian and English coverage.
- Operational instructions must include prerequisites, expected result and
  failure behavior.
- Security-sensitive workflows must state trust boundaries and audit evidence.
- Examples must use neutral placeholders.
- Release evidence must distinguish passed, failed, skipped and waived checks.
- Maintained docs must show the current release banner.

## Remaining Gaps

These gaps should be tracked before a stable release:

1. Environment-specific installation appendices for external TLS/LB, managed
   PostgreSQL and offline installs.
2. OpenAPI/public API contract.
3. Internal agent API contract.
4. Service-specific troubleshooting matrix in Russian and English.
5. Client configuration examples per service.
6. Traffic camouflage and Nginx edge profile documentation after feature
   implementation.

## Maintenance Policy

Every new operator-visible feature must update:

1. `docs/DOCUMENTATION.md` and `docs/DOCUMENTATION_RU.md` if a new document or
   ownership area is added.
2. `docs/USER_GUIDE_EN.md` and `docs/USER_GUIDE_RU.md` if the UI workflow
   changes.
3. `docs/THREAT_MODEL.md` if trust boundaries, secrets or public endpoints
   change.
4. `docs/RELEASE_GATES.md` if the feature needs release evidence.
5. `docs/OPERATIONS_RUNBOOK.md` if the feature affects deployment, backup,
   restore, upgrade or incident response.
