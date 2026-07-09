# FE8 8.0.0 Morning Audit

Branch: `release/8.0.0-frontend-console`

Audited functional batch HEAD SHA: `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481`

Morning audit commit: `e1677b35f3682d5fbff6a417178cfd15cbabb0b3`

Failed markdown normalization attempts:

- `b2a9b99c7e47babe26a0ef9e2fca8779fffeb715`
- `9ab4dbfc38a88fcc08ce62bbffb6989f4676cbbc`
- `3487c140f594a3db2bbd2dcd564031e312816900`
- `ceba0a422d35b9c32e3786e858f864855176683d`

Generated UTC: `2026-07-09T08:20:00Z`

Final recommendation:

- GO for PR review.
- GO for controlled staging validation.
- NO-GO for final production cutover.

CI status for latest failed raw GitHub validation attempt:

- GitHub Actions run `29003755984` passed for `ceba0a422d35b9c32e3786e858f864855176683d`.
- That run is insufficient for this issue because CI did not yet include `scripts/ci/docs-markdown-shape.sh`.

## Repository State

| Check | Result |
| --- | --- |
| Branch | `release/8.0.0-frontend-console` |
| Functional batch evidence | `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481` |
| Morning audit commit | `e1677b35f3682d5fbff6a417178cfd15cbabb0b3` |
| Working tree before audit | clean |
| `/legacy/` rollback path | present at `web/legacy` |
| Package manager | npm-only |
| `frontend/package-lock.json` | present |
| `frontend/pnpm-lock.yaml` | absent |

## Overnight Commits

| Task | Commit | CI | Status |
| --- | --- | --- | --- |
| FE8-P0-04B service pack evidence hygiene | `c4a7884d75dea52bb2ecd4e199f23b01e6b08ab8` | `28977495779` PASS | DONE |
| FE8-P0-05A nodes diagnostics inventory | `5be5a33e16c7eef0578e122f919f9932ef5cbcf0` | `28979061764` PASS | DONE |
| FE8-P0-05B nodes bootstrap security | `7b564e81dd576fbf1de29c7da559090a69debe7a` | `28980167212` PASS | DONE |
| FE8-P0-06A client maintenance | `0934f97b9da38154b87dada4e1387d54ca7df765` | `28982369259` PASS | DONE |
| FE8-P0-07A certificates/PKI | `b92c78679b60d46bc51f49f94db589ee6e1b0b09` | `28983219205` PASS | DONE |
| FE8-P0-07B platform settings/access | `f94b2bbf6efa1c4fe403ae98865bc5b4da19db70` | `28984118898` PASS | DONE |
| FE8-P0-08A backhaul/route policy | `9ed3965fcdaa18554acf78680bc61317b9108564` | `28985121588` PASS | DONE |
| Final overnight report/debt docs | `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481` | `28985369032` PASS | DONE |
| Morning audit docs | `e1677b35f3682d5fbff6a417178cfd15cbabb0b3` | `28994536686` PASS | DONE |
| Failed markdown normalization attempt | `b2a9b99c7e47babe26a0ef9e2fca8779fffeb715` | `28998178046` PASS | INSUFFICIENT |
| Failed markdown normalization attempt | `9ab4dbfc38a88fcc08ce62bbffb6989f4676cbbc` | `28999052460` PASS | INSUFFICIENT |
| Failed raw GitHub validation attempt | `3487c140f594a3db2bbd2dcd564031e312816900` | `29001994538` FAILURE | INSUFFICIENT |

| Failed raw GitHub validation attempt | `ceba0a422d35b9c32e3786e858f864855176683d` | `29003755984` PASS but guard absent | INSUFFICIENT |
## Completed Tasks

- FE8-P0-04B service pack evidence hygiene.
- FE8-P0-05A nodes observability, diagnostics and inventory.
- FE8-P0-05B nodes bootstrap, security and control.
- FE8-P0-06A client routes, access rotation and config cleanup.
- FE8-P0-07A certificates and PKI.
- FE8-P0-07B platform settings, mail, users and sessions.
- FE8-P0-08A backhaul and route policy mutations.
- Final overnight report and debt docs.
- Morning audit report.

## Partial Tasks

- No overnight task was left partially implemented.
- The release remains partial only at the full 8.0.0 cutover level.
- Backend-missing and future-scope sub-actions remain documented debt.

## Blocked Tasks

