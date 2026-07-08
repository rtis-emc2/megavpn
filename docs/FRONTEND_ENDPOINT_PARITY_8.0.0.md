# RTIS MegaVPN Frontend Endpoint Parity 8.0.0

Release scope: MegaVPN Console 8.0.0 RC1.

Status: parity baseline for the new React console. Source of truth for backend
routes is `internal/api/http/server.go`.

## 1. Status Legend

| Status | Meaning |
| --- | --- |
| `connected` | New frontend has an API wrapper and UI use. |
| `read-only` | New frontend reads backend state but does not expose mutations. |
| `disabled` | New frontend shows an explicit disabled action with reason. |
| `legacy-only` | Workflow remains available through `/legacy/` for RC1. |
| `backend-missing` | No browser backend endpoint was found. |
| `deprecated` | Endpoint exists for compatibility but is not part of the new primary flow. |

## 2. Common Error Handling Contract

All new frontend calls must go through `frontend/src/shared/api`.

| HTTP status | UI handling |
| --- | --- |
| `401` | Treat as expired/missing session; redirect or show login-required state. |
| `403` | Show permission denied state; include required permission when backend payload provides it. |
| `409` | Show conflict details and keep user input intact. |
| `422` | Show validation errors near affected fields where payload structure allows it. |
| `5xx` | Show safe backend error text and request/correlation ID if present; never show raw HTML. |

Unsafe methods must preserve `X-MegaVPN-CSRF: 1` and `credentials: include`.

## 3. Current Frontend API Modules

| Frontend file | Purpose | Current limitation |
| --- | --- | --- |
| `frontend/src/shared/api/client.ts` | Fetch wrapper, API base, CSRF, cookie credentials, typed API error. | No field-level validation mapper yet. |
| `frontend/src/shared/api/endpoints.ts` | Current endpoint wrappers. | Mostly read-path; mutation coverage incomplete. |
| `frontend/src/shared/query/hooks.ts` | TanStack Query read hooks. | Mutation hooks and invalidation policy incomplete. |

Raw `/api/v1` strings are allowed only under `frontend/src/shared/api` and
tests. `scripts/ci/frontend-static-guards.sh` enforces this rule.

## 4. Endpoint Matrix

### 4.1 Static, Public, Auth Bootstrap

