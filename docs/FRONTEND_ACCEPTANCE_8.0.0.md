# RTIS MegaVPN Frontend Acceptance 8.0.0

Branch: `release/8.0.0-frontend-console`

Latest evidence commit:
`5be5a33e16c7eef0578e122f919f9932ef5cbcf0`

FE8-P0-05B Nodes bootstrap/security/control feature commit:
`pending final feat: connect node bootstrap security workflows commit; final SHA is recorded in the task handoff after commit creation`

FE8-P0-05A Nodes observability/diagnostics/inventory feature commit:
`5be5a33e16c7eef0578e122f919f9932ef5cbcf0`

FE8-P0-04B service pack instance creation feature commit:
`2c080a555b5d0460fe3b8c876907a67823185917`

FE8-P0-04A instance runtime control feature commit:
`e07b2b766949d3aa867717972b4834fa51aa84d2`

FE8-P0-04A evidence commit:
`711b06dec5076fabcd7488fd11d010b65e6c8276`

FE8-P0-03B client delivery feature commit:
`8dbcb97bcf225d34686c0eb555a6697425f12c37`

FE8-P0-03B evidence commit:
`70c5242fa9d9e99763aa60a797bdc4729980179f`

FE8-P0-03A feature commit:
`326499a6691833dc3d9be406cf9e84a91544a358`

FE8-P0-03A evidence commit:
`f825a3d67eb2bca36fda9e806c88ae0a95adeec9`

VLESS implementation commit:
`f33070aee76e9fb11100a5ea954fded09c0d4a10`

FE8-RC1 hygiene implementation commit:
`9044647110cd5cbaeb4d5a866b96f56008fcb338`

Firewall implementation commit:
`5bfae8dfd629592dfa44aec9a3cea8b1db4b2c47`

Firewall evidence alignment commit:
`d0c6af9db88018c5cae14be4542b453a310b658f`

Current evidence CI:
Current FE8-P0-05B evidence is local until the final feature commit is pushed.
Previous accepted FE8-P0-05A evidence CI run `28979061764` passed for
`5be5a33e16c7eef0578e122f919f9932ef5cbcf0`.

Current evidence date UTC: `2026-07-08T22:21:57Z`

Status: FE8-P0-05B is locally verified and reviewable. Final 8.0.0 cutover
remains NO-GO until the remaining non-migrated workflows, live/staging operator
validation, integrated disposable-data smoke and backend version synchronization
are complete.

VLESS is connected in the new UI without `/legacy/`. Firewall is connected in
the new UI without `/legacy/`. Clients core/artifacts are connected in the new
UI without `/legacy/`. Client delivery workflows are connected in the new UI
without `/legacy/`. Existing Instances runtime control is connected in the new
UI without `/legacy/`. Service pack instance creation, manual instance creation,
instance spec replace and runtime artifact list/import are connected in the new
UI without `/legacy/`. Nodes observability, diagnostics, inventory,
capabilities, service discovery and maintenance workflows for existing nodes are
connected in the new UI without `/legacy/`. Nodes bootstrap/security/control
workflows are connected in the new UI without `/legacy/` for configured nodes:
enrollment token create/rotate/revoke, bootstrap/reinstall job queueing,
host-key scan/pin, SSH session ticket launch, agent token rotation and
retire/force-retire. Remaining workflows listed below are still not migrated.

## 1. Summary

This evidence records the current 8.0.0 frontend branch after FE8-P0-05B:

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
- `Clients -> Delivery` is connected without `/legacy/` for share link
  create/rotate/revoke, VLESS subscription create-or-rotate/revoke and email
  delivery.
- Existing `Instances` runtime control is connected without `/legacy/` for
  list/detail, runtime state, revisions/rollback, apply/reapply, lifecycle
  actions, diagnostics, delete/force-delete, read-only access group
  materialization and async job tracking.
