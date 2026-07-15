# RTIS MegaVPN Frontend Security Review 8.0.0

Release scope: MegaVPN Console 8.0.0 RC1.

Status: security review baseline for the new React console.

## 1. Security Summary

The new console preserves the existing browser security model:

- cookie-based session auth;
- `credentials: include` for all API requests;
- `X-MegaVPN-CSRF: 1` on unsafe methods;
- no bearer/session/auth token stored in `localStorage` or `sessionStorage`;
- backend RBAC remains authoritative;
- frontend permission checks are UX affordances only;
- legacy console remains available at `/legacy/` for rollback.

RC1 intentionally keeps high-risk write workflows disabled or legacy-only unless
they are fully wired to backend endpoints with confirmation, error handling and
job tracking.

## 2. Verified Controls

| Control | Status | Evidence / enforcement |
| --- | --- | --- |
| No browser auth token storage in new frontend | enforced | `scripts/ci/frontend-static-guards.sh` scans `frontend/src`. |
| Cookie/session auth preserved | implemented | `frontend/src/shared/api/client.ts` sends `credentials: include`. |
| CSRF header on unsafe methods | implemented | `frontend/src/shared/api/client.ts` sets `X-MegaVPN-CSRF: 1` for non-safe methods. |
| Raw API calls outside API layer blocked | enforced | `scripts/ci/frontend-static-guards.sh`. |
| Unreviewed `dangerouslySetInnerHTML` blocked | enforced | `scripts/ci/frontend-static-guards.sh`. |
| Production console response logging blocked | enforced | `scripts/ci/frontend-static-guards.sh`. |
| SPA fallback does not shadow backend routes | tested | `internal/api/http/static_serving_test.go`. |
| `/legacy/` rollback preserved | tested | `internal/api/http/static_serving_test.go` and serving smoke. |
| Missing assets return 404, not index HTML | tested | `internal/api/http/static_serving_test.go`. |

## 3. Auth and Session

The new frontend must never store:

- session IDs;
- bearer tokens;
- auth tokens;
- passwords;
- invite accept passwords;
- generated credentials.

Allowed browser storage keys:

| Key | Purpose | Secret? |
| --- | --- | --- |
| `megavpn.locale` | UI language preference | no |
| `megavpn.apiBase` | local development API base override | no |

The API client must remain same-origin by default. `megavpn.apiBase` is a local
operator override and must not be used to persist credentials.

## 4. CSRF

Unsafe methods are:

- `POST`;
- `PUT`;
- `PATCH`;
- `DELETE`.

For these methods the frontend API client sets:

```http
X-MegaVPN-CSRF: 1
```

Pages must not bypass the API client for mutations.

## 5. RBAC

Frontend permission checks:

- may hide or disable navigation/actions for usability;
- must not be treated as a security boundary;
- must not enable a mutation unless the backend endpoint enforces permission.

Backend wrappers in `internal/api/http/server.go` remain the enforcement point:

- `authenticated`;
- `protected`;
- `protectedAll`.

## 6. Secret Redaction Requirements

The frontend must treat these values as sensitive:

- private keys;
- API tokens;
- share tokens;
- subscription tokens;
- bearer values;
- generated passwords;
- one-time URLs;
- bootstrap secrets;
- SSH credentials;
- sensitive job output.

Rules:

- do not log API responses to browser console;
- do not persist generated secrets in browser storage;
- do not keep share/subscription tokens in long-lived React state;
- show one-time values only when the backend intentionally returns them;
- prefer token hints/fingerprints over full values after creation.

## 7. XSS-Safe Rendering

The following data is untrusted and must be rendered as text, not HTML:

- job logs;
- diagnostic output;
- certificate fields;
- audit details;
- backend validation errors;
- node inventory strings;
- service discovery data;
- artifact content previews.

No `dangerouslySetInnerHTML` usage is approved for RC1.

## 8. Downloads and Public URLs

Public and download flows remain backend-owned:

- `/share/{token}`;
- `/subscribe/vless/{token}`;
- `/api/v1/clients/{id}/artifacts/{artifact_id}/download`;
- `/agent/binary-artifacts/{artifact_id}/download`.

The React console must not rewrite or proxy these responses client-side.
Backend cache/no-store and rate limits remain authoritative.

## 9. Dangerous Operations