| Backend endpoint | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET /`, `GET /assets/{path...}` | root static UI | Go serving only | all pages | connected | Strict asset 404; SPA fallback only for frontend routes. |
| `GET /legacy`, `GET /legacy/`, `GET /legacy/assets/{path...}` | n/a | Go serving only | Legacy link | connected | Rollback path; no removal before stable release. |
| `GET /share/{token}` | artifact share | none | external public flow | legacy-compatible | Public token endpoint; not persisted in new UI. |
| `GET /subscribe/vless/{token}` | subscription document | none | external public flow | legacy-compatible | Public token endpoint; do not store token in UI state. |
| `GET /health`, `GET /healthz` | ops smoke | none | none | connected backend-only | Must not be shadowed by SPA fallback. |
| `GET /api/v1/ready`, `GET /api/v1/version` | readiness/version | `endpoints.ready`, `endpoints.version`; `useReady`, `useVersion` | shell/status | connected | No auth token storage. |
| `GET /api/v1/auth/invites/{token}`, `POST /api/v1/auth/invites/{token}/accept`, `POST /api/v1/auth/login` | invite/login | `getInvite`, `acceptInvite`, `login` | Auth | connected | Rate limited backend; password never stored. |
| `GET /api/v1/auth/me`, `POST /api/v1/auth/logout`, `POST /api/v1/auth/change-password` | session/account | `getSession`, `logout`; change-password missing | Auth/Platform | partial | Session query invalidated after login/logout. |

### 4.2 Platform Access, Settings, Certificates

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET/POST /api/v1/admin/users`, `POST /api/v1/admin/users/invite`, `GET /api/v1/admin/user-invites` | users/invites | `endpoints.users`; invites missing | Platform / Access | read-only | Mutations legacy-only until forms, confirmation and errors are wired. |
| `POST /api/v1/admin/users/{id}/status`, `/reset-password`, `/resend-invite`, `DELETE /api/v1/admin/users/{id}` | user lifecycle | missing | Platform / Access | legacy-only | Invalidate users, invites, audit when wired. |
| `GET /api/v1/admin/sessions`, `POST /api/v1/admin/sessions/{id}/revoke` | active sessions | `endpoints.sessions`; revoke missing | Platform / Access | read-only | Revoke must confirm target session. |
| `GET/PUT /api/v1/settings/mail`, `POST /api/v1/settings/mail/test` | mail settings | `endpoints.mailSettings`; mutations missing | Platform / Mail | read-only | Do not log SMTP secrets. |
| `GET/PUT /api/v1/settings/control-plane-tls`, `POST /api/v1/settings/control-plane-tls/apply` | TLS settings | `endpoints.controlPlaneTLS`; mutations missing | Platform / Settings | read-only | Apply returns job; needs job tracking. |
| `GET /api/v1/runtime/preflight` | runtime checks | `endpoints.runtimePreflight` | Diagnostics/Settings | connected read | No mutation. |
| `GET /api/v1/platform/certificates`, `POST /preview`, `/import`, `/self-signed`, `/authorities`, `/issue-from-ca`, `POST /{id}/default`, `POST /{id}/revoke`, `DELETE /{id}` | certificate management | `endpoints.certificates`; mutations missing | Platform / Certificates | read-only / legacy-only | Private key/cert payloads must be redacted; revoke/delete require confirmation. |
| `GET/POST /api/v1/platform/pki-roots` | service PKI roots | `endpoints.pkiRoots`; create missing | Platform / Certificates | read-only | CA material must be rendered as text only. |
| `POST /api/v1/secret-refs` | secret upload for bootstrap | missing | Nodes legacy flow | legacy-only | Secret values one-way only; never persist in browser storage. |

### 4.3 Dashboard, Service Catalog, Runtime Artifacts

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/dashboard` | dashboard | `endpoints.dashboard`; `useDashboard` | Dashboard | connected | Poll without overwriting forms. |
| `GET /api/v1/services`, `GET /api/v1/service-drivers`, `GET /api/v1/services/installers` | catalog/installers | services missing, installers missing | Services | partial | Add wrappers before write forms. |
| `GET/PUT /api/v1/service-packs`, `POST /{key}/enable`, `/disable`, `DELETE /{key}` | service pack CRUD | `endpoints.servicePacks`; mutations missing | Service Packs | read-only / legacy-only | Mutations invalidate service packs and instances. |
| `GET/PUT /api/v1/vless-groups`, `POST /{key}/enable`, `/disable`, `DELETE /{key}` | VLESS templates | missing except legacy | Clients -> Groups / Services | legacy-only | Primary VLESS management belongs under Clients -> Groups in new IA. |
| `GET /api/v1/binary-artifacts`, `POST /api/v1/binary-artifacts`, `/import`, `/import-url` | runtime binary repository | `endpoints.binaryArtifacts`; mutations missing | Runtime Artifacts | read-only / legacy-only | Downloads/imports must preserve no-store and safe error rendering. |

### 4.4 Client Access Groups

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/client-access-services` | services for access groups | `endpoints.clientAccessServices`; `useClientAccessServices` | Clients -> Groups | connected |
| `GET/POST /api/v1/client-access-groups` | group list/create | `endpoints.clientAccessGroups`; `useClientAccessGroups`; create missing | Clients -> Groups | read-only |
| `GET /api/v1/client-access-groups/available-clients`, `/migration-conflicts` | member picker/conflicts | missing | Clients -> Groups | legacy-only |
| `GET/PATCH/DELETE /api/v1/client-access-groups/{group_id}` | group policy | get/update/delete missing | Clients -> Groups | legacy-only |
| `POST /api/v1/client-access-groups/{group_id}/enable`, `/disable` | status change | missing | Clients -> Groups | legacy-only |
| `GET /api/v1/client-access-groups/{group_id}/members` | member list | missing | Clients -> Groups | legacy-only |
| `POST /api/v1/client-access-groups/{group_id}/members:preview`, `/members:bulk-apply`, `/members:bulk-add`, `/members:bulk-move`, `DELETE /members/{client_id}` | bulk member operations | missing | Clients -> Groups | disabled / legacy-only | No fake preview/apply; one backend bulk mutation only. |
| `GET/PATCH /api/v1/client-access-groups/{group_id}/scope`, `POST /sync:preview`, `POST /sync:apply`, `GET /sync-state` | scope/sync | missing | Clients -> Groups | legacy-only | Preview invalidates on selection or scope changes. |

