# RTIS MegaVPN Frontend Audit 8.0.0

Release scope: 8.0.0 frontend console migration.

Status: Phase 1 working document. This file is intentionally created before frontend code changes.

RC1 addendum:

- Go serving now needs and tests a narrow SPA fallback for non-backend frontend
  routes such as `/clients`, `/operations/jobs` and `/network-policy/firewall`.
- `/legacy/` remains the rollback path and must stay available for at least one
  stable release.
- `docs/FRONTEND_RC1_PLAN_8.0.0.md`,
  `docs/FRONTEND_ENDPOINT_PARITY_8.0.0.md`,
  `docs/FRONTEND_WRITE_WORKFLOWS_8.0.0.md` and
  `docs/FRONTEND_SECURITY_REVIEW_8.0.0.md` refine this audit into RC1 working
  gates.
- The new console must not expose fake preview/apply flows. Unsupported write
  actions are disabled or legacy-only until backed by real endpoint wrappers,
  mutation hooks, invalidation, confirmation and job tracking.

## 1. Executive Summary

The current operator console is a static, hand-written JavaScript application served by the Go API from `web/`.
It is functional and broad, but it is not structured as a modern frontend application. The UI is assembled through
global `window.MegaVPN*` modules, string-template rendering, direct DOM mutation, and one monolithic stylesheet.

The 8.0.0 migration must preserve backend behavior and operational semantics while replacing the implementation with
a typed React + TypeScript + Vite console. The existing UI must remain available as legacy until the new console is
validated.

Non-negotiable migration constraints:

- preserve cookie-based session auth and `credentials: include`;
- preserve `X-MegaVPN-CSRF: 1` for unsafe API methods;
- do not introduce bearer-token or password storage in browser storage;
- preserve all backend API operations used by the current console;
- preserve public `/share/{token}`, `/subscribe/vless/{token}` and agent endpoints untouched;
- preserve or explicitly migrate release gates that currently validate static assets;
- keep old UI under `/legacy` before replacing `/`;
- move all operator text to ru/en i18n files with key parity checks;
- keep VLESS primary management under `Clients -> Groups`; `Instances` can show readonly linkage/context only.

## 2. Current Frontend Inventory

Current web root:

- `web/index.html`
- `web/assets/*.js`
- `web/assets/styles.css`
- static images: `rtis-logo.svg`, `world-map.svg`

Measured size:

| Area | Files / lines | Notes |
| --- | ---: | --- |
| Total static frontend | 30,341 lines | `web/index.html`, all JS, CSS |
| Stylesheet | 7,503 lines | Single global stylesheet |
| Largest page module | `clients-page.js`, 2,647 lines | Clients, delivery, groups, memberships |
| Largest workflow module | `node-workflows.js`, 1,866 lines | Node management, bootstrap, terminal, diagnostics |
| Bootstrap/composition | `app.js`, 663 lines | Wires all globals, refresh loop, lifecycle |
| Routing | `app-router.js`, 197 lines | Manual route switch on `state.page` |

Current load model:

- `index.html` manually references every JS module with `?v=7.1.1.0`.
- Scripts are loaded with `defer`.
- The application starts when `app.js` runs after all global modules are available.
- Bootstrap success is flagged through `window.__MegaVPNBootReady = true`.
- Failure path renders a minimal static bootstrap error in `authGate`.

Current module pattern:

```text
window.MegaVPNAppConfig
window.MegaVPNAppState.createInitialState()
window.MegaVPNAPIClient.create()
window.MegaVPNShellUI.create()
window.MegaVPN<Page>Page.create()
window.MegaVPN<Domain>Workflows.create()
```

Primary risk: there is no import graph, type contract, bundling contract, or compile-time API boundary.

## 3. Current Page and Route Map

Navigation comes from `web/assets/app-config.js`. Registered routes are validated in `web/assets/app-router.js`.

