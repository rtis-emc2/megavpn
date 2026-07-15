# RTIS MegaVPN Frontend Write Workflows 8.0.0

Release scope: MegaVPN Console 8.0.0 RC1.

Status: workflow matrix for the new React console.

## 1. RC1 Rule

The new console must not show fake-success actions. Every action is classified
as one of:

- `fully connected`;
- `read-only`;
- `disabled intentionally`;
- `legacy-only`;
- `backend-missing`.

If a backend mutation is not wired with request, error handling, invalidation,
confirmation and job tracking where needed, the action stays disabled or links
to `/legacy/`.

## 2. Workflow Matrix

### Auth

| Workflow | Status | Notes |
| --- | --- | --- |
| Login | fully connected | Cookie session; no token storage. |
| Logout | fully connected | Invalidates session query. |
| Invite accept | fully connected | Token is taken from query and should be removed from URL after accept. |
| Session refresh/current user | fully connected | `GET /api/v1/auth/me`. |
| Change password | legacy-only | Backend exists; new form not wired in RC1. |

### Jobs

| Workflow | Status | Notes |
| --- | --- | --- |
| Jobs list | fully connected | Polling through TanStack Query. |
| Job detail | fully connected | Detail drawer reads backend job. |
| Job logs | fully connected | Render text safely; no HTML sink. |
| Job cancel | fully connected | Backend endpoint wired with confirmation, error display and query invalidation. |
| Job retry/requeue | legacy-only generic / connected under Nodes diagnostics | Generic retry UX is not migrated. Domain-specific selected node diagnostics retry/requeue is wired under `Nodes`. |

### Clients

| Workflow | Status | Notes |
| --- | --- | --- |
| List/search/filter | fully connected | `GET /api/v1/clients`; search/status filters run in the workspace over the backend list. |
| Create client | fully connected | `POST /api/v1/clients`; validation/conflict responses are preserved and mapped in the form where possible. |
| Edit client | fully connected | `PATCH /api/v1/clients/{id}` updates display name, email, notes and expiry with backend validation and query invalidation. |
| Activate/suspend | fully connected | `POST /api/v1/clients/{id}/suspend` and `/activate`; query invalidation is wired. |
| Delete client | fully connected | `DELETE /api/v1/clients/{id}` with confirmation and client list invalidation. |
| Provision through access groups | fully connected for single-client VLESS | Client detail uses the same access-group member preview/apply backend model as `Clients -> Groups`; no one-job-per-client bulk path is introduced. Non-VLESS provisioning remains legacy-only. |
| Revoke client | fully connected | `POST /api/v1/clients/{id}/revoke`; returned job is shown in the client workspace. |
| Routes list | fully connected | `GET /api/v1/clients/{id}/routes`; routes render in the client detail drawer without `/legacy/`. |
| Route create | fully connected | `POST /api/v1/clients/{id}/routes`; form targets an active service access, preserves backend validation errors and does not fake success. |
| Route delete | fully connected | `DELETE /api/v1/clients/{id}/routes/{route_id}` with destructive confirmation; backend revokes the route and queues route policy convergence where applicable. |
| Route update | fully connected | `PATCH /api/v1/clients/{id}/routes/{route_id}` updates explicit routes with backend validation and route-policy convergence; baseline service routes remain backend-protected. |
| Accesses list | fully connected | `GET /api/v1/clients/{id}/accesses`; service access identity is redacted and UUID/credential metadata is not displayed. |
| Access rotation | fully connected | `POST /api/v1/clients/{id}/accesses/{access_id}/rotate-*`; driver suffix is whitelisted, confirmation is required, returned job is linked/tracked and no frontend secret generation is used. Backend has no preview endpoint. |
| Access delete | fully connected | `DELETE /api/v1/clients/{id}/accesses/{access_id}` with confirmation; cleanup counts and queued job counts are shown. |
| Access revoke | fully connected | `POST /api/v1/clients/{id}/accesses/{access_id}/revoke` requires confirmation, revokes the selected service access without deleting it, revokes related active routes/share links and shows queued convergence counts. |
| Config cleanup | fully connected | `DELETE /api/v1/clients/{id}/configs` with destructive confirmation; result counts are shown and no config payloads/tokens are rendered. |
| Artifact build | fully connected | `POST /api/v1/clients/{id}/artifacts`; returned job is tracked in the drawer. |
| Artifact download | fully connected | `GET /api/v1/clients/{id}/artifacts/{artifact_id}/download` opened through a backend URL; no token storage. |
| Artifact delete | fully connected | `DELETE /api/v1/clients/{id}/artifacts/{artifact_id}` with confirmation and artifact invalidation. |
| Email delivery | fully connected | `POST /api/v1/clients/{id}/deliver-email`; synchronous backend result is shown safely. Backend sends the client's available artifacts/configs and has no artifact-specific email payload yet. |
| Share link create/rotate/revoke | fully connected | `GET/POST /api/v1/clients/{id}/share-links`, `POST /share-links/{link_id}/rotate`, `POST /share-links/{link_id}/revoke`; create/rotate show one-time URL only in transient UI state and revoke/rotate require confirmation. |
| VLESS subscription create-or-rotate/revoke | fully connected for VLESS | `GET /api/v1/clients/{id}/subscriptions`, `POST /subscriptions/rotate`, `POST /subscriptions/{subscription_id}/revoke`; backend exposes create-or-rotate rather than separate create. One-time subscription URL is not persisted. |
| Delivery history | fully connected | `GET /api/v1/clients/{id}/deliveries`; UI renders masked destination hints, counts, statuses and redacted safe error summaries only. |