- `Services -> Service Packs` is connected without `/legacy/` for service pack
  list/detail, JSON create/update, enable/disable/delete and create instance
  from pack.
- `Instances` manual create and spec replace are connected without `/legacy/`
  through backend `POST /api/v1/instances` and `PUT /api/v1/instances/{id}/spec`.
- `Services -> Runtime Artifacts` is connected without `/legacy/` for runtime
  artifact list, safe metadata view and URL import.
- `Nodes` is connected without `/legacy/` for existing node list/detail,
  agent/runtime state, maintenance mode, inventory view/sync, capability
  install/verify, diagnostics retry/run, service discovery list/import and async
  job tracking.
- `Nodes -> Security/Bootstrap/Terminal/Lifecycle` is connected without
  `/legacy/` for enrollment token create/rotate/revoke, bootstrap/reinstall job
  queueing, host-key scan/pin for existing SSH methods, SSH session ticket
  launch, agent token rotation and retire/force-retire.
- Share/subscription one-time URLs are shown only in transient local UI state
  after backend create/rotate responses and are cleared on close.
- Enrollment token plaintext and SSH terminal ticket URLs are shown only in
  transient local UI state after backend create/rotate/session responses and are
  cleared on close.
- `/legacy/` remains the rollback UI and still covers non-migrated workflows.

## 2. Commands Run

| Check | Status | Evidence |
| --- | --- | --- |
| `gofmt -l cmd internal` | PASS | No files listed. |
| `go vet ./...` | PASS | No vet findings. |
| `go test ./...` | PASS | All Go package tests pass, including `internal/api/http`. |
| `go test -race ./...` | PASS | Race detector tests pass. |
| `go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin` | PASS | All operational binaries build. |
| `cd frontend && npm ci` | SKIP | This workstation session exposes bundled Node `v24.14.0`, but no native `npm` or `corepack` binary in `PATH`; existing `frontend/node_modules` was used for script-equivalent verification. GitHub CI remains configured for plain `npm ci`. |
| `cd frontend && npm run typecheck` | PASS | Equivalent command run through bundled Node and local TypeScript: `tsc --noEmit` plus `tsc -p tsconfig.node.json --noEmit`. |
| `cd frontend && npm run lint` | PASS | Equivalent command run through bundled Node and local ESLint. |
| `cd frontend && npm run test` | PASS | Equivalent Vitest run through bundled Node: 7 files, 62 tests passed. |
| `cd frontend && npm run i18n:check` | PASS | Equivalent command run through bundled Node: i18n key parity ok, 666 keys. |
| `cd frontend && npm run build` | PASS | Equivalent build run through bundled Node; Vite wrote `web/index.html`, `web/.vite/manifest.json`, `web/assets/index-CbDfBc6_.js`, `web/assets/index-CMdslovF.css`. |
| `scripts/ci/frontend-serving-smoke.sh` | PASS | Root/deep links/legacy/API non-shadowing/static asset 404 contract holds. |
| `scripts/ci/frontend-static-guards.sh` | PASS | Static frontend security guards pass. |
| `scripts/ci/docs-consistency.sh` | PASS | Documentation consistency ok for backend release `7.1.1.0`. |
| `scripts/smoke/vless-client-access-groups-smoke.sh` | SKIP | No `MEGAVPN_PUBLIC_BASE_URL` or `MEGAVPN_API_URL` was provided for a disposable/local API. |
| `scripts/smoke/service-pack-smoke.sh --plan <node-id> <endpoint-domain> [certificate-id]` | SKIP | No disposable/local API, target node or endpoint domain was available in this workstation session. |

Local note: this workstation did not expose a native `npm` or `corepack` binary
in `PATH`; frontend checks were run through the bundled Node runtime and local
`frontend/node_modules` binaries. The repository standard and GitHub CI path
remain plain `npm ci` and `npm run ...`.

## FE8-P0-05B Nodes Security/Control Test Evidence