| Navigation group | Current routes | Current module |
| --- | --- | --- |
| Operations | `dashboard`, `nodes`, `nodeMap`, `instances`, `clients`, `jobs` | dashboard, nodes, node map, instances, clients, jobs |
| Provisioning | `backhaul`, `addressPools`, `artifacts`, `shareLinks` | backhaul, address pools, artifacts |
| Security | `firewall`, `traffic`, `certificates`, `audit` | firewall, traffic, certificates, ops |
| Control | `services`, `telemetry`, `revisions`, `settings` | services, ops, revisions, settings |
| Hidden workflow routes | `nodeManage`, `instanceManage` | node workflows, instance workflows |

Current router behavior:

- unauthenticated users see Login or Invitation flow;
- authenticated users see shell + nav + page content;
- unknown route renders an "Unknown route" card;
- auto-refresh runs every 5 seconds by default;
- auto-refresh is suppressed during dirty node/instance forms, active terminal, group member modal, and selected create-pack flow.

Migration implication:

- React routes must model both navigation pages and workflow pages.
- Dirty forms and terminal sessions need explicit query invalidation / polling pause rules.
- If history-based routes are used, Go static serving must add SPA fallback for non-API paths. Current Go server only serves `GET /` and `GET /assets/{path...}`.

## 4. Current Backend Static Serving Contract

Relevant Go files:

- `cmd/api/main.go`
- `internal/api/http/server.go`
- `internal/api/http/runtime_preflight.go`

Current web root resolution order:

1. configured `MEGAVPN_WEB_ROOT`;
2. `web`;
3. `/opt/megavpn/web`;
4. executable-local `web`;
5. executable parent `../web`.

Current HTTP handlers:

| Handler | Purpose |
| --- | --- |
| `GET /` | serves `index.html` if available, otherwise fallback API status HTML |
| `GET /assets/{path...}` | serves files below `web/assets` |
| `GET /share/{token}` | public share download, rate limited |
| `GET /subscribe/vless/{token}` | public VLESS subscription, rate limited |
| `GET /agent/binary-artifacts/{artifact_id}/download` | agent binary artifact download |
| `/agent/*` | agent control-plane protocol |
| `/api/v1/*` | authenticated or public API |

Static asset security behavior:

- `Cache-Control: no-store, no-cache, must-revalidate, max-age=0`;
- `Pragma: no-cache`;
- `Expires: 0`;
- path traversal is partially blocked by rejecting `..` in relative asset path;
- security headers wrap the full handler stack.

Migration implications:

- `/legacy` support does not exist yet and must be added deliberately.
- Vite output paths must either fit the existing `/assets/{path...}` handler or the handler must support a new asset root.
- Long-cache hashed assets are not currently enabled because backend forces `no-store`.
- Runtime preflight checks only `webRoot/index.html`; it does not validate Vite manifest, legacy index, or asset completeness.

## 5. Current Auth and Session Model

Frontend API client behavior:

- `fetch(..., { credentials: 'include' })` for all requests;
- unsafe methods automatically receive `X-MegaVPN-CSRF: 1`;
- response parsing prefers JSON and falls back to text;
- errors carry `status` and JSON payload when available;
- legacy `localStorage.megavpn.authToken` is removed on startup.

Auth flows:

| Flow | Endpoint(s) | Current behavior |
| --- | --- | --- |
| Session load | `GET /api/v1/auth/me` | populates user, session, roles, permissions |
| Login | `POST /api/v1/auth/login` | cookie session, then refresh |
| Logout | `POST /api/v1/auth/logout` | server logout, then local cleanup |
| Invite preview | `GET /api/v1/auth/invites/{token}` | token from `invite_token` query |
| Invite accept | `POST /api/v1/auth/invites/{token}/accept` | password set, query token removed |
| Password change | `POST /api/v1/auth/change-password` | settings workflow |

Backend auth boundaries:

- login and invite accept are rate limited;
- most `/api/v1/*` routes are permission guarded;
- public share/subscription URLs are token based and rate limited;
- agent protocol is separate from browser session.

