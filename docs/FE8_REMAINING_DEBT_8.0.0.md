# FE8 Remaining Debt For 8.0.0

Branch: `release/8.0.0-frontend-console`

Generated UTC: `2026-07-09T07:22:36Z`

Final cutover status: **NO-GO** until every required item below is completed or explicitly waived by release owners with a dated rationale.

## Release-Blocking Debt

| Area | Status |
| --- | --- |
| Version sync | OPEN |
| Live disposable smoke | OPEN |
| Full release gate | OPEN |
| Legacy rollback | REQUIRED |
| GitHub Actions Node.js 20 deprecation | OPEN |
| Responsive evidence | OPEN |
| i18n final review | OPEN |
| Static/raw API guard review | OPEN |
| Acceptance matrix cleanup | OPEN |
| PR readiness checklist | OPEN |

Release-blocking details:

- Version sync: update backend, frontend and release metadata to `8.0.0`.
- Live disposable smoke: run all connected workflows against disposable data.
- Full release gate: execute final CI/security gate after version sync.
- Legacy rollback: keep `/legacy/` until cutover is signed off and tested.
- GitHub Actions Node.js 20 deprecation: review and update pinned Actions.
- Responsive evidence: capture desktop, pad and phone workflow evidence.
- i18n final review: review English/Russian operator wording beyond key parity.
- Static/raw API guard review: rerun and inspect guard coverage before tag.
- Acceptance matrix cleanup: replace historical wording after live checks.
- PR readiness checklist: prepare summary, release diff, CI and rollback notes.

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