### Clients -> Groups

| Workflow | Status | Notes |
| --- | --- | --- |
| List access groups | fully connected | `GET /api/v1/client-access-groups`. |
| List access services | fully connected | `GET /api/v1/client-access-services`. |
| Create VLESS group | fully connected | `POST /api/v1/client-access-groups`; unsupported services stay catalog-only/disabled. |
| Update VLESS policy/status | fully connected | `PATCH /api/v1/client-access-groups/{group_id}`; group key remains immutable in UI. |
| Delete group | legacy-only | Backend exists; destructive confirmation is not exposed in the new VLESS workflow. |
| Disable VLESS group | fully connected | Status can be changed through the edit form and backend PATCH validation. |
| Scope read/update | fully connected | `GET/PATCH /api/v1/client-access-groups/{group_id}/scope`; selected/all/except modes are wired. |
| Member list | fully connected | `GET /api/v1/client-access-groups/{group_id}/members`. |
| Available client picker | fully connected | `GET /api/v1/client-access-groups/available-clients` with search/status/assignment/page size. |
| Member preview | fully connected | `POST /api/v1/client-access-groups/{group_id}/members:preview`; no local fake preview. |
| Member apply | fully connected | `POST /api/v1/client-access-groups/{group_id}/members:bulk-apply`; requires a fresh successful backend preview. |
| Member remove | fully connected | `DELETE /api/v1/client-access-groups/{group_id}/members/{client_id}` with confirmation. |
| Sync preview | fully connected | `POST /api/v1/client-access-groups/{group_id}/sync:preview`. |
| Sync apply | fully connected | `POST /api/v1/client-access-groups/{group_id}/sync:apply`; requires backend preview. |
| Migration conflict handling | legacy-only | Backend exists; conflict UI not migrated. |

### Network Policy

| Workflow | Status | Notes |
| --- | --- | --- |
| Firewall inventory | fully connected | `GET /api/v1/firewall`; policies, rules, address groups, entries and node states render in the new UI. |
| Firewall address group CRUD | fully connected | `POST/PUT/DELETE /api/v1/firewall/address-lists`; entries use `/address-lists/{id}/entries`. DNS-only and zero-renderable group warnings are visible. |
| Firewall policy CRUD | fully connected | `POST/PUT/DELETE /api/v1/firewall/policies`; default input/forward/output policy is editable. |
| Firewall rule CRUD | fully connected | `POST/PUT/DELETE /api/v1/firewall/policies/{id}/rules`; rules show chain/action/priority/source/destination/protocol/ports/state. Rule reorder remains disabled because the backend has no reorder endpoint. |
| Firewall management/safety settings | connected read | `GET /api/v1/firewall/management-settings`; UI displays trusted control-plane/operator/SSH source presence. Settings update wrapper exists, but the page does not expose a write form in this task. |
| Node firewall preview | fully connected | `POST /api/v1/nodes/{id}/firewall/preview`; preview job payload/result is rendered as text, not HTML. |
| Node firewall apply | fully connected | `POST /api/v1/nodes/{id}/firewall/apply`; Apply is disabled until backend preview request succeeds and becomes disabled again when preview is stale or blocking errors exist. |
| Node firewall disable | fully connected | `POST /api/v1/nodes/{id}/firewall/disable`; emergency confirmation states the exact managed table removal scope and shows job tracking. |
| Route policy UI | fully connected | Node-scoped route policy list/detail is derived from `GET /api/v1/nodes`; preview uses `GET /api/v1/nodes/{id}/routes/preview`, Apply uses `POST /routes/apply`, cleanup uses `POST /routes/cleanup`. Apply is disabled until a fresh successful preview for the selected node; stale preview blocks Apply and jobs are tracked. |
| Traffic overview | fully connected | `GET /api/v1/traffic/accounting`. |
| Traffic export | fully connected | Backend export URL opened directly; no token storage. |