Migration requirements:

- no auth token in `localStorage` or `sessionStorage`;
- keep invite token removal with `history.replaceState`;
- never display full subscription/share tokens after initial response unless backend intentionally returns them;
- permission-aware UI must be an affordance only; backend remains the enforcement boundary;
- typed API layer must propagate 401/403 distinctly for session expiry and missing permission states.

## 6. Backend API Surface Used by Current Frontend

The current frontend calls API paths directly from page modules. This is the surface that must be preserved or mapped.

| Domain | Representative endpoints used by frontend |
| --- | --- |
| Readiness/version | `GET /api/v1/ready`, `GET /api/v1/version`, `GET /api/v1/dashboard` |
| Auth | `GET /api/v1/auth/me`, `POST /api/v1/auth/login`, `POST /api/v1/auth/logout`, `POST /api/v1/auth/change-password`, invite preview/accept |
| Admin/auth management | `/api/v1/admin/users`, `/api/v1/admin/user-invites`, `/api/v1/admin/sessions`, user status/reset/resend/delete, session revoke |
| Settings | `/api/v1/settings/mail`, `/api/v1/settings/mail/test`, `/api/v1/settings/control-plane-tls`, `/api/v1/settings/control-plane-tls/apply`, `/api/v1/runtime/preflight` |
| Certificates / PKI | `/api/v1/platform/certificates`, preview/import/self-signed/authorities/issue/default/revoke/delete, `/api/v1/platform/pki-roots` |
| Dashboard | `/api/v1/dashboard` |
| Services/catalog | `/api/v1/services`, `/api/v1/services/installers`, `/api/v1/service-packs`, `/api/v1/service-packs?include_inactive=1`, service-pack create/update/status/delete |
| VLESS group templates | `/api/v1/vless-groups`, `/api/v1/vless-groups?include_inactive=1`, template upsert/status/delete |
| Client access groups | `/api/v1/client-access-services`, `/api/v1/client-access-groups`, migration conflicts, available clients, group scope, member preview/apply, sync preview/apply |
| Binary repository | `/api/v1/binary-artifacts`, `/api/v1/binary-artifacts/import`, `/api/v1/binary-artifacts/import-url` |
| Nodes | `/api/v1/nodes`, node detail/update/delete/force-retire/maintenance, diagnostics, route preview/apply/cleanup |
| Node bootstrap/trust | access methods, SSH host-key scan, SSH terminal session, bootstrap, bootstrap runs, bundle, enrollment tokens, agent token rotate, agent identity revoke |
| Node capabilities/services | node capabilities, install/verify/drift/install events, service discovery, inventory sync |
| Instances | `/api/v1/instances`, runtime states, detail, runtime-state, observations, revisions, spec replace, rollback, lifecycle actions, diagnose, delete/force-delete |
| Address pools | `/api/v1/address-pools`, `/api/v1/address-pools/spaces`, routing toggle |
| Firewall | `/api/v1/firewall`, policies, rules, address lists, entries, management settings, node firewall preview/apply/disable |
| Traffic accounting | `/api/v1/traffic/accounting`, `/api/v1/traffic/accounting/export` |
| Clients | `/api/v1/clients`, client detail/create/delete/status/provision/revoke, accesses, routes, configs, delivery email |
| Client artifacts/share/subscriptions | client artifacts build/content/download/delete, share links publish/revoke, subscriptions rotate/revoke |
| Backhaul | `/api/v1/backhaul/drivers`, `/api/v1/backhaul-links`, apply/probe/promote/route/delete |
| Artifacts aggregate | `/api/v1/artifacts`, `/api/v1/share-links` |
| Jobs | `/api/v1/jobs`, `/api/v1/jobs/{id}`, `/api/v1/jobs/{id}/logs`, cancel |
| Audit/telemetry | `/api/v1/audit?limit=200`; telemetry is currently rendered from loaded node/job state |

High-risk API characteristics:

