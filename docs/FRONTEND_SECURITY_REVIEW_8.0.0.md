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

## 10. RC1 Limitations

The new console remains incomplete for final write parity. The following are
intentionally disabled, backend-missing or legacy-only after FE8-P0-09B step 1:

- non-VLESS access service materialization and access-group migration conflict UI;
- node agent registration/onboarding, new SSH access method creation with
  secret material, manual bootstrap bundle reveal, agent identity revoke,
  reboot, emergency cleanup and stale rotation cleanup;
- node service discovery ignore/unignore;
- runtime artifact delete;
- separate service pack validation, instance spec preview and instance
  draft-save endpoints;
- Platform invite revoke and direct Platform user lifecycle mutations;
- backhaul create/delete;
- backup/restore browser UI.

This is a security-positive limitation: operators must not see a clickable
action unless it is backed by real endpoint behavior and safe UX.

## 11. Required Checks

For RC1 evidence, run:

```bash
scripts/ci/frontend-static-guards.sh
scripts/ci/frontend-serving-smoke.sh
go test ./internal/api/http
cd frontend && npm ci && npm run typecheck && npm run lint && npm run test && npm run i18n:check && npm run build
```

Any skipped check must be documented in `docs/FRONTEND_ACCEPTANCE_8.0.0.md`.
