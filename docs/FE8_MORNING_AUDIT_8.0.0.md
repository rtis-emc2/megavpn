# FE8 8.0.0 Morning Audit

Branch: `release/8.0.0-frontend-console`

Audited HEAD SHA:
`4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481`

Morning audit commit:
`e1677b35f3682d5fbff6a417178cfd15cbabb0b3`

Pre-audit working tree: clean.

Generated UTC: `2026-07-09T04:36:45Z`

CI status for audited HEAD:
[GitHub Actions run 28985369032][ci-28985369032] PASS for
`4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481`.

Final recommendation:

- GO for PR review.
- GO for controlled staging validation.
- NO-GO for final production cutover.

## Repository State

| Check | Result |
| --- | --- |
| Branch | `release/8.0.0-frontend-console` |
| Audited HEAD | `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481` |
| Morning audit commit | `e1677b35f3682d5fbff6a417178cfd15cbabb0b3` |
| Working tree before docs correction | clean |
| `/legacy/` rollback path | present at `web/legacy` |
| Package manager | npm-only |
| `frontend/package-lock.json` | present |
| `frontend/pnpm-lock.yaml` | absent |

## Overnight Commits

| Task | Commit | CI | Status |
| --- | --- | --- | --- |
| FE8-P0-04B service pack evidence hygiene | `c4a7884d75dea52bb2ecd4e199f23b01e6b08ab8` | [28977495779][ci-28977495779] PASS | DONE |
| FE8-P0-05A nodes diagnostics inventory | `5be5a33e16c7eef0578e122f919f9932ef5cbcf0` | [28979061764][ci-28979061764] PASS | DONE |
| FE8-P0-05B nodes bootstrap security | `7b564e81dd576fbf1de29c7da559090a69debe7a` | [28980167212][ci-28980167212] PASS | DONE |
| FE8-P0-06A client maintenance | `0934f97b9da38154b87dada4e1387d54ca7df765` | [28982369259][ci-28982369259] PASS | DONE |
| FE8-P0-07A certificates/PKI | `b92c78679b60d46bc51f49f94db589ee6e1b0b09` | [28983219205][ci-28983219205] PASS | DONE |
| FE8-P0-07B platform settings/access | `f94b2bbf6efa1c4fe403ae98865bc5b4da19db70` | [28984118898][ci-28984118898] PASS | DONE |
| FE8-P0-08A backhaul/route policy | `9ed3965fcdaa18554acf78680bc61317b9108564` | [28985121588][ci-28985121588] PASS | DONE |
| Final overnight report/debt docs | `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481` | [28985369032][ci-28985369032] PASS | DONE |

## Commit Details

- FE8-P0-04B:
  - Message: `docs: align service pack 8.0.0 evidence`.
  - Files: acceptance and release docs only.
  - Docs SHA alignment: yes.

- FE8-P0-05A:
  - Message: `feat: connect node diagnostics inventory workflow`.
  - Files: Nodes UI/tests, typed API/types/hooks, i18n, docs and built assets.
  - Docs SHA alignment: yes.

- FE8-P0-05B:
  - Message: `feat: connect node bootstrap security workflows`.
  - Files: Nodes UI/tests, typed API/types/hooks, i18n, docs, backend trust route/store updates and built assets.
  - Docs SHA alignment: yes.

- FE8-P0-06A:
  - Message: `feat: connect client access maintenance workflows`.
  - Files: Clients UI/tests, typed API/types/hooks, i18n, docs and built assets.
  - Docs SHA alignment: yes.

- FE8-P0-07A:
  - Message: `feat: connect certificates pki workflows`.
  - Files: Certificates UI/tests, typed API/types/hooks, i18n, docs and built assets.
  - Docs SHA alignment: yes.

- FE8-P0-07B:
  - Message: `feat: connect platform settings access workflows`.
  - Files: Platform Settings/Mail/Access UI/tests, typed API/types/hooks, i18n, docs and built assets.
  - Docs SHA alignment: yes.

- FE8-P0-08A:
  - Message: `feat: connect backhaul route policy workflows`.
  - Files: Backhaul/Route Policy UI/tests, typed API/types/hooks, i18n, docs and built assets.
  - Docs SHA alignment: yes.