- Several modules embed dynamic paths inline instead of using a shared client method.
- Some operations use method override patterns such as `POST` for actions, `PATCH` for route/scope state, and `DELETE` with no body.
- Existing frontend silently falls back on some fetch errors through `fetchJSON(path, fallback)`, which can hide partial-load failures.
- Polling is coarse grained: `loadCore()` fans out many requests every refresh.

Required new API layer:

- one typed function per backend operation;
- domain-specific query keys;
- centralized 401/403/5xx handling;
- mutation wrappers that preserve CSRF header behavior;
- explicit polling configuration per page/operation;
- no raw `/api/v1/...` strings inside React pages except generated or typed API modules.

## 7. Current Page Functionality Baseline

### 7.1 Dashboard

Current functionality:

- overview metrics from dashboard, nodes, instances, jobs;
- service instance table;
- failed jobs summary;
- navigation buttons into operational pages.

Risks:

- metrics depend on shared loaded state rather than page-owned queries;
- failed job summary can stale under partial refresh errors.

### 7.2 Nodes

Current functionality:

- node cards/list, create/edit/delete/retire;
- agent update and bulk update over bootstrap job;
- node management page with overview, profile, operations, SSH terminal, communication health, trust lifecycle, bootstrap runs, enrollment tokens;
- diagnostics retry/reconcile/requeue/channel probe;
- route policy preview/apply/cleanup;
- SSH access methods and secret-ref creation;
- emergency cleanup and reboot.

Risks:

- broad destructive operations live in modal forms with string confirmation only;
- terminal activity suppresses refresh but lifecycle ownership is manual;
- bootstrap secrets, host-key scan and terminal UX must avoid exposing sensitive material in logs or copied UI text.

### 7.3 Node Map

Current functionality:

- world map visualization using `world-map.svg`;
- GeoIP pending list;
- backhaul topology;
- mapped nodes and link inspector;
- backhaul route toggle through `PATCH /api/v1/backhaul-links/{id}/route`.

Risks:

- static SVG/map geometry is custom;
- mobile layout has a fixed large map height;
- map state is not typed or isolated from DOM measurements.

### 7.4 Instances

Current functionality:

- instance list with list/grouped view;
- create from service pack;
- manual instance creation;
- service pack catalog management;
- VLESS group template management currently exists under Instances;
- delete and force-delete instance;
- navigation into instance manage page.

Required 8.0.0 correction:

- primary VLESS group management must move out of Instances and remain under `Clients -> Groups`;
- Instances may show service pack creation and instance operations only;
- VLESS membership on instance manage can remain readonly/contextual with link to `Clients -> Groups`.

### 7.5 Instance Manage

Current functionality:

- desired/runtime/job log load;
- runtime timeline and diagnostics;
- spec editor/update;
- apply/start/stop/restart/enable/disable;
- revision state and validation;
- VLESS materialized access groups section for Xray/VLESS instances.

Risks:

- dirty state is manual;
- JSON/spec editing needs stronger validation and error mapping;
- mutation and polling interactions are fragile.

### 7.6 Clients

Current functionality:

- clients table and provisioning workflow;
- create/delete client;
- provision service access across instances;
- send access email;
- manage client service accesses and routes;
- build configs/artifacts;
- preview/download/delete client artifacts;
- publish/revoke share links;
- rotate/revoke VLESS subscription;
- clear generated configs.

Risks:

- very large module mixes list, provisioning, delivery, artifact and route workflows;
- secret/token display rules are embedded in templates;
- config preview uses textarea content and must not leak beyond permissions.

### 7.7 Clients -> Groups

Current functionality:

- client access group list;
- service filter;
- create/edit group policy;
- group scope across runtime instances;
- available client pagination/filtering;
- preview and bulk-apply membership changes;
- sync preview/apply materialization to runtime instances.

This is the target home for VLESS group operations in 8.0.0.

Risks:

- modal is full-screen and suppresses refresh through interaction locks;
- selection/pagination state is DOM-owned;
- preview-before-apply must remain mandatory.

