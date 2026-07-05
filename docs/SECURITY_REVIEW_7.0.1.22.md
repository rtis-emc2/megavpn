# Security and Release Review: 7.0.1.22

**Release:** `7.0.1.22`

## Scope

- Client lifecycle cleanup in Control Plane API and Web UI.
- Generated client config cleanup for artifacts, share links and VLESS
  subscription tokens.
- Build-config modal layout fix for large client/service selections.

## Changes

- Added `DELETE /api/v1/clients/{id}/configs` to remove generated client
  artifacts, share links, VLESS subscription tokens and managed artifact files
  without removing service access.
- Changed `DELETE /api/v1/clients/{id}` from soft status mutation to hard client
  deletion with transactional cleanup of service accesses, access routes, email
  delivery records, generated configs and `service_access` secret refs.
- Hard client deletion now queues instance apply jobs and route-policy apply jobs
  for affected instances/nodes after the database transaction commits.
- Added UI actions and confirmation modals for clearing configs and permanently
  deleting clients.
- Reworked the client config build modal to use responsive service-access cards
  instead of a constrained multi-select.
- Removed fixed desktop max-height caps from modals so large workflows use the
  available viewport height.
- Bumped release metadata and web asset cache keys to `7.0.1.22`.

## Security Assessment

- Client deletion removes service-access scoped secret refs, reducing retained
  credential material after account removal.
- Generated artifact files are deleted only when their paths remain under the
  configured artifact root; unmanaged paths are reported and skipped.
- Database cleanup commits before filesystem deletion, preventing a rollback from
  leaving DB rows pointing to removed files.
- Dangerous UI actions require an explicit confirmation flow. Full client delete
  requires typing the client username.
- Audit and job history remain intact for traceability.

## Verification Evidence

- `go test ./...`: passed.
- `node --check web/assets/clients-page.js` with bundled Node.js: passed.
- `MEGAVPN_RELEASE_RUN_RACE=0 MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`:
  passed locally with `10` passed and `7` skipped. Skips were race test
  override, PostgreSQL integration, backup/restore drill, systemd verify, nginx
  verify, API smoke and VPN service matrix because this workstation has no
  disposable DB, systemd/nginx target or live test node configured.

## Residual Risk

- If post-delete instance apply queueing fails for an affected instance, the API
  returns the deletion result with `queue_errors`; operators must re-apply the
  affected instance manually.
- Artifact root cleanup removes the client artifact directory after tracked file
  deletion; deployments that manually placed unrelated files under a client
  artifact directory should move those files before using cleanup.
