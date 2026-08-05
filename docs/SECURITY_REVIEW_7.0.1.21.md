# Security and Release Review: 7.0.1.21

**Release:** `7.0.1.21`

## Scope

- Harden Nginx fallback installation when the node's global apt update is
  degraded but local package metadata still contains an install candidate.
- Avoid removing an existing Nginx package until the Ubuntu repository fallback
  has confirmed a safe candidate.
- Improve diagnostics for failed Nginx fallback installs.

## Changes

- `installNginxUbuntuRepo` now checks Ubuntu `nginx` package availability before
  purging any existing Nginx package.
- If `apt-get update` fails during Ubuntu fallback but `apt-cache policy nginx`
  still reports a candidate, the agent continues with package install and
  records an explicit diagnostic note.
- Nginx fallback install now reuses the same apt retry, lock handling and
  sandbox-user fallback logic as the generic Ubuntu package installer.
- Nginx fallback failures now include `last_failed_command` and
  `last_failed_exit_code` fields through the shared apt failure result path.
- Bumped release metadata and web asset cache keys to `7.0.1.21`.

## Security Assessment

- Existing Nginx is not purged until the agent verifies that Ubuntu's package
  candidate is available and does not resolve back to nginx.org metadata.
- The fallback still refuses to install when apt policy points at nginx.org after
  nginx.org sources are removed.
- The change does not alter rendered Nginx configs, VLESS routing, TLS material
  or node firewall behavior.
- Diagnostics include command lines and truncated output only; no secret values
  are introduced by this path.

## Verification Evidence

- `go test ./cmd/agent ./internal/api/http`: passed.
- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...`: passed with no
  vulnerabilities found.
- Release docs consistency check for `7.0.1.21`: passed.
- `git diff --check`: passed.
- `MEGAVPN_RELEASE_RUN_RACE=0 MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`:
  passed locally with `10` passed and `7` skipped. Skips were race test
  override, PostgreSQL integration, backup/restore drill, systemd verify, nginx
  verify, API smoke and VPN service matrix because this workstation has no
  disposable DB, systemd/nginx target or test node configured.

## Residual Risk

- If the node has no valid Ubuntu package candidate and global apt update is
  broken, automated installation must still fail. The job payload should then
  show the exact failed command and apt output.
- If the agent version reported in the job payload is below `7.0.1.21`, the node
  is still running older installer logic and must be updated before retrying.
