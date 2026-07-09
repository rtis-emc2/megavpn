# FE8 Remaining Debt For 8.0.0

Branch: `release/8.0.0-frontend-console`

Generated UTC: `2026-07-09T04:36:45Z`

Final cutover status: **NO-GO** until every required item below is completed
or explicitly waived by release owners with a dated rationale.

## Release-Blocking Debt

| Area | Required work | Current status |
| --- | --- | --- |
| Version sync | Synchronize backend/frontend/release metadata from `7.1.1.0` to `8.0.0`. | OPEN |
| Live disposable smoke | Run connected VLESS, Firewall, Clients, Instances/Services, Nodes, Certificates/PKI, Platform, Backhaul and Route Policy flows against disposable API/DB data. | OPEN |
| Full release gate | Run the full release gate in the release environment after version sync and live smoke. | OPEN |
| Legacy rollback | Keep `/legacy/` available until final cutover is signed off and rollback has been exercised. | REQUIRED |
| GitHub Actions Node.js 20 deprecation | Review workflow Node runtime warnings and update pinned Actions/runtime versions where required. | OPEN |
| Responsive evidence | Capture Desktop/Pad/Phone manual evidence for critical operator workflows. | OPEN |
| i18n final review | Review English/Russian wording for operator clarity beyond key parity. | OPEN |
| Static/raw API guard review | Re-run and inspect static guard coverage before release tag. | OPEN |
| Acceptance matrix cleanup | Replace historical/pending handoff wording with final release evidence after all live checks. | OPEN |
| PR readiness checklist | Prepare final PR summary, release notes diff, CI evidence and rollback notes. | OPEN |

## Backend-Missing Sub-Actions

| Domain | Sub-action | Required backend or contract decision |
| --- | --- | --- |
| Clients | Generic edit | No generic `PATCH/PUT /api/v1/clients/{id}` endpoint. |
| Clients | Route update | No `PUT/PATCH /api/v1/clients/{id}/routes/{route_id}` endpoint. |
| Clients | Per-access revoke | Backend supports client-level revoke and service-access delete, not exact per-access revoke. |
| Clients | Delivery history | No client-scoped delivery history list/status endpoint. |
| Services | Runtime artifact delete | No binary runtime artifact DELETE endpoint. |
| Services | Service pack validation | No separate validation endpoint; create/update uses backend validation directly. |
| Instances | Spec preview/draft-save | No separate preview endpoint or draft-save HTTP route. |
| Platform Access | Invite revoke | No browser backend endpoint for invite revoke. |
| Backhaul | Dedicated repair action | No dedicated repair endpoint; UI exposes apply/probe/promote/route state only. |
| Operations | Backup/restore browser UI | Browser parity endpoint/UX is not implemented for this release candidate. |

## Future-Scope Sub-Actions

| Domain | Sub-action | Current release decision |
| --- | --- | --- |
| Clients -> Groups | Non-VLESS materialization | Connected workflow is VLESS-only; non-VLESS services remain catalog-only/future. |
| Clients -> Groups | Migration conflict UI | Backend inventory exists, UI remains future scope. |
| Nodes | Create/register/edit | Not migrated in FE8-P0-05A/05B. |
| Nodes | New SSH access method with secret material | Not exposed to avoid browser secret handling without a reviewed flow. |
| Nodes | Manual bootstrap bundle reveal | Not exposed; one-time bootstrap values remain backend-controlled. |
| Nodes | Agent identity revoke, reboot, emergency cleanup, stale rotation cleanup | Destructive remediation remains future scope/legacy-only. |
| Nodes | Service discovery ignore/unignore | Not migrated in FE8-P0-05A. |
| Platform Access | User lifecycle mutations | Status change, reset-password, resend-invite and delete remain future scope. |
| Backhaul | Link create/delete | Backend routes exist, but new console exposes list/detail/actions only after FE8-P0-08A. |

## Live Smoke Plan

Minimum disposable smoke coverage before final release decision:

1. VLESS group create/edit/scope/member preview/apply/remove/sync.
2. Firewall address group/policy/rule CRUD, preview, apply and emergency
   disable on disposable nodes.
3. Client create/status/revoke/delete, artifacts build/download/delete,
   delivery share/subscription/email, route create/delete, access
   rotation/delete and config cleanup.
4. Instance runtime lifecycle, service-pack instance create, manual create,
   spec replace and runtime artifact URL import.
5. Nodes diagnostics/inventory/capabilities/service discovery plus
   bootstrap/security/control for configured disposable nodes.
6. Certificates/PKI import preview/apply, self-signed, managed CA,
   issue-from-CA, default/revoke/delete and PKI root create.
7. Platform settings save/apply, mail settings/test, users/invites/sessions.
8. Backhaul apply/probe/promote/route projection and Route Policy
   preview/apply/cleanup on disposable topology.

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
3. Re-run `go test ./...`, `go test -race ./...`, frontend build/tests and
   `scripts/ci/docs-consistency.sh`.
4. Confirm release notes and acceptance evidence reference the synchronized
   version on the final SHA.

## Responsive Evidence Plan

Required manual screenshots or recorded checks:

1. Desktop operator viewport for VLESS, Firewall, Clients, Nodes, Certificates,
   Platform, Backhaul and Route Policy.
2. Tablet/pad viewport for the same high-risk workflow entry points.
3. Phone/narrow viewport for navigation, destructive confirmations,
   one-time-secret panels and job tracking.
4. Confirmation that text does not overlap, destructive controls remain
   reachable and secret material is not exposed.

## PR Readiness Checklist

- Branch is pushed to `origin/release/8.0.0-frontend-console`.
- `/legacy/` rollback path exists and remains documented.
- Latest GitHub Actions CI run is green on the evidence SHA.
- Package manager remains npm-only: `frontend/package-lock.json` present and
  `frontend/pnpm-lock.yaml` absent.
- PR summary lists connected workflows, backend-missing sub-actions, release
  blockers and rollback plan.
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
