# RTIS MegaVPN Frontend Acceptance 8.0.0

Branch: `release/8.0.0-frontend-console`

Latest FE8-P0-03A feature/evidence commit:
`326499a6691833dc3d9be406cf9e84a91544a358`

VLESS implementation commit:
`f33070aee76e9fb11100a5ea954fded09c0d4a10`

FE8-RC1 hygiene implementation commit:
`9044647110cd5cbaeb4d5a866b96f56008fcb338`

Firewall implementation commit:
`5bfae8dfd629592dfa44aec9a3cea8b1db4b2c47`

Firewall evidence alignment commit:
`d0c6af9db88018c5cae14be4542b453a310b658f`

Previous accepted evidence CI:
GitHub Actions run `28967364873` passed for
`f1d8769b63cdfe705afb29a71b8927d4c4abe147`.

Current evidence date UTC: `2026-07-08T19:49:21Z`

Status: FE8-P0-03A is locally verified and reviewable. Final 8.0.0 cutover
remains NO-GO until the remaining non-migrated workflows, live/staging operator
validation and backend version synchronization are complete.

VLESS is connected in the new UI without `/legacy/`. Firewall is connected in
the new UI without `/legacy/`. Clients core, single-client VLESS assignment and
client artifacts are connected in the new UI without `/legacy/`. Remaining
workflows listed below are still not migrated.

## 1. Summary

This evidence records the current 8.0.0 frontend branch after FE8-P0-03A:

- CI push coverage includes `release/8.0.0-frontend-console` and `release/**`;
  pull request coverage remains enabled.
- The frontend package manager standard is `npm`: `package-lock.json` is kept,
  `pnpm-lock.yaml` is not used, and CI/docs use `npm ci` / `npm run ...`.
- `Clients -> Groups -> VLESS` is connected without `/legacy/` for create/edit,
  member preview/apply/remove, scope update and sync preview/apply.
- `Firewall` address groups, policies, rules, node preview/apply/state and
  emergency disable are connected without `/legacy/`.
- `Clients` core workspace is connected without `/legacy/` for list/detail,
  create, activate/suspend, revoke/delete, single-client VLESS group
  preview/apply/remove, artifact list/build/download/delete and client-scoped
  job tracking.
- Share links, subscriptions and email delivery remain deferred to FE8-P0-03B.
- `/legacy/` remains the rollback UI and still covers non-migrated workflows.

## 2. Commands Run

| Check | Status | Evidence |
| --- | --- | --- |
| `gofmt -l cmd internal` | PASS | No files listed. |
| `go vet ./...` | PASS | No vet findings. |
| `go test ./...` | PASS | All Go package tests pass. |
| `go test -race ./...` | PASS | Race detector tests pass. |
| `go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin` | PASS | All operational binaries build. |
| `cd frontend && npm ci` | PASS | npm `11.7.0`; 251 packages installed; audit found 0 vulnerabilities. |
| `cd frontend && npm run typecheck` | PASS | TypeScript checks pass. |
| `cd frontend && npm run lint` | PASS | ESLint passes. |
| `cd frontend && npm run test` | PASS | Vitest: 4 files, 38 tests passed. |
| `cd frontend && npm run i18n:check` | PASS | i18n key parity ok: 365 keys. |
| `cd frontend && npm run build` | PASS | Vite build wrote `web/index.html`, `web/.vite/manifest.json`, `web/assets/index-BcepIhBb.js`, `web/assets/index-B_z-3Pv-.css`. |
| `scripts/ci/frontend-serving-smoke.sh` | PASS | Root/deep links/legacy/API non-shadowing/static asset 404 contract holds. |
| `scripts/ci/frontend-static-guards.sh` | PASS | Static frontend security guards pass. |
| `scripts/ci/docs-consistency.sh` | PASS | Documentation consistency ok for backend release `7.1.1.0`. |
| `scripts/smoke/vless-client-access-groups-smoke.sh` | SKIP | No `MEGAVPN_PUBLIC_BASE_URL` or `MEGAVPN_API_URL` was provided for a disposable/local API. |

Local note: this workstation did not expose a native `npm` binary in `PATH`;
frontend checks were run through npm CLI `11.7.0` on the bundled Node runtime.
The repository standard and GitHub CI path remain plain `npm ci` and
`npm run ...`.

## 3. Clients Core Test Evidence

`frontend/src/pages/clients/ClientsPage.test.tsx` verifies FE8-P0-03A against
mocked backend API responses:

