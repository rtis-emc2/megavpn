# FE8 Remaining Debt For 8.0.0

Branch: `release/8.0.0-frontend-console`

Generated UTC: `2026-07-15T13:18:28Z`

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
| LF-only raw evidence validation | CLOSED |

Release-blocking details:

- Version sync: backend source of truth is still
  `internal/platform/version.Version = "7.1.1.0"` while the frontend package
  metadata is `8.0.0`; HEAD is not tagged. Do not partially update version
  metadata without release owner approval, tag/version policy, install metadata
  review, live smoke and a full final gate.
- Live disposable smoke: no disposable API, DB, live external node, endpoint
  domain or certificate id was available in this workstation session. Guided
  Agent Registration/Onboarding has implementation and disposable
  HTTP/PostgreSQL protocol evidence, but live/staging validation of actual
  node-side bootstrap execution, registration, heartbeat and inventory remains
  a release blocker. Use `docs/FE8_LIVE_SMOKE_PLAN_8.0.0.md` before final
  cutover.
- Full release gate: local diagnostic gate can run with explicit skips, but it
  is not production release evidence while clean npm install, backup/restore,
  API smoke and service matrix inputs are missing. Disposable PostgreSQL
  integration evidence now exists for the tested backend suites, including
  secure SSH access-method creation, manual bootstrap bundle reveal/download
  and guided agent onboarding protocol flows; local workstation PostgreSQL
  availability is not equivalent to that GitHub CI service evidence.
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
- LF-only raw evidence validation: closed by
  `772c7371387777045de990d19357f8c767c38dc5`; GitHub CI run `29010925982`
  passed and GitHub raw validation showed LF-only multiline files for
  `.gitattributes`, `.github/workflows/ci.yml`, docs guards and evidence docs.

## Current Release Readiness Evidence

| Item | Status | Evidence |
| --- | --- | --- |
| LF guard recovery HEAD inspected | PASS | `772c7371387777045de990d19357f8c767c38dc5` |
| Latest LF guard recovery CI | PASS | GitHub Actions run `29010925982` for `772c7371387777045de990d19357f8c767c38dc5` |
| `/legacy/` rollback path | PASS | `web/legacy` exists and serving smoke passes. |
| Package manager | PASS | npm-only: `frontend/package-lock.json` exists and `frontend/pnpm-lock.yaml` is absent. |
| Version sync | OPEN | Backend version `7.1.1.0`, frontend package `8.0.0`, HEAD has no version tag. |
| GitHub Actions runtime pins | CLOSED | checkout `v7.0.0`, setup-go `v6.5.0`, setup-node `v6.4.0`, upload-artifact `v7.0.1`; each inspected `action.yml` uses `node24`. |
| Static frontend guards | PASS | `scripts/ci/frontend-static-guards.sh` passed locally. |
| Text LF guards | PASS | `scripts/ci/text-lf-guard.sh`, `scripts/ci/docs-markdown-shape.sh` and `scripts/ci/docs-consistency.sh` passed locally and in CI. |
| SSH access-method PostgreSQL evidence | PASS | Evidence HEAD `1ffda5b00efb98fa9f60d22a998f1e9e2c52daf2`; GitHub Actions run `29361072970`, job `PostgreSQL integration tests`. |
| Manual bootstrap bundle PostgreSQL/HTTP evidence | PASS | Evidence HEAD `3abc200c3d7c5525eaded994244af488d0728b41`; CI run `29391281058` required bundle infra/http groups without skips. |
| Agent onboarding protocol and React workflow evidence | PASS | Disposable HTTP/PostgreSQL protocol evidence; guided React operator workflow; live external-node smoke OPEN. |
| Release gate | PARTIAL | Diagnostic run with `MEGAVPN_RELEASE_ALLOW_SKIPS=1` passed 19 gates and skipped 7 workstation/live-env gates. |
| Live disposable smoke | OPEN | Required API/DB/node inputs are unavailable. |
| Responsive evidence | OPEN | Real workflow screenshots are not captured. |
| i18n wording review | PARTIAL | Key parity passed; human wording review remains open. |

## Backend-Missing Sub-Actions

Backend-Missing count: **6**.

| Domain | Sub-action |
| --- | --- |
| Services | Runtime artifact delete |
| Services | Service pack validation |
| Instances | Spec preview/draft-save |
| Platform Access | Invite revoke |
| Backhaul | Dedicated repair action |
| Operations | Backup/restore browser UI |

Backend-missing reasons:

- Runtime artifact delete: no binary runtime artifact DELETE endpoint exists.
- Service pack validation: no separate validation endpoint exists.
- Instance spec preview/draft-save: no separate preview or draft-save route exists.
- Platform invite revoke: no browser backend endpoint exists.
- Backhaul dedicated repair: no dedicated repair endpoint exists.
- Backup/restore browser UI: browser parity endpoint and UX are not implemented.

## Future-Scope Sub-Actions

Future-Scope count: **6**. Previous count was 7; removed completed item:
`Nodes | Agent registration/onboarding`.

