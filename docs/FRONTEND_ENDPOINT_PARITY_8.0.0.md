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
| `frontend/src/shared/api/client.ts` | Fetch wrapper, API base, CSRF, cookie credentials, typed API error. | Field-level validation mapping is currently implemented in focused forms, not as a global helper. |
| `frontend/src/shared/api/endpoints.ts` | Current endpoint wrappers. | Clients core, delivery, routes/access maintenance/config cleanup, VLESS groups, Firewall, Services/Instances provisioning, Nodes observability/diagnostics/inventory/capability/discovery/bootstrap bundle, Certificates/PKI and Platform settings/mail/access workflows are wired where backend endpoints exist; remaining domains stay explicitly incomplete. |
| `frontend/src/shared/query/hooks.ts` | TanStack Query hooks. | Clients core, delivery, routes/access maintenance/config cleanup, VLESS groups, Firewall, Services/Instances provisioning, Nodes observability/diagnostics/inventory/capability/discovery/bootstrap bundle, Certificates/PKI and Platform settings/mail/access invalidation are wired where backend endpoints exist; remaining domains stay explicitly incomplete. |

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
| `GET /api/v1/admin/users`, `POST /api/v1/admin/users/invite`, `GET /api/v1/admin/user-invites` | users/invites | `listUsers`, `getUser` derived from list, `listInvites`, `createInvite`; matching hooks | Platform / Access | connected | User list/detail and invite create are real backend calls. Invite create does not render or persist returned invite URLs/tokens. |
| `POST /api/v1/admin/users/{id}/status`, `/reset-password`, `/resend-invite`, `DELETE /api/v1/admin/users/{id}` | user lifecycle | missing | Platform / Access | legacy-only | Direct user status/reset/resend/delete is outside FE8-P0-07B and remains legacy/future scope. |
| Invite revoke endpoint | invite lifecycle | disabled wrapper only | Platform / Access | backend-missing | No browser backend endpoint exists for invite revoke in this release, so the UI disables the exact action with reason instead of faking success. |
| `GET /api/v1/admin/sessions`, `POST /api/v1/admin/sessions/{id}/revoke` | active sessions | `listSessions`, `revokeSession`; matching hooks | Platform / Access | connected | Revoke requires confirmation and invalidates session/admin queries. No session token is rendered or stored. |
| `GET/PUT /api/v1/settings/mail`, `POST /api/v1/settings/mail/test` | mail settings | `getMailSettings`, `updateMailSettings`, `testMailSettings`; matching hooks | Platform / Mail | connected | SMTP password is write-only/masked, preserved through secret ref when unchanged, never logged/stored/rendered. Mail test calls the real backend. |
| `GET/PUT /api/v1/settings/control-plane-tls`, `POST /api/v1/settings/control-plane-tls/apply` | TLS settings | `getPlatformSettings`, `updatePlatformSettings`, `applyTlsSettings`; matching hooks | Platform / Settings | connected | Save maps field errors. Apply requires confirmation, returns a job and is tracked in the UI. |
| `GET /api/v1/runtime/preflight` | runtime checks | `endpoints.runtimePreflight` | Diagnostics/Settings | connected read | No mutation. |
| `GET /api/v1/platform/certificates`, `POST /preview`, `/import`, `/self-signed`, `/authorities`, `/issue-from-ca`, `POST /{id}/default`, `POST /{id}/revoke`, `DELETE /{id}` | certificate management | `listCertificates`, `getCertificate` derived from list, `previewCertificateImport`, `importCertificate`, `createSelfSignedCertificate`, `createManagedCertificateAuthority`, `issueCertificate`, `setDefaultCertificate`, `revokeCertificate`, `deleteCertificate`; matching hooks | Platform / Certificates | connected | Import requires backend preview before apply; stale preview disables apply. Private key PEM is only form state, cleared on close/success, never logged/stored/rendered. Default/revoke/delete require confirmation. |
| `GET/POST /api/v1/platform/pki-roots` | service PKI roots | `listPkiRoots`, `createPkiRoot`, `importPkiRoot` alias; `usePkiRoots`, `useCreatePkiRoot` | Platform / Certificates | connected | Managed root creation calls backend; CA private key material is generated/stored backend-side and not returned to the browser. |
| `POST /api/v1/secret-refs` | secret upload for bootstrap | missing for React SSH creation | Legacy/internal secret flows | legacy/internal | Secret values are one-way only. The React SSH access-method creation flow does not call this endpoint; it uses the dedicated atomic node SSH endpoint instead. |

