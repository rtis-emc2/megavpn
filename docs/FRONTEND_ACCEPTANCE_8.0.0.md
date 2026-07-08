# RTIS MegaVPN Frontend Acceptance 8.0.0

Branch: `release/8.0.0-frontend-console`

Evidence HEAD/current commit:
`f1d8769b63cdfe705afb29a71b8927d4c4abe147`

Evidence CI:
GitHub Actions run `28967364873` passed for
`f1d8769b63cdfe705afb29a71b8927d4c4abe147`.

FE8-RC1 hygiene implementation commit:
`9044647110cd5cbaeb4d5a866b96f56008fcb338`

FE8-P0-02 Firewall task implementation commit:
`5bfae8dfd629592dfa44aec9a3cea8b1db4b2c47`

Evidence date UTC: `2026-07-08T18:43:23Z`

Status: RC1 frontend branch is reviewable and CI-verifiable. Final 8.0.0
cutover is still blocked by non-Firewall/non-VLESS write workflow migration and
backend version synchronization.

## 1. Summary

This pass makes the current 8.0.0 frontend branch review-ready after the VLESS
Clients -> Groups implementation:

- CI push coverage now includes `release/8.0.0-frontend-console` and
  `release/**`; pull request coverage remains enabled.
- The frontend package manager standard is `npm`: `package-lock.json` is kept,
  `pnpm-lock.yaml` is removed, and CI/docs use `npm ci` / `npm run ...`.
- `Clients -> Groups -> VLESS` is connected in the new React console without
  `/legacy/` for create/edit, member preview/apply/remove, scope update and sync
  preview/apply.
- `Firewall` address groups, policies, rules, preview, apply and emergency
  disable are connected in the new React console without `/legacy/`.
- Non-VLESS access services remain catalog-only until backend materialization is
  implemented.
- A lightweight API smoke script exists for disposable/local VLESS group
  endpoint checks: `scripts/smoke/vless-client-access-groups-smoke.sh`.
- `/legacy/` remains the rollback UI and still covers non-migrated workflows.

## 2. Commands Run

| Check | Status | Evidence |
| --- | --- | --- |
| `gofmt -l cmd internal` | PASS | No files listed. |
| `go vet ./...` | PASS | No vet findings. |
| `go test ./...` | PASS | All Go package tests pass. |
| `go test -race ./...` | PASS | Race detector tests pass. |
| `go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin` | PASS | All operational binaries build. |
| `cd frontend && npm ci` | PASS | npm `11.18.0`; 251 packages installed; audit found 0 vulnerabilities. |
| `cd frontend && npm run typecheck` | PASS | TypeScript checks pass. |
| `cd frontend && npm run lint` | PASS | ESLint passes. |
| `cd frontend && npm run test` | PASS | Vitest: 3 files, 30 tests passed. |
| `cd frontend && npm run i18n:check` | PASS | i18n key parity ok: 327 keys. |
| `cd frontend && npm run build` | PASS | Vite build wrote `web/index.html`, `web/.vite/manifest.json`, `web/assets/index-kuNMHIpl.js`, `web/assets/index-C9SCsY-r.css`. |
| `scripts/ci/frontend-serving-smoke.sh` | PASS | Backend serving smoke passed; root/deep links/legacy/API non-shadowing/static asset 404 contract holds. |
| `scripts/ci/frontend-static-guards.sh` | PASS | Static frontend security guards pass. |
| `scripts/ci/docs-consistency.sh` | PASS | Documentation consistency ok for backend release `7.1.1.0`. |
| `scripts/smoke/vless-client-access-groups-smoke.sh` | SKIP | No `MEGAVPN_PUBLIC_BASE_URL` or `MEGAVPN_API_URL` was provided for a disposable/local API. |

Local note: this workstation did not expose a native `npm` binary in `PATH`;
the checks above were run through npm CLI `11.18.0` installed in a temporary
tooling directory. The repository standard and GitHub CI path remain plain
`npm ci` and `npm run ...`.

## 3. VLESS Workflow Test Evidence

`frontend/src/pages/clients/ClientGroupsPage.test.tsx` verifies the migrated
VLESS workflow against mocked backend API responses:

| Required behavior | Test evidence |
| --- | --- |
| Create VLESS group | `creates VLESS groups through the client access group API`; asserts `POST /api/v1/client-access-groups`. |
| Update VLESS group policy/status | `updates VLESS group policy and status through the client access group API`; asserts `PATCH /api/v1/client-access-groups/{group_id}` with status and policy JSON. |
| Member preview/apply | `previews and applies VLESS membership with backend bulk endpoints`; asserts `/members:preview` and `/members:bulk-apply`. |
| Member remove | `removes VLESS group members through the backend member delete endpoint`; asserts `DELETE /members/{client_id}`. |
| Scope update | `updates VLESS group scope through the backend scope endpoint`; asserts `PATCH /scope`. |
| Sync preview/apply | `previews and applies VLESS group sync with backend sync endpoints`; asserts `/sync:preview` and `/sync:apply`. |
| Preview stale disables apply | `invalidates VLESS membership preview and disables apply when selection inputs change`; apply remains disabled after mode change. |
| No `/legacy` calls | Every VLESS workflow test asserts no request path starts with `/legacy`. |

The existing app shell test also remains green:
`frontend/src/app/App.test.tsx`.

## 4. Firewall Workflow Test Evidence