- Final overnight report:
  - Message: `docs: add 8.0.0 overnight batch report`.
  - Files: night report, remaining debt and acceptance evidence.
  - Docs SHA alignment: corrected by the morning audit commit.

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

No overnight task was left partially implemented.

The release remains partial only at the full 8.0.0 cutover level because
backend-missing and future-scope sub-actions remain documented debt.

## Blocked Tasks

Final 8.0.0 production cutover is blocked by:

- version sync;
- live disposable smoke;
- staging operator validation;
- final release gate;
- responsive evidence;
- i18n wording review;
- explicit disposition of backend-missing and future-scope sub-actions.

## Disabled Backend-Missing Sub-Actions

- Generic client edit:
  no generic `PATCH/PUT /api/v1/clients/{id}` endpoint.
- Client route update:
  no `PUT/PATCH /api/v1/clients/{id}/routes/{route_id}` endpoint.
- Per-access revoke:
  no exact per-access revoke endpoint.
- Client delivery history:
  no client-scoped delivery history list/status endpoint.
- Runtime artifact delete:
  no binary runtime artifact DELETE endpoint.
- Service pack validation:
  no separate validation endpoint.
- Instance spec preview/draft-save:
  no separate preview endpoint or draft-save HTTP route.
- Platform invite revoke:
  no browser invite revoke endpoint.
- Backhaul dedicated repair:
  no dedicated repair endpoint.
- Backup/restore browser UI:
  browser parity remains backend-missing/future scope.

## Future-Scope Sub-Actions

- Non-VLESS access service materialization.
- Access-group migration conflict UI.
- Nodes create/register/edit.
- New SSH access method creation with secret material.
- Manual bootstrap bundle reveal.
- Agent identity revoke.
- Node reboot.
- Node emergency cleanup.
- Node stale rotation cleanup.
- Node service discovery ignore/unignore.
- Platform user status/reset/resend/delete.
- Backhaul link create/delete.

## Functional Matrix Spot-Check

| Domain | Status |
| --- | --- |
| VLESS Clients -> Groups | verified |
| Firewall | verified; rule reorder backend-missing |
| Clients core/artifacts/delivery/maintenance | verified; selected sub-actions backend-missing |
| Instances runtime control | verified |
| Service packs/runtime artifacts | verified; runtime artifact delete backend-missing |
| Nodes observability/security/control | verified; destructive remediation remains disabled/legacy-only |
| Certificates/PKI | verified |
| Platform settings/mail/access | verified; invite revoke backend-missing |
| Backhaul | verified; create/delete future scope |
| Route Policy | verified |

## Functional Matrix Evidence Paths

- VLESS Clients -> Groups:
  - UI: `frontend/src/pages/clients/ClientGroupsPage.tsx`.
  - API/hooks: `frontend/src/shared/api/endpoints.ts`,
    `frontend/src/shared/query/hooks.ts`.
  - Tests: `frontend/src/pages/clients/ClientGroupsPage.test.tsx`.

- Firewall:
  - UI: `frontend/src/pages/network-policy/FirewallPage.tsx`.
  - API/hooks: `frontend/src/shared/api/endpoints.ts`,
    `frontend/src/shared/query/hooks.ts`.
  - Tests: `frontend/src/pages/network-policy/FirewallPage.test.tsx`.

- Clients core/artifacts/delivery/maintenance:
  - UI: `frontend/src/pages/clients/ClientsPage.tsx`.
  - API/hooks: `frontend/src/shared/api/endpoints.ts`,
    `frontend/src/shared/query/hooks.ts`.
  - Tests: `frontend/src/pages/clients/ClientsPage.test.tsx`.

- Instances runtime control:
  - UI: `frontend/src/pages/services/InstancesPage.tsx`.
  - API/hooks: `frontend/src/shared/api/endpoints.ts`,
    `frontend/src/shared/query/hooks.ts`.
  - Tests: `frontend/src/pages/services/InstancesPage.test.tsx`.

- Service packs/runtime artifacts:
  - UI: `frontend/src/pages/services/ServicePacksPage.tsx`,
    `frontend/src/pages/services/RuntimeArtifactsPage.tsx`.
  - API/hooks: `frontend/src/shared/api/endpoints.ts`,
    `frontend/src/shared/query/hooks.ts`.
  - Tests: `frontend/src/pages/services/ServiceWorkspace.test.tsx`.