`frontend/src/pages/infrastructure/NodesPage.test.tsx` verifies Nodes
bootstrap/security/control workflows against mocked backend API responses:

| Required behavior | Test evidence |
| --- | --- |
| Enrollment token list loads metadata only | Security tab renders `GET /api/v1/nodes/{id}/enrollment-tokens` hints/status without plaintext token. |
| Enrollment token create requires confirmation | `runs node bootstrap, security and lifecycle workflows safely` asserts `POST /api/v1/nodes/{id}/enrollment-token?ttl_hours=48` is not called before `Confirm`. |
| Enrollment token one-time value is transient | Same test verifies returned plaintext is masked, appears only after `Reveal`, and disappears after `Close`. |
| Enrollment token revoke requires confirmation | Same test asserts confirmed `DELETE /api/v1/nodes/{id}/enrollment-tokens/{token_id}`. |
| Host-key scan calls backend | Same test asserts `POST /api/v1/nodes/{id}/ssh/host-key-scan`. |
| Changed host-key warning visible | Same test renders the changed fingerprint warning when scanned fingerprint differs from the current pin. |
| Host-key pin requires confirmation | Same test asserts `PUT /api/v1/nodes/{id}/access-methods` is not called before `Confirm` and updates only `ssh_host_key_sha256`. |
| Agent token rotate requires confirmation | Same test asserts confirmed `POST /api/v1/nodes/{id}/agent-token/rotate` and job tracking. |
| Bootstrap requires confirmation | Same test asserts confirmed `POST /api/v1/nodes/{id}/bootstrap` and returned job tracking. |
| SSH session launch calls backend | Same test asserts confirmed `POST /api/v1/nodes/{id}/ssh/sessions`; returned terminal ticket URL is transient and cleared on close. |
| Retire/force-retire require confirmation | Same test asserts confirmed `DELETE /api/v1/nodes/{id}` and `POST /api/v1/nodes/{id}/force-retire` with exact typed confirmation and reason. |
| No token browser storage | Same test spies on `Storage.setItem` and verifies enrollment token / SSH ticket material is not persisted to localStorage or sessionStorage. |
| Backend revoke route is real | `TestPostgresIntegrationRevokeNodeEnrollmentTokenKeepsSecretHidden` verifies token status changes to revoked and revoke/list responses do not include plaintext token. |
| No `/legacy` workflow | Nodes tests assert implemented workflows never request `/legacy`. |
| No raw page API calls | Nodes tests keep raw `/api/v1`, raw `fetch`, `dangerouslySetInnerHTML` and `/legacy` out of the page component. |

Node bootstrap/security/control workflows work in the new UI without `/legacy/`
for configured nodes.

## FE8-P0-05A Nodes Test Evidence

`frontend/src/pages/infrastructure/NodesPage.test.tsx` verifies Nodes
observability, diagnostics, inventory, capabilities and service discovery
workflows against mocked backend API responses:

| Required behavior | Test evidence |
| --- | --- |
| Node list loads | `loads node detail, observability data and renders backend text safely`; asserts `GET /api/v1/nodes`. |
| Node detail loads | Same test asserts `GET /api/v1/nodes/{id}` and opens the detail drawer. |
| Agent/runtime state displays | Same test renders heartbeat, communication state, agent metadata and timestamps from `GET /api/v1/nodes/{id}/diagnostics`. |
| Diagnostics output is text | Same test renders script-like backend strings and asserts no `<script>` element is created. |
| Inventory view/sync | Inventory tab renders payload as text; `runs maintenance, inventory, capabilities, diagnostics and discovery only after confirmation` asserts confirmed `POST /api/v1/nodes/{id}/inventory/sync` and returned job tracking. |
| Maintenance mode | Same mutation test asserts maintenance is not called before confirmation and then calls `POST /api/v1/nodes/{id}/maintenance/enable`. |
| Capability install/verify | Same mutation test asserts confirmed `POST /api/v1/nodes/{id}/capabilities/install` and `/verify` with backend payload. |
| Diagnostics run/retry | Same mutation test asserts confirmed `POST /diagnostics/channel-probe`, `/retry-inventory`, `/retry-discovery`, `/reconcile-runtime` and `/requeue-stuck-job`. |
| Service discovery import | Same mutation test asserts confirmed `POST /services/discover` and `POST /services/discovered/{discovery_id}/import`. |
| Backend error safety | `shows backend 403, 422 and 409 errors safely`; renders permission, validation and conflict errors as text. |
| No `/legacy` workflow | Tests assert no request path starts with `/legacy` for implemented Nodes workflows. |
| No raw page API calls | `keeps raw API paths and legacy workflow links out of the Nodes page component`; verifies no `/api/v1`, raw `fetch`, `dangerouslySetInnerHTML` or `/legacy` in the page component. |