### Instances / Services

| Workflow | Status | Notes |
| --- | --- | --- |
| Instances list | fully connected | Read path. |
| Instance detail/runtime/revisions | fully connected | Detail drawer loads `GET /api/v1/instances/{id}`, runtime state, runtime observations and revisions. Runtime/diagnostic output is rendered as text. |
| Create from service pack | fully connected | `POST /api/v1/service-packs/{key}/instances`; confirmation required when no separate validation/preview endpoint exists. Created/existing instances and returned jobs are shown. |
| Manual create | fully connected | `POST /api/v1/instances`; service type and node options come from backend APIs. Spec is edited as JSON text and submitted only through the shared API client. |
| Draft/spec replace | partially connected | Spec replace is fully connected through `PUT /api/v1/instances/{id}/spec` with confirmation and backend validation. Separate preview/validate and draft-save HTTP routes are backend-missing, so those sub-actions are not exposed. |
| Apply/reapply | fully connected | `POST /api/v1/instances/{id}/apply`; confirmation required; returned job is tracked. Backend has no separate preview endpoint. |
| Lifecycle start/stop/restart/enable/disable | fully connected | Real backend lifecycle endpoints are wired with confirmation and job tracking. |
| Rollback | fully connected | Explicit revision selection and confirmation required. Backend rollback creates a new revision; when it is apply-ready, UI queues a real apply job for runtime effect. |
| Diagnostics | fully connected | `POST /api/v1/instances/{id}/diagnose`; runtime observations are rendered safely as text. |
| Delete/force-delete | fully connected | Delete and force-delete call real backend endpoints. Force-delete requires exact confirmation text. |
| Service pack list/detail | fully connected | `GET /api/v1/service-packs`; detail is derived from the list because the backend has no separate detail route. |
| Service pack CRUD/status | fully connected | `PUT /api/v1/service-packs/{key}`, `POST /enable`, `POST /disable`, `DELETE /service-packs/{key}`; backend validation/conflict/permission errors are surfaced safely. Separate service pack validation endpoint is backend-missing. |
| Access group materialization | read-only connected | Instances show materialized access groups and link to `Clients -> Groups`; no add/move/remove/create VLESS group actions are exposed here. |
| VLESS templates | legacy-only/deprecated | Primary access group management belongs under Clients -> Groups. |
| Runtime artifact list/import/delete | partially connected | List and URL import are fully connected through `GET /api/v1/binary-artifacts` and `POST /api/v1/binary-artifacts/import-url`. Metadata is rendered as text. Delete remains backend-missing because no binary runtime artifact DELETE route exists. |

### Nodes