### 7.8 Firewall

Current functionality:

- firewall model overview;
- policy/rule/address-list/entry CRUD;
- node firewall state;
- enforcement posture;
- management source settings in Settings;
- preview/apply/disable node firewall.

Risks:

- destructive firewall operations can affect control-plane access;
- UI must make management-source and lockout risk visible;
- current copy mentions implementation details such as `inet megavpn_firewall`.

### 7.9 Traffic

Current functionality:

- traffic accounting summary;
- filters persisted in session storage;
- client usage, collector status, recent samples;
- export URL builder for `/api/v1/traffic/accounting/export`.

Risks:

- filter state is custom and not typed;
- export path builds direct URL through API base;
- table minimum width is high on mobile.

### 7.10 Jobs

Current functionality:

- active/recent/failed/cancelled job tabs;
- search/sort;
- job detail modal;
- final result and execution logs;
- watch job helper used by mutations.

Risks:

- job polling is workflow-local and duplicated;
- cancellation support exists in backend but current frontend usage should be rechecked during migration.

### 7.11 Settings

Current functionality:

- runtime preflight;
- control-plane TLS settings/apply;
- platform CA center;
- firewall management sources;
- platform users, sessions, invites;
- mail settings/test;
- own password change;
- local browser API base setting.

Risks:

- mixes admin, runtime and local browser settings;
- local API base override changes trust boundary and should be explicit;
- session revocation and user deletion require strong confirmation UX.

### 7.12 Certificates

Current functionality:

- certificate overview;
- leaf cert import and preview;
- self-signed leaf issue;
- managed CA create;
- issue from managed CA;
- managed service PKI roots;
- set default, revoke leaf, delete CA;
- ACME is present as paused copy.

Risks:

- PEM/key input must remain client-side transient;
- private keys must never be logged, persisted, or rendered after submit;
- validation/error detail must distinguish key mismatch from parse failure.

### 7.13 Backhaul

Current functionality:

- create ingress-to-egress backhaul;
- driver selection/help;
- transport profiles;
- apply/probe/promote route projection;
- delete backhaul and cleanup jobs.

Risks:

- link operations may enqueue jobs on multiple nodes;
- route projection changes should be audit-visible and clearly confirmed.

### 7.14 Artifacts / Share Links

Current functionality:

- aggregate artifact list;
- queue client artifact export;
- preview generated config content;
- download artifact;
- publish/revoke delivery share link;
- aggregate share-link list.

Risks:

- generated config content can contain credentials;
- preview/download/publish actions must keep permission checks and not cache sensitive content.

### 7.15 Services

Current functionality:

- runtime operations;
- binary repository;
- capability matrix;
- service catalog;
- capability events;
- add runtime artifact by upload or URL;
- install/verify service capability on selected node.

Risks:

- binary artifact upload has a backend-specific request size exception;
- URL import is a backend SSRF-sensitive operation and must remain permission gated and explicit in UX.

### 7.16 Revisions

Current functionality:

- instance revision timeline;
- validation table;
- diff summary;
- rendered spec preview;
- rollback and rollback+apply.

Risks:

- rendered specs can contain managed file content;
- rollback/apply must be separate and auditable.

### 7.17 Audit / Telemetry

Current functionality:

- audit fetch from `/api/v1/audit?limit=200`;
- telemetry is rendered by `ops-pages.js` from existing state.

Risks:

- audit page needs proper filtering/pagination;
- telemetry is not a strong independent observability view today.

## 8. Hard-Coded Text and i18n Baseline

Current state:

- `index.html`, `app-config.js`, all page modules, workflow modules, `shell-ui.js`, `auth-view.js`, and `styles.css` contain user-facing English and Russian text.
- No translation keys exist.
- Locale is fixed at `<html lang="ru">`, while most operational copy is English.
- `formatDate()` uses `toLocaleString('ru-RU')`; relative time strings are English.