Nodes observability, diagnostics and inventory workflows work in the new UI
without `/legacy/`.

## 3. Clients Core Test Evidence

`frontend/src/pages/clients/ClientsPage.test.tsx` verifies FE8-P0-03A and
FE8-P0-03B client workflows against mocked backend API responses:

| Required behavior | Test evidence |
| --- | --- |
| Clients page loads list | `loads the client list and opens a real detail drawer`; asserts `GET /api/v1/clients`. |
| Client detail loads | Same test asserts `GET /api/v1/clients/{id}` and renders metadata. |
| Create client mutation | `creates clients through the backend and handles 409 and 422 responses`; asserts `POST /api/v1/clients`. |
| Conflict/validation handling | Same test renders `409` conflict and maps a `422` username field error. |
| Status action | `runs status, revoke and delete actions only through confirmed backend mutations`; asserts `POST /suspend`. |
| Delete/revoke confirmation | Same test confirms before `POST /revoke` and `DELETE /clients/{id}`. |
| Current VLESS group visible | `assigns a single client to a VLESS group with preview, stale guard and apply`; renders current VLESS group. |
| VLESS preview endpoint | Same test asserts `POST /client-access-groups/{group_id}/members:preview`. |
| Preview enables Apply | Same test verifies Apply becomes enabled only after successful preview. |
| Stale preview disables Apply | Same test changes mode after preview and verifies Apply disables. |
| VLESS apply endpoint | Same test asserts `POST /client-access-groups/{group_id}/members:bulk-apply`. |
| Remove VLESS membership | `removes VLESS membership through the backend after confirmation`; asserts backend member `DELETE`. |
| Artifact list/build/download/delete | `lists, builds, downloads and deletes client artifacts through backend endpoints`; asserts list, build, download URL open and delete. |
| Job tracking | Revoke/artifact tests render job tracking for returned job IDs. |
| Permission handling | `shows permission errors safely and keeps Clients workflows away from legacy`; renders `403` text. |
| No `/legacy` workflow | Tests assert no request path starts with `/legacy`. |
| No raw page API calls | `keeps raw API paths out of the Clients page component`; verifies no `/api/v1` string or raw `fetch` in the page component. |

Clients core workflow works in the new UI without `/legacy/`.

Single-client VLESS access assignment works in the new UI without `/legacy/`.

Client artifacts workflow works in the new UI without `/legacy/`.

## 4. Client Delivery Test Evidence

`frontend/src/pages/clients/ClientsPage.test.tsx` verifies FE8-P0-03B delivery
workflows against mocked backend API responses:

