# FE8 Remaining Debt For 8.0.0

Branch: `release/8.0.0-frontend-console`

Generated UTC: `2026-07-09T04:36:45Z`

Final cutover status: **NO-GO** until every required item below is completed
or explicitly waived by release owners with a dated rationale.

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

- Version sync:
  synchronize backend/frontend/release metadata from `7.1.1.0` to `8.0.0`.
- Live disposable smoke:
  run connected VLESS, Firewall, Clients, Instances/Services, Nodes,
  Certificates/PKI, Platform, Backhaul and Route Policy flows against
  disposable API/DB data.
- Full release gate:
  run the full release gate in the release environment after version sync and
  live smoke.
- Legacy rollback:
  keep `/legacy/` available until final cutover is signed off and rollback has
  been exercised.
- GitHub Actions Node.js 20 deprecation:
  review workflow Node runtime warnings and update pinned Actions/runtime
  versions where required.
- Responsive evidence:
  capture Desktop/Pad/Phone manual evidence for critical operator workflows.
- i18n final review:
  review English/Russian wording for operator clarity beyond key parity.
- Static/raw API guard review:
  re-run and inspect static guard coverage before release tag.
- Acceptance matrix cleanup:
  replace historical/pending handoff wording with final release evidence after
  all live checks.
- PR readiness checklist:
  prepare final PR summary, release notes diff, CI evidence and rollback notes.

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

- Generic client edit:
  no generic `PATCH/PUT /api/v1/clients/{id}` endpoint.
- Client route update:
  no `PUT/PATCH /api/v1/clients/{id}/routes/{route_id}` endpoint.
- Per-access revoke:
  backend supports client-level revoke and service-access delete, not exact
  per-access revoke.
- Client delivery history:
  no client-scoped delivery history list/status endpoint.
- Runtime artifact delete:
  no binary runtime artifact DELETE endpoint.
- Service pack validation:
  no separate validation endpoint; create/update uses backend validation
  directly.
- Instance spec preview/draft-save:
  no separate preview endpoint or draft-save HTTP route.
- Platform invite revoke:
  no browser backend endpoint for invite revoke.
- Backhaul dedicated repair action:
  no dedicated repair endpoint; UI exposes apply/probe/promote/route state.
- Backup/restore browser UI:
  browser parity endpoint/UX is not implemented for this release candidate.

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

- Non-VLESS materialization:
  connected workflow is VLESS-only; non-VLESS services remain catalog-only.
- Migration conflict UI:
  backend inventory exists, UI remains future scope.
- Nodes create/register/edit:
  not migrated in FE8-P0-05A/05B.
- New SSH access method with secret material:
  not exposed to avoid browser secret handling without a reviewed flow.
- Manual bootstrap bundle reveal:
  not exposed; one-time bootstrap values remain backend-controlled.
- Agent identity revoke, reboot, emergency cleanup and stale rotation cleanup:
  destructive remediation remains future scope or legacy-only.
- Service discovery ignore/unignore:
  not migrated in FE8-P0-05A.
- Platform user lifecycle mutations:
  status change, reset-password, resend-invite and delete remain future scope.
- Backhaul link create/delete:
  backend routes exist, but new console exposes list/detail/actions only after
  FE8-P0-08A.

## Live Smoke Plan

Minimum disposable smoke coverage before final release decision:

1. VLESS group create/edit/scope/member preview/apply/remove/sync.
2. Firewall address group/policy/rule CRUD, preview, apply and emergency
   disable on disposable nodes.
3. Client create/status/revoke/delete.
4. Client artifacts build/download/delete.
5. Client delivery share/subscription/email.
6. Client route create/delete.
7. Client access rotation/delete and config cleanup.
8. Instance runtime lifecycle.
9. Service-pack instance create.
10. Manual instance create.
11. Instance spec replace.
12. Runtime artifact URL import.
13. Nodes diagnostics/inventory/capabilities/service discovery.
14. Nodes bootstrap/security/control for configured disposable nodes.
15. Certificates/PKI import preview/apply.
16. Certificates/PKI self-signed, managed CA and issue-from-CA.
17. Certificates/PKI default/revoke/delete and PKI root create.
18. Platform settings save/apply.
19. Mail settings/test.
20. Users/invites/sessions.
21. Backhaul apply/probe/promote/route projection.
22. Route Policy preview/apply/cleanup on disposable topology.

## Release-Gate Plan

1. Freeze a final candidate SHA after version sync and live smoke.
2. Run the full backend/frontend/security CI gate on that SHA.
3. Re-run local smoke/guard scripts from a clean worktree.
4. Attach CI links, live smoke logs and rollback notes to the PR/release
   evidence.
5. Require release owner sign-off before removing any `/legacy/` dependency.

## Version Sync Plan

1. Update backend binary/version metadata from `7.1.1.0` to `8.0.0`.
2. Update frontend visible version/release metadata if it is derived outside
   the backend `/api/v1/version` response.
3. Re-run `go test ./...`.
4. Re-run `go test -race ./...`.
5. Re-run frontend build/tests.
6. Re-run `scripts/ci/docs-consistency.sh`.
7. Confirm release notes and acceptance evidence reference the synchronized
   version on the final SHA.

## Responsive Evidence Plan

Required manual screenshots or recorded checks:

1. Desktop operator viewport for VLESS, Firewall, Clients, Nodes, Certificates,
   Platform, Backhaul and Route Policy.
2. Tablet/pad viewport for the same high-risk workflow entry points.
3. Phone/narrow viewport for navigation, destructive confirmations,
   one-time-secret panels and job tracking.
4. Confirmation that text does not overlap.
5. Confirmation that destructive controls remain reachable.
6. Confirmation that secret material is not exposed.

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
- Do not expose auth/session/bearer/bootstrap/share/subscription/enrollment
  tokens in browser storage.
- Do not log secrets, private keys, generated configs, credentials, one-time
  URLs, tokens or runtime configs.
- Do not render backend logs/errors/diagnostics/specs/cert material/routes as
  HTML.
- Keep RBAC, CSRF, signed jobs, audit logging and backend validation as the
  source of truth.
- Keep preview-before-apply where the backend requires or benefits from it:
  Firewall, VLESS bulk membership/sync, certificate import and Route Policy.
- Keep missing exact sub-actions disabled with literal backend reason instead
  of fake success.

## Final Cutover Decision

NO-GO because version metadata still reports `7.1.1.0`, live disposable
API/DB smoke and staging operator validation are not complete, the full final
release gate has not been run on a version-synchronized final SHA, responsive
evidence and i18n wording review are still open, and multiple backend-missing
or future-scope sub-actions remain explicitly outside the 8.0.0 frontend
cutover.