Before enabling any dangerous operation, the UI must include:

1. preview when backend supports it;
2. explicit confirmation;
3. impact summary;
4. required permission state;
5. real backend mutation;
6. job tracking if async;
7. conflict/validation error display;
8. query invalidation;
9. no fake success.

Affected operation classes:

- apply;
- delete;
- force-delete;
- revoke;
- rotate;
- bootstrap;
- firewall apply/disable;
- route policy apply/cleanup;
- certificate revoke/delete;
- settings apply;
- rollback;
- node retire/force-retire.
- manual bootstrap bundle reveal/download.

## 10. Secure SSH Access Method Creation Review

Nodes -> Security now supports creating a new SSH access method with secret
material without `/legacy/`. The reviewed scope is secure configuration and
encrypted persistence, not live SSH connectivity or bootstrap success.

Backend controls:

- purpose-specific `POST /api/v1/nodes/{id}/access-methods/ssh` endpoint;
- `node.bootstrap` authorization and existing CSRF/session handling;
- unknown-field rejection, including caller-supplied `secret_ref_id`;
- private-key parsing and rejection of public, malformed or passphrase-protected
  private keys;
- encrypted secret storage through the existing secret service;
- one PostgreSQL transaction for duplicate check, secret ref insert,
  access-method insert and audit insert;
- advisory-lock/duplicate protection for concurrent creates;
- redacted response containing `secret_configured` instead of secret reference
  or ciphertext fields;
- redacted logs and audit payloads, with rollback preventing orphan access
  methods or orphan committed secrets.

Frontend controls:

- host-key scan runs before key entry;
- the first returned fingerprint is not selected automatically;
- operators must explicitly select a fingerprint and confirm independent
  verification before the private-key input is shown;
- private-key material is not stored in localStorage, sessionStorage or
  IndexedDB and is not placed in query keys or global app state;
- form state is cleared on target/trust changes, submit, close/cancel, backend
  failure, success and modal unmount;
- the UI does not render `secret_ref_id`, does not call the generic secret-ref
  endpoint, and does not request `/legacy/`.

Evidence controls:

- real PostgreSQL store tests cover atomic persistence, retired-node rejection
  and concurrent duplicate behavior;
- real HTTP/router/PostgreSQL test covers auth, CSRF, handler, store,
  encrypted persistence and redacted response;
- GitHub Actions run `29361072970`, job `PostgreSQL integration tests`, ran the
  required SSH integration tests without skips.

The UI minimizes secret lifetime and prevents application-level persistence;
JavaScript runtime memory erasure is not guaranteed.

## 11. Manual Bootstrap Bundle Reveal/Download Review

Nodes -> Bootstrap now supports manual bootstrap bundle reveal and download
without `/legacy/`. The reviewed scope is secure operator retrieval from an
already completed manual bootstrap run; live operator onboarding validation
remains release-validation debt.

Backend/API controls used by the UI:

- purpose-specific `POST /api/v1/nodes/{id}/bootstrap-runs/{run_id}/bundle/reveal`;
- purpose-specific `POST /api/v1/nodes/{id}/bootstrap-runs/{run_id}/bundle/download`;
- `node.bootstrap` authorization and existing CSRF/session handling;
- direct `node_id` + `run_id` lookup;
- run ownership, `manual_bundle` mode and `succeeded` status validation;
- expected secret type and metadata validation for node id,
  `material=agent_bootstrap_env` and bootstrap run id when present;
- maximum bundle size enforcement;
- targeted `manual_bundle_available` projection;
- removal of secret reference and plaintext fields from public bootstrap-run
  projections;
- backend-owned encrypted secret resolution; the UI does not inspect secret
  refs or `result_payload` to discover bundle contents;
- no-store/private response headers, no `ETag` and no `Last-Modified`;
- safe server-owned attachment filename;
- exact-byte download through the backend endpoint;
- separate `node.bootstrap_bundle.reveal` and
  `node.bootstrap_bundle.download` audit actions;
- fail-closed audit before secret-bearing responses;
- bundle absence from logs and audit payloads.

Frontend controls:

- the new UI does not call deprecated compatibility `GET /bundle`;
- the new UI does not call `/api/v1/secret-refs`;
- the new UI does not call `/legacy/`;
- availability is based only on `manual_bundle_available`;
- reveal and download both require explicit confirmation plus an
  acknowledgement checkbox;
