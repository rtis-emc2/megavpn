# RTIS MegaVPN Frontend Acceptance 8.0.0

Branch: `release/8.0.0-frontend-console`

Commit at evidence time: `b51d0c134c486be3db4686326147cb97872f7a12`

Evidence date UTC: `2026-07-08T17:29:40Z`

Status: RC1 groundwork improved; final 8.0.0 frontend cutover is not complete.

## 1. Summary

This pass completed the RC1 safety layer:

- created the RC1 execution plan before functional changes;
- added narrow Go SPA fallback for new frontend deep links;
- preserved `/legacy/` rollback;
- added backend serving tests;
- added frontend serving smoke gate;
- added frontend static security guards;
- removed fake local preview/apply enablement from Clients -> Groups and Firewall;
- connected Jobs cancel with confirmation, backend mutation and query invalidation;
- updated endpoint parity, write workflow and security review documentation;
- rebuilt Vite assets into `web/` with a safe pre-build cleanup that does not remove legacy assets.

The new console still does not provide full write-workflow parity with the
legacy UI. It should not be declared final release-ready as the only operator UI.

## 2. Commands Run

| Check | Status | Evidence |
| --- | --- | --- |
| `gofmt -w internal/api/http/server.go internal/api/http/static_serving_test.go` | PASS | Applied formatting before tests. |
| `go test ./internal/api/http -run 'TestStaticServingRoutes|TestShouldServeFrontendFallback' -count=1` | PASS | Serving route contract passes. |
| `scripts/ci/frontend-static-guards.sh` | PASS | Static frontend guards pass. |
| `scripts/ci/frontend-serving-smoke.sh` | PASS | Root/deep links/legacy/API non-shadowing/asset 404 pass. |
| `cd frontend && pnpm run typecheck` with bundled Node | PASS | TypeScript checks pass. |
| `cd frontend && pnpm run lint` with bundled Node | PASS | ESLint passes. |
| `cd frontend && pnpm run test` with bundled Node | PASS | Vitest: 1 file, 1 test passed. |
| `cd frontend && pnpm run i18n:check` with bundled Node | PASS | i18n key parity ok: 260 keys. |
| `cd frontend && pnpm run build` with bundled Node | PASS | Vite build wrote `web/index.html`, `web/.vite/manifest.json`, `web/assets/index-B-l2116U.js`, `web/assets/index-C9SCsY-r.css`. |
| `PATH=... node --check web/assets/*.js` | PASS | Built JS parses. |
| `PATH=... pnpm dlx npm@11.18.0 ci --ignore-scripts` | PASS | npm-compatible clean install completed; `node_modules` removed afterward to keep workspace clean. |
| `go test ./cmd/api` | PASS | No test files; package builds for tests. |
| `go vet ./...` | PASS | No vet findings. |
| `go test ./...` | PASS | All Go package tests pass. |
| `go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin` | PASS | All operational binaries build. |
| `go test -race ./...` | PASS | Race detector tests pass after removing frontend `node_modules`. |
| `PATH=... node scripts/ci/frontend-bootstrap-smoke.js` | PASS | Legacy bootstrap smoke ok: 38 assets from `web/legacy/index.html`. |
| `scripts/ci/install-web-wrapper-smoke.sh` | PASS | Web install wrapper smoke passes. |
| `scripts/ci/docs-consistency.sh` | PASS | Documentation consistency ok for release `7.1.1.0`. |
| `bash -n scripts/ci/frontend-static-guards.sh scripts/ci/frontend-serving-smoke.sh scripts/ci/self-test.sh scripts/ci/release-gate.sh` | PASS | Shell syntax passes. |

## 3. Not Run

