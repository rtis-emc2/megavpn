# RTIS MegaVPN Frontend RC1 Plan 8.0.0

Release scope: MegaVPN Console 8.0.0 release candidate.

Status: execution plan created before functional RC1 changes.

## 1. Current Readiness Summary

The repository already contains the first frontend migration baseline:

- `docs/FRONTEND_AUDIT_8.0.0.md`;
- `docs/FRONTEND_ARCHITECTURE_8.0.0.md`;
- `docs/releases/8.0.0.md`;
- new React + TypeScript + Vite workspace under `frontend/`;
- Vite build output under `web/`;
- legacy static console under `web/legacy/`;
- shared API client preserving cookie sessions and unsafe-method CSRF header behavior;
- TanStack Query read-path hooks;
- Russian and English i18n resources;
- primary navigation and read-oriented pages;
- partially expanded CI/release-gate checks.

The migration is not release-complete yet. The new console is usable as a
read-oriented operator surface, but several normal operator write workflows are
currently disabled or represented only as explicit preview gates. RC1 must not
present any fake-success action.

## 2. Route Matrix

| Route | Page | Current state | RC1 target |
| --- | --- | --- | --- |
| `/` | Dashboard | connected read path | keep connected |
| `/auth` | Login / invite | connected | keep connected |
| `/infrastructure/nodes` | Nodes | read path, write actions disabled | read path plus documented disabled/legacy actions |
| `/infrastructure/node-map` | Node map | read path | keep connected |
| `/infrastructure/backhaul` | Backhaul | read path, actions disabled | read path plus documented disabled/legacy actions |
| `/infrastructure/address-pools` | Address pools | read path | keep connected |
| `/services/instances` | Instances | read path, mutations disabled | keep read path; disable unsupported mutations honestly |
| `/services/service-packs` | Service packs | read path, create disabled | keep read path; disable unsupported mutations honestly |
| `/services/runtime-artifacts` | Runtime artifacts | read path, create disabled | keep read path; disable unsupported mutations honestly |
| `/services/revisions` | Revisions | read path, preview disabled | keep read path; disable unsupported mutations honestly |
| `/clients` | Clients | read path, create disabled | P0 candidate for basic mutations if endpoint wiring is safe |
| `/clients/groups` | Client access groups | read path, local-only preview flag | P0 candidate for real preview/apply wiring or explicit disabled state |
| `/clients/delivery` | Delivery / artifacts | read path, actions disabled | read path plus documented disabled/legacy actions |
| `/clients/subscriptions` | Subscriptions | catalog/unsupported view | keep explicit limitation |
| `/network-policy/firewall` | Firewall | read path, local-only preview flag | P0 candidate for real node preview/apply/disable or explicit disabled state |
| `/network-policy/route-policy` | Route policy | unsupported/read path | keep explicit disabled/legacy state |
| `/network-policy/traffic` | Traffic accounting | connected read/export link | keep connected |
| `/operations/jobs` | Jobs | connected list/detail/logs | keep connected; cancel only if wired |
| `/operations/audit` | Audit | connected read path | keep connected |
| `/operations/diagnostics` | Diagnostics | connected read path | keep connected |
| `/operations/backup-restore` | Backup/restore | unsupported action | remove from primary nav or keep clearly disabled if backend scope is absent |
| `/platform/settings` | Settings | read path, save disabled | keep read path; disable unsupported mutations honestly |
| `/platform/certificates` | Certificates | read path, actions disabled | keep read path; disable unsupported mutations honestly |
| `/platform/access` | Users/sessions | read path, create disabled | keep read path; disable unsupported mutations honestly |
| `/platform/mail` | Mail settings | read path, action disabled | keep read path; disable unsupported mutations honestly |
| `/legacy/` | Legacy static console | available | keep available for rollback |

Go serving must support direct refresh and deep links for all non-backend
frontend routes above.

## 3. Endpoint / Action Matrix

This plan uses the audit endpoint inventory as the source of truth. A dedicated
parity document will be created in `docs/FRONTEND_ENDPOINT_PARITY_8.0.0.md`.

| Domain | Current wrapper coverage | Current UI action coverage | RC1 handling |
| --- | --- | --- | --- |
| Auth/session/invite | partial connected | login/logout/invite connected | keep connected |
| Ready/version/dashboard | connected | read-only | keep connected |
| Nodes | list/detail partial | dangerous actions disabled | document disabled/legacy; wire only safe P0 gaps if scoped |
| Node bootstrap/control | mostly missing | disabled | legacy-only unless fully wired with confirm/job tracking |
| Node diagnostics/inventory/capabilities | partial/missing | disabled | legacy-only unless fully wired |
| Instances | list/runtime partial | mutations disabled | disable honestly unless fully wired |
| Service packs | list partial | mutations disabled | disable honestly unless fully wired |
| Runtime artifacts | list partial | mutations disabled | disable honestly unless fully wired |
| Clients | list partial | mutations disabled | P0 candidate for basic create/status if complete |
| Client access groups | list/services partial | preview/apply local-only | P0 candidate for real preview/apply or disabled state |
| Client artifacts/share/subscriptions | read partial | actions disabled | legacy-only unless fully wired |
| Firewall | inventory partial | preview/apply local-only | P0 candidate for real preview/apply or disabled state |
| Route policy | route preview/apply wrappers missing | disabled | legacy-only unless fully wired |
| Traffic accounting | connected | read/export | keep connected |
| Jobs | list/detail/logs connected | cancel not wired | keep connected; cancel disabled unless wired |
| Audit | connected read path | read-only | keep connected |
| Settings/mail/TLS | read partial | save/test/apply disabled | disable honestly unless fully wired |
| Certificates/PKI | list partial | mutations disabled | legacy-only unless fully wired |
| Backhaul | list/drivers partial | actions disabled | legacy-only unless fully wired |