- Nodes observability/security/control:
  - UI: `frontend/src/pages/infrastructure/NodesPage.tsx`.
  - API/hooks: `frontend/src/shared/api/endpoints.ts`,
    `frontend/src/shared/query/hooks.ts`.
  - Tests: `frontend/src/pages/infrastructure/NodesPage.test.tsx`.

- Certificates/PKI:
  - UI: `frontend/src/pages/platform/CertificatesPage.tsx`.
  - API/hooks: `frontend/src/shared/api/endpoints.ts`,
    `frontend/src/shared/query/hooks.ts`.
  - Tests: `frontend/src/pages/platform/CertificatesPage.test.tsx`.

- Platform settings/mail/access:
  - UI: `frontend/src/pages/platform/SettingsPage.tsx`,
    `frontend/src/pages/platform/MailPage.tsx`,
    `frontend/src/pages/platform/AccessPage.tsx`.
  - API/hooks: `frontend/src/shared/api/endpoints.ts`,
    `frontend/src/shared/query/hooks.ts`.
  - Tests: `frontend/src/pages/platform/PlatformSettingsAccess.test.tsx`.

- Backhaul:
  - UI: `frontend/src/pages/infrastructure/BackhaulPage.tsx`.
  - API/hooks: `frontend/src/shared/api/endpoints.ts`,
    `frontend/src/shared/query/hooks.ts`.
  - Tests: `frontend/src/pages/network-policy/BackhaulRoutePolicyPage.test.tsx`.

- Route Policy:
  - UI: `frontend/src/pages/network-policy/RoutePolicyPage.tsx`.
  - API/hooks: `frontend/src/shared/api/endpoints.ts`,
    `frontend/src/shared/query/hooks.ts`.
  - Tests: `frontend/src/pages/network-policy/BackhaulRoutePolicyPage.test.tsx`.

## Checks Run

| Check | Result |
| --- | --- |
| `git status --short` before docs correction | PASS, clean |
| `git branch --show-current` | PASS |
| `git rev-parse HEAD` | PASS |
| `gh run view 28985369032 --json ...` | PASS |
| `gofmt -l cmd internal` | PASS |
| `go vet ./...` | PASS |
| `go test ./...` | PASS |
| `go test -race ./...` | PASS |
| `go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin` | PASS |
| `cd frontend && npm ci` | SKIP locally |
| `npm run typecheck` equivalent through bundled Node | PASS |
| `npm run lint` equivalent through bundled Node | PASS |
| `npm run test` equivalent through bundled Node | PASS |
| `npm run i18n:check` equivalent through bundled Node | PASS |
| `npm run build` equivalent through bundled Node | PASS |
| `scripts/ci/frontend-serving-smoke.sh` | PASS |
| `scripts/ci/frontend-static-guards.sh` | PASS |
| `scripts/ci/docs-consistency.sh` | PASS before and after docs correction |

Check evidence details:

- Branch check returned `release/8.0.0-frontend-console`.
- `git rev-parse HEAD` returned
  `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481`.
- GitHub run `28985369032` had conclusion `success`.
- Frontend tests passed with 10 files and 83 tests.
- i18n parity passed with 868 keys.
- Build passed with a Vite chunk-size warning only.

## Checks Skipped

- Local `cd frontend && npm ci` was skipped because `npm` is absent from the
  workstation shell.
- Exact local failure: `zsh:1: command not found: npm`.
- GitHub CI run `28985369032` is authoritative for the npm-path CI.
- Live disposable API/DB smoke was not run because no disposable
  `MEGAVPN_PUBLIC_BASE_URL`, `MEGAVPN_API_URL`, target node inventory,
  endpoint domain or seeded database credentials were provided.

## Static Guard Results

- `scripts/ci/frontend-static-guards.sh`: PASS.
- Raw API spot-check: PASS.
- No production React page/app component contains raw `fetch('/api/v1...')`.
- No production React page/app component contains `/api/v1` literals.
- No production React page/app component contains `dangerouslySetInnerHTML`.
- API paths remain centralized in `frontend/src/shared/api/endpoints.ts` and
  `frontend/src/shared/api/client.ts`.