- Final 8.0.0 production cutover is blocked by version sync.
- Final 8.0.0 production cutover is blocked by live disposable smoke.
- Final 8.0.0 production cutover is blocked by staging operator validation.
- Final 8.0.0 production cutover is blocked by the final release gate.
- Final 8.0.0 production cutover is blocked by responsive evidence.
- Final 8.0.0 production cutover is blocked by i18n wording review.
- Final cutover also needs release-owner disposition of remaining debt.

## Disabled Backend-Missing Sub-Actions

- Generic client edit: no generic `PATCH/PUT /api/v1/clients/{id}` endpoint.
- Client route update: no `PUT/PATCH /api/v1/clients/{id}/routes/{route_id}` endpoint.
- Per-access revoke: no exact per-access revoke endpoint.
- Client delivery history: no client-scoped delivery history endpoint.
- Runtime artifact delete: no binary runtime artifact DELETE endpoint.
- Service pack validation: no separate validation endpoint.
- Instance spec preview/draft-save: no separate backend endpoints.
- Platform invite revoke: no browser invite revoke endpoint.
- Backhaul dedicated repair: no dedicated repair endpoint.
- Backup/restore browser UI: browser parity remains backend-missing.

## Future-Scope Sub-Actions

- Non-VLESS access service materialization.
- Access-group migration conflict UI.
- Nodes create, register and edit.
- New SSH access method creation with secret material.
- Manual bootstrap bundle reveal.
- Agent identity revoke.
- Node reboot.
- Node emergency cleanup.
- Node stale rotation cleanup.
- Node service discovery ignore and unignore.
- Platform user status, reset, resend and delete.
- Backhaul link create and delete.

## Functional Matrix Spot-Check

| Domain | Status |
| --- | --- |
| VLESS Clients -> Groups | verified |
| Firewall | verified; rule reorder backend-missing |
| Clients core/artifacts/delivery/maintenance | verified; selected sub-actions backend-missing |
| Instances runtime control | verified |
| Service packs/runtime artifacts | verified; runtime artifact delete backend-missing |
| Nodes observability/security/control | verified; destructive remediation remains disabled |
| Certificates/PKI | verified |
| Platform settings/mail/access | verified; invite revoke backend-missing |
| Backhaul | verified; create/delete future scope |
| Route Policy | verified |

## Functional Matrix Evidence Paths

- VLESS Clients -> Groups:
  - UI: `frontend/src/pages/clients/ClientGroupsPage.tsx`.
  - Tests: `frontend/src/pages/clients/ClientGroupsPage.test.tsx`.
- Firewall:
  - UI: `frontend/src/pages/network-policy/FirewallPage.tsx`.
  - Tests: `frontend/src/pages/network-policy/FirewallPage.test.tsx`.
- Clients:
  - UI: `frontend/src/pages/clients/ClientsPage.tsx`.
  - Tests: `frontend/src/pages/clients/ClientsPage.test.tsx`.
- Instances:
  - UI: `frontend/src/pages/services/InstancesPage.tsx`.
  - Tests: `frontend/src/pages/services/InstancesPage.test.tsx`.
- Service packs/runtime artifacts:
  - UI: `frontend/src/pages/services/ServicePacksPage.tsx`.
  - UI: `frontend/src/pages/services/RuntimeArtifactsPage.tsx`.
  - Tests: `frontend/src/pages/services/ServiceWorkspace.test.tsx`.
- Nodes:
  - UI: `frontend/src/pages/infrastructure/NodesPage.tsx`.
  - Tests: `frontend/src/pages/infrastructure/NodesPage.test.tsx`.
- Certificates/PKI:
  - UI: `frontend/src/pages/platform/CertificatesPage.tsx`.
  - Tests: `frontend/src/pages/platform/CertificatesPage.test.tsx`.
- Platform settings/mail/access:
  - UI: `frontend/src/pages/platform/SettingsPage.tsx`.
  - UI: `frontend/src/pages/platform/MailPage.tsx`.
  - UI: `frontend/src/pages/platform/AccessPage.tsx`.
  - Tests: `frontend/src/pages/platform/PlatformSettingsAccess.test.tsx`.
- Backhaul:
  - UI: `frontend/src/pages/infrastructure/BackhaulPage.tsx`.
  - Tests: `frontend/src/pages/network-policy/BackhaulRoutePolicyPage.test.tsx`.