| Required behavior | Test evidence |
| --- | --- |
| Delivery tab renders | Delivery tests open `Clients -> Delivery` from the client drawer. |
| Artifact row delivery action | `opens delivery from an artifact row and creates a one-time share link safely`; opens Delivery from a ready artifact row. |
| Share links list loads | Same test renders mocked `GET /api/v1/clients/{id}/share-links` data. |
| Create share link | Same test asserts `POST /api/v1/clients/{id}/share-links`. |
| One-time share URL warning | Same test shows one-time warning and masked value before reveal. |
| One-time panel close clears secret | Same test closes the panel and verifies the token disappears from UI. |
| Explicit clipboard copy | Same test verifies clipboard write happens only after user clicks `Copy`. |
| Share revoke confirmation | `requires confirmation for share link revoke and rotate`; backend revoke is not called before `Confirm`. |
| Share rotate confirmation | Same test asserts confirmed `POST /share-links/{link_id}/rotate` and one-time rotated URL display. |
| Subscription list loads | `manages VLESS subscriptions with one-time URL display and confirmed revoke`; renders mocked subscription rows. |
| Create/rotate VLESS subscription | Same test asserts `POST /api/v1/clients/{id}/subscriptions/rotate`. |
| Subscription one-time URL | Same test shows the backend returned subscription URL only after reveal. |
| Subscription revoke confirmation | Same test asserts confirmed `POST /subscriptions/{subscription_id}/revoke`. |
| Email delivery | `sends client delivery email through the backend and renders status safely`; asserts `POST /api/v1/clients/{id}/deliver-email`. |
| Backend error safety | Permission test renders backend errors as text and does not use `/legacy`. |
| No HTML sink | Static guard and raw API test keep delivery code away from unreviewed `dangerouslySetInnerHTML`. |

Backend route coverage added in `internal/api/http/share_links_test.go`:

| Required behavior | Test evidence |
| --- | --- |
| Share link rotate invalidates old token | `TestRotateShareLinkTokenRevokesOldLinkAndPublishesSameTarget` verifies revoke of old link and publish of a new link for the same target with the requested TTL. |

Client share links workflow works in the new UI without `/legacy/`.

Client subscriptions workflow works in the new UI without `/legacy/`.

Client email delivery workflow works in the new UI without `/legacy/`.

## 5. Instances Runtime Control Test Evidence

`frontend/src/pages/services/InstancesPage.test.tsx` verifies FE8-P0-04A
Instances runtime workflows against mocked backend API responses:

| Required behavior | Test evidence |
| --- | --- |
| Instance list loads | `loads instance list, opens detail, and shows runtime state`; asserts `GET /api/v1/instances`. |
| Instance detail loads | Same test asserts `GET /api/v1/instances/{id}` and renders the detail drawer. |
| Runtime state visible | Same test opens Runtime and renders backend `runtime_status`, `health_status` and config hash. |
| Apply confirmation | `requires confirmation for apply and shows the returned job`; verifies no backend call before `Confirm`. |
| Apply endpoint/job | Same test asserts `POST /api/v1/instances/{id}/apply` and renders the returned job ID. |
| Reapply endpoint | Same test verifies Reapply uses the real backend apply endpoint after confirmation. |
| Rollback explicit revision | `rolls back an explicit revision and queues a real apply job`; requires selected revision and confirmation. |
| Rollback endpoint/apply job | Same test asserts `POST /rollback`, then a real `POST /apply` when backend returns `can_apply`. |
| Diagnostics rendered safely | `renders diagnostics as text and runs backend diagnostics after confirmation`; asserts script-like backend text is not executed. |
| Diagnostics endpoint | Same test asserts `POST /diagnose` only after confirmation. |
| Lifecycle actions | `runs lifecycle, delete and force-delete only after confirmation`; asserts `restart`, `start`, `stop`, `enable` and `disable` endpoints. |
| Delete confirmation | Same test asserts confirmed `DELETE /api/v1/instances/{id}`. |
| Force-delete stronger confirmation | Same test requires exact `DELETE <instance>` confirmation and reason before `POST /force-delete`. |
| Access groups read-only | `keeps access groups read-only and links management to Clients Groups`; renders materialized groups and `/clients/groups` link. |
| No primary VLESS management | Same test verifies no Create VLESS group or Add clients action is exposed under Instances. |
| 403/422/409 handling | `shows backend 403, 422 and 409 errors safely`; renders permission, validation and conflict text. |
| No `/legacy` workflow | Access group test asserts no request path starts with `/legacy`. |
| No raw page API calls | `keeps raw API paths and legacy workflow links out of the Instances page component`; verifies no `/api/v1`, raw `fetch`, `dangerouslySetInnerHTML` or `/legacy` in the page component. |