- confirmation dialogs show only safe metadata: node label, short bootstrap run
  ID and action type;
- revealed bundle content is held only in local component state and is cleared
  on close, target/run/permission changes, new reveal, stale response and
  unmount;
- no browser-storage, query-key or global-store persistence is used for bundle
  content;
- reveal mutation variables contain node ID and run ID, not the bundle content;
- download always calls the dedicated POST download endpoint, even when a reveal
  panel is already open;
- download uses an object URL and temporary anchor, then removes the anchor and
  revokes the object URL;
- copy uses `navigator.clipboard.writeText` only on explicit operator click;
- stale 404 clears matching revealed content and refetches bootstrap runs;
- 403 clears matching revealed content and reports a safe permission error.

Evidence controls:

- fake-store/unit HTTP tests cover POST reveal/download, CSRF, credentials,
  no-store download, filename parsing/sanitization and HTML-response rejection;
- frontend API/page tests cover confirmation gating, acknowledgement gating,
  copy/download, 404 stale clearing/refetch, permission gating, no `/legacy/`,
  no `/api/v1/secret-refs` and no browser-storage persistence for bundle
  content;
- real PostgreSQL scoped lookup and secret resolution are covered by
  `TestPostgresIntegrationGetNodeBootstrapRunScopedAndResolvesManualBundleSecret`;
- real HTTP/router/PostgreSQL evidence is covered by
  `TestPostgresIntegrationNodeBootstrapBundleRevealDownloadHTTP`;
- encrypted-at-rest, public projection redaction, reveal, exact-byte download,
  persisted audit, RBAC rejection, CSRF rejection, cross-node rejection,
  missing-bundle rejection and log-redaction assertions are covered by the
  PostgreSQL-backed tests;
- GitHub Actions run `29391281058`, job `PostgreSQL integration tests`, ran
  `postgres-bootstrap-bundle-infra` and `postgres-bootstrap-bundle-http`
  without skips.

The UI minimizes bundle lifetime and prevents application-level persistence;
JavaScript runtime memory erasure is not guaranteed.

## 12. Guided Agent Onboarding Status, Token, Bootstrap and Inventory Sync Review

Nodes -> Onboarding now provides a guided status model plus secure enrollment
token issue/reissue actions, guided bootstrap mode selection/job submission,
registration waiting, heartbeat waiting and guided inventory-sync submission
without `/legacy/`. The reviewed mutation scope is limited to existing
operator enrollment-token endpoints, the existing operator bootstrap endpoint
and the existing operator inventory-sync endpoint. It does not include browser
agent registration, manual bundle reveal/download or live external-node
connectivity proof.

Backend/API controls used by the UI:

- existing operator `GET /api/v1/nodes/{id}` detail;
- existing operator `GET /api/v1/nodes/{id}/diagnostics`;
- existing operator `GET /api/v1/nodes/{id}/enrollment-tokens`;
- existing operator `GET /api/v1/nodes/{id}/bootstrap-runs`;
- existing operator `GET /api/v1/nodes/{id}/inventory`;
- existing operator `GET /api/v1/nodes/{id}/access-methods`;
- existing operator `POST /api/v1/nodes/{id}/enrollment-token`;
- existing operator `POST /api/v1/nodes/{id}/enrollment-token/rotate`;
- existing operator `POST /api/v1/nodes/{id}/bootstrap`;
- existing operator `POST /api/v1/nodes/{id}/inventory/sync`;
- redacted diagnostics projections for enrollment-token metadata and bootstrap
  run summaries;
- backend-derived `heartbeat_state`, `communication_state` and
  `token_rotation_status`;
- backend job claim/result projections for source-defined inventory job types
  `node.inventory` and `node.inventory.sync`.

Frontend controls:

- the onboarding model is a pure typed derivation module and does not accept
  `/agent/*` responses, registration responses, plaintext enrollment tokens,
  bootstrap bundle contents, private keys or secret references;
- the Onboarding tab exposes issue/reissue enrollment-token actions when the
  typed status model recommends the action and the operator has
  `node.bootstrap`;
- the Onboarding tab derives conservative `ssh_bootstrap` and `manual_bundle`
  readiness from safe node/access-method/bootstrap-run data and never displays
  `secret_ref_id`;
