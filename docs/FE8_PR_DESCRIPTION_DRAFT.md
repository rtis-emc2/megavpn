# FE8 PR Description Draft

Branch: `release/8.0.0-frontend-console`

Generated UTC: `2026-07-09T10:57:57Z`

PR readiness evidence HEAD before this status-only PR body update:
`6c69879c9c0d5e0a4d182e17140a059f0b62509c`

PR readiness evidence CI before this status-only PR body update:
GitHub Actions run `29013369835` PASS.

## Status

Ready for human review and controlled staging validation.

Not ready for final 8.0.0 production cutover.

Production cutover remains blocked by:

- backend version metadata still reporting `7.1.1.0`;
- full release gate without skips not yet passed;
- live disposable API/DB/node smoke not yet completed;
- responsive evidence not yet attached;
- human i18n wording review still open;
- backend-missing/future-scope deferred items not yet explicitly accepted or
  closed.

## PR Title

MegaVPN Console 8.0.0 frontend migration RC

## Summary

This PR prepares the MegaVPN Console 8.0.0 frontend migration release
candidate for human review and controlled staging validation.

This PR is for review/staging validation, not final 8.0.0 production cutover.

The branch keeps the legacy static console available under `/legacy/` while
the new React console serves the root UI and migrated workflows.

## Migrated Workflows

- `Clients -> Groups -> VLESS`: group create/update, member preview/apply,
  member remove, scope update and sync preview/apply.
- `Clients`: list/detail, create, activate/suspend, revoke/delete, VLESS
  assignment, artifacts, share links, subscriptions and email delivery.
- `Clients -> Routes/Maintenance`: route list/create/delete, access
  list/rotation/delete and generated config cleanup where backend endpoints
  exist.
- `Firewall`: address groups, policies, rules, node preview/apply, node state
  and emergency disable.
- `Instances`: existing runtime control, revisions/rollback, apply/reapply,
  lifecycle actions, diagnostics, delete/force-delete and job tracking.
- `Services`: service pack list/detail/create/update/enable/disable/delete,
  create instance from pack and runtime artifact list/metadata/import.
- `Nodes`: observability, diagnostics, inventory, bootstrap, enrollment tokens,
  SSH session ticket launch, host-key scan/pin, agent token rotation and retire
  workflows where backend endpoints exist.
- `Platform -> Certificates`: certificate list/detail, import preview/apply,
  self-signed create, managed CA create, issue, set default, revoke/delete and
  managed PKI root creation where backend endpoints exist.
- `Platform -> Settings`, `Mail / Delivery` and `Access / RBAC`: TLS/runtime
  settings, mail settings/test, users, invites and sessions where backend
  endpoints exist.
- `Infrastructure -> Backhaul`: link list/detail, apply, probe, transport
  promote and route projection enable/disable where backend endpoints exist.
- `Network Policy -> Route Policy`: list/detail, preview, apply, cleanup and
  job tracking where backend endpoints exist.

## Safety And Security Notes

- No implemented workflow should call `/legacy/`; tests and static guards cover
  the migrated frontend surfaces.
- `/legacy/` remains available as rollback and must not be removed before final
  release-owner cutover approval.
- Dangerous operations use explicit confirmation, backend validation and job
  tracking where applicable.
- Preview/apply flows keep stale preview protection where supported.
- Private keys, SMTP passwords, invite/session secrets, access secrets,
  subscription tokens and generated config payloads are handled as transient
  data and must not be logged or persisted in browser storage.
- Frontend static guards block page-level raw API calls, production console
  logging, unreviewed HTML sinks and unsafe browser token storage.
- RBAC, CSRF, audit and backend validation remain backend-owned controls.

## CI Evidence

- Current HEAD: `24638d090d44bd9d6f3eea53b07d334533333e90`.
- GitHub Actions CI: `29012228790` PASS.
- Local Go checks passed: `gofmt -l cmd internal`, `go vet ./...`,
  `go test ./...`, `go test -race ./...` and
  `go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin`.
- Local frontend checks passed through bundled Node because this workstation
  has no local npm CLI: typecheck, lint, test, i18n parity and build.
- Documentation and static guards passed:
  `scripts/ci/text-lf-guard.sh`, `scripts/ci/docs-markdown-shape.sh`,
  `scripts/ci/docs-consistency.sh`, `scripts/ci/frontend-serving-smoke.sh`,
  `scripts/ci/frontend-static-guards.sh` and `git diff --check`.
- Clean `npm ci` is covered by GitHub CI for the branch because the local
  workstation does not provide npm or corepack.

## Release Gate Status

Release gate status is PARTIAL / OPEN.

The local diagnostic gate is not final production evidence while clean npm
install, disposable DB, backup/restore, API smoke and service matrix inputs are
not all available in the release environment.

## Live Smoke Status

Live disposable smoke is OPEN.

No disposable API, DB, node, endpoint domain or certificate input was available
in this workstation session. Use `docs/FE8_LIVE_SMOKE_PLAN_8.0.0.md` before
any final production cutover decision.

## Remaining Blockers

- Backend binary/version metadata still reports `7.1.1.0`.
- Version tag and release metadata are not synchronized to final `8.0.0`.
- Full production release gate has not passed without skips.
- Live disposable API/DB/node smoke has not run.
- Backup/restore and PostgreSQL disposable integration evidence are missing.
- Responsive desktop/tablet/phone workflow evidence is missing.
- Human English/Russian i18n wording review remains open.
- Backend-missing and future-scope sub-actions remain documented in
  `docs/FE8_REMAINING_DEBT_8.0.0.md`.

## Rollback

`/legacy/` remains served by the Go API and covered by frontend serving smoke.

Rollback for review and staging validation is to keep `/legacy/` enabled and
route operators back to `/legacy/` for any blocked, backend-missing or
non-migrated workflow.

## Staging Validation Plan

1. Deploy this exact branch SHA to a disposable or controlled staging
   environment.
2. Confirm root UI, deep links, `/legacy/`, API non-shadowing and static asset
   behavior.
3. Run migrated workflow smoke with disposable data for Clients, VLESS groups,
   Firewall, Instances, Services, Nodes, Certificates, Settings, Backhaul and
   Route Policy.
4. Verify dangerous operations with least-privilege accounts and expected
   `403`, `409` and `422` backend responses.
5. Capture desktop, tablet and phone evidence for critical operator flows.
6. Run live API/DB/node smoke using `docs/FE8_LIVE_SMOKE_PLAN_8.0.0.md`.
7. Keep `/legacy/` available for rollback throughout the staging window.
8. Attach CI, smoke logs, screenshots, rollback notes and release-owner
   sign-off to the final release evidence package.

## Cutover Decision

PR review: GO.

Controlled staging validation: GO.

Final 8.0.0 production cutover: NO-GO.