Instances runtime control works in the new UI without `/legacy/`.

Unsupported or deferred Instances sub-actions:

- backend has no separate instance apply preview/validate endpoint, so apply and
  reapply are direct backend mutations guarded by explicit confirmation;
- backend has no separate service pack validation endpoint, so service pack
  create/update uses backend validation during the real `PUT` mutation;
- backend has no separate instance spec preview endpoint or draft-save HTTP
  route, so spec replace uses local JSON object validation, explicit
  confirmation and backend validation during `PUT /api/v1/instances/{id}/spec`;
- backend has no runtime binary artifact delete endpoint in this release, so
  runtime artifact delete remains disabled with the exact backend reason;
- Instances show access group materialization read-only and do not own primary
  VLESS group/member management.

## 6. Service Packs / Instance Creation / Runtime Artifacts Test Evidence

`frontend/src/pages/services/ServiceWorkspace.test.tsx` and
`frontend/src/pages/services/InstancesPage.test.tsx` verify FE8-P0-04B service
provisioning workflows against mocked backend API responses:

| Required behavior | Test evidence |
| --- | --- |
| Services workspace tabs | `renders Services workspace tabs and opens service pack detail`; asserts links for `Instances`, `Service Packs` and `Runtime Artifacts`. |
| Service pack list loads | Same test renders mocked `GET /api/v1/service-packs` rows. |
| Service pack detail opens | Same test opens a pack drawer and renders recommendations safely as text/JSON. |
| Service pack create/update/delete/status | `updates and deletes service packs through backend management endpoints`; asserts `PUT /api/v1/service-packs/{key}`, `POST /disable` and `DELETE /service-packs/{key}`. |
| Create from service pack confirmation | `creates instances from a service pack and shows instance and job links`; verifies no backend create call before `Confirm`. |
| Create from service pack endpoint | Same test asserts `POST /api/v1/service-packs/{key}/instances`. |
| Create result links | Same test renders created instance/job evidence and links to `/services/instances` and `/operations/jobs`. |
| 403 handling | `shows service pack create errors distinctly for 403, 422 and 409`; renders permission text safely. |
| 422 handling | Same test renders validation text safely. |
| 409 handling | Same test renders conflict text safely. |
| Manual instance create | `creates manual instances and replaces specs through backend endpoints`; asserts `POST /api/v1/instances` with node, service and name. |
| Spec replace | Same test asserts `PUT /api/v1/instances/{id}/spec` only after confirmation. |
| Runtime artifact list | `lists, imports and safely renders runtime artifact metadata without delete support`; renders mocked `GET /api/v1/binary-artifacts` rows. |
| Runtime artifact import | Same test asserts `POST /api/v1/binary-artifacts/import-url`. |
| Runtime artifact delete unsupported | Same test verifies delete is disabled with `Backend has no binary runtime artifact delete endpoint in this release.` |
| Artifact metadata safe rendering | Same test renders script-like metadata as text and verifies no `img` element is created. |
| No `/legacy` workflow | Services workspace tests assert no `/legacy` workflow links or requests for implemented pages. |
| No raw page API calls | Services workspace tests assert no `/api/v1` string or raw `fetch` in Services page components. |

Service pack instance creation works in the new UI without `/legacy/`.

Manual instance creation works in the new UI without `/legacy/`.

Runtime artifacts workflow works in the new UI without `/legacy/` for list,
safe metadata view and URL import. Runtime artifact delete remains disabled
because the backend has no binary runtime artifact delete endpoint in this
release.