No React page or feature component may call `fetch('/api/v1/...')` directly.
API calls must be routed through `frontend/src/shared/api` or an approved
domain API module.

## 4. Write Workflow Matrix

The detailed workflow matrix will be tracked in
`docs/FRONTEND_WRITE_WORKFLOWS_8.0.0.md`.

| Workflow group | RC1 default |
| --- | --- |
| Auth login/logout/invite/session | fully connected |
| Jobs read/detail/logs | fully connected |
| Jobs cancel/retry | disabled unless endpoint hook and UX are complete |
| Clients basic CRUD/status | implement only if full error/permission states are wired; otherwise disabled |
| Clients -> Groups member preview/apply | implement real backend preview/apply or remove local fake preview |
| Firewall node preview/apply/disable | implement real backend preview/apply or remove local fake preview |
| Nodes bootstrap/control/terminal | legacy-only unless full secure workflow is wired |
| Instances apply/reapply/rollback/delete | legacy-only or disabled unless fully wired |
| Certificates import/issue/revoke/delete | legacy-only or disabled unless fully wired |
| Settings/mail/TLS apply | legacy-only or disabled unless fully wired |
| Backhaul advanced actions | legacy-only or disabled unless fully wired |
| Backup/restore | remove from primary nav or mark unsupported if no browser backend scope exists |

RC1 priority is to eliminate fake-success affordances, not to make every
mutation clickable.

## 5. Implementation In This Pass

This pass will focus on RC1 safety and primary-UI readiness:

1. Add Go SPA fallback for non-backend frontend routes.
2. Add backend tests for root, deep links, legacy, API non-shadowing, and asset
   404 behavior.
3. Add `scripts/ci/frontend-serving-smoke.sh`.
4. Create endpoint parity and write workflow documentation.
5. Add CI/static guard checks for:
   - raw `/api/v1` calls outside approved frontend API locations;
   - forbidden browser auth token storage;
   - unreviewed `dangerouslySetInnerHTML`;
   - production `console.log`/`console.debug` response leaks.
6. Replace local-only fake preview/apply affordances with either real backend
   wiring where scoped and safe, or explicit disabled/legacy-only states.
7. Update release notes with honest RC1 status.

Full legacy write-workflow parity is not promised by this single pass unless
the endpoint, UX, test, and security requirements can be completed without
regressing existing backend behavior.

## 6. Disabled / Legacy-Only Policy

An action may be visible in the new console only when it is one of:

1. fully connected to a real backend endpoint;
2. disabled with a visible reason;
3. linked to `/legacy/` with a documented RC1 limitation;
4. removed from primary navigation when backend browser scope is not ready.

Unsupported operations must not:

- show success locally;
- create fake preview results;
- hide backend errors behind generic success messages;
- create one mutation/job per client for bulk workflows;
- expose secrets or sensitive job output.

## 7. Risks

| Risk | Impact | Mitigation |
| --- | --- | --- |
| SPA fallback shadows backend route | API breakage | explicit exclusion list and tests |
| Asset miss returns index | broken browser diagnostics and wrong cache behavior | `/assets/*` remains strict 404 |
| Fake preview/apply remains | operator trust and safety risk | static grep and explicit workflow matrix |
| Partial mutation wiring | data corruption or hidden errors | disable unless endpoint, invalidation, confirmation and error mapping are complete |
| Secret leakage in UI/logs | security incident | safe text rendering, no response console logging, redaction helpers/tests |
| Version mismatch | release confusion | document RC1 version state; synchronize only with full release-chain update |
| Legacy broken during migration | rollback loss | smoke test `/legacy/` and legacy assets |

## 8. Test Plan

Minimum checks for this pass:

- `gofmt -l cmd internal`;
- `go test ./internal/api/http`;
- `go test ./cmd/api`;
- `go test ./...` when time permits;
- `go vet ./...` when time permits;
- `cd frontend && npm ci`;
- `cd frontend && npm run typecheck`;
- `cd frontend && npm run lint`;
- `cd frontend && npm run test`;
- `cd frontend && npm run i18n:check`;
- `cd frontend && npm run build`;
- `scripts/ci/frontend-serving-smoke.sh`;
- `node scripts/ci/frontend-bootstrap-smoke.js`;
- `scripts/ci/install-web-wrapper-smoke.sh`;
- static frontend guard checks.

Any skipped check must be documented with an exact reason in the final
acceptance document.

## 9. Rollback Plan

Rollback for RC1 remains `/legacy/`:

1. Keep `web/legacy/index.html` and `web/legacy/assets/*` deployable.
2. Keep Go handlers for `/legacy/` and `/legacy/assets/*`.
3. Keep legacy smoke tests in CI/release gates.
4. Keep the new navigation link to Legacy UI.
5. Document all legacy-only workflows in release notes and parity documents.

The legacy UI must not be removed before one stable release with the new console
as the primary operator UI.