| Workflow | Status | Notes |
| --- | --- | --- |
| Nodes list/detail | fully connected | Existing node list/detail uses `GET /api/v1/nodes` and `GET /api/v1/nodes/{id}` with search/status/role filters and a detail drawer. |
| Create node record | fully connected | `POST /api/v1/nodes` through `createNode` / `useCreateNode`; form exposes only safe profile metadata and leaves enrollment/bootstrap/agent registration to dedicated workflows. |
| Edit node metadata | fully connected | `PUT /api/v1/nodes/{id}` through `updateNode` / `useUpdateNode`; generic edit does not mutate status, maintenance, retire state, agent identity, tokens, secret refs, heartbeat/runtime state or access credentials. |
| Agent registration/onboarding | read-only guided status connected / mutations pending | The new Onboarding tab derives a typed status model from existing operator read APIs: node detail, diagnostics, enrollment-token metadata, bootstrap runs, heartbeat and inventory. It answers the current stage, completed/pending/unhealthy milestones, safe backend evidence and next existing tab to open. It does not call `/agent/*`, create/rotate/revoke enrollment tokens, start bootstrap, reveal/download bundles or queue inventory sync. Guided onboarding mutations remain Step 4C.2, final acceptance/debt closure remains Step 4D and live external-node smoke remains release-validation debt. |
| Retire | fully connected | `DELETE /api/v1/nodes/{id}` with confirmation and backend dependency validation. |
| Force retire | fully connected | `POST /api/v1/nodes/{id}/force-retire` with typed node-name confirmation, reason and backend cleanup validation. |
| Maintenance mode | fully connected | `POST /api/v1/nodes/{id}/maintenance/enable` and `/disable`; confirmation required, backend error states are rendered safely. |
| Bootstrap/reinstall | fully connected for configured nodes | `POST /api/v1/nodes/{id}/bootstrap`; SSH bootstrap/manual bundle job queueing and reinstall require confirmation and show jobs. |
| Manual bootstrap bundle reveal/download | fully connected | Uses only `POST /api/v1/nodes/{id}/bootstrap-runs/{run_id}/bundle/reveal` and `POST /api/v1/nodes/{id}/bootstrap-runs/{run_id}/bundle/download`; the deprecated compatibility `GET /bundle` endpoint and `/api/v1/secret-refs` are not used. Availability is `manual_bundle_available === true`; reveal/download require `node.bootstrap`, explicit confirmation and acknowledgement. Revealed content is transient component state, cleared on close/target changes/permission loss/new reveal/unmount, and download uses a no-store blob response with temporary object URL revocation. Repeated operations are allowed while backend material remains available and each action is separately audited. Evidence: backend commit `a6aee38cedec281d2037741c6ba2dbac5e47840f`, frontend commit `27fcaf4a0e7fe90e3cb6ee80a0f2b22de05722cb`, evidence HEAD `3abc200c3d7c5525eaded994244af488d0728b41`, CI `29391281058`, required PostgreSQL/HTTP tests executed without skips. Live operator onboarding validation remains release-validation debt. |
| SSH terminal/session launch | fully connected for configured SSH methods | `POST /api/v1/nodes/{id}/ssh/sessions`; UI shows the backend-issued short-lived terminal URL only in transient state. No frontend SSH implementation and no browser credential storage. |
| Host-key scan/pin | fully connected for existing SSH methods and new SSH method creation | `POST /ssh/host-key-scan` and `PUT /access-methods`; changed fingerprint warning is visible and pin requires confirmation for configured methods. New SSH method creation also uses host-key scan through the dedicated create workflow below. |
| Create SSH access method | fully connected | Three-stage flow: enter host/port/user, scan host key and explicitly select a fingerprint with independent verification, then enter a dedicated private key and submit it once to encrypted backend storage. Uses `node.bootstrap`, dedicated `POST /api/v1/nodes/{id}/access-methods/ssh`, no automatic fingerprint trust, no `secret_ref_id` display, no browser persistence, no `/legacy/` dependency, safe conflict/error handling and access-method query invalidation after success. |
| Enrollment tokens | fully connected | `GET /enrollment-tokens`, `POST /enrollment-token`, `POST /enrollment-token/rotate`, `DELETE /enrollment-tokens/{token_id}`. Plaintext tokens are shown only once from create/rotate responses and cleared on close. |
| Agent token rotation | fully connected | `POST /agent-token/rotate`; confirmation required, returned job is tracked, no new token plaintext is exposed to the browser. |
| Agent identity revoke / reboot / emergency cleanup / stale rotation cleanup | legacy-only | Backend exists, but FE8-P0-05B does not expose these destructive remediation paths. |
| Agent/runtime state | fully connected | Node diagnostics payload shows heartbeat, communication state and agent job/inventory/discovery/runtime timestamps without rendering secrets or HTML. |
| Diagnostics retry/run | fully connected | `GET /diagnostics`, `POST /diagnostics/retry-inventory`, `/retry-discovery`, `/channel-probe`, `/requeue-stuck-job` and `/reconcile-runtime`; confirmation and job tracking required. Runtime reconcile may queue backend-defined dependent runtime jobs. |
| Inventory view/sync | fully connected | `GET /inventory` and `POST /inventory/sync`; sync requires confirmation and shows returned job. Inventory payload is rendered as text. |
| Capabilities install/verify | fully connected | `GET /capabilities`, `GET /capabilities/drift`, `GET /capabilities/install-events`, `GET /services/installers`, `POST /capabilities/install`, `POST /capabilities/verify`; install/verify require confirmation and job tracking. |
| Service discovery list/import | fully connected | `GET /services/discovered`, `GET /services/discovery-summary`, `POST /services/discover`, `POST /services/discovered/{id}/import`, `POST /services/import-all`; import requires confirmation. Ignore/unignore stays legacy-only. |
| Route policy preview/apply/cleanup | fully connected | `GET /routes/preview`, `POST /routes/apply`, `POST /routes/cleanup`; preview output is rendered as text/tables, Apply requires a fresh selected-node preview and cleanup requires confirmation. |

### Platform