## 7. VLESS Groups Test Evidence

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

## 8. Firewall Test Evidence

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

## 9. Integrated API Smoke

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

Current live API smoke evidence is SKIP, not PASS: no disposable DB/API base
URL was available in this workstation session. FE8-P0-03B added a backend unit
test for share link rotation and frontend/API-contract tests for delivery, but
not a live DB delivery smoke.

Service pack runtime provisioning can be smoke-tested against a disposable node
with the existing service pack smoke command sequence:

```bash
export MEGAVPN_PUBLIC_BASE_URL=http://127.0.0.1:8080
export MEGAVPN_AUTH_TOKEN=<operator-or-test-token>
scripts/smoke/service-pack-smoke.sh --plan <node-id> <endpoint-domain> [certificate-id]
scripts/smoke/service-pack-smoke.sh --matrix <node-id> <endpoint-domain> [certificate-id]
```

Current service pack live smoke evidence is SKIP, not PASS: no disposable API,
target node or endpoint domain was available in this workstation session.

## 10. Static Serving Evidence

Backend tests and `scripts/ci/frontend-serving-smoke.sh` cover:

- `GET /` returns the new UI;
- frontend deep links return the new UI;
- `GET /legacy/` returns the rollback UI;
- `/api/*` and `/agent/*` are not shadowed by SPA fallback;
- missing root `/assets/*` return 404 rather than SPA HTML.

## 11. Security / Review Hygiene

Current enforced hygiene:

- no raw `/api/v1` calls outside `frontend/src/shared/api` and tests;
- unsafe methods keep the shared API client, cookie credentials and CSRF header;
- no browser auth token/session storage in new frontend source;
- no share/subscription token storage in browser storage;
- no unreviewed `dangerouslySetInnerHTML`;
- no production console logging in new frontend source;
- backend errors and job output are rendered as text;
- artifact download uses a backend URL and is not persisted in browser storage;
- share and subscription one-time URLs live only in transient component state
  and are cleared on close;
- clipboard copy is explicit and user-triggered;
- share/subscription revoke and rotate require confirmation;
- VLESS apply actions require backend preview and stale preview disables apply;
- Clients revoke/delete/artifact delete require confirmation;
- Instances apply/reapply, rollback, diagnostics, lifecycle and delete actions
  require confirmation and use backend-accepted responses before showing
  success;
- Instances diagnostics, runtime observations and backend errors are rendered as
  text, not HTML;
- service pack definitions, instance specs and runtime artifact metadata are
  rendered as text/JSON, not HTML;
- Nodes diagnostics, inventory, capability and discovery payloads are rendered as
  text/JSON, not HTML;
- Nodes maintenance, inventory sync, capability install/verify, diagnostics
  actions and service discovery import require confirmation and use
  backend-accepted responses before showing success;
- unsupported non-VLESS services remain catalog-only or legacy-only.

## 12. Write Workflow Summary

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
- `Clients -> Delivery` share link create/rotate/revoke, VLESS subscription
  create-or-rotate/revoke and email delivery;
- `Firewall` address groups, policies, rules, preview, apply, node state and
  emergency disable;
- `Instances` existing-instance runtime control: list/detail, runtime state,
  revisions/rollback, apply/reapply, start/stop/restart/enable/disable,
  diagnostics, delete/force-delete, read-only access group materialization and
  async job tracking;
- `Services -> Service Packs` list/detail/create/update/enable/disable/delete
  and create instance from pack;
- `Instances` manual create and spec replace;
- `Services -> Runtime Artifacts` list, safe metadata view and URL import;
- `Nodes` existing-node observability: list/detail, agent/runtime state,
  maintenance mode, inventory view/sync, capability install/verify, diagnostics
  retry/run, service discovery list/import and async job tracking;
- `Nodes` bootstrap/security/control for configured nodes: enrollment token
  create/rotate/revoke, bootstrap/reinstall job queueing, host-key scan/pin,
  SSH session ticket launch, agent token rotation and retire/force-retire.