### 4.3 Dashboard, Service Catalog, Runtime Artifacts

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/dashboard` | dashboard | `endpoints.dashboard`; `useDashboard` | Dashboard | connected | Poll without overwriting forms. |
| `GET /api/v1/services`, `GET /api/v1/service-drivers`, `GET /api/v1/services/installers` | catalog/installers | `listServiceTypeCapabilities`; service-drivers/installers missing | Services | partial | Service type list is used by manual instance create. Driver/installers remain legacy-only. |
| `GET/PUT /api/v1/service-packs`, `POST /{key}/enable`, `/disable`, `DELETE /{key}` | service pack CRUD | `listServicePacks`, `getServicePack` derived from list, `createServicePack`, `updateServicePack`, `deleteServicePack`, `setServicePackEnabled`; matching hooks | Service Packs | connected | Mutations invalidate service packs, instances and jobs where relevant. Backend has no separate validation endpoint. |
| `GET/PUT /api/v1/vless-groups`, `POST /{key}/enable`, `/disable`, `DELETE /{key}` | VLESS templates | missing except legacy | Clients -> Groups / Services | legacy-only | Primary VLESS management belongs under Clients -> Groups in new IA. |
| `GET /api/v1/binary-artifacts`, `POST /api/v1/binary-artifacts`, `/import`, `/import-url` | runtime binary repository | `listRuntimeArtifacts`, `getRuntimeArtifact` derived from list, `importRuntimeArtifact`; matching hooks | Runtime Artifacts | connected for list/import-url | Metadata is rendered as text. URL import is wired. Direct file import/create is not exposed. Delete remains backend-missing because no browser DELETE route exists. |

### 4.4 Client Access Groups

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/client-access-services` | services for access groups | `endpoints.clientAccessServices`; `useClientAccessServices` | Clients -> Groups | connected |
| `GET/POST /api/v1/client-access-groups` | group list/create | `listClientAccessGroups`; `createClientAccessGroup`; `useClientAccessGroups`; `useCreateClientAccessGroup` | Clients -> Groups | connected for VLESS |
| `GET /api/v1/client-access-groups/available-clients` | member picker | `getAvailableClientsForGroup`; `useAvailableClientsForGroup` | Clients -> Groups | connected for VLESS |
| `GET /api/v1/client-access-groups/migration-conflicts` | migration conflict inventory | missing | Clients -> Groups | legacy-only |
| `GET/PATCH /api/v1/client-access-groups/{group_id}` | group policy/status edit | `getClientAccessGroup`; `updateClientAccessGroup`; `useUpdateClientAccessGroup` | Clients -> Groups | connected for VLESS | Policy edit preserves backend validation and CSRF. |
| `DELETE /api/v1/client-access-groups/{group_id}` | destructive group removal | `deleteOrDisableClientAccessGroup`; UI confirmation missing | Clients -> Groups | legacy-only |
| `POST /api/v1/client-access-groups/{group_id}/enable`, `/disable` | status change | missing separate action | Clients -> Groups | partial | VLESS status can be changed through PATCH; explicit enable/disable buttons are not exposed. |
| `GET /api/v1/client-access-groups/{group_id}/members` | member list | `getClientAccessGroupMembers`; `useClientAccessGroupMembers` | Clients -> Groups | connected for VLESS |
| `POST /api/v1/client-access-groups/{group_id}/members:preview`, `/members:bulk-apply`, `DELETE /members/{client_id}` | preview/apply/remove members | `previewClientAccessGroupMembers`; `applyClientAccessGroupMembers`; `removeClientAccessGroupMember` hooks | Clients -> Groups | connected for VLESS | Apply stays disabled until successful backend preview; selection/filter/mode changes invalidate preview. |
| `POST /api/v1/client-access-groups/{group_id}/members:bulk-add`, `/members:bulk-move` | direct add/move aliases | backend-compatible | Clients -> Groups | deprecated in new UI | New UI uses `/members:bulk-apply` with `mode=add_only` or `mode=add_or_move`. |
| `GET/PATCH /api/v1/client-access-groups/{group_id}/scope`, `POST /sync:preview`, `POST /sync:apply`, `GET /sync-state` | scope/sync | typed wrappers and hooks for scope/sync preview/apply/state | Clients -> Groups | connected for VLESS | Scope and sync apply return materialization/job state and invalidate groups, jobs and sync-state. |