- guided bootstrap requires explicit mode selection when multiple backend
  modes are available, requires confirmation/acknowledgement and submits only
  `{ bootstrap_mode }`;
- guided bootstrap does not set `reinstall_agent` or `force_reenroll`, does not
  send plaintext enrollment tokens, token IDs, SSH private keys, secret
  references, node profile fields or raw payloads, and does not instantiate a
  separate bootstrap hook inside `NodeOnboardingTab`;
- manual bundle reveal/download buttons remain only in the Bootstrap tab; the
  Onboarding tab may navigate to Bootstrap when
  `manual_bundle_available === true`;
- the Onboarding tab has no buttons for token revoke or manual bundle
  reveal/download; guided inventory sync is exposed only after backend
  registration and heartbeat evidence, requires `node.write` separately from
  `node.bootstrap`, and uses the shared operator inventory-sync endpoint;
- issue/reissue requires explicit confirmation, a valid TTL between 1 and 720
  hours and an acknowledgement that the plaintext token is sensitive and shown
  only once;
- issue/reissue uses the shared create/rotate hook with `gcTime: 0`, a void
  mutation result and a one-time consumer callback. The plaintext token is
  extracted only from the immediate API response, then placed in the transient
  `OneTimeSecretPanel`;
- stale create/rotate responses are discarded if the drawer closes, selected
  node changes, `node.bootstrap` permission disappears, another one-time secret
  action starts or the component unmounts;
- the existing Nodes -> Security token create/rotate controls use the same
  one-time consumer path as Onboarding;
- next-step buttons only change the selected existing Nodes tab;
- guided bootstrap records returned job/run identifiers, keeps the selected
  node drawer on Onboarding and lets the operator navigate to Jobs without
  treating accepted jobs as success;
- after real registration and heartbeat evidence, guided inventory sync uses
  the same `syncNodeInventory` / `useSyncNodeInventory` wrapper and mutation
  hook as the standalone Inventory tab; the hook remains owned by `NodeDrawer`
  and is not instantiated inside `NodeOnboardingTab`;
- guided inventory sync requires `node.write` separately from `node.bootstrap`,
  is not exposed when the operator lacks `node.write`, requires explicit
  confirmation plus acknowledgement, queues only
  `POST /api/v1/nodes/{id}/inventory/sync`, keeps the operator on Onboarding
  and stores returned job IDs only in bounded component-local state;
- guided inventory sync is never queued automatically after token issue,
  bootstrap job acceptance, bootstrap success, registration or heartbeat, and
  it is not retried automatically after failure;
- accepted, queued, running, stalled, failed, superseded and unknown inventory
  job states are derived only from safe operator diagnostics, inventory
  snapshots and transient returned job IDs. Accepted/queued/running jobs are
  not treated as synchronized inventory;
- ready status requires agent registration, heartbeat evidence, inventory
  evidence and a healthy backend communication state;
- queued bootstrap or queued inventory jobs are not treated as successful
  onboarding;
- unknown bootstrap-run status and unknown inventory job/result state fail
  closed for guided mutation submission;
- retired/deleted nodes are blocked;
- partial query failures are rendered as safe per-source alerts rather than
  collapsing to fake `not_started`;
- 10-second onboarding polling is limited to safe read queries while the
  Onboarding tab is active and stops when the model becomes ready/blocked, the
  tab changes or the drawer closes;
- token plaintext, token hashes, `secret_ref_id`, request signatures, nonces,
  authorization headers, bootstrap bundle content, raw diagnostics, raw job
  payloads, complete inventory payloads and raw secret metadata are not
  rendered by the onboarding panel except for the intentional one-time
  plaintext token reveal after a successful issue/reissue response;
- plaintext tokens are not retained in query data, mutation data, mutation
  variables, global state, Zustand, localStorage, sessionStorage, IndexedDB,
  URL query parameters, router state, logs, errors, toasts, fixtures or
  snapshots.

Evidence controls:

- backend registration hardening commit
  `2a8784b36f47d35f758968a382b33c785ee534af`, GitHub Actions run
  `29393548655` PASS;
- retry/reissue and response-loss recovery commit
  `54dfcb83c2fdd2444d8b868289b5c995a14dfbdf`, GitHub Actions run
  `29398570940` PASS;
