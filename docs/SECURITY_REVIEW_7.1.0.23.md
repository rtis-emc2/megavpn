# Security and Release Review: 7.1.0.23

**Release:** `7.1.0.23`

## Scope

This hotfix covers operator workflow stability and runtime hygiene:

- Node management SSH bootstrap access forms now mark the page as dirty on
  focus/input/change, which disables background auto-refresh until the operator
  saves, cancels or leaves the page.
- `nodeManageDirty` state is reset on explicit reload, successful SSH access
  save/remove and route changes away from Node management.
- Agent runtime report ingestion now ignores reports for deleted instances or
  instances that do not belong to the reporting node.
- Runtime observation pruning now removes `instance_runtime_observations` and
  `instance_runtime_states` for deleted or missing instances before retention
  and per-instance row-limit pruning.
- Web UI asset cache-key bump to `7.1.0.23`.

## Security Notes

- The SSH bootstrap secret textarea is no longer lost by passive background
  refresh while credentials are being entered. This reduces operator retry
  behavior and accidental secret handling mistakes.
- Runtime reports from stale agents cannot recreate runtime state for instances
  that the control plane has already deleted.
- Runtime cleanup only removes rows whose instance is missing or has
  `status='deleted'`; active instance observability history remains retained
  under the normal policy.

## Validation

- `go test ./internal/infra/postgres ./internal/api/http`
- `go test ./...`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/node-workflows.js`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/app-router.js`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/app-state.js`
- static multi-command SQL scan for production Go runtime paths
- `scripts/docs-consistency.sh`
- `git diff --check`

## Residual Risk

- Existing production rows for deleted/missing instances should be removed with
  the documented database cleanup command once after deployment. Future runtime
  report batches will also prune the same orphan rows.
