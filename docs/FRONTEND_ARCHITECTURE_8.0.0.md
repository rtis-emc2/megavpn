# RTIS MegaVPN Frontend Architecture 8.0.0

Release scope: 8.0.0 operator console.

## Stack

- React
- TypeScript
- Vite
- TanStack Query
- react-i18next / i18next
- CSS variables and MegaVPN UI components
- Go API static serving from `web/`

No Docker, CDN runtime libraries, Bootstrap, jQuery, SSR, or browser-stored auth tokens are required.

## Directory Structure

```text
frontend/
  package.json
  package-lock.json
  pnpm-lock.yaml
  vite.config.ts
  src/
    app/
    shared/
      api/
      auth/
      config/
      i18n/
      layout/
      permissions/
      query/
      ui/
      utils/
    entities/
    features/
    pages/
    styles/
```

Build output goes to `web/`.

Legacy UI is preserved under:

```text
web/legacy/index.html
web/legacy/assets/*
```

## Design System

Design tokens live in `frontend/src/styles/tokens.css`.

Required token groups:

- color;
- status;
- spacing;
- radius;
- shadow;
- typography;
- breakpoints.

Reusable UI primitives live in `frontend/src/shared/ui/`.

Pages should not create ad hoc buttons, badges, cards, tables, forms, overlays or status labels. New patterns should be added to `shared/ui` first.

## i18n Model

Resources:

```text
frontend/src/shared/i18n/resources/ru.json
frontend/src/shared/i18n/resources/en.json
```

Rules:

- default locale is browser locale if supported, otherwise Russian;
- selected locale is stored in `localStorage` key `megavpn.locale`;
- user-facing page text must use translation keys;
- `npm run i18n:check` fails if ru/en keys diverge or values are empty.

## Responsive Model

Breakpoints:

- phone: `< 768px`;
- tablet: `768-1279px`;
- desktop: `>= 1280px`;
- wide: `>= 1536px`.

Phone layout converts data tables into record cards by default. Technical matrices may remain horizontally scrollable only when a card view would hide essential structure.

## API Client Model

The API boundary lives in `frontend/src/shared/api/`.

Security behavior preserved from the legacy console:

- same-origin API by default;
- optional local API base override via `megavpn.apiBase`;
- `credentials: include` on all requests;
- `X-MegaVPN-CSRF: 1` on unsafe methods;
- no bearer token or password persistence.

Pages should use endpoint functions and query hooks instead of raw `fetch`.

## Query / Mutation Model

TanStack Query owns server state.

Rules:

- domain-specific query keys;
- page-scoped polling;
- mutation invalidation after state changes;
- 401 means unauthenticated;
- 403 means authenticated but not permitted;
- form state must not be destroyed by background refresh.

## Permission Model

Frontend permission checks are UX affordances only. Backend RBAC remains authoritative.

Navigation filters by permission where possible. A hidden button or page is not a security boundary.

## Legacy Migration Model

The Go API serves:

- `/` and non-backend browser frontend paths from new Vite output;
- `/assets/*` from new built assets;
- `/legacy/` from the old static console;
- `/legacy/assets/*` from old static assets.

API, public share/subscription, and agent endpoints remain unchanged.

SPA fallback is deliberately narrow. It must not shadow:

- `/api/*`;
- `/agent/*`;
- `/share/*`;
- `/subscribe/*`;
- `/download/*`;
- `/downloads/*`;
- `/exports/*`;
- `/assets/*`;
- `/legacy/*`;
- `/health`;
- `/healthz`;
- `/ready`;
- file-like paths such as `/favicon.ico` or `/unknown/file.json`.

`scripts/ci/frontend-serving-smoke.sh` validates root serving, deep links,
legacy rollback, API non-shadowing and static asset 404 behavior.

## Adding a Page

1. Add route and navigation metadata if it is navigable.
2. Add typed API endpoint functions and query hooks.
3. Implement page under `frontend/src/pages/<domain>/`.
4. Use `PageHeader`, `DataTable`, `MobileRecordList`, `Card`, `Drawer`, `Modal`, and shared form components.
5. Add translations in both ru/en.
6. Add or update tests.

## Adding a UI Component

1. Add component under `frontend/src/shared/ui/`.
2. Add styling through tokens and shared CSS.
3. Export from `frontend/src/shared/ui/index.ts`.
4. Keep keyboard/focus behavior explicit.
5. Avoid page-specific assumptions.

## Adding Translations

1. Add the same key to `ru.json` and `en.json`.
2. Run `npm run i18n:check`.
3. Use the key through `t('namespace.key')`.

## Testing Commands

```bash
cd frontend
npm ci
npm run typecheck
npm run lint
npm run test
npm run i18n:check
npm run build
```

Repository-level gates also validate Go, shell scripts, generated web assets and legacy bootstrap.

Additional RC1 frontend gates:

```bash
scripts/ci/frontend-serving-smoke.sh
scripts/ci/frontend-static-guards.sh
```

`frontend-static-guards.sh` fails on raw `/api/v1` usage outside the API layer,
browser storage of auth/session/token material, unreviewed `dangerouslySetInnerHTML`
and production console logging.

## RC1 Mutation Policy

The new console must not show fake-success actions. A mutating action is allowed
only when it is fully wired to a backend endpoint with CSRF, permission-aware
disabled state, error handling, invalidation and confirmation/job tracking where
needed. Otherwise the action must be disabled, removed from primary navigation
or linked to `/legacy/` with an explicit limitation.