- Route Policy:
  - UI: `frontend/src/pages/network-policy/RoutePolicyPage.tsx`.
  - Tests: `frontend/src/pages/network-policy/BackhaulRoutePolicyPage.test.tsx`.
- Shared API paths:
  - `frontend/src/shared/api/endpoints.ts`.
  - `frontend/src/shared/query/hooks.ts`.

## Checks Run

| Check | Result |
| --- | --- |
| `git status --short` | PASS, clean before audit |
| `git branch --show-current` | PASS |
| `git rev-parse HEAD` | PASS |
| `gh run view 28999052460 --json ...` | PASS, conclusion `success` |
| `gofmt -l cmd internal` | PASS from morning audit evidence |
| `go vet ./...` | PASS from morning audit evidence |
| `go test ./...` | PASS from morning audit evidence |
| `go test -race ./...` | PASS from morning audit evidence |
| `go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin` | PASS |
| `cd frontend && npm ci` | SKIP locally; no `npm` binary |
| bundled Node typecheck equivalent | PASS from morning audit evidence |
| bundled Node lint equivalent | PASS from morning audit evidence |
| bundled Node Vitest | PASS, 10 files / 83 tests |
| bundled Node i18n check | PASS, 868 keys |
| bundled Node build | PASS from morning audit evidence |
| `scripts/ci/frontend-serving-smoke.sh` | PASS from morning audit evidence |
| `scripts/ci/frontend-static-guards.sh` | PASS from morning audit evidence |
| `scripts/ci/docs-consistency.sh` | PASS |

## Checks Skipped

- Local `cd frontend && npm ci` was skipped because `npm` is absent.
- Exact local failure: `zsh:1: command not found: npm`.
- GitHub CI remains authoritative for the npm-path CI.
- Live disposable API/DB smoke was not run.
- No disposable API URL, DB seed, node inventory or endpoint domain was provided.

## Static Guard Results

- `scripts/ci/frontend-static-guards.sh`: PASS.
- Raw API spot-check: PASS.
- No production page component contains raw `fetch(/api/v1...)`.
- No production page component contains `dangerouslySetInnerHTML`.
- API paths remain centralized in shared API modules.
- `/legacy/` exists as rollback navigation and filesystem content.

## Secret Storage And Logging Review

- Production `localStorage` use is limited to API base and UI locale.
- Tests assert one-time enrollment/share tokens are not stored.
- No production console logging was found in `frontend/src`.
- No unreviewed `dangerouslySetInnerHTML` sink was found in production pages.

## Documentation Consistency Result

- Acceptance evidence was updated to separate functional and documentation evidence.
- Night report records final report commit `4d3f571cec7d9f8c9e3adb8bc7b74ecc5a6d1481`.
- Remaining debt is split into release-blocking, backend-missing and future-scope sections.
- No document now claims final 8.0.0 cutover readiness.

## Workflow Matrix Consistency Result

- `docs/FRONTEND_WRITE_WORKFLOWS_8.0.0.md` remains the workflow source.
- `docs/FRONTEND_ENDPOINT_PARITY_8.0.0.md` remains the endpoint source.
- `docs/releases/8.0.0.md` remains consistent with NO-GO status.
- Fully connected rows have typed API wrappers, hooks, UI entry points and tests.
- Missing exact sub-actions remain backend-missing, legacy-only or future scope.
- No connected workflow was converted into fake UI success.

## Remaining Debt Summary

- Version sync from `7.1.1.0` to `8.0.0`.
- Live disposable API/DB smoke.
- Full final release gate on a version-synchronized SHA.
- GitHub Actions Node.js 20 deprecation review.
- Desktop, pad and phone responsive evidence.
- English/Russian operator wording review beyond key parity.
- Final static/raw API guard review before release tag.
- Acceptance matrix cleanup after final live checks.
- PR summary, release notes diff, CI evidence and rollback notes.
- Release-owner decision for backend-missing and future-scope sub-actions.

## Final Decision

Final 8.0.0 cutover is NO-GO because version metadata still reports
`7.1.1.0`, live disposable API/DB smoke and staging operator validation are
not complete, the full final release gate has not been run on a
version-synchronized final SHA, responsive evidence and i18n wording review
are still open, and multiple backend-missing or future-scope sub-actions remain
explicitly outside the 8.0.0 frontend cutover.
