# Security and Release Review: 7.0.1.20

**Release:** `7.0.1.20`

## Scope

- Fix Nginx capability installation for camouflage ingress nodes when the
  preferred nginx.org repository path fails during apt update, repository setup,
  signing-key import or package install.
- Preserve `nginx_org_repo` as the preferred strategy while allowing a controlled
  fallback to Ubuntu's `nginx` package.
- Keep the operator-visible installer description aligned with the real
  fallback behavior.

## Changes

- Added `nginxOrgFallbackReason` in the agent capability installer.
- `nginx_org_repo` now falls back to `ubuntu_repo` for repository/package-stage
  failures, including `apt update failed before nginx install`.
- Root permission failures and runtime verification failures do not fall back,
  so real privilege or service-start problems remain visible.
- Added unit coverage for Nginx fallback classification.
- Updated service installer API description.
- Bumped release metadata and web asset cache keys to `7.0.1.20`.

## Security Assessment

- The fallback does not execute untrusted scripts or import alternate signing
  keys. It removes nginx.org apt source/pinning files and installs Ubuntu's
  distro package through apt.
- Fingerprint mismatch on nginx.org signing key is treated as an unsafe
  nginx.org repository path and falls back to Ubuntu repository instead of
  trusting the mismatched key.
- Privilege failures are not hidden by fallback. Operators still see a failed
  install when the agent lacks root privileges.
- Existing Nginx service verification remains required before the capability is
  reported healthy.

## Verification Evidence

- `go test ./cmd/agent ./internal/api/http`: passed.
- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...`: passed with no
  vulnerabilities found.
- Release docs consistency check for `7.0.1.20`: passed.
- `git diff --check`: passed.
- `MEGAVPN_RELEASE_RUN_RACE=0 MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`:
  passed locally with `10` passed and `7` skipped. Skips were race test
  override, PostgreSQL integration, backup/restore drill, systemd verify, nginx
  verify, API smoke and VPN service matrix because this workstation has no
  disposable DB, systemd/nginx target or test node configured.

## Residual Risk

- If the node's general apt configuration or network access is broken, both
  nginx.org and Ubuntu repository installs can still fail. In that case the job
  payload will include both the primary result and Ubuntu fallback steps.
- Operators can still explicitly select `ubuntu_repo` or `manual_present` when
  they want to bypass nginx.org repository usage entirely.