Representative hard-coded text groups:

| Group | Examples / source |
| --- | --- |
| Branding | `RTIS`, `MegaVPN Control Plane`, `Distributed VPN & Edge Platform` in `index.html`, auth views |
| Navigation | Operations, Provisioning, Security, Control; Dashboard, Nodes, Node map, Instances, Clients, Jobs, Backhaul, Address pools, Artifacts, Share links, Firewall, Traffic, Certificates, Audit, Services, Telemetry, Revisions, Settings |
| Auth | `Вход`, `Логин`, `Пароль`, `Войти`, `Operator onboarding`, `Activate protected access`, `Задайте пароль` |
| Common actions | Refresh, Close, Delete, Save, Cancel, Preview, Apply, Sync, Download, Publish link, Revoke, Enable, Disable |
| Status labels | ready, failed, pending, active, disabled, expired, missing, configured, loopback-only |
| Safety copy | destructive cleanup, firewall disable/apply, delete node/client/user/instance, force retire/delete |
| Domain copy | VLESS groups, Client access groups, Firewall model, Backhaul topology, Runtime Preflight, Control Plane TLS, Platform CA Center |
| Empty states | `Нет данных для отображения`, `No managed instances available yet`, `No clients for this filter` |
| Error states | `Unable to load control plane UI`, `Last UI/API error`, backend error text passthrough |

Migration requirements:

- create `frontend/src/i18n/locales/ru.json` and `en.json`;
- enforce key parity in CI;
- no page/component user-facing literal strings except test IDs, ARIA-hidden symbols, and protocol identifiers;
- use locale-aware date/time/number formatting;
- keep security/protocol terms untranslated where operationally safer, but still key them.

## 9. UI Pattern and Component Audit

Current reusable patterns:

- `tableCard(title, rows, columns, tools)`;
- `metric(label, value, caption)`;
- global modal through `openModal()`;
- global `openActionOutcomeModal()`;
- `statusTag()`;
- `form-enhancer.js` for required markers, trimming, validation summary;
- `table-enhancer.js` for resizable table columns;
- global button classes: `primary-btn`, `secondary-btn`, `danger-btn`, `icon-btn`;
- `page-tabs` pattern repeated across services, jobs, settings, certificates, firewall, revisions, clients, traffic.

Duplications / inconsistencies:

- page tabs are hand-rendered in each page with similar but not identical markup;
- tables vary between `tableCard`, custom cards, custom table wrappers and specialized CSS;
- forms are rendered with repeated HTML string templates and inline IDs;
- modals are the only overlay primitive; no typed drawer/confirm/wizard abstraction;
- confirmation patterns differ between destructive operations;
- action result rendering varies between raw JSON, outcome modal, inline tag and custom cards;
- icons are text glyphs and symbols rather than a consistent icon system;
- status color mapping is string-list based and shared only through runtime helper;
- page-level state is split across global `state`, `localStorage`, `sessionStorage`, DOM dataset attributes, and modal DOM.

New component kit minimum:

- AppShell, Sidebar, Topbar, UserMenu;
- Button, IconButton, Tooltip;
- Badge/StatusBadge;
- Tabs/SegmentedControl;
- Table/DataGrid with responsive behavior;
- Modal, Drawer, ConfirmDialog, Wizard;
- FormField, Input, Select, Checkbox, Toggle, Textarea, PasswordInput;
- EmptyState, ErrorState, LoadingState;
- Metric, SummaryStrip, Timeline;
- CodeBlock/SecretValue with reveal/copy controls;
- PageHeader and Toolbar.

## 10. Responsive and Accessibility Audit

Current responsive behavior:

- main shell becomes single-column below 860px;
- sidebar becomes relative top block, not a compact mobile navigation drawer;
- tables usually rely on horizontal scrolling with `min-width: 760px`;
- page tabs become auto-fit grids on mobile;
- modals become nearly full viewport below 860px;
- node map keeps a large fixed height on mobile.