- real HTTP/router/PostgreSQL onboarding evidence commit
  `8206a42cfab7a6218fdcc7caf2222050b694fdca`, GitHub Actions run
  `29401792602` PASS;
- read-only onboarding UI commit
  `40c769278e7098c9662d2129d4f7b568012374cb`, GitHub Actions run
  `29404400407` PASS;
- secure token actions commit
  `dfeb94276c9d996003e6a3785bd41afdf625df16`, GitHub Actions run
  `29407238972` PASS;
- guided bootstrap commit
  `5d5532b26bc8bda228dc51d079b823f1ea2b232f`, GitHub Actions run
  `29411127362` PASS;
- guided registration, heartbeat and inventory progression commit
  `42065d6ac765a66ac983c611c0f0fdfaf8cb67a2`, GitHub Actions run
  `29415883087` PASS, including PostgreSQL integration;
- PostgreSQL integration covers disposable PostgreSQL, real router,
  session/RBAC/CSRF, real agent registration, signed heartbeat, signed job
  claim/result flow, replay protection, real inventory persistence, real
  diagnostics, replacement-token recovery and backend-derived
  `communication_state = inventory_ok`;
- pure-model tests cover not-started, active token, bootstrap queued/running,
  successful/failed bootstrap ordering, registration, revoked agent, heartbeat
  states, unhealthy communication states, inventory evidence, ready criteria,
  unknown statuses, source-array immutability, action recommendations for
  token issue/reissue, guided bootstrap and guided inventory sync, exact
  inventory job-type recognition, accepted/running/stalled/failed/superseded
  inventory states, failed relevant results, unrelated failed results, unknown
  successful-without-inventory results, no destructive recommendation for
  unknown statuses and plaintext token omission;
- bootstrap-readiness tests cover exact supported modes, retired/deleted/local
  blocks, SSH prerequisite checks, manual bundle without SSH, active/unknown
  run fail-closed behavior, deterministic latest-run selection, conservative
  recommendation and secret-reference omission;
- Nodes UI tests cover the Onboarding tab, existing tab preservation, six
  ordered steps, safe evidence rendering, secret redaction, issue/reissue
  confirmation, TTL validation, one-time reveal/copy/close behavior,
  stale-response discard, guided bootstrap mode selection, safe SSH/manual
  confirmation, exact `{ bootstrap_mode }` request shape, returned job/run
  tracking, manual-bundle-ready navigation without reveal/download,
  firewall-prerequisite error handling, guided registration waiting, guided
  heartbeat waiting, guided inventory confirmation/acknowledgement,
  `node.write` versus `node.bootstrap` permission separation, accepted job
  tracking, running/stalled/failed/ready inventory progression, 409 conflict
  handling, permission-aware action hiding/disabling, no `/agent/*` browser
  calls, no `/legacy/`, polling lifecycle, partial query failure display,
  browser-storage safety and absence of production console logging.

Guided Agent Onboarding security review is accepted for PR-review evidence in
Step 4D.1. Release debt accounting remains Step 4D.2, and live external-node
smoke remains release-validation debt.

## 13. RC1 Limitations

The new console remains incomplete for final write parity. The following are
intentionally disabled, backend-missing or legacy-only after FE8-P0-09B:

- non-VLESS access service materialization and access-group migration conflict UI;
- live external-node onboarding smoke and Step 4D.2 release debt closure;
- agent identity revoke, reboot, emergency cleanup and stale rotation cleanup;
- node service discovery ignore/unignore;
- runtime artifact delete;
- separate service pack validation, instance spec preview and instance
  draft-save endpoints;
- Platform invite revoke and direct Platform user lifecycle mutations;
- backhaul create/delete;
- backup/restore browser UI.

This is a security-positive limitation: operators must not see a clickable
action unless it is backed by real endpoint behavior and safe UX.

## 14. Required Checks

For RC1 evidence, run:

```bash
scripts/ci/frontend-static-guards.sh
scripts/ci/frontend-serving-smoke.sh
go test ./internal/api/http
cd frontend && npm ci && npm run typecheck && npm run lint && npm run test && npm run i18n:check && npm run build
```

Any skipped check must be documented in `docs/FRONTEND_ACCEPTANCE_8.0.0.md`.
