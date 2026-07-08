# Security and Release Review: 7.0.1.14

**Release:** `7.0.1.14`

## Scope

- Security hardening for release toolchain, CI and release gates.
- Supply-chain hardening for NGINX.org repository bootstrap on remote nodes.
- Bootstrap env rendering hardening for remote node enrollment.
- Generic job API constraints for job type and initial status.
- Legacy release artifact cleanup.

## Changes Reviewed

- Go release baseline raised to patch-level enforcement; the maintained
  repository baseline is now `1.26.5`, and CI/release gate run
  `govulncheck@v1.5.0`.
- `scripts/control-plane-install.sh` now compares full Go semver including patch
  version, instead of accepting any matching major/minor toolchain.
- NGINX.org signing key import no longer uses a curl pipe. The agent downloads
  the key, checks the expected fingerprint, then imports the keyring.
- Bootstrap env files are rendered through a single writer that rejects invalid
  keys and control characters in values.
- Node profile validation rejects control characters in node name and address at
  both HTTP and store boundaries.
- Generic job creation is restricted to an explicit allowlist and forces new
  jobs to start as `queued`; unknown job types are rejected by `jobschema`.
- The old early-stage control-plane installer was removed and the API smoke
  script was renamed to `api-smoke.sh`.

## Security Assessment

- Toolchain risk reduced: known reachable stdlib vulnerabilities in Go `1.26.3`
  are blocked by the release baseline and by `govulncheck`.
- Supply-chain risk reduced: downloaded NGINX repository trust material is no
  longer trusted before fingerprint verification.
- Bootstrap integrity improved: attacker-controlled node fields can no longer
  inject additional env-file lines into manual or SSH bootstrap material.
- Job auditability improved: operators cannot create already-completed or
  arbitrary unknown jobs through the generic API.
- Legacy ambiguity reduced: stale early-stage release paths are no longer
  present in the active repository.

## Verification Evidence

- Focused tests for changed packages passed:
  `go test ./cmd/agent ./cmd/worker ./internal/api/http ./internal/jobschema ./internal/infra/postgres ./internal/backhaul`.
- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...`: passed with
  no vulnerabilities found.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`: passed locally with
  `11` passed and `6` skipped. Skips were PostgreSQL integration,
  backup/restore drill, systemd verify, nginx verify, API smoke and VPN service
  matrix because this workstation has no disposable DB, systemd/nginx target or
  test node configured.

## Residual Risk

- Runtime E2E still needs a disposable control plane and node to prove service
  matrix behavior, including VLESS ingress-to-egress routing.
- NGINX.org package installation should still be tested on supported Ubuntu
  targets because package availability and CPU ISA behavior are host-specific.