### 4.5 Nodes, Bootstrap, Inventory

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET/POST /api/v1/nodes`, `GET/PUT/DELETE /api/v1/nodes/{id}` | node CRUD | `endpoints.nodes`; `useNodes`; mutations missing | Nodes | read-only / legacy-only | Retire/delete require confirmation and audit-friendly reason. |
| `POST /api/v1/nodes/{id}/force-retire`, `/maintenance/enable`, `/maintenance/disable` | dangerous lifecycle | missing | Nodes | legacy-only | Confirm impact and invalidate nodes/jobs. |
| `GET /api/v1/nodes/{id}/diagnostics`, `POST /diagnostics/retry-*`, `/reconcile-runtime`, `/requeue-stuck-job`, `/channel-probe`, `/clear-stale-rotation` | diagnostics actions | diagnostics missing | Nodes/Diagnostics | legacy-only | Async jobs need tracking. |
| `GET /api/v1/nodes/{id}/routes/preview`, `POST /routes/apply`, `/routes/cleanup` | route policy | missing | Route Policy | legacy-only | Preview-before-apply required. |
| `GET/PUT /api/v1/nodes/{id}/access-methods`, `POST /ssh/host-key-scan`, `/ssh/sessions`, `GET /ssh/terminal` | bootstrap/terminal | missing | Nodes | legacy-only | Host key pinning, WebSocket terminal, no secret logging. |
| `POST /api/v1/nodes/{id}/bootstrap`, `GET /bootstrap-runs`, `GET /bootstrap-runs/{run_id}/bundle` | bootstrap | missing | Nodes | legacy-only | Generated material must be one-time/secret-safe. |
| `POST /api/v1/nodes/{id}/agent-token/rotate`, `/enrollment-token`, `/enrollment-token/rotate`, `/agent-identity/revoke`, `/reboot`, `/emergency-cleanup` | token and control actions | missing | Nodes | legacy-only | Rotate/revoke/reboot require confirmation and job tracking. |
| `GET /api/v1/nodes/{id}/inventory`, `GET /api/v1/nodes/capabilities`, `GET /nodes/{id}/capabilities`, `POST /capabilities/install`, `/verify`, `GET /capabilities/drift`, `/install-events` | inventory/capabilities | capabilities partially read in legacy only | Nodes/Services | legacy-only | Install/verify async job tracking required. |
| `GET /api/v1/nodes/{id}/services/discovered`, `/discovery-summary`, `/{discovery_id}`, `POST /ignore`, `/unignore`, `/import`, `/services/import-all`, `/services/discover`, `/inventory/sync` | service discovery | missing | Nodes/Services | legacy-only | Import is a mutating workflow; confirm impact. |

### 4.6 Instances and Revisions

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET/POST /api/v1/instances`, `POST /api/v1/service-packs/{key}/instances` | instance list/create | `endpoints.instances`; `useInstances`; create missing | Instances | read-only / legacy-only |
| `GET /api/v1/instances/runtime-states`, `GET /instances/{id}/runtime-state`, `/runtime-observations` | runtime state | `endpoints.instanceRuntimeStates`; detail missing | Instances | partial |
| `GET /api/v1/instances/{id}`, `GET /instances/{id}/revisions` | detail/revisions | revisions missing | Instances/Revisions | partial |
| `PUT /api/v1/instances/{id}/spec`, `POST /rollback`, `DELETE /{id}`, `POST /force-delete` | spec/rollback/delete | missing | Instances/Revisions | legacy-only | Confirm destructive actions; invalidate instances/jobs/revisions. |
| `POST /api/v1/instances/{id}/apply`, `/restart`, `/start`, `/stop`, `/enable`, `/disable`, `/diagnose` | lifecycle/jobs | missing | Instances | legacy-only | Async job tracking required. |
| `GET/POST/PATCH/DELETE /api/v1/instances/{id}/vless-groups/...` | legacy instance VLESS membership | missing | Instances read-only link | legacy-only/deprecated | New primary flow is Clients -> Groups. |