| Domain | Sub-action |
| --- | --- |
| Clients -> Groups | Non-VLESS materialization |
| Clients -> Groups | Migration conflict UI |
| Nodes | Agent identity revoke/reboot/cleanup |
| Nodes | Service discovery ignore/unignore |
| Platform Access | User lifecycle mutations |
| Backhaul | Link create/delete |

Future-scope decisions:

- Non-VLESS materialization stays catalog-only in this frontend cutover.
- Migration conflict UI remains future scope.
- Nodes create/edit safe control-plane profile metadata is migrated in
  FE8-P0-09B step 1.
- Agent Registration/Onboarding is completed for the guided React operator
  workflow and removed from Future-Scope. Completed scope includes hardened
  agent registration, explicit enrollment and legacy modes, atomic
  enrollment-token consumption, hash-only agent-token persistence,
  transactional registration audit, deterministic retry and reissue-required
  handling, explicit operator replacement-token recovery, real signed
  heartbeat, request-signature and replay protection, real inventory job
  polling/result submission/persistence, backend-derived diagnostics, guided
  onboarding status, secure token issue/reissue, guided SSH/manual-bundle
  bootstrap, registration and first-heartbeat waiting, guided inventory
  synchronization, backend-derived ready state, no browser `/agent/*` calls
  and no `/legacy/` dependency for the guided operator workflow.
- Agent Registration/Onboarding evidence: backend hardening commit
  `2a8784b36f47d35f758968a382b33c785ee534af`, retry/reissue commit
  `54dfcb83c2fdd2444d8b868289b5c995a14dfbdf`, real HTTP/PostgreSQL evidence
  commit `8206a42cfab7a6218fdcc7caf2222050b694fdca` with GitHub Actions run
  `29401792602`, final functional onboarding commit
  `42065d6ac765a66ac983c611c0f0fdfaf8cb67a2` with GitHub Actions run
  `29415883087`, and acceptance/operator documentation commit
  `51bc714728baec8fcd2355ba87146fdb19a9dcd1` with GitHub Actions run
  `29417560392`.
- Live external-node onboarding smoke remains OPEN. The remaining release
  blocker is live/staging validation of actual enrollment-material delivery,
  node-side bootstrap execution, registration, heartbeat, inventory and
  rollback, not missing browser implementation.
- New SSH access method creation was completed through a dedicated atomic
  backend endpoint, explicit host-key verification workflow, transient
  private-key form handling, encrypted PostgreSQL secret storage and
  non-skipping PostgreSQL/HTTP integration evidence. Evidence: backend commit
  `9dd92d299415c91058fc2bf0df2d6ac26a8b2838`, frontend commit
  `d5dc323856677324ced54f14a8c2a5b79d84b025`, PostgreSQL evidence HEAD
  `1ffda5b00efb98fa9f60d22a998f1e9e2c52daf2`, CI run `29361072970`.
- Manual bootstrap bundle reveal/download is completed for secure retrieval of
  an already completed manual bootstrap run. The workflow has hardened POST
  reveal/download endpoints, direct node/run scoping, encrypted backend secret
  resolution, targeted public projection through `manual_bundle_available`, no
  public secret-reference exposure, no-store reveal/download behavior,
  fail-closed audit, React explicit confirmation, transient local reveal state,
  audited backend download and non-skipping PostgreSQL plus real HTTP/router
  integration evidence. Evidence: backend commit
  `a6aee38cedec281d2037741c6ba2dbac5e47840f`, frontend commit
  `27fcaf4a0e7fe90e3cb6ee80a0f2b22de05722cb`, PostgreSQL/HTTP evidence HEAD
  `3abc200c3d7c5525eaded994244af488d0728b41`, CI run `29391281058`.
  Disposable PostgreSQL and real HTTP/router evidence exists for this isolated
  workflow; live external-node bootstrap and onboarding smoke remains open.
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
6. Client delivery share, subscription, email and history.
7. Client route create, update and delete.
8. Client access rotation, revoke, delete and config cleanup.
9. Instance runtime lifecycle.
10. Service-pack instance create.
11. Manual instance create.
12. Instance spec replace.
13. Runtime artifact URL import.
14. Nodes create/edit, diagnostics, inventory, capabilities and discovery.
15. Nodes bootstrap, security and control for disposable nodes.
16. Guided Agent Registration/Onboarding on a disposable external node.
17. Certificates/PKI import preview/apply.
18. Certificates/PKI self-signed, managed CA and issue-from-CA.
19. Certificates/PKI default, revoke, delete and PKI root create.
20. Platform settings save/apply.
21. Mail settings/test.
22. Users, invites and sessions.
23. Backhaul apply, probe, promote and route projection.
24. Route Policy preview, apply and cleanup on disposable topology.

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
API/DB/node smoke and staging operator validation are not complete, the full
final release gate has not been run on a version-synchronized final SHA,
responsive evidence and i18n wording review are still open, and multiple
backend-missing or future-scope sub-actions remain outside the 8.0.0 frontend
cutover.
