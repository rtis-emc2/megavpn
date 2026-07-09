# FE8 8.0.0 Morning Audit

Branch: `release/8.0.0-frontend-console`

Audited HEAD SHA: `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481`

Pre-audit working tree: clean.

Generated UTC: `2026-07-09T04:36:45Z`

CI status for audited HEAD:
[GitHub Actions run 28985369032](https://github.com/rtis-emc2/megavpn/actions/runs/28985369032)
PASS for `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481`.

Final recommendation:

- GO for PR review.
- GO for controlled staging validation.
- NO-GO for final production cutover.

## Repository State

| Check | Result |
| --- | --- |
| Branch | `release/8.0.0-frontend-console` |
| HEAD | `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481` |
| Working tree before docs correction | clean |
| `/legacy/` rollback path | present at `web/legacy` |
| Package manager | npm-only |
| `frontend/package-lock.json` | present |
| `frontend/pnpm-lock.yaml` | absent |

## Overnight Commits

| Task | Commit | Message | Files changed summary | Docs SHA aligned | CI | Status |
| --- | --- | --- | --- | --- | --- | --- |
| FE8-P0-04B-service-packs-evidence-hygiene | `c4a7884d75dea52bb2ecd4e199f23b01e6b08ab8` | `docs: align service pack 8.0.0 evidence` | Acceptance and release docs only. | Yes | [28977495779](https://github.com/rtis-emc2/megavpn/actions/runs/28977495779) PASS | DONE |
| FE8-P0-05A-nodes-observability-diagnostics-inventory-e2e | `5be5a33e16c7eef0578e122f919f9932ef5cbcf0` | `feat: connect node diagnostics inventory workflow` | Nodes UI/tests, typed API/types/hooks, i18n, docs and built web assets. | Yes | [28979061764](https://github.com/rtis-emc2/megavpn/actions/runs/28979061764) PASS | DONE |
| FE8-P0-05B-nodes-bootstrap-security-control-e2e | `7b564e81dd576fbf1de29c7da559090a69debe7a` | `feat: connect node bootstrap security workflows` | Nodes UI/tests, typed API/types/hooks, i18n, docs, backend trust route/store updates and built web assets. | Yes | [28980167212](https://github.com/rtis-emc2/megavpn/actions/runs/28980167212) PASS | DONE |
| FE8-P0-06A-client-routes-access-rotation-config-cleanup-e2e | `0934f97b9da38154b87dada4e1387d54ca7df765` | `feat: connect client access maintenance workflows` | Clients UI/tests, typed API/types/hooks, i18n, docs and built web assets. | Yes | [28982369259](https://github.com/rtis-emc2/megavpn/actions/runs/28982369259) PASS | DONE |
| FE8-P0-07A-platform-certificates-pki-e2e | `b92c78679b60d46bc51f49f94db589ee6e1b0b09` | `feat: connect certificates pki workflows` | Certificates UI/tests, typed API/types/hooks, i18n, docs and built web assets. | Yes | [28983219205](https://github.com/rtis-emc2/megavpn/actions/runs/28983219205) PASS | DONE |
| FE8-P0-07B-platform-settings-mail-users-sessions-e2e | `f94b2bbf6efa1c4fe403ae98865bc5b4da19db70` | `feat: connect platform settings access workflows` | Platform Settings/Mail/Access UI/tests, typed API/types/hooks, i18n, docs and built web assets. | Yes | [28984118898](https://github.com/rtis-emc2/megavpn/actions/runs/28984118898) PASS | DONE |
| FE8-P0-08A-backhaul-route-policy-mutations-e2e | `9ed3965fcdaa18554acf78680bc61317b9108564` | `feat: connect backhaul route policy workflows` | Backhaul/Route Policy UI/tests, typed API/types/hooks, i18n, docs and built web assets. | Yes | [28985121588](https://github.com/rtis-emc2/megavpn/actions/runs/28985121588) PASS | DONE |
| Final overnight report/debt docs | `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481` | `docs: add 8.0.0 overnight batch report` | Night report, remaining debt and acceptance evidence. | Corrected by this morning audit | [28985369032](https://github.com/rtis-emc2/megavpn/actions/runs/28985369032) PASS | DONE |

## Completed Tasks

- FE8-P0-04B service pack evidence hygiene.
- FE8-P0-05A nodes observability/diagnostics/inventory.
- FE8-P0-05B nodes bootstrap/security/control.
- FE8-P0-06A client routes/access rotation/config cleanup.
- FE8-P0-07A certificates/PKI.
- FE8-P0-07B platform settings/mail/users/sessions.
- FE8-P0-08A backhaul/route policy mutations.
- Final overnight report/debt docs.

## Partial Tasks

No overnight task was left partially implemented. The release remains partial
only at the full 8.0.0 cutover level because backend-missing and future-scope
sub-actions remain documented debt.

## Blocked Tasks

Final 8.0.0 production cutover is blocked by version sync, live disposable
smoke, staging operator validation, final release gate, responsive evidence,
i18n wording review and explicit disposition of backend-missing/future-scope
sub-actions.

## Disabled Backend-Missing Sub-Actions

- Generic client edit: no generic `PATCH/PUT /api/v1/clients/{id}` endpoint.
- Client route update: no `PUT/PATCH /api/v1/clients/{id}/routes/{route_id}` endpoint.
- Per-access revoke: no exact per-access revoke endpoint.
- Client delivery history: no client-scoped delivery history list/status endpoint.
- Runtime artifact delete: no binary runtime artifact DELETE endpoint.
- Service pack validation: no separate validation endpoint.
- Instance spec preview/draft-save: no separate preview endpoint or draft-save HTTP route.
- Platform invite revoke: no browser invite revoke endpoint.
- Backhaul dedicated repair: no dedicated repair endpoint.
- Backup/restore browser UI: browser parity remains backend-missing/future scope.

## Future-Scope Sub-Actions

- Non-VLESS access service materialization and access-group migration conflict UI.
- Nodes create/register/edit.
- New SSH access method creation with secret material.
- Manual bootstrap bundle reveal.
- Agent identity revoke, reboot, emergency cleanup and stale rotation cleanup.
- Node service discovery ignore/unignore.
- Platform user status/reset/resend/delete.
- Backhaul link create/delete.

## Functional Matrix Spot-Check

| Domain | Page/component path | API/hooks path | Test path | Status |
| --- | --- | --- | --- | --- |
| VLESS Clients -> Groups | `frontend/src/pages/clients/ClientGroupsPage.tsx` | `frontend/src/shared/api/endpoints.ts`, `frontend/src/shared/query/hooks.ts` | `frontend/src/pages/clients/ClientGroupsPage.test.tsx` | verified |
| Firewall | `frontend/src/pages/network-policy/FirewallPage.tsx` | `frontend/src/shared/api/endpoints.ts`, `frontend/src/shared/query/hooks.ts` | `frontend/src/pages/network-policy/FirewallPage.test.tsx` | verified; rule reorder backend-missing |
| Clients core/artifacts/delivery/maintenance | `frontend/src/pages/clients/ClientsPage.tsx` | `frontend/src/shared/api/endpoints.ts`, `frontend/src/shared/query/hooks.ts` | `frontend/src/pages/clients/ClientsPage.test.tsx` | verified; route update/per-access revoke/delivery history backend-missing |
| Instances runtime control | `frontend/src/pages/services/InstancesPage.tsx` | `frontend/src/shared/api/endpoints.ts`, `frontend/src/shared/query/hooks.ts` | `frontend/src/pages/services/InstancesPage.test.tsx` | verified |
| Service packs/runtime artifacts | `frontend/src/pages/services/ServicePacksPage.tsx`, `frontend/src/pages/services/RuntimeArtifactsPage.tsx` | `frontend/src/shared/api/endpoints.ts`, `frontend/src/shared/query/hooks.ts` | `frontend/src/pages/services/ServiceWorkspace.test.tsx` | verified; runtime artifact delete backend-missing |
| Nodes observability/security/control | `frontend/src/pages/infrastructure/NodesPage.tsx` | `frontend/src/shared/api/endpoints.ts`, `frontend/src/shared/query/hooks.ts` | `frontend/src/pages/infrastructure/NodesPage.test.tsx` | verified; future-scope destructive remediation remains disabled/legacy-only |
| Certificates/PKI | `frontend/src/pages/platform/CertificatesPage.tsx` | `frontend/src/shared/api/endpoints.ts`, `frontend/src/shared/query/hooks.ts` | `frontend/src/pages/platform/CertificatesPage.test.tsx` | verified |
| Platform settings/mail/access | `frontend/src/pages/platform/SettingsPage.tsx`, `frontend/src/pages/platform/MailPage.tsx`, `frontend/src/pages/platform/AccessPage.tsx` | `frontend/src/shared/api/endpoints.ts`, `frontend/src/shared/query/hooks.ts` | `frontend/src/pages/platform/PlatformSettingsAccess.test.tsx` | verified; invite revoke backend-missing |
| Backhaul | `frontend/src/pages/infrastructure/BackhaulPage.tsx` | `frontend/src/shared/api/endpoints.ts`, `frontend/src/shared/query/hooks.ts` | `frontend/src/pages/network-policy/BackhaulRoutePolicyPage.test.tsx` | verified; create/delete future scope |
| Route Policy | `frontend/src/pages/network-policy/RoutePolicyPage.tsx` | `frontend/src/shared/api/endpoints.ts`, `frontend/src/shared/query/hooks.ts` | `frontend/src/pages/network-policy/BackhaulRoutePolicyPage.test.tsx` | verified |

## Checks Run

| Check | Result |
| --- | --- |
| `git status --short` before docs correction | PASS, clean |
| `git branch --show-current` | PASS, `release/8.0.0-frontend-console` |
| `git rev-parse HEAD` | PASS, `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481` |
| `gh run view 28985369032 --json ...` | PASS, conclusion `success` |
| `gofmt -l cmd internal` | PASS |
| `go vet ./...` | PASS |
| `go test ./...` | PASS |
| `go test -race ./...` | PASS |
| `go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin` | PASS |
| `cd frontend && npm ci` | SKIP locally: workstation shell has no `npm` binary (`zsh:1: command not found: npm`). GitHub CI run 28985369032 is authoritative for npm-path CI. |
| `npm run typecheck` equivalent through bundled Node | PASS |
| `npm run lint` equivalent through bundled Node | PASS |
| `npm run test` equivalent through bundled Node | PASS, 10 files / 83 tests |
| `npm run i18n:check` equivalent through bundled Node | PASS, 868 keys |
| `npm run build` equivalent through bundled Node | PASS, Vite chunk-size warning only |
| `scripts/ci/frontend-serving-smoke.sh` | PASS |
| `scripts/ci/frontend-static-guards.sh` | PASS |
| `scripts/ci/docs-consistency.sh` | PASS before and after docs correction |

## Checks Skipped

- Local `cd frontend && npm ci` was skipped because `npm` is absent from the
  workstation shell. The exact failure was `zsh:1: command not found: npm`.
- Live disposable API/DB smoke was not run because no disposable
  `MEGAVPN_PUBLIC_BASE_URL` / `MEGAVPN_API_URL`, target node inventory,
  endpoint domain or seeded database credentials were provided.

## Static Guard Results

- `scripts/ci/frontend-static-guards.sh`: PASS.
- Raw API spot-check: no production React page/app component contains raw
  `fetch('/api/v1...')`, `/api/v1` literals or `dangerouslySetInnerHTML`.
  API paths remain centralized in `frontend/src/shared/api/endpoints.ts` and
  `frontend/src/shared/api/client.ts`; tests/docs may contain literal routes.
- `/legacy/` exists and remains reachable only as rollback navigation:
  `frontend/src/shared/config/navigation.ts`,
  `frontend/src/shared/layout/AppShell.tsx` and `web/legacy`.

## Secret Storage And Logging Review

- `localStorage` production use is limited to `megavpn.apiBase` and UI locale.
- Test files explicitly clear storage and assert one-time enrollment/share
  tokens are not written to `localStorage` or `sessionStorage`.
- No production `console.log/error/warn/debug/info` use was found in
  `frontend/src`; the only console output found is the i18n checker script.
- No unreviewed `dangerouslySetInnerHTML` sink was found in production pages.

## Documentation Consistency Result

Docs drift found and corrected by this morning audit:

- `docs/FRONTEND_ACCEPTANCE_8.0.0.md` still referenced FE8-P0-08A commit
  `9ed3965fcdaa18554acf78680bc61317b9108564` and CI run `28985121588` as the
  latest evidence instead of final overnight evidence
  `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481` / run `28985369032`.
- `docs/FE8_NIGHT_BATCH_REPORT.md` still said
  `Final report commit: pending`; it now records
  `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481`.
- `docs/FE8_REMAINING_DEBT_8.0.0.md` was readable, but has been expanded into
  explicit release-blocking, backend-missing, future-scope, live smoke,
  release-gate, version sync, responsive evidence and PR readiness sections.

## Workflow Matrix Consistency Result

`docs/FRONTEND_WRITE_WORKFLOWS_8.0.0.md`,
`docs/FRONTEND_ENDPOINT_PARITY_8.0.0.md` and `docs/releases/8.0.0.md` are
consistent with the audited code at the domain level:

- Fully connected rows have typed API wrappers in
  `frontend/src/shared/api/endpoints.ts`, query/mutation hooks in
  `frontend/src/shared/query/hooks.ts`, UI entry points and tests.
- Missing exact sub-actions remain `backend-missing`, `legacy-only` or
  `future scope` with a reason.
- No connected workflow was downgraded or converted into fake UI success.

## Remaining Debt Summary

- Version sync from `7.1.1.0` to `8.0.0`.
- Live disposable API/DB smoke.
- Full final release gate on a version-synchronized SHA.
- GitHub Actions Node.js 20 deprecation review.
- Desktop/Pad/Phone responsive evidence.
- English/Russian operator wording review beyond key parity.
- Final static/raw API guard review before release tag.
- Acceptance matrix cleanup after final live checks.
- PR readiness package: summary, release notes diff, CI evidence and rollback
  notes.
- Explicit release-owner decision for backend-missing/future-scope sub-actions.

## Final Decision

Final 8.0.0 cutover is NO-GO because: version metadata still reports
`7.1.1.0`, live disposable API/DB smoke and staging operator validation are
not complete, the full final release gate has not been run on a
version-synchronized final SHA, responsive evidence and i18n wording review
are still open, and multiple backend-missing or future-scope sub-actions remain
explicitly outside the 8.0.0 frontend cutover.