`frontend/src/pages/network-policy/FirewallPage.test.tsx` verifies the migrated
Firewall workflow against mocked backend API responses:

| Required behavior | Test evidence |
| --- | --- |
| Load policies/address groups | `loads policies and address groups from mocked API`; asserts `GET /api/v1/firewall`. |
| Address group create/update/delete | Tests assert `POST`, `PUT` and `DELETE /api/v1/firewall/address-lists`. |
| DNS-only/empty group warnings | `shows DNS-only and empty renderable address group warnings`. |
| Rule create/update/delete | Tests assert `POST`, `PUT` and `DELETE /api/v1/firewall/policies/{id}/rules`. |
| Preview disabled until node/policy selected | `keeps Preview disabled until node and policy are selected`. |
| Successful preview enables Apply | `enables Apply after successful backend preview`. |
| Stale preview disables Apply | `marks preview stale and disables Apply after policy changes`. |
| Blocking preview errors prevent Apply | `blocks Apply when preview returns blocking errors`. |
| Apply confirmation/job | Tests assert confirmation, real `POST /api/v1/nodes/{id}/firewall/apply` and job link. |
| Emergency disable | Test asserts confirmation text and real `POST /api/v1/nodes/{id}/firewall/disable`. |
| 403/422/409 handling | Tests assert distinct permission, validation and conflict messages. |
| Safe rendered output | `renders backend rendered output as text, not HTML`. |
| No `/legacy` core workflow | `does not expose /legacy for Firewall core workflow`. |

Firewall preview/apply/disable works in the new UI without `/legacy`.

Unsupported Firewall sub-action:

- rule reorder remains disabled because the backend has no reorder endpoint.

## 5. Integrated API Smoke

Added script:

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

By default the script runs read endpoints, sync preview and membership preview
only. To apply membership changes against disposable data:

```bash
MEGAVPN_VLESS_SMOKE_APPLY=1 scripts/smoke/vless-client-access-groups-smoke.sh
```

Current evidence is SKIP, not PASS: no disposable DB/API base URL was available
in this workstation session. Do not claim final release readiness without a
real smoke run against disposable data.

## 6. Static Serving Evidence

Backend tests and `scripts/ci/frontend-serving-smoke.sh` cover:

- `GET /` returns the new UI;
- frontend deep links return the new UI;
- `GET /legacy/` returns the rollback UI;
- `/api/*` and `/agent/*` are not shadowed by SPA fallback;
- missing root `/assets/*` return 404 rather than SPA HTML.

## 7. Security / Review Hygiene

Current enforced hygiene:

- no raw `/api/v1` calls outside `frontend/src/shared/api` and tests;
- unsafe methods keep the shared API client, cookie credentials and CSRF header;
- no browser auth token/session storage in new frontend source;
- no unreviewed `dangerouslySetInnerHTML`;
- no production console logging in new frontend source;
- VLESS apply actions require backend preview and stale preview disables apply;
- unsupported non-VLESS services are visible as catalog-only and fail closed.
- Firewall Apply requires a successful backend Preview, stale preview disables
  Apply, and preview blocking errors prevent Apply.

## 8. Write Workflow Summary

Fully connected in the new console:

- auth login/logout/invite/session;
- dashboard/readiness/version and primary read paths;
- jobs list/detail/logs/cancel;
- traffic export URL;
- `Clients -> Groups -> VLESS` create/edit, member preview/apply/remove, scope
  update and sync preview/apply.
- `Firewall` address groups, policies, rules, preview, apply, node state and
  emergency disable.

Still disabled, read-only or legacy-only:

- non-VLESS access group materialization workflows;
- migration conflict UI for access groups;
- nodes bootstrap/control/terminal/diagnostics mutations;
- instances create/apply/rollback/lifecycle/delete;
- client provisioning, artifacts, shares and subscriptions;
- certificates import/issue/default/revoke/delete;
- platform settings save, mail test and TLS apply;
- backhaul mutations;
- backup/restore browser UI.

## 9. Known Limitations

- Backend binary/version metadata remains `7.1.1.0`; synchronizing it to
  `8.0.0` is a separate release task.
- Full normal operator work still requires `/legacy/` for many non-Firewall and
  non-VLESS write workflows.
- Endpoint DTOs are still broad for multiple non-VLESS domains.
- No browser screenshot/responsive Playwright evidence was produced in this
  pass.
- Integrated VLESS API smoke was added but not run against a disposable DB/API
  in this session.

## 10. Go / No-Go

Recommendation:

- GO for PR review and CI validation of the 8.0.0 frontend branch.
- GO for using new UI `Clients -> Groups -> VLESS` workflow in controlled
  staging after disposable API smoke evidence is produced.
- GO for using new UI `Firewall` preview/apply/disable in controlled staging
  with operator review of backend preview jobs.
- NO-GO for final 8.0.0 release cutover or removing `/legacy/`.

Remaining blockers for final cutover:

1. run the VLESS integrated API smoke against disposable DB/API data;
2. migrate Nodes bootstrap/control/diagnostics workflows;
3. migrate Instances lifecycle/apply/rollback/delete workflows;
4. migrate Clients provisioning/artifacts/share/subscriptions workflows;
5. migrate Certificates and Platform settings write workflows;
6. add E2E/browser responsive evidence for critical operator flows;
7. synchronize backend/frontend version and release-chain artifacts to `8.0.0`;
8. run full release gate in the release environment.
