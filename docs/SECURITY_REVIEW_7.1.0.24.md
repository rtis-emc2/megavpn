# Security and Release Review: 7.1.0.24

**Release:** `7.1.0.24`

## Scope

This hotfix closes the SSH bootstrap usability gap while preserving strict
host-key pinning:

- Added `POST /api/v1/nodes/{id}/ssh/host-key-scan`, protected by the
  `node.bootstrap` permission.
- Node management SSH access form now exposes `Scan host key` next to
  `ssh_host_key_sha256`.
- The scan uses the current SSH host and port from the form, prefers ED25519
  fingerprints when present and fills the required field for operator review.
- Empty `ssh_host_key_sha256` is rejected in the UI with a clear action message
  before the access-method save request is sent.
- Web UI asset cache-key bump to `7.1.0.24`.

## Security Notes

- `ssh_host_key_sha256` remains mandatory for SSH bootstrap and web terminal
  use. The new scan does not remove MITM protection.
- The operator must verify the returned fingerprint out-of-band before saving
  SSH access. Scanning a host key over the same network path is discovery, not
  trust establishment.
- The API endpoint does not execute a shell. It validates host/port input,
  invokes `ssh-keyscan` and `ssh-keygen` through argv, and applies a bounded
  context timeout.
- The endpoint requires `node.bootstrap`, the same privileged boundary used for
  changing node bootstrap access methods.
- Known-host output is filtered before fingerprint calculation; comments and
  malformed lines are ignored.

## Validation

- `go test ./internal/api/http`
- `/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node --check web/assets/node-workflows.js`
- Full release validation is expected to include `go test ./...`,
  `scripts/docs-consistency.sh`, static SQL multi-command scan and
  `git diff --check` before promotion.

## Residual Risk

- A privileged operator can request a host-key scan for an arbitrary syntactically
  valid host. This is acceptable for the `node.bootstrap` role because the same
  role can configure bootstrap access, but production environments should grant
  it only to trusted infrastructure operators.
- If the Control Plane host lacks `ssh-keyscan` or `ssh-keygen`, the scan action
  fails closed and manual out-of-band fingerprint entry is still required.