Current accessibility strengths:

- modal has `role="dialog"` and `aria-modal="true"`;
- tabs set `role="tab"` and `aria-selected`;
- form enhancer creates validation summary with `role="alert"`;
- auth forms use browser autocomplete for login/password.

Current accessibility gaps:

- focus trap is not implemented for modal;
- focus return after modal close is not guaranteed;
- keyboard navigation for custom tabs/tables is incomplete;
- many icon/symbol controls have inconsistent labels;
- dynamic content updates are not consistently announced;
- table resizing handle is `aria-hidden` and pointer-focused only;
- color/status semantics rely heavily on visual color;
- mobile sidebar is not a proper disclosure/drawer navigation.

Migration requirements:

- modal/drawer focus trap and return focus;
- keyboard-operable tabs and dialogs;
- visible focus rings;
- accessible names for icon buttons;
- no text clipping in buttons/cards at desktop/tablet/phone widths;
- no horizontal page overflow except explicit table scrollers;
- page layout verified at desktop, tablet and phone viewports.

## 11. Security Audit for Frontend Migration

Security properties to preserve:

- cookie session auth only;
- unsafe request CSRF marker;
- permission-aware rendering aligned with backend permissions;
- no persistence of passwords, tokens, private keys, generated config contents, or subscription URLs;
- escaping of dynamic text before rendering;
- public share/subscription routes remain server-controlled and rate limited.

Current security risks:

| Risk | Current source | Migration response |
| --- | --- | --- |
| XSS by template mistakes | extensive `innerHTML` string templates | React rendering by default, avoid `dangerouslySetInnerHTML`, central sanitizer only where unavoidable |
| Token/secret leakage in UI | artifact previews, subscription rotation, bootstrap bundles, secret refs | dedicated SecretValue/OneTimeSecret components, no storage, no logs |
| CSRF regression | custom header in `api-client.js` | central fetch client with tests for unsafe methods |
| Permission confusion | UI hides some data via `hasPermission` while backend enforces | typed permission gates and 403 states |
| Overbroad polling | `loadCore()` fetches many domains | domain query keys and page-scoped polling |
| Destructive action ambiguity | inconsistent confirm flows | shared ConfirmDialog with exact object name, scope and backend permission |
| SSRF-sensitive URL import | binary artifact import by URL | explicit permission/error UX, no client-side URL prefetch |
| Sensitive preview caching | artifact/config preview textareas | no query persistence, no local storage, clear on modal close |

## 12. Current CI / Release Gates

Current CI files:

- `.github/workflows/ci.yml`
- `scripts/ci/self-test.sh`
- `scripts/ci/release-gate.sh`
- `scripts/ci/frontend-bootstrap-smoke.js`
- `scripts/ci/install-web-wrapper-smoke.sh`
- `scripts/ci/docs-consistency.sh`

Current frontend checks:

| Gate | Current behavior |
| --- | --- |
| `frontend-js-syntax` | `node --check web/assets/*.js` |
| `frontend-bootstrap-smoke` | loads `web/index.html` scripts in a browser-like VM and expects `__MegaVPNBootReady` |
| `install-web-wrapper-smoke` | verifies `scripts/install-web.sh` copies `index.html` and `assets/app.js` |
| `frontend-asset-manifest` | `web/index.html` references existing `web/assets/*` files |
| `frontend-page-module-exports` | validates `window.MegaVPN<Page>Page.create` contracts |
| `static-security-patterns` | rejects banned shell/SQL command patterns |
| `docs-consistency` | release docs and web asset cache-key sanity |

Gaps for React/Vite:

- no `npm ci`;
- no TypeScript compile;
- no ESLint;
- no unit/component tests;
- no i18n key parity check;
- no Vite build artifact validation;
- no Playwright smoke;
- no bundle size or sourcemap policy;
- no check that legacy UI remains available.

8.0.0 CI requirements:

