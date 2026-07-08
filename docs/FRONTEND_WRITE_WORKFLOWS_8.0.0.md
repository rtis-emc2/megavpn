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
| Job retry/requeue | legacy-only | Domain-specific retry endpoints exist for selected node diagnostics, not generic retry UX. |

### Clients

| Workflow | Status | Notes |
| --- | --- | --- |
| List/search/filter | read-only | Current page lists clients; advanced filters are not complete. |
| Create client | disabled intentionally | Backend exists; form and validation mapping not wired. |
| Edit client | backend-missing | No generic `PATCH/PUT /clients/{id}` route found. |
| Activate/suspend | legacy-only | Backend exists; needs confirmation and invalidation. |
| Delete client | legacy-only | Backend exists; destructive confirmation required. |
| Provision through access groups | legacy-only | Must use group bulk endpoints; no one-job-per-client bulk implementation. |
| Revoke client | legacy-only | Backend exists; async job tracking required. |
| Routes | legacy-only | Backend exists; detail workflow not migrated. |
| Accesses | legacy-only | Backend exists; rotation requires secret-safe handling. |
| Config cleanup | legacy-only | Backend exists; destructive confirmation required. |
| Artifact build | legacy-only | Backend exists; async job tracking required. |
| Artifact download | read-only / legacy-only | Aggregate list exists; per-client download workflow not migrated. |
| Artifact delete | legacy-only | Backend exists; destructive confirmation required. |
| Email delivery | legacy-only | Backend exists; form not migrated. |
| Share link create/revoke | legacy-only | Backend exists; token one-time display required. |
| Subscription rotate/revoke | disabled intentionally | Backend exists; token safety UX not migrated. |

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
| Route policy UI | legacy-only | Backend preview/apply/cleanup exists; new UI not migrated. |
| Traffic overview | fully connected | `GET /api/v1/traffic/accounting`. |
| Traffic export | fully connected | Backend export URL opened directly; no token storage. |

### Instances / Services

| Workflow | Status | Notes |
| --- | --- | --- |
| Instances list | fully connected | Read path. |
| Instance detail/runtime/revisions | read-only | Partial runtime/revisions coverage. |
| Create from service pack | legacy-only | Backend exists; create form not migrated. |
| Manual create | legacy-only | Backend exists; spec form not migrated. |
| Draft/spec replace | legacy-only | Backend exists; validation mapping required. |
| Apply/reapply | legacy-only | Backend exists; async job tracking required. |
| Lifecycle start/stop/restart/enable/disable | legacy-only | Backend exists; confirmation and job tracking required. |
| Rollback | legacy-only | Backend exists; confirmation required. |
| Diagnostics | legacy-only | Backend exists; job tracking required. |
| Delete/force-delete | legacy-only | Backend exists; destructive confirmation required. |
| Service pack CRUD | legacy-only | Backend exists; form not migrated. |
| VLESS templates | legacy-only/deprecated | Primary access group management belongs under Clients -> Groups. |
| Runtime artifact import/list/delete | read-only / legacy-only | List connected; import not migrated; delete endpoint not found for binary artifacts. |

### Nodes

| Workflow | Status | Notes |
| --- | --- | --- |
| Nodes list/detail | fully connected read path | Detail drawer is read-only. |
| Create/register | disabled intentionally | Backend exists; form not migrated. |
| Edit metadata | legacy-only | Backend exists. |
| Retire | legacy-only | Backend exists; destructive confirmation required. |
| Force retire | legacy-only | Backend exists; stronger confirmation required. |
| Maintenance mode | legacy-only | Backend exists. |
| Bootstrap | legacy-only | Backend exists; host-key/secret-safe workflow required. |
| SSH terminal/session launch | legacy-only | Backend exists; terminal security review required. |
| Host-key scan | legacy-only | Backend exists; host-key pinning required. |
| Token rotate/enrollment tokens | legacy-only | Backend exists; one-time token display required. |
| Diagnostics retry | legacy-only | Backend exists; job tracking required. |
| Inventory sync | legacy-only | Backend exists; job tracking required. |
| Capabilities install/verify | legacy-only | Backend exists; job tracking required. |
| Service discovery import | legacy-only | Backend exists; confirmation required. |
| Route policy preview/apply/cleanup | legacy-only | Backend exists; preview-before-apply required. |

### Platform

| Workflow | Status | Notes |
| --- | --- | --- |
| Settings read | read-only | Runtime/TLS/mail read paths exist. |
| Settings save | legacy-only | Backend exists; form not migrated. |
| Mail test | legacy-only | Backend exists; no SMTP secret logging. |
| TLS apply | legacy-only | Backend exists; async job tracking required. |
| Users/invites/sessions read | read-only | Users/sessions read path exists; invites list missing. |
| Users/invites/sessions mutations | legacy-only | Backend exists; confirmation and permission states required. |
| Certificate list | fully connected read path | Expiry/status shown read-only. |
| Certificate import/self-signed/CA/issue/default/revoke/delete | legacy-only | Backend exists; secret-safe forms and destructive confirmations required. |
| PKI roots | read-only / legacy-only | List read path exists; create not migrated. |

### Backhaul

| Workflow | Status | Notes |
| --- | --- | --- |
| List links/drivers | fully connected read path | Current page lists links. |
| Create/apply/probe/promote/route/delete | legacy-only | Backend exists; job tracking and confirmation required. |

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
