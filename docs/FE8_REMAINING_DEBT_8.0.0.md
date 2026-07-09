# FE8 Remaining Debt For 8.0.0

Branch: `release/8.0.0-frontend-console`

Generated UTC: `2026-07-09T09:57:57Z`

Final cutover status: **NO-GO** until every required item below is completed or explicitly waived by release owners with a dated rationale.

## Release-Blocking Debt

| Area | Status |
| --- | --- |
| Version sync | OPEN |
| Live disposable smoke | OPEN |
| Full release gate | PARTIAL / OPEN |
| Legacy rollback | REQUIRED |

## Additional Release-Blocking Review Items

| Area | Status |
| --- | --- |
| GitHub Actions Node.js 20 deprecation | CLOSED |
| Responsive evidence | OPEN |
| i18n final review | PARTIAL / OPEN |
| Static/raw API guard review | CLOSED |
| Acceptance matrix cleanup | PARTIAL / OPEN |
| PR readiness checklist | ADDED |
| LF-only raw evidence validation | PASS LOCALLY / PENDING FINAL CI |

Release-blocking details:

- Version sync: backend source of truth is still
  `internal/platform/version.Version = "7.1.1.0"` while the frontend package
  metadata is `8.0.0`; HEAD is not tagged. Do not partially update version
  metadata without release owner approval, tag/version policy, install metadata
  review, live smoke and a full final gate.
- Live disposable smoke: no disposable API, DB, node, endpoint domain or
  certificate id was available in this workstation session. Use
  `docs/FE8_LIVE_SMOKE_PLAN_8.0.0.md` before final cutover.
- Full release gate: local diagnostic gate can run with explicit skips, but it
  is not production release evidence while clean npm install, disposable DB,
  backup/restore, API smoke and service matrix inputs are missing.
- Legacy rollback: keep `/legacy/` until cutover is signed off and tested.
- GitHub Actions Node.js 20 deprecation: closed by moving pinned Actions to
  upstream node24 major pins while preserving commit-SHA pinning.
- Responsive evidence: no real workflow screenshot set was captured because no
  disposable backend/API data environment was available. Use
  `docs/FE8_RESPONSIVE_EVIDENCE_PLAN_8.0.0.md`.
- i18n final review: key parity passed, but a human English/Russian operator
  wording review for dangerous operations, secrets and final errors is still
  required.
- Static/raw API guard review: local guards pass; raw `/api/v1` calls are
  contained in `frontend/src/shared/api/endpoints.ts`, production
  `console.log` / `console.debug` and `dangerouslySetInnerHTML` are absent from
  `frontend/src`, and new-UI browser storage is limited to API base and locale.
- Acceptance matrix cleanup: evidence is aligned for PR review, but final
  release wording must be refreshed after live smoke, version sync and final
  gate evidence.
- PR readiness checklist: added in `docs/FE8_PR_READINESS_8.0.0.md`.

## Current Release Readiness Evidence

| Item | Status | Evidence |
| --- | --- | --- |
| Baseline HEAD inspected | PASS | `3d4e1ae2d69649eaa88a2baadc17c3dbf03efe05` |
| Latest baseline CI | PASS | GitHub Actions run `29008173288` for `3d4e1ae2d69649eaa88a2baadc17c3dbf03efe05` |
| `/legacy/` rollback path | PASS | `web/legacy` exists and serving smoke passes. |
| Package manager | PASS | npm-only: `frontend/package-lock.json` exists and `frontend/pnpm-lock.yaml` is absent. |
| Version sync | OPEN | Backend version `7.1.1.0`, frontend package `8.0.0`, HEAD has no version tag. |
| GitHub Actions runtime pins | CLOSED | checkout `v7.0.0`, setup-go `v6.5.0`, setup-node `v6.4.0`, upload-artifact `v7.0.1`; each inspected `action.yml` uses `node24`. |
| Static frontend guards | PASS | `scripts/ci/frontend-static-guards.sh` passed locally. |
| Docs guards | PASS | `scripts/ci/docs-markdown-shape.sh` and `scripts/ci/docs-consistency.sh` passed locally. |
| Release gate | PARTIAL | Diagnostic run with `MEGAVPN_RELEASE_ALLOW_SKIPS=1` passed 19 gates and skipped 7 workstation/live-env gates. |
| Live disposable smoke | OPEN | Required API/DB/node inputs are unavailable. |
| Responsive evidence | OPEN | Real workflow screenshots are not captured. |
| i18n wording review | PARTIAL | Key parity passed; human wording review remains open. |

## Backend-Missing Sub-Actions

| Domain | Sub-action |
| --- | --- |
| Clients | Generic edit |
| Clients | Route update |
| Clients | Per-access revoke |
| Clients | Delivery history |
| Services | Runtime artifact delete |
| Services | Service pack validation |
| Instances | Spec preview/draft-save |
| Platform Access | Invite revoke |
| Backhaul | Dedicated repair action |
| Operations | Backup/restore browser UI |

Backend-missing reasons:

- Generic client edit: no generic `PATCH/PUT /api/v1/clients/{id}` endpoint.
- Client route update: no `PUT/PATCH /api/v1/clients/{id}/routes/{route_id}` endpoint.
- Per-access revoke: backend supports client-level revoke and access delete only.
- Client delivery history: no client-scoped delivery history endpoint exists.
- Runtime artifact delete: no binary runtime artifact DELETE endpoint exists.
- Service pack validation: no separate validation endpoint exists.
- Instance spec preview/draft-save: no separate preview or draft-save route exists.
- Platform invite revoke: no browser backend endpoint exists.
- Backhaul dedicated repair: no dedicated repair endpoint exists.
- Backup/restore browser UI: browser parity endpoint and UX are not implemented.