- `npm ci` from `frontend/`;
- `npm run typecheck`;
- `npm run lint`;
- `npm test`;
- `npm run test:i18n`;
- `npm run build`;
- Go self-test should validate built frontend artifacts and `/legacy`;
- release-gate should include browser smoke at least for login shell, dashboard shell and one authenticated API mock flow.

## 13. Migration Risks

| Severity | Risk | Mitigation |
| --- | --- | --- |
| High | Losing a backend operation during page migration | API inventory + typed client coverage + page acceptance checklist |
| High | Auth/CSRF regression | centralized fetch client tests and backend smoke |
| High | Legacy UI becomes unavailable during rollout | move existing `web/` app to `web/legacy` or equivalent and add Go `/legacy` route before replacing `/` |
| High | VLESS group ownership remains split | remove primary VLESS group management from Instances; implement under Clients -> Groups |
| High | Sensitive artifact/subscription/config leakage | secret display components, no persistence, modal cleanup |
| Medium | New build breaks install scripts | adapt `scripts/install-web.sh`, wrapper smoke and release gate |
| Medium | Polling overload | TanStack Query stale times, page-scoped polling, mutation invalidation |
| Medium | Responsive regressions | Playwright screenshots desktop/tablet/phone |
| Medium | i18n partial translation | key parity CI and no-literal lint convention |
| Medium | Backend static handler lacks SPA fallback | either hash routing or Go fallback for non-API frontend paths |

## 14. Working Plan After This Audit

1. Preserve legacy:
   - move/copy current static console under a legacy path;
   - add Go static route for `/legacy` and `/legacy/assets/*`;
   - keep current boot smoke adapted for legacy until removed by explicit decision.

2. Create frontend workspace:
   - `frontend/package.json`;
   - Vite + React + TypeScript;
   - strict TS config;
   - source layout: `app`, `routes`, `features`, `shared`, `api`, `i18n`, `ui`, `styles`, `test`.

3. Create design system:
   - design tokens;
   - base layout and components;
   - desktop/tablet/phone shell.

4. Create i18n:
   - ru/en dictionaries;
   - key parity script;
   - locale provider and formatting helpers.

5. Create typed API layer:
   - central fetch client preserving credentials and CSRF;
   - endpoint functions by domain;
   - TanStack Query client, keys and mutation helpers.

6. Migrate app shell and auth:
   - login;
   - invite accept;
   - session load/logout;
   - permission-aware navigation.

7. Migrate pages by blast radius:
   - dashboard;
   - jobs/audit/read-only pages;
   - nodes and node map;
   - instances and instance manage;
   - clients and groups;
   - firewall/traffic;
   - settings/certificates/backhaul/services/artifacts/revisions.

8. Add tests and gates:
   - unit/component tests;
   - i18n parity;
   - Vite build in CI;
   - Playwright smoke or equivalent;
   - release-gate integration.

9. Final verification:
   - compare route/page coverage against this document;
   - compare API client coverage against current frontend usage;
   - verify `/` new console and `/legacy` old console;
   - run Go tests and frontend gates.

## 15. Acceptance Checklist for 8.0.0 Frontend

- [ ] `docs/FRONTEND_AUDIT_8.0.0.md` exists before code migration.
- [ ] Old UI is available under `/legacy`.
- [ ] New React console is served from Go backend at `/`.
- [ ] No browser token/password/private-key persistence.
- [ ] Unsafe API methods send `X-MegaVPN-CSRF: 1`.
- [ ] All current navigation pages are implemented or intentionally redirected.
- [ ] VLESS group primary management is under `Clients -> Groups`.
- [ ] Instances page has no primary VLESS groups tab.
- [ ] ru/en i18n files have identical keys.
- [ ] TypeScript compile passes.
- [ ] Frontend lint/test/build pass.
- [ ] Go tests pass.
- [ ] Release gate includes new frontend checks.
- [ ] Desktop/tablet/phone responsive behavior is verified.
- [ ] Sensitive preview/one-time secret flows are explicitly handled.