### 4.7 Address Pools, Firewall, Traffic

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/address-pools`, `POST/PUT/DELETE /address-pools/spaces`, `POST /spaces/{id}/routing` | address pools | `endpoints.addressPools`; `useAddressPools`; mutations missing | Address Pools | read-only / legacy-only |
| `GET /api/v1/firewall` | firewall inventory | `endpoints.firewallInventory`; `useFirewallInventory` | Firewall | connected read |
| `POST/PUT/DELETE /api/v1/firewall/policies`, `/address-lists`, `/address-lists/{id}/entries`, `/policies/{id}/rules` | firewall model CRUD | missing | Firewall | legacy-only | Invalidate firewall inventory after mutations. |
| `GET/PUT /api/v1/firewall/management-settings` | firewall safety settings | read wrapper missing | Settings/Firewall | legacy-only | Safety settings require clear warnings. |
| `POST /api/v1/nodes/{id}/firewall/preview`, `/apply`, `/disable` | node firewall apply | missing | Firewall | disabled / legacy-only | Preview mandatory before strict apply; no fake preview. |
| `GET /api/v1/traffic/accounting`, `GET /api/v1/traffic/accounting/export` | traffic overview/export | `endpoints.trafficAccounting`, `trafficAccountingExportURL`; `useTrafficAccounting` | Traffic | connected | Export opens backend URL; backend no-store applies. |

### 4.8 Clients, Delivery, Shares, Subscriptions

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET/POST /api/v1/clients`, `GET/DELETE /api/v1/clients/{id}`, `POST /suspend`, `/activate` | clients CRUD/status | `endpoints.clients`; `useClients`; mutations missing | Clients | read-only / legacy-only |
| `DELETE /api/v1/clients/{id}/configs`, `POST /provision`, `/revoke` | provisioning | missing | Clients | legacy-only | Bulk provisioning must use group endpoints, not one job per client. |
| `GET /api/v1/clients/{id}/accesses`, `DELETE /accesses/{access_id}`, `POST /accesses/{access_id}/rotate-*` | accesses | missing | Clients detail | legacy-only | Rotation returns jobs and secrets must be redacted. |
| `GET/POST/DELETE /api/v1/clients/{id}/routes` | client routes | missing | Clients detail | legacy-only |
| `GET /api/v1/clients/{id}/access-groups`, `PATCH /access-groups/{service_code}` | per-client group membership | missing | Clients detail | legacy-only |
| `GET/POST/DELETE /api/v1/clients/{id}/artifacts`, `GET /content`, `GET /download` | client artifacts | artifacts aggregate only | Delivery | read-only / legacy-only | Artifact content rendered text-safe; downloads no-store. |
| `GET/POST /api/v1/clients/{id}/share-links`, `POST /share-links/{link_id}/revoke` | share links | aggregate `endpoints.shareLinks`; mutations missing | Delivery | read-only / legacy-only | Share token visible only on creation response. |
| `GET /api/v1/clients/{id}/subscriptions`, `POST /subscriptions/rotate`, `POST /subscriptions/{subscription_id}/revoke` | subscriptions | missing | Subscriptions | disabled / legacy-only | Subscription token should not persist in long-lived UI state. |
| `POST /api/v1/clients/{id}/deliver-email` | email delivery | missing | Delivery | legacy-only | Email delivery errors shown safely. |