| Workflow | Status | Notes |
| --- | --- | --- |
| Runtime preflight | fully connected | `GET /api/v1/runtime/preflight`; checks render as text in Platform Settings. |
| Control-plane TLS read/save | fully connected | `GET/PUT /api/v1/settings/control-plane-tls`; field-level backend validation is surfaced near matching fields. |
| Control-plane TLS apply | fully connected | `POST /api/v1/settings/control-plane-tls/apply`; confirmation required and returned job is tracked. |
| Mail settings read/save | fully connected | `GET/PUT /api/v1/settings/mail`; SMTP password is masked/write-only and existing secret ref is preserved when unchanged. |
| Mail test | fully connected | `POST /api/v1/settings/mail/test`; no fake delivery success and no SMTP secret logging/storage. |
| Users list/detail | fully connected | `GET /api/v1/admin/users`; detail drawer is derived from the real admin user list because there is no separate browser detail route. |
| Invite list/create | fully connected | `GET /api/v1/admin/user-invites` and `POST /api/v1/admin/users/invite`; returned invite URL/token is not rendered, logged or persisted. |
| Invite revoke | backend-missing | No browser backend endpoint exists for invite revoke; UI disables the action with an explicit backend-missing reason. |
| Sessions list/revoke | fully connected | `GET /api/v1/admin/sessions` and `POST /api/v1/admin/sessions/{id}/revoke`; revoke requires confirmation. |
| Direct user lifecycle mutations | legacy-only | User status, reset-password, resend-invite and delete endpoints exist, but FE8-P0-07B does not expose these higher-risk operations. |
| Certificate list/detail | fully connected | `GET /api/v1/platform/certificates`; detail is derived from the real list response because backend has no separate detail route. Expiry/status/usage are shown without rendering secret refs or PEM material. |
| Certificate import | fully connected | `POST /api/v1/platform/certificates/preview` then `POST /api/v1/platform/certificates/import`; stale preview disables apply. Certificate/private-key PEM is cleared on close/success and is not logged, stored or rendered after submit. |
| Certificate self-signed / managed CA / issue-from-CA | fully connected | `POST /self-signed`, `/authorities`, `/issue-from-ca`; backend generates key material and stores it server-side. No private key is shown to the browser. |
| Certificate default/revoke/delete | fully connected | `POST /{id}/default`, `POST /{id}/revoke`, `DELETE /{id}`; confirmations required. Delete is exposed for backend-supported CA cascade delete; unsupported row kinds are disabled. |
| PKI roots | fully connected | `GET/POST /api/v1/platform/pki-roots`; managed root create calls backend and does not expose CA private key material to the browser. |

### Backhaul

| Workflow | Status | Notes |
| --- | --- | --- |
| List links/drivers | fully connected read path | `GET /api/v1/backhaul-links` and `GET /api/v1/backhaul-links/{id}` power list/detail. Drivers remain backend catalog data. Transport `config` and `secret_refs` are intentionally not rendered. |
| Apply/probe/promote/route state | fully connected | `POST /backhaul-links/{id}/apply`, `/probe`, `/promote` and `PATCH /route`; confirmation required, backend jobs are tracked and route/Xray convergence remains backend-owned. Backend has no dedicated repair endpoint, so no fake repair action is exposed. |
| Create/delete links | legacy-only | Backend routes exist, but FE8-P0-08A does not expose create/delete; destructive delete still needs a separate confirmation/evidence pass. |

### Operations

| Workflow | Status | Notes |
| --- | --- | --- |
| Audit list | fully connected read path | Basic read path. |
| Audit advanced filters | disabled intentionally | Backend currently exposes limit-based list. |
| Diagnostics overview | read-only | Aggregates existing read hooks. |
| Diagnostics actions | legacy-only | Domain-specific node actions exist. |
| Backup/restore | backend-missing for browser UI | Remove from primary nav in a future pass or keep clearly disabled. Existing operational scripts remain CLI/server-side. |

## 3. Dangerous Operation UX Requirement

Before any `legacy-only` dangerous action is promoted to `fully connected`, it
must use:

1. backend preview when available;
2. confirmation dialog with impact summary;
3. explicit action name confirmation for destructive operations;
4. permission-aware disabled state;
5. real mutation through the API layer;
6. query invalidation;
7. job tracking when an async job is returned;
8. safe backend error rendering;
9. no generated secret in browser logs or long-lived state.

## 4. RC1 Decision

RC1 is safer as a primary read console with legacy rollback than as a partially
wired mutation console. The current pass removes fake preview/apply affordances
and records legacy-only write workflows explicitly. Full write parity remains a
follow-up implementation track per domain.
