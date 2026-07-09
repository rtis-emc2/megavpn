# FE8 8.0.0 Overnight Batch Report

Branch: `release/8.0.0-frontend-console`

Batch input baseline commit: `24a0d129a0ca89caf3ec0e5f9c18616c0af9d95f`

Report start commit: `9ed3965fcdaa18554acf78680bc61317b9108564`

Final report commit: `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481`

Generated UTC: `2026-07-09T00:27:35Z`

Final 8.0.0 cutover status: **NO-GO**. The release branch is reviewable and
CI-verifiable, but final cutover still requires live/disposable API smoke,
release-gate execution, version synchronization and remaining parity decisions
listed in `docs/FE8_REMAINING_DEBT_8.0.0.md`.

## Completed Tasks

| Task | Commit | CI |
| --- | --- | --- |
| Task 0: FE8-P0-04B service pack evidence hygiene | `c4a7884d75dea52bb2ecd4e199f23b01e6b08ab8` | [28977495779](https://github.com/rtis-emc2/megavpn/actions/runs/28977495779) PASS |
| Task 1: FE8-P0-05A nodes observability/diagnostics/inventory | `5be5a33e16c7eef0578e122f919f9932ef5cbcf0` | [28979061764](https://github.com/rtis-emc2/megavpn/actions/runs/28979061764) PASS |
| Task 2: FE8-P0-05B nodes bootstrap/security/control | `7b564e81dd576fbf1de29c7da559090a69debe7a` | [28980167212](https://github.com/rtis-emc2/megavpn/actions/runs/28980167212) PASS |
| Task 3: FE8-P0-06A client routes/access rotation/config cleanup | `0934f97b9da38154b87dada4e1387d54ca7df765` | [28982369259](https://github.com/rtis-emc2/megavpn/actions/runs/28982369259) PASS |
| Task 4: FE8-P0-07A certificates/PKI | `b92c78679b60d46bc51f49f94db589ee6e1b0b09` | [28983219205](https://github.com/rtis-emc2/megavpn/actions/runs/28983219205) PASS |
| Task 5: FE8-P0-07B platform settings/mail/users/sessions | `f94b2bbf6efa1c4fe403ae98865bc5b4da19db70` | [28984118898](https://github.com/rtis-emc2/megavpn/actions/runs/28984118898) PASS |
| Task 6: FE8-P0-08A backhaul/route policy mutations | `9ed3965fcdaa18554acf78680bc61317b9108564` | [28985121588](https://github.com/rtis-emc2/megavpn/actions/runs/28985121588) PASS |

No task in this batch was left in a dirty or broken repository state.

## Connected Workflows

- `Clients -> Groups -> VLESS`: create/edit, members preview/apply/remove,
  scope update and sync preview/apply.
- `Firewall`: address groups, policies, rules, node preview, apply, node state
  and emergency disable.
- `Clients`: core list/detail/create/status/revoke/delete, single-client VLESS
  assignment and artifacts.
- `Clients -> Delivery`: share links, VLESS subscriptions and email delivery.
- `Instances` and `Services`: existing runtime control, service packs, manual
  create, spec replace and runtime artifact list/import.
- `Nodes`: observability, diagnostics, inventory, capabilities, service
  discovery, bootstrap/security/control for configured nodes, host-key scan/pin,
  SSH session ticket launch and retire/force-retire.
- `Clients -> Routes/Maintenance`: route list/create/delete, service access
  list/rotation/delete and generated config cleanup.
- `Platform -> Certificates`: list/detail, import preview/apply, self-signed,
  managed CA, issue-from-CA, default/revoke/delete and PKI roots.
- `Platform -> Settings`, `Mail / Delivery`, `Access / RBAC`: settings
  read/save/apply, mail settings/test, users list/detail, invite list/create
  and session list/revoke.
- `Infrastructure -> Backhaul`: existing link list/detail, apply, probe,
  transport promote and route projection enable/disable.
- `Network Policy -> Route Policy`: node-scoped list/detail, preview, apply and
  cleanup with fresh-preview gating.

## Checks Run

Each feature task was pushed and covered by its CI run above. The latest local
verification set for `9ed3965fcdaa18554acf78680bc61317b9108564` included:

| Check | Result |
| --- | --- |
| `gofmt -l cmd internal` | PASS |
| `go vet ./...` | PASS |
| `go test ./...` | PASS |
| `go test -race ./...` | PASS |
| `go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin` | PASS |
| `cd frontend && npm ci` | SKIP locally: workstation shell has no `npm` binary (`zsh: command not found: npm`). GitHub CI executed the npm path successfully. |
| `npm run typecheck` equivalent through bundled Node | PASS |
| `npm run lint` equivalent through bundled Node | PASS |
| `npm run test` equivalent through bundled Node | PASS, 10 files / 83 tests |
| `npm run i18n:check` equivalent through bundled Node | PASS, 868 keys |
| `npm run build` equivalent through bundled Node | PASS |
| `scripts/ci/frontend-serving-smoke.sh` | PASS |
| `scripts/ci/frontend-static-guards.sh` | PASS |
| `scripts/ci/docs-consistency.sh` | PASS |
| `git diff --check` | PASS |

## Disabled Or Backend-Missing Sub-Actions

- Non-VLESS access service materialization and access-group migration conflict
  UI remain outside the connected VLESS flow.
- Generic client edit is disabled because there is no generic
  `PATCH/PUT /api/v1/clients/{id}` endpoint.
- Client route update is disabled because there is no
  `PUT/PATCH /api/v1/clients/{id}/routes/{route_id}` endpoint.
- Per-access revoke is disabled because backend exposes client-level revoke and
  service-access delete, but no per-access revoke endpoint.
- Client delivery history is disabled because there is no client-scoped
  delivery history list/status endpoint.
- Runtime artifact delete is disabled because there is no binary runtime
  artifact DELETE endpoint.
- Service pack pre-validation, instance spec preview and instance draft-save are
  disabled because there are no separate backend endpoints for those sub-actions.
- Platform invite revoke is disabled because there is no browser invite revoke
  endpoint.
- Direct Platform user lifecycle mutations remain future scope: status change,
  reset-password, resend-invite and delete user.
- Nodes create/register/edit, new SSH access method creation with secret
  material, manual bootstrap bundle reveal, agent identity revoke, reboot,
  emergency cleanup, stale rotation cleanup and service discovery ignore/unignore
  remain future scope or legacy-only.
- Backhaul create/delete are not exposed in the new console after FE8-P0-08A.
- Backup/restore browser UI remains backend-missing/future scope.

## Smoke Skips

- Live/disposable API and DB smoke was not run in this workstation session
  because no disposable `MEGAVPN_PUBLIC_BASE_URL` / `MEGAVPN_API_URL`, target
  node inventory, endpoint domain or seeded database credentials were provided.
- The evidence is frontend/API-contract test coverage with mocked backend
  responses plus Go unit/integration tests and GitHub CI. It is not a substitute
  for staging operator validation.

## Go / No-Go

- GO for PR review and CI validation of the release branch.
- GO for controlled staging validation of connected workflows listed above.
- NO-GO for final 8.0.0 production cutover or removing `/legacy/`.