### 4.9 Backhaul, Artifacts Aggregate, Jobs, Audit

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/backhaul/drivers`, `GET/POST /api/v1/backhaul-links`, `GET /backhaul-links/{id}` | backhaul list/create | `endpoints.backhaulDrivers`, `endpoints.backhaulLinks`; mutations missing | Backhaul | read-only / legacy-only |
| `POST /api/v1/backhaul-links/{id}/apply`, `/probe`, `/promote`, `PATCH /route`, `DELETE /{id}` | backhaul actions | missing | Backhaul | legacy-only | Apply/probe return jobs; track jobs. |
| `GET /api/v1/artifacts`, `GET /api/v1/share-links` | aggregate delivery | `endpoints.artifacts`, `endpoints.shareLinks` | Delivery | connected read |
| `GET/POST /api/v1/jobs`, `GET /jobs/{id}`, `GET /jobs/{id}/logs`, `POST /jobs/{id}/cancel` | jobs and logs | `endpoints.jobs`, `job`, `jobLogs`, `cancelJob`; create missing | Jobs | connected for list/detail/logs/cancel | Logs rendered as text; no HTML injection; cancel invalidates job queries. |
| `GET /api/v1/audit` | audit | `endpoints.audit` | Audit | connected read | Details rendered as text. |

### 4.10 Agent Endpoints

Agent endpoints are not browser operator UI endpoints and must not be wrapped
by the React console:

- `POST /agent/register`;
- `POST /agent/heartbeat`;
- `POST /agent/inventory`;
- `GET /agent/runtime/instances`;
- `POST /agent/runtime/instances`;
- `POST /agent/traffic/accounting`;
- `GET /agent/jobs/next`;
- `POST /agent/jobs/{id}/result`;
- `GET /agent/binary-artifacts/{artifact_id}/download`.

SPA fallback must not shadow any `/agent/*` path.

## 5. Invalidation Strategy

| Mutation family | Query keys to invalidate when wired |
| --- | --- |
| Auth login/logout/change password | `auth/session`, permission-filtered navigation state |
| Users/invites/sessions | `admin-users`, `admin-sessions`, invite list, audit |
| Settings/mail/TLS | `settings-mail`, `control-plane-tls`, `runtime-preflight`, jobs |
| Certificates/PKI | `certificates`, `pki-roots`, instances where certificate refs are shown |
| Service packs/VLESS templates | `service-packs`, services catalog, client access services/groups |
| Client access groups | `client-access-groups`, members, scope, sync-state, clients, jobs |
| Nodes | `nodes`, node detail, diagnostics, capabilities, jobs, dashboard |
| Instances | `instances`, runtime-states, revisions, jobs, dashboard |
| Address pools | `address-pools`, instances, firewall when address groups depend on pools |
| Firewall | `firewall-inventory`, node detail/firewall state, jobs |
| Clients/artifacts/share/subscriptions | `clients`, artifacts, share-links, jobs, dashboard |
| Backhaul | `backhaul-links`, nodes, jobs |
| Jobs cancel/create | `jobs`, job detail/logs |

## 6. RC1 Gaps

RC1 is incomplete until:

- mutation wrappers and hooks exist for each non-read-only endpoint family;
- disabled/legacy-only actions are either fully wired or intentionally left out
  of primary navigation;
- endpoint wrappers carry typed DTOs instead of broad `Record<string, unknown>`
  where data is rendered or submitted;
- field-level `422` mapping is implemented for forms;
- all dangerous operations use confirm/preview/job tracking/error UX.