| Required behavior | Test evidence |
| --- | --- |
| Clients page loads list | `loads the client list and opens a real detail drawer`; asserts `GET /api/v1/clients`. |
| Client detail loads | Same test asserts `GET /api/v1/clients/{id}` and renders metadata. |
| Create client mutation | `creates clients through the backend and handles 409 and 422 responses`; asserts `POST /api/v1/clients`. |
| Conflict/validation handling | Same test renders `409` conflict and maps a `422` username field error. |
| Status action | `runs status, revoke and delete actions only through confirmed backend mutations`; asserts `POST /suspend`. |
| Delete/revoke confirmation | Same test confirms before `POST /revoke` and `DELETE /clients/{id}`. |
| Current VLESS group visible | `assigns single-client VLESS access through backend preview and apply`; renders current VLESS group. |
| VLESS preview endpoint | Same test asserts `POST /client-access-groups/{group_id}/members:preview`. |
| Preview enables Apply | Same test verifies Apply becomes enabled only after successful preview. |
| Stale preview disables Apply | Same test changes mode after preview and verifies Apply disables. |
| VLESS apply endpoint | Same test asserts `POST /client-access-groups/{group_id}/members:bulk-apply`. |
| Remove VLESS membership | `removes VLESS membership only after confirmation`; asserts backend member `DELETE`. |
| Artifact list/build/download/delete | `manages client artifacts through backend endpoints without logging tokens`; asserts list, build, download URL open and delete. |
| Job tracking | Revoke/artifact tests render job tracking for returned job IDs. |
| Permission handling | `shows permission errors safely and does not call /legacy`; renders `403` text. |
| No `/legacy` workflow | Tests assert no request path starts with `/legacy`. |
| No raw page API calls | `keeps Clients page free of raw API calls`; verifies no `/api/v1` string or raw `fetch` in the page component. |
| i18n coverage | `npm run i18n:check` passed with 365 matching keys. |

Clients core workflow works in the new UI without `/legacy/`.

Single-client VLESS access assignment works in the new UI without `/legacy/`.

Client artifacts workflow works in the new UI without `/legacy/`.

## 4. VLESS Groups Test Evidence

`frontend/src/pages/clients/ClientGroupsPage.test.tsx` verifies the migrated
VLESS group workflow against mocked backend API responses:

| Required behavior | Test evidence |
| --- | --- |
| Create VLESS group | `creates VLESS groups through the client access group API`; asserts `POST /api/v1/client-access-groups`. |
| Update VLESS group policy/status | `updates VLESS group policy and status through the client access group API`; asserts `PATCH /api/v1/client-access-groups/{group_id}`. |
| Member preview/apply | `previews and applies VLESS membership with backend bulk endpoints`; asserts `/members:preview` and `/members:bulk-apply`. |
| Member remove | `removes VLESS group members through the backend member delete endpoint`; asserts `DELETE /members/{client_id}`. |
| Scope update | `updates VLESS group scope through the backend scope endpoint`; asserts `PATCH /scope`. |
| Sync preview/apply | `previews and applies VLESS group sync with backend sync endpoints`; asserts `/sync:preview` and `/sync:apply`. |
| Preview stale disables apply | `invalidates VLESS membership preview and disables apply when selection inputs change`; apply remains disabled after mode change. |
| No `/legacy` calls | Every VLESS workflow test asserts no request path starts with `/legacy`. |

## 5. Firewall Test Evidence

`frontend/src/pages/network-policy/FirewallPage.test.tsx` verifies the migrated
Firewall workflow against mocked backend API responses:

| Required behavior | Test evidence |
| --- | --- |
| Load policies/address groups | `loads policies and address groups from mocked API`; asserts `GET /api/v1/firewall`. |
| Address group create/update/delete | Tests assert `POST`, `PUT` and `DELETE /api/v1/firewall/address-lists`. |
| DNS-only/empty group warnings | `shows DNS-only and empty renderable address group warnings`. |
| Rule create/update/delete | Tests assert `POST`, `PUT` and `DELETE /api/v1/firewall/policies/{id}/rules`. |
| Preview/apply guards | Tests cover disabled preview, successful preview, stale preview and blocking preview errors. |
| Apply confirmation/job | Tests assert confirmation, real `POST /api/v1/nodes/{id}/firewall/apply` and job link. |
| Emergency disable | Test asserts confirmation text and real `POST /api/v1/nodes/{id}/firewall/disable`. |
| 403/422/409 handling | Tests assert distinct permission, validation and conflict messages. |
| Safe rendered output | `renders backend rendered output as text, not HTML`. |
| No `/legacy` core workflow | `does not expose /legacy for Firewall core workflow`. |

Unsupported Firewall sub-action:

- rule reorder remains disabled because the backend has no reorder endpoint.

## 6. Integrated API Smoke

Added script from the VLESS workflow pass:

```bash
scripts/smoke/vless-client-access-groups-smoke.sh
```

