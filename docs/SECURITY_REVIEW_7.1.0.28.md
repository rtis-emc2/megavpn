# Security and Release Review: 7.1.0.28

**Release:** `7.1.0.28`

## Scope

- Clarifies active Jobs UI diagnostics for node-agent handled jobs.
- `instance.apply` and other agent jobs no longer show a generic `n/a` result
  while queued/running/retrying.
- Expanded job rows now expose target node context and current lock/lease
  context when available.
- Web asset cache keys are bumped to avoid mixed frontend bundles after deploy.

## Security Notes

- No agent authentication, authorization, job-claim or result-submission
  semantics are changed.
- Job payload/result redaction remains server-side through the existing
  redaction helpers.
- The change reduces operator blind spots during runtime apply incidents:
  queued jobs can be distinguished from claimed jobs and control-plane worker
  jobs without reading PostgreSQL directly.
- The UI still treats the database/API as the source of truth; it does not
  synthesize success or failure states.

## Validation

- `node --check web/assets/job-workflows.js`
- `go test ./cmd/migrate ./internal/infra/postgres -count=1`
- `go test ./...`
- `scripts/docs-consistency.sh`
- `git diff --check`
- SQL prepared-statement guard over `cmd` and `internal` remained clean.

## Residual Risk

- A queued `instance.apply` still requires the target node agent to poll
  `/agent/jobs/next`.
- A running `instance.apply` still requires the target node agent to complete
  runtime apply and submit `/agent/jobs/{id}/result`.
- If the target host is unavailable, operators must use node diagnostics,
  `systemctl status megavpn-agent` and `journalctl -u megavpn-agent` on the
  node, or force-retire the node when it is permanently lost.