- Tests and docs may contain literal routes.
- `/legacy/` exists and remains reachable only as rollback navigation through:
  - `frontend/src/shared/config/navigation.ts`;
  - `frontend/src/shared/layout/AppShell.tsx`;
  - `web/legacy`.

## Secret Storage And Logging Review

- `localStorage` production use is limited to `megavpn.apiBase` and UI locale.
- Test files explicitly clear storage.
- Tests assert one-time enrollment/share tokens are not written to
  `localStorage` or `sessionStorage`.
- No production `console.log/error/warn/debug/info` use was found in
  `frontend/src`.
- The only console output found is the i18n checker script.
- No unreviewed `dangerouslySetInnerHTML` sink was found in production pages.

## Documentation Consistency Result

Docs drift found and corrected by this morning audit:

- `docs/FRONTEND_ACCEPTANCE_8.0.0.md` still referenced FE8-P0-08A commit
  `9ed3965fcdaa18554acf78680bc61317b9108564` as latest evidence.
- The same acceptance header still referenced CI run `28985121588` as latest
  evidence.
- The actual final overnight evidence is
  `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481` and run `28985369032`.
- `docs/FE8_NIGHT_BATCH_REPORT.md` still said
  `Final report commit: pending`.
- The night report now records
  `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481`.
- `docs/FE8_REMAINING_DEBT_8.0.0.md` was readable but needed explicit
  release-blocking, backend-missing, future-scope, live smoke, release-gate,
  version sync, responsive evidence and PR readiness sections.

## Workflow Matrix Consistency Result

`docs/FRONTEND_WRITE_WORKFLOWS_8.0.0.md`,
`docs/FRONTEND_ENDPOINT_PARITY_8.0.0.md` and `docs/releases/8.0.0.md` are
consistent with the audited code at the domain level.

Verified consistency points:

- Fully connected rows have typed API wrappers in
  `frontend/src/shared/api/endpoints.ts`.
- Fully connected rows have query/mutation hooks in
  `frontend/src/shared/query/hooks.ts`.
- Fully connected rows have UI entry points and tests.
- Missing exact sub-actions remain `backend-missing`, `legacy-only` or
  `future scope` with a reason.
- No connected workflow was downgraded.
- No connected workflow was converted into fake UI success.

## Remaining Debt Summary

- Version sync from `7.1.1.0` to `8.0.0`.
- Live disposable API/DB smoke.
- Full final release gate on a version-synchronized SHA.
- GitHub Actions Node.js 20 deprecation review.
- Desktop/Pad/Phone responsive evidence.
- English/Russian operator wording review beyond key parity.
- Final static/raw API guard review before release tag.
- Acceptance matrix cleanup after final live checks.
- PR readiness package:
  - summary;
  - release notes diff;
  - CI evidence;
  - rollback notes.
- Explicit release-owner decision for backend-missing/future-scope sub-actions.

## Final Decision

Final 8.0.0 cutover is NO-GO because: version metadata still reports
`7.1.1.0`, live disposable API/DB smoke and staging operator validation are
not complete, the full final release gate has not been run on a
version-synchronized final SHA, responsive evidence and i18n wording review
are still open, and multiple backend-missing or future-scope sub-actions remain
explicitly outside the 8.0.0 frontend cutover.

[ci-28977495779]: https://github.com/rtis-emc2/megavpn/actions/runs/28977495779
[ci-28979061764]: https://github.com/rtis-emc2/megavpn/actions/runs/28979061764
[ci-28980167212]: https://github.com/rtis-emc2/megavpn/actions/runs/28980167212
[ci-28982369259]: https://github.com/rtis-emc2/megavpn/actions/runs/28982369259
[ci-28983219205]: https://github.com/rtis-emc2/megavpn/actions/runs/28983219205
[ci-28984118898]: https://github.com/rtis-emc2/megavpn/actions/runs/28984118898
[ci-28985121588]: https://github.com/rtis-emc2/megavpn/actions/runs/28985121588
[ci-28985369032]: https://github.com/rtis-emc2/megavpn/actions/runs/28985369032
