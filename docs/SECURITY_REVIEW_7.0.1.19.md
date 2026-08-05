# Security and Release Review: 7.0.1.19

**Release:** `7.0.1.19`

## Scope

- Remove duplicate service-pack templates from `Instances -> Create from pack`.
- Add backend semantic deduplication so API consumers receive one effective
  active template even when historical rows have different keys but identical
  pack semantics.
- Add a database repair migration that archives duplicate default templates on
  existing installations.

## Changes

- Added `migrations/000010_service_pack_semantic_dedup.up.sql`.
- Added service-pack semantic fingerprinting in the PostgreSQL repository list
  path.
- Added global frontend service-pack normalization in `core-loader.js`.
- Added page-level protection in `instances-page.js` so stale/cached catalog
  payloads cannot render duplicate cards.
- Bumped release metadata and web asset cache keys to `7.0.1.19`.

## Security Assessment

- The migration only archives duplicate `source='default'` active templates by
  marking the later duplicate as `deleted`; it does not modify instances,
  revisions, secrets or node runtime state.
- Custom operator templates remain selectable unless they are semantic clones of
  another active template, in which case the runtime list chooses the operator
  clone over the default clone.
- Create/apply authorization is unchanged; service-pack catalog management still
  requires `settings.manage`.
- No new secret material is stored and no node-side behavior changes.

## Verification Evidence

- `go test ./internal/infra/postgres ./internal/api/http`: passed.
- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...`: passed with no
  vulnerabilities found.
- Bundled Node.js syntax checks for `web/assets/core-loader.js` and
  `web/assets/instances-page.js`: passed.
- Migration sequence check through `000010`: passed.
- Release docs consistency check for `7.0.1.19`: passed.
- `git diff --check`: passed.
- `MEGAVPN_RELEASE_RUN_RACE=0 MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`:
  passed locally with `10` passed and `7` skipped. Skips were race test
  override, PostgreSQL integration, backup/restore drill, systemd verify, nginx
  verify, API smoke and VPN service matrix because this workstation has no
  disposable DB, systemd/nginx target or test node configured.

## Residual Risk

- Existing browsers must load the new `7.0.1.19` asset URLs; if a reverse proxy
  pins old HTML, the operator may still see stale JS until the web root is
  updated and cache is refreshed.
- The migration intentionally archives duplicate default templates only. If an
  operator intentionally created a custom clone, runtime lists hide the duplicate
  but the underlying custom row is not destroyed.