### 4.5 Nodes, Bootstrap, Inventory

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET/POST /api/v1/nodes`, `GET/PUT/DELETE /api/v1/nodes/{id}` | node CRUD | `listNodes`; `getNode`; `createNode`; `updateNode`; `retireNode`; `useNodes`; `useNodeDetail`; `useCreateNode`; `useUpdateNode`; `useRetireNode` | Nodes | connected for list/detail/create/edit/retire | Create and edit use safe node profile metadata only: name, address, kind, role, location label, OS family/version, architecture and execution mode. UI does not expose ID, status, agent identity, tokens, secret refs, heartbeat/runtime state or dedicated maintenance/retire fields. Retire requires confirmation and backend validation. |
| `POST /api/v1/nodes/{id}/maintenance/enable`, `/maintenance/disable` | maintenance lifecycle | `setNodeMaintenance`; `useSetNodeMaintenance` | Nodes | connected | Confirmation required; invalidates nodes, node detail, diagnostics, inventory, capabilities, discovery and jobs. |
| `POST /api/v1/nodes/{id}/force-retire` | dangerous lifecycle | `forceRetireNode`; `useForceRetireNode` | Nodes/Lifecycle | connected | Requires typed node-name confirmation, reason, backend validation and query invalidation. |
| `GET /api/v1/nodes/{id}/diagnostics`, `POST /diagnostics/retry-inventory`, `/retry-discovery`, `/reconcile-runtime`, `/requeue-stuck-job`, `/channel-probe` | diagnostics actions | `getNodeDiagnostics`; `getNodeAgentState`; `listNodeDiagnostics`; `runNodeDiagnostics`; `retryNodeDiagnostics`; matching hooks | Nodes/Diagnostics | connected | Output is rendered as text, not HTML. Async actions require confirmation and show returned jobs. Runtime reconcile may queue backend-defined dependent runtime jobs. |
| `GET /api/v1/nodes/{id}`, `GET /api/v1/nodes/{id}/diagnostics`, `GET /api/v1/nodes/{id}/enrollment-tokens`, `GET /api/v1/nodes/{id}/bootstrap-runs`, `GET /api/v1/nodes/{id}/inventory`, `GET /api/v1/nodes/{id}/access-methods`, `POST /api/v1/nodes/{id}/enrollment-token`, `POST /api/v1/nodes/{id}/enrollment-token/rotate`, `POST /api/v1/nodes/{id}/bootstrap` | guided agent onboarding status, enrollment-token issue/reissue and guided bootstrap job submission | existing node detail/diagnostics/enrollment/bootstrap/inventory/access-method hooks plus `nodeOnboarding` and `nodeBootstrapReadiness` derivation; `createEnrollmentToken`; `rotateEnrollmentToken`; `bootstrapNode`; matching hooks | Nodes/Onboarding | connected for status, enrollment-token issue/reissue and guided bootstrap mode selection/job submission | Guided status UI derives milestones from existing operator read APIs. Onboarding can issue or reissue an enrollment token only when the typed model recommends it and the operator has `node.bootstrap`. It now derives safe availability for `ssh_bootstrap` and `manual_bundle`, requires explicit operator selection when needed, submits through the existing operator `POST /api/v1/nodes/{id}/bootstrap` endpoint, and sends only `{ bootstrap_mode }` for the guided initial request. Guided onboarding does not set `reinstall_agent` or `force_reenroll`, does not call `/agent/*`, does not revoke tokens, does not reveal/download manual bundles and does not queue inventory sync. Plaintext token values are consumed only from the immediate create/rotate response through the one-time panel path and are not retained in query data, mutation data, storage, URLs, logs or toasts. Job acceptance is not treated as bootstrap success, registration, heartbeat or inventory evidence. Guided registration/heartbeat waiting and inventory sync remain Step 4C.2B2; final acceptance/debt closure remains pending Step 4D; live external-node smoke remains open. |
| `POST /api/v1/nodes/{id}/diagnostics/clear-stale-rotation` | stale rotation cleanup | missing | Nodes | legacy-only | Destructive cleanup path is not exposed in FE8-P0-05B. |
| `GET /api/v1/nodes/{id}/routes/preview`, `POST /routes/apply`, `/routes/cleanup` | route policy | `listRoutePolicies`; `getRoutePolicy`; `previewRoutePolicy`; `applyRoutePolicy`; `cleanupRoutePolicy`; matching hooks | Route Policy | connected | List/detail are node-scoped projections from `GET /api/v1/nodes`; preview is real backend output, Apply stays disabled until a fresh successful preview for the selected node, cleanup requires confirmation and returned jobs are tracked. |
| `GET/PUT /api/v1/nodes/{id}/access-methods`, `POST /ssh/host-key-scan`, `/ssh/sessions`, `GET /ssh/terminal` | bootstrap/terminal | `listNodeAccessMethods`; `acceptNodeHostKey`; `scanNodeHostKey`; `launchNodeSshSession`; matching hooks | Nodes/Security + Terminal | connected for configured SSH methods | Listing configured methods, existing host-key scan/pin/update and SSH session ticket launch are connected. The generic `PUT /access-methods` remains the existing collection/update workflow. SSH credentials stay server-side; terminal URL is shown only as a short-lived one-time backend URL. |
| `POST /api/v1/nodes/{id}/access-methods/ssh` | secure SSH access-method creation | `createNodeSSHAccessMethod`; `useCreateNodeSSHAccessMethod` | Nodes/Security | connected | Dedicated atomic creation endpoint protected by `node.bootstrap`. Host-key scan and explicit fingerprint/independent verification precede private-key submission. Response is redacted to `secret_configured`; private key is transient form state; `secret_ref_id` is not displayed. This workflow does not use `/legacy/`, `POST /api/v1/secret-refs` or generic `PUT /api/v1/nodes/{id}/access-methods` for creation. |
| `POST /api/v1/nodes/{id}/bootstrap`, `GET /bootstrap-runs`, `POST /bootstrap-runs/{run_id}/bundle/reveal`, `POST /bootstrap-runs/{run_id}/bundle/download`, compatibility `GET /bootstrap-runs/{run_id}/bundle` | bootstrap | `bootstrapNode`; `reinstallOrUpdateNodeAgent`; `listNodeBootstrapRuns`; `revealNodeBootstrapBundle`; `downloadNodeBootstrapBundle`; matching hooks | Nodes/Bootstrap | connected for queue/reinstall, guided onboarding submission and secure bundle reveal/download | SSH bootstrap/manual bundle job queueing and reinstall are connected with confirmation and job tracking. The Onboarding tab reuses the same `bootstrapNode` wrapper/hook owned by `NodeDrawer` and does not instantiate a separate mutation hook. Manual bundle reveal/download use only the dedicated POST endpoints from the Bootstrap tab, require `node.bootstrap`, use `manual_bundle_available === true` as the UI availability signal, require explicit acknowledgement, keep revealed content in local component state only, do not display `secret_ref_id`, download through the exact backend POST endpoint as a no-store Blob response, revoke temporary object URLs and avoid `/api/v1/secret-refs` and `/legacy/`. The retained GET endpoint is compatibility/deprecated and is not used by the new UI. Evidence: real PostgreSQL store test, real HTTP/router/PostgreSQL test and non-skipping CI groups in GitHub Actions run `29391281058`. Live operator onboarding validation remains release-validation debt. |
| `POST /api/v1/nodes/{id}/agent-token/rotate`, `/enrollment-token`, `/enrollment-token/rotate`, `GET /enrollment-tokens`, `DELETE /enrollment-tokens/{token_id}` | token control | `rotateNodeAgentToken`; `listEnrollmentTokens`; `createEnrollmentToken`; `rotateEnrollmentToken`; `revokeEnrollmentToken`; matching hooks | Nodes/Security | connected | Security token issue/rotate uses the same one-time consumer path as Onboarding. Enrollment token plaintext is shown only from create/rotate mutation responses and cleared on close, target change, permission loss, stale response or unmount. List/revoke show metadata/hints only. Agent token rotation returns a redacted signed job. |
| `POST /api/v1/nodes/{id}/agent-identity/revoke`, `/reboot`, `/emergency-cleanup`, `/diagnostics/clear-stale-rotation` | destructive/remediation controls | missing | Nodes | legacy-only | Not in FE8-P0-05B UI scope; requires separate destructive confirmation and operational validation. |
| `GET /api/v1/nodes/{id}/inventory`, `POST /api/v1/nodes/{id}/inventory/sync` | inventory view/sync | `getNodeInventory`; `syncNodeInventory`; `useNodeInventory`; `useSyncNodeInventory` | Nodes/Inventory | connected | Sync requires confirmation and shows returned job. Inventory payload is rendered as text. |
| `GET /api/v1/nodes/capabilities`, `GET /nodes/{id}/capabilities`, `POST /capabilities/install`, `/verify`, `GET /capabilities/drift`, `/install-events`, `GET /services/installers` | inventory/capabilities | `listNodeCapabilities`; `installNodeCapability`; `installNodeCapabilities`; `verifyNodeCapability`; `verifyNodeCapabilities`; drift/events/installers hooks | Nodes/Capabilities | connected | Install/verify require confirmation and async job tracking. Installer catalog is backend-driven. |
| `GET /api/v1/nodes/{id}/services/discovered`, `/discovery-summary`, `/{discovery_id}`, `POST /import`, `/services/import-all`, `/services/discover` | service discovery | `listNodeServiceDiscoveries`; `listNodeServiceDiscovery`; `discoverNodeServices`; `importNodeServiceDiscovery`; `importNodeServiceDiscoveryById`; matching hooks | Nodes/Service Discovery | connected | Discover queues a job; import requires confirmation. Backend-rendered payload is text only. |
| `POST /api/v1/nodes/{id}/services/discovered/{discovery_id}/ignore`, `/unignore` | service discovery triage | missing | Nodes/Service Discovery | legacy-only | Not exposed in FE8-P0-05A. |

### 4.6 Instances and Revisions

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/instances` | instance list | `listInstances`; `useInstances` | Instances | connected | List merges runtime summary from `GET /api/v1/instances/runtime-states`. |
| `POST /api/v1/instances`, `POST /api/v1/service-packs/{key}/instances` | instance create | `createInstanceManual`, `createInstanceFromServicePack`; `useCreateInstanceManual`, `useCreateInstanceFromServicePack` | Instances / Service Packs | connected | Create from service pack and manual create use real backend mutations, confirmation where needed, invalidation and safe error rendering. |
| `GET /api/v1/instances/runtime-states`, `GET /instances/{id}/runtime-state`, `/runtime-observations` | runtime state and diagnostics observations | `listInstanceRuntimeStates`; `getInstanceRuntimeState`; `getInstanceRuntimeObservations`; matching hooks | Instances | connected | Runtime and diagnostic output is rendered as text, not HTML. |
| `GET /api/v1/instances/{id}`, `GET /instances/{id}/revisions` | detail/revisions | `getInstance`; `getInstanceRevisions`; matching hooks | Instances/Revisions | connected | Revisions are read-only except rollback workflow below. |
| `PUT /api/v1/instances/{id}/spec` | spec replace | `replaceInstanceSpec`; `useReplaceInstanceSpec` | Instances / Spec | connected | Spec JSON is rendered as text, locally checked as an object, then backend-validated by the real replace mutation with confirmation. Separate preview and draft-save routes are backend-missing. |
| `POST /api/v1/instances/{id}/rollback`, `DELETE /{id}`, `POST /force-delete` | rollback/delete | `rollbackInstance`; `deleteInstance`; `forceDeleteInstance`; matching hooks | Instances | connected | Rollback requires explicit revision and confirmation; if backend returns apply-ready revision, UI queues a real apply job. Delete/force-delete require confirmation; force-delete requires exact confirmation text. |
| `POST /api/v1/instances/{id}/apply`, `/restart`, `/start`, `/stop`, `/enable`, `/disable`, `/diagnose` | lifecycle/jobs | `applyInstance`; `reapplyInstance`; `runInstanceLifecycleAction`; `runInstanceDiagnostics`; matching hooks | Instances | connected | Actions require confirmation, invalidate instances/runtime/jobs and render returned job state through `JobStatusPanel`. Backend has no separate apply preview endpoint. |
| `GET /api/v1/instances/{id}/vless-groups/members` | materialized access groups | `getInstanceAccessGroups`; `useInstanceAccessGroups` | Instances / Access groups | connected read-only | Shows materialization context and links to `Clients -> Groups`. No add/move/remove/create group actions are exposed under Instances. |
| `POST/PATCH/DELETE /api/v1/instances/{id}/vless-groups/...` | legacy instance VLESS membership writes | missing in new UI | Instances | legacy-only/deprecated | Primary group/member management belongs under Clients -> Groups. |

### 4.7 Address Pools, Firewall, Traffic

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/address-pools`, `POST/PUT/DELETE /address-pools/spaces`, `POST /spaces/{id}/routing` | address pools | `endpoints.addressPools`; `useAddressPools`; mutations missing | Address Pools | read-only / legacy-only |
| `GET /api/v1/firewall` | firewall inventory | `endpoints.firewallInventory`; `useFirewallInventory`; `useFirewallAddressGroups`; `useFirewallPolicies`; `useFirewallRules`; `useNodeFirewallState` | Firewall | connected |
| `POST/PUT/DELETE /api/v1/firewall/policies`, `/address-lists`, `/address-lists/{id}/entries`, `/policies/{id}/rules` | firewall model CRUD | `create/update/deleteFirewallAddressGroup`; entry wrappers; `create/update/deleteFirewallPolicy`; `create/update/deleteFirewallRule`; matching mutation hooks | Firewall | fully connected | Invalidate firewall inventory and jobs after mutations. Rule reorder remains unsupported because no backend endpoint exists. |
| `GET/PUT /api/v1/firewall/management-settings` | firewall safety settings | `getFirewallSafetySettings`; `updateFirewallSafetySettings`; `useFirewallSafetySettings`; `useUpdateFirewallSafetySettings` | Firewall/Safety | connected | UI shows trusted management source presence and strict-policy safety posture. |
| `POST /api/v1/nodes/{id}/firewall/preview`, `/apply`, `/disable` | node firewall apply | `previewNodeFirewall`; `applyNodeFirewall`; `disableNodeFirewall`; `usePreviewNodeFirewall`; `useApplyNodeFirewall`; `useDisableNodeFirewall` | Firewall | fully connected | Preview is mandatory before Apply in UI; stale preview disables Apply; backend strict apply still requires matching successful preview hash/job. |
| `GET /api/v1/traffic/accounting`, `GET /api/v1/traffic/accounting/export` | traffic overview/export | `endpoints.trafficAccounting`, `trafficAccountingExportURL`; `useTrafficAccounting` | Traffic | connected | Export opens backend URL; backend no-store applies. |

### 4.8 Clients, Delivery, Shares, Subscriptions

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET/POST /api/v1/clients`, `GET/PATCH/DELETE /api/v1/clients/{id}`, `POST /suspend`, `/activate` | clients CRUD/status | `listClients`; `getClient`; `createClient`; `updateClient`; `updateClientStatus`; `deleteClient`; matching hooks | Clients | connected | Create maps validation/conflict errors; generic metadata edit uses backend `PATCH`, delete is confirmed and query invalidation is wired. |
| `POST /api/v1/clients/{id}/revoke` | client revoke | `revokeClient`; `useRevokeClient` | Clients | connected | Revoke requires confirmation and shows the returned job. |
| `DELETE /api/v1/clients/{id}/configs` | config cleanup | `cleanupClientConfigs`; `useCleanupClientConfigs` | Clients detail / Maintenance | connected | Destructive confirmation required. Result counts are rendered safely; generated config payloads and tokens are never displayed or stored. |
| `POST /api/v1/clients/{id}/provision` | direct provisioning | missing | Clients detail | legacy-only | New UI uses VLESS access-group assignment instead of direct one-job-per-client provisioning. |
| `GET /api/v1/clients/{id}/accesses` | accesses | `listClientAccesses`; `getClientAccessOverview`; `useClientAccesses`; `useClientAccessOverview` | Clients detail | connected read | Access identity is redacted; UUIDs, credentials and secret metadata are not displayed. |
| `POST /api/v1/clients/{id}/accesses/{access_id}/rotate-*` | access rotation | `rotateClientAccess`; `useRotateClientAccess` | Clients detail / Maintenance | connected | Whitelisted driver suffixes only; confirmation required. Backend has no preview endpoint, so UI does not fake preview. Returned jobs are linked/tracked. |
| `DELETE /api/v1/clients/{id}/accesses/{access_id}` | service access delete | `deleteClientAccess`; `useDeleteClientAccess` | Clients detail / Maintenance | connected | Confirmation required; result counts and queued job counts are shown. |
| `POST /api/v1/clients/{id}/accesses/{access_id}/revoke` | per-access revoke | `revokeClientAccess`; `useRevokeClientAccess` | Clients detail / Maintenance | connected | Confirmation required; backend revokes the selected service access without deleting the row, revokes related active routes/share links and shows queued convergence counts. |
| `GET/POST /api/v1/clients/{id}/routes`, `PATCH/DELETE /api/v1/clients/{id}/routes/{route_id}` | client routes | `listClientRoutes`; `createClientRoute`; `updateClientRoute`; `deleteClientRoute`; matching hooks | Clients detail / Routes | connected | List/create/update/delete are wired with backend validation. Delete is confirmed; update shows impact text and uses backend route-policy convergence. |
| `GET /api/v1/clients/{id}/access-groups`, `PATCH /access-groups/{service_code}` | per-client group membership | read through `getClientAccessOverview`; assignment uses `previewSingleClientAccessGroupAssignment` / `applySingleClientAccessGroupAssignment` against `/client-access-groups/{group_id}/members:*` | Clients detail / Access | connected for VLESS | Preview is mandatory; group/mode/client changes make preview stale and disable Apply. |
| `GET/POST/DELETE /api/v1/clients/{id}/artifacts`, `GET /content`, `GET /download` | client artifacts | `listClientArtifacts`; `buildClientArtifact`; `getClientArtifactDownload`; `deleteClientArtifact`; matching hooks | Clients detail / Artifacts | connected | Build returns job tracking; download opens backend URL without token storage; delete is confirmed. Inline content preview is not exposed in this task. |
| `GET/POST /api/v1/clients/{id}/share-links`, `POST /share-links/{link_id}/rotate`, `POST /share-links/{link_id}/revoke` | share links | `listClientShareLinks`; `createClientShareLink`; `rotateClientShareLink`; `revokeClientShareLink`; matching hooks | Clients detail / Delivery | connected | Share token is converted to a one-time `/share/{token}` URL, stripped from the cached share row and shown only in transient local UI state after create/rotate. Rotate revokes the old link and publishes a new token for the same target. |
| `GET /api/v1/clients/{id}/subscriptions`, `POST /subscriptions/rotate`, `POST /subscriptions/{subscription_id}/revoke` | VLESS subscriptions | `listClientSubscriptions`; `createClientSubscription`; `rotateClientSubscription`; `revokeClientSubscription`; matching hooks | Clients detail / Delivery | connected for VLESS | Backend has a create-or-rotate endpoint rather than a separate create route. Returned subscription URL is shown only in transient local UI state. Non-VLESS subscriptions are not exposed. |
| `POST /api/v1/clients/{id}/deliver-email`, `GET /api/v1/clients/{id}/deliveries` | email delivery/history | `sendClientArtifactEmail`; `listClientDeliveryHistory`; `useSendClientArtifactEmail`; `useClientDeliveryHistory` | Clients detail / Delivery | connected | Backend returns a synchronous delivery result rather than an async job. History uses safe DTOs with masked destination hints and redacted error summaries. Artifact-specific email payload remains backend-missing. Errors are rendered as text. |

### 4.9 Backhaul, Artifacts Aggregate, Jobs, Audit

| Backend endpoint family | Legacy usage | New wrapper / hook | UI page | Status | Invalidation / security notes |
| --- | --- | --- | --- | --- | --- |
| `GET /api/v1/backhaul/drivers`, `GET/POST /api/v1/backhaul-links`, `GET /backhaul-links/{id}` | backhaul list/create/detail | `endpoints.backhaulDrivers`; `listBackhaulLinks`; `getBackhaulLink`; `useBackhaulLinks`; `useBackhaulLink` | Backhaul | connected for list/detail; create not exposed | Link/transport detail is rendered through safe fields only; transport `config` and `secret_refs` are not rendered. |
| `POST /api/v1/backhaul-links/{id}/apply`, `/probe`, `/promote`, `PATCH /route`, `DELETE /{id}` | backhaul actions | `applyBackhaulLink`; `probeBackhaulLink`; `promoteBackhaulLink`; `updateBackhaulRouteState`; matching hooks | Backhaul | connected for apply/probe/promote/route-state; delete not exposed | Confirmation required; returned jobs are tracked. Backend has no separate repair endpoint, so no fake repair action is shown. |
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
| Users/invites/sessions | `platform-users`, `platform-invites`, `platform-sessions`, `auth/session`, audit |
| Settings/mail/TLS | `platform-settings`, `mail-settings`, `runtime-preflight`, jobs |
| Certificates/PKI | `certificates`, `pki-roots`, instances where certificate refs are shown |
| Service packs/VLESS templates | `service-packs`, services catalog, client access services/groups |
| Client access groups | `client-access-groups`, members, scope, sync-state, clients, jobs |
| Nodes | `nodes`, node detail, diagnostics, capabilities, jobs, dashboard |
| Instances | `instances`, runtime-states, revisions, jobs, dashboard |
| Address pools | `address-pools`, instances, firewall when address groups depend on pools |
| Firewall | `firewall-inventory`, node detail/firewall state, jobs, policies, rules, address groups |
| Clients/routes/accesses/artifacts/share/subscriptions/delivery history | `clients`, client detail, accesses, routes, artifacts, share-links, subscriptions, delivery history, jobs, dashboard |
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