Required environment for a disposable/local run:

```bash
export MEGAVPN_PUBLIC_BASE_URL=http://127.0.0.1:8080
export MEGAVPN_AUTH_TOKEN=<operator-or-test-token>
export MEGAVPN_VLESS_SMOKE_GROUP_ID=<disposable-vless-group-id>
export MEGAVPN_VLESS_SMOKE_CLIENT_REF=<disposable-client-username-email-or-id>

scripts/smoke/vless-client-access-groups-smoke.sh
```

Current evidence is SKIP, not PASS: no disposable DB/API base URL was available
in this workstation session. FE8-P0-03A did not add backend routes, so current
Clients evidence is frontend/API-contract test coverage plus the existing Go
route tests, not a live DB artifact smoke.

## 7. Static Serving Evidence

Backend tests and `scripts/ci/frontend-serving-smoke.sh` cover:

- `GET /` returns the new UI;
- frontend deep links return the new UI;
- `GET /legacy/` returns the rollback UI;
- `/api/*` and `/agent/*` are not shadowed by SPA fallback;
- missing root `/assets/*` return 404 rather than SPA HTML.

## 8. Security / Review Hygiene

Current enforced hygiene:

- no raw `/api/v1` calls outside `frontend/src/shared/api` and tests;
- unsafe methods keep the shared API client, cookie credentials and CSRF header;
- no browser auth token/session storage in new frontend source;
- no unreviewed `dangerouslySetInnerHTML`;
- no production console logging in new frontend source;
- backend errors and job output are rendered as text;
- artifact download uses a backend URL and is not persisted in browser storage;
- VLESS apply actions require backend preview and stale preview disables apply;
- Clients revoke/delete/artifact delete require confirmation;
- unsupported non-VLESS services remain catalog-only or legacy-only.

## 9. Write Workflow Summary

Fully connected in the new console:

- auth login/logout/invite/session;
- dashboard/readiness/version and primary read paths;
- jobs list/detail/logs/cancel;
- traffic export URL;
- `Clients -> Groups -> VLESS` create/edit, member preview/apply/remove, scope
  update and sync preview/apply;
- `Clients` core list/detail/create/status/delete/revoke, single-client VLESS
  preview/apply/remove, artifact list/build/download/delete and client-scoped
  job tracking;
- `Firewall` address groups, policies, rules, preview, apply, node state and
  emergency disable.

Still disabled, read-only or legacy-only:

- non-VLESS access group materialization workflows;
- migration conflict UI for access groups;
- client routes, access rotation and config cleanup;
- FE8-P0-03B share links, subscriptions and email delivery;
- nodes bootstrap/control/terminal/diagnostics mutations;
- instances create/apply/rollback/lifecycle/delete;
- certificates import/issue/default/revoke/delete;
- platform settings save, mail test and TLS apply;
- backhaul mutations;
- backup/restore browser UI.

## 10. Known Limitations

- Backend binary/version metadata remains `7.1.1.0`; synchronizing it to
  `8.0.0` is a separate release task.
- Full normal operator work still requires `/legacy/` for many non-Firewall,
  non-VLESS and non-Clients-core write workflows.
- Generic client edit stays disabled because the backend has no generic
  `PATCH/PUT /clients/{id}` endpoint.
- Client disable stays disabled because the backend exposes activate/suspend
  but no separate browser disable endpoint.
- Client routes, service access delete/rotation and config cleanup are not part
  of FE8-P0-03A.
- Share links, VLESS subscriptions and email delivery are FE8-P0-03B.
- No browser screenshot/responsive Playwright evidence was produced in this
  pass.
- Integrated VLESS API smoke was not run against a disposable DB/API in this
  session.

## 11. Go / No-Go

Recommendation:

- GO for PR review and CI validation of the 8.0.0 frontend branch.
- GO for using new UI `Clients -> Groups -> VLESS`, Clients core/artifacts and
  Firewall preview/apply/disable in controlled staging after operator review.
- NO-GO for final 8.0.0 release cutover or removing `/legacy/`.

Remaining blockers for final cutover:

1. run integrated smoke/e2e against disposable DB/API data for VLESS, Clients
   core/artifacts and Firewall operator flows;
2. migrate Nodes bootstrap/control/diagnostics workflows;
3. migrate Instances lifecycle/apply/rollback/delete workflows;
4. migrate Clients routes, access rotation, config cleanup, share links,
   subscriptions and email delivery;
5. migrate Certificates and Platform settings write workflows;
6. add E2E/browser responsive evidence for critical operator flows;
7. synchronize backend/frontend version and release-chain artifacts to `8.0.0`;
8. run full release gate in the release environment.