Still disabled, read-only or legacy-only:

- non-VLESS access group materialization workflows;
- migration conflict UI for access groups;
- client routes, access rotation and config cleanup;
- client delivery history;
- nodes create/register/edit, new SSH access method creation with secret
  material, manual bootstrap bundle reveal, agent identity revoke, reboot,
  emergency cleanup, stale rotation cleanup and route policy preview/apply/cleanup;
- node service discovery ignore/unignore;
- runtime artifact delete;
- separate service pack validation, instance spec preview and instance draft-save
  endpoints;
- certificates import/issue/default/revoke/delete;
- platform settings save, mail test and TLS apply;
- backhaul mutations;
- backup/restore browser UI.

## 13. Known Limitations

- Backend binary/version metadata remains `7.1.1.0`; synchronizing it to
  `8.0.0` is a separate release task.
- Full normal operator work still requires `/legacy/` for many non-Firewall,
  non-VLESS and non-Clients workflows.
- Generic client edit stays disabled because the backend has no generic
  `PATCH/PUT /clients/{id}` endpoint.
- Client disable stays disabled because the backend exposes activate/suspend
  but no separate browser disable endpoint.
- Client routes, service access delete/rotation and config cleanup are not part
  of FE8-P0-03B.
- Client delivery history stays unavailable because the backend has no
  client-scoped delivery history list/status endpoint in this release.
- Client email delivery is connected, but the backend endpoint is synchronous,
  sends the client's available artifacts/configs and does not accept an
  artifact-specific email payload yet.
- Instances apply/reapply has no separate backend preview/validate endpoint in
  this release; the new UI uses explicit confirmation and then calls the real
  backend apply endpoint.
- Instances rollback returns an apply-ready revision rather than a job; the new
  UI queues a real apply job when backend reports `can_apply`.
- Service pack create/update has no separate backend validation endpoint; the
  new UI submits real backend mutations after explicit operator action and
  renders backend validation/conflict errors safely.
- Instance spec editing has no separate preview endpoint or draft-save HTTP
  route in this release; spec replace is confirmed and validated by the real
  backend `PUT` mutation.
- Runtime artifact delete remains disabled because the backend has no binary
  runtime artifact delete endpoint in this release.
- No browser screenshot/responsive Playwright evidence was produced in this
  pass.
- Integrated live API smoke was not run against a disposable DB/API in this
  session; FE8-P0-05B evidence is frontend/API-contract test coverage against
  mocked backend responses plus the local command set recorded above.

## 14. Go / No-Go

Recommendation:

- GO for PR review and CI validation of the 8.0.0 frontend branch.
- GO for using new UI `Clients -> Groups -> VLESS`, Clients core/artifacts,
  Clients delivery, Firewall preview/apply/disable, existing Instances runtime
  control, service pack instance creation, manual instance creation, runtime
  artifact list/import, existing Nodes observability/diagnostics/inventory and
  Nodes bootstrap/security/control workflows in controlled staging after
  operator review.
- NO-GO for final 8.0.0 release cutover or removing `/legacy/`.

Remaining blockers for final cutover:

1. run integrated smoke/e2e against disposable DB/API data for VLESS, Clients
   core/artifacts/delivery, Firewall and Instances/Service Packs runtime
   operator flows;
2. migrate remaining Nodes create/register/edit, route policy and destructive
   remediation workflows not included in FE8-P0-05B, including agent identity
   revoke, reboot, emergency cleanup and stale rotation cleanup;
3. add backend/browser support for runtime artifact delete if it is required for
   final operator parity;
4. migrate Clients routes, access rotation and config cleanup;
5. migrate Certificates and Platform settings write workflows;
6. add E2E/browser responsive evidence for critical operator flows;
7. synchronize backend/frontend version and release-chain artifacts to `8.0.0`;
8. run full release gate in the release environment.