## Future-Scope Sub-Actions

| Domain | Sub-action |
| --- | --- |
| Clients -> Groups | Non-VLESS materialization |
| Clients -> Groups | Migration conflict UI |
| Nodes | Create/register/edit |
| Nodes | New SSH access method with secret material |
| Nodes | Manual bootstrap bundle reveal |
| Nodes | Agent identity revoke/reboot/cleanup |
| Nodes | Service discovery ignore/unignore |
| Platform Access | User lifecycle mutations |
| Backhaul | Link create/delete |

Future-scope decisions:

- Non-VLESS materialization stays catalog-only in this frontend cutover.
- Migration conflict UI remains future scope.
- Nodes create/register/edit are not migrated in FE8-P0-05A/05B.
- New SSH access method creation needs a reviewed browser secret flow.
- Manual bootstrap bundle reveal remains backend-controlled.
- Agent identity revoke, reboot and cleanup remain future scope or legacy-only.
- Service discovery ignore/unignore is not migrated in FE8-P0-05A.
- Platform user lifecycle mutations remain future scope.
- Backhaul link create/delete remain outside FE8-P0-08A.

## Live Smoke Plan

Minimum disposable smoke coverage before final release decision:

1. VLESS group create/edit/scope/member preview/apply/remove/sync.
2. Firewall address group, policy and rule CRUD.
3. Firewall preview, apply and emergency disable on disposable nodes.
4. Client create, status, revoke and delete.
5. Client artifacts build, download and delete.
6. Client delivery share, subscription and email.
7. Client route create and delete.
8. Client access rotation/delete and config cleanup.
9. Instance runtime lifecycle.
10. Service-pack instance create.
11. Manual instance create.
12. Instance spec replace.
13. Runtime artifact URL import.
14. Nodes diagnostics, inventory, capabilities and discovery.
15. Nodes bootstrap, security and control for disposable nodes.
16. Certificates/PKI import preview/apply.
17. Certificates/PKI self-signed, managed CA and issue-from-CA.
18. Certificates/PKI default, revoke, delete and PKI root create.
19. Platform settings save/apply.
20. Mail settings/test.
21. Users, invites and sessions.
22. Backhaul apply, probe, promote and route projection.
23. Route Policy preview, apply and cleanup on disposable topology.

## Release-Gate Plan

1. Freeze a final candidate SHA after version sync and live smoke.
2. Run the full backend, frontend and security CI gate on that SHA.
3. Re-run local smoke and guard scripts from a clean worktree.
4. Attach CI links, live smoke logs and rollback notes to release evidence.
5. Require release owner sign-off before removing any `/legacy/` dependency.

Current diagnostic release-gate evidence:

- Command:
  `MEGAVPN_RELEASE_ALLOW_SKIPS=1 MEGAVPN_RELEASE_NODE_BIN=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node scripts/ci/release-gate.sh`
- Result: PASS as a local diagnostic gate, `passed=19 skipped=7`.
- Skips:
  frontend workspace checks because this workstation has no npm, PostgreSQL
  migrations/integration because `MEGAVPN_RELEASE_DATABASE_DSN` is unset,
  backup/restore because release and restore DSNs are unset, systemd verify
  because `systemd-analyze` is unavailable, nginx verify because `nginx` is
  unavailable, API smoke because release/public base URL is unset, and VPN
  service matrix because `MEGAVPN_RELEASE_RUN_SERVICE_MATRIX=1` plus live
  inputs were not provided.
- This is not production release evidence because skips were explicitly
  allowed.

## Version Sync Plan

1. Update backend binary/version metadata from `7.1.1.0` to `8.0.0`.
2. Update frontend visible version/release metadata if separately derived.
3. Re-run `go test ./...`.
4. Re-run `go test -race ./...`.
5. Re-run frontend build and tests.
6. Re-run `scripts/ci/docs-consistency.sh`.
7. Confirm release notes and acceptance evidence reference `8.0.0`.

## Responsive Evidence Plan

1. Capture desktop operator viewport for critical workflows.
2. Capture tablet/pad viewport for critical workflow entry points.
3. Capture phone/narrow viewport for navigation and confirmations.
4. Confirm text does not overlap.
5. Confirm destructive controls remain reachable.
6. Confirm secret material is not exposed.

## PR Readiness Checklist

- Branch is pushed to `origin/release/8.0.0-frontend-console`.
- `/legacy/` rollback path exists and remains documented.
- Latest GitHub Actions CI run is green on the evidence SHA.
- Package manager remains npm-only.
- `frontend/package-lock.json` is present.
- `frontend/pnpm-lock.yaml` is absent.
- PR summary lists connected workflows.
- PR summary lists backend-missing sub-actions.
- PR summary lists release blockers.
- PR summary includes rollback plan.
- No automatic merge or main-branch mutation before review.

## Security And Safety Requirements Still Active

- Do not remove `/legacy/` before final cutover approval.
- Do not expose auth/session/bearer/bootstrap/share tokens in storage.
- Do not expose subscription or enrollment tokens in browser storage.
- Do not log secrets, private keys, generated configs or credentials.
- Do not render backend logs, diagnostics, specs or cert material as HTML.
- Keep RBAC, CSRF, signed jobs, audit logging and backend validation.
- Keep preview-before-apply for Firewall, VLESS, certificates and routes.
- Keep missing exact sub-actions disabled with literal backend reasons.

## Final Cutover Decision

NO-GO because version metadata still reports `7.1.1.0`, live disposable
API/DB smoke and staging operator validation are not complete, the full final
release gate has not been run on a version-synchronized final SHA, responsive
evidence and i18n wording review are still open, and multiple backend-missing
or future-scope sub-actions remain outside the 8.0.0 frontend cutover.