| Check | Status | Reason |
| --- | --- | --- |
| Full `scripts/ci/self-test.sh` | SKIP | Component checks were run directly; full self-test also requires broader host/environment evidence and would reinstall frontend dependencies. |
| Full `scripts/ci/release-gate.sh` | SKIP | Release gate includes govulncheck and environment-dependent production gates. It should be run in the release environment. |
| PostgreSQL migration drill | SKIP | No disposable release DSN was provided. |
| Backup/restore drill | SKIP | No disposable source/restore DSNs were provided. |
| Live API smoke | SKIP | No deployed release base URL was provided. |
| VPN/service smoke matrix | SKIP | No live node/service matrix environment was provided. |
| Browser screenshot responsive evidence | SKIP | No Playwright/browser automation was added in this pass. |

## 4. Build Evidence

Current frontend build artifacts:

```text
web/index.html
web/.vite/manifest.json
web/assets/index-B-l2116U.js
web/assets/index-C9SCsY-r.css
web/legacy/index.html
web/legacy/assets/*
```

The Vite build script now runs `frontend/scripts/clean-build-output.mjs` before
`vite build`. It removes only root Vite artifacts and leaves `web/legacy/*`
untouched.

## 5. Static Serving Evidence

Backend tests cover:

- `GET /` returns new UI;
- `GET /clients` returns new UI;
- `GET /operations/jobs` returns new UI;
- `GET /network-policy/firewall` returns new UI;
- `GET /legacy/` returns legacy UI;
- `GET /api/v1/ready` is not shadowed;
- missing `/assets/*` returns 404;
- missing `/download/*` does not return SPA HTML.

## 6. Security Review Summary

Implemented/enforced in this pass:

- no raw `/api/v1` calls in page/feature components;
- no auth/session/token storage in browser storage in new frontend source;
- no unreviewed `dangerouslySetInnerHTML`;
- no production console logging in new frontend source;
- no fake local preview/apply enablement in Clients -> Groups or Firewall;
- Jobs cancel uses real backend endpoint and confirmation;
- Go SPA fallback excludes backend/public/static prefixes.

## 7. Write Workflow Summary

Fully connected in the new console:

- auth login/logout/invite/session;
- dashboard/readiness/version;
- main read paths for nodes, instances, clients, access groups, firewall,
  traffic, jobs, audit, settings/certificates subsets;
- traffic export URL;
- jobs list/detail/logs;
- jobs cancel.

Still disabled or legacy-only:

- client access group create/update/delete/scope/member preview/apply/sync;
- firewall CRUD and node preview/apply/disable;
- node bootstrap/control/terminal/diagnostics mutations;
- instance create/apply/rollback/lifecycle/delete;
- client create/status/delete/provision/revoke/artifacts/share/subscriptions;
- certificates import/issue/default/revoke/delete;
- settings save/mail test/TLS apply;
- backhaul mutations;
- backup/restore browser UI.

## 8. Known Limitations

- Full normal operator work still requires `/legacy/` for many write workflows.
- Version synchronization is incomplete: stable docs and backend version remain
  `7.1.1.0`; `8.0.0` is documented as frontend RC work.
- Endpoint DTOs are still broad for multiple domains.
- Mutation hooks and invalidation are complete only for Jobs cancel among new
  write workflows added in this pass.
- No browser-based responsive screenshot evidence was produced.

## 9. Go / No-Go

Recommendation:

- GO for RC1 serving/security groundwork and continued domain-by-domain frontend
  migration.
- NO-GO for declaring 8.0.0 final or removing operator reliance on `/legacy/`.

Blocking issues for final cutover:

1. migrate Clients -> Groups member preview/apply/sync with real backend calls;
2. migrate Firewall CRUD and node preview/apply/disable with safety UX;
3. migrate Nodes bootstrap/control/diagnostics workflows;
4. migrate Instances lifecycle/apply/rollback/delete workflows;
5. migrate Clients provisioning/artifacts/share/subscriptions workflows;
6. migrate Certificates and Platform settings write workflows;
7. add E2E or integration tests for critical operator flows;
8. synchronize backend/frontend version and release-chain artifacts to `8.0.0`;
9. run full release gate in the release environment.
