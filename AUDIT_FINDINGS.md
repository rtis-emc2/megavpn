# MegaVPN Audit Findings: Cleanup and Hardening Baseline

**Branch:** `audit/cleanup-hardening`  
**Date:** `2026-07-08`  
**Repository:** `github.com/rtis-emc2/megavpn`  
**Scope:** P0 build, CI, release gates, Go toolchain, ignored errors, security
regression coverage and documentation evidence.

## Inventory Evidence

| Check | Result |
| --- | --- |
| `git status --short` | clean before branch creation; dirty only with this audit changeset |
| `git ls-files` | 400 tracked files |
| `go version` / `go env GOVERSION` | `go1.26.5` after toolchain update |
| `go list ./...` | 23 Go packages |
| `gofmt -l ./cmd ./internal` | clean |
| `go vet ./...` | pass |
| `go test ./...` | pass |
| `go test -race ./...` | pass |
| `go run golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...` | pass, no vulnerabilities found |
| `scripts/build.sh` | pass; builds api, migrate, worker, agent, admin |
| `make build` | pass; now builds the same production/admin binaries |
| `bash -n` over `scripts`, `deploy`, `deploy-local.sh` | pass |
| Web UI `node --check` | pass using bundled Codex Node runtime |
| `scripts/docs-consistency.sh` | pass for release `7.1.0.30` |
| `scripts/self-test.sh` | pass locally: 17 passed, 0 failed, 7 skipped |
| `scripts/release-gate.sh` | pass in local diagnostic mode: 14 passed, 7 skipped |

Skipped gates require external release infrastructure: disposable PostgreSQL
databases, restore DSN, systemd host, nginx host config, live API URL and
disposable node/service-matrix inputs. These skips are not production release
evidence.

## Findings

### High: Incomplete `make build` Binary Coverage

| Field | Detail |
| --- | --- |
| File/path | `Makefile` |
| Reason | `make build` built only `api`, `agent` and `worker`; `scripts/build.sh` and release gates build `api`, `migrate`, `worker`, `agent`, `admin`. |
| Impact | Operators and CI could validate different binary sets, allowing `migrate` or `admin` build regressions to escape local build checks. |
| Fix | `make build` now builds all production/admin binaries and adds `run-migrate` / `run-admin`. |
| Validation | `make build`; `scripts/build.sh`; `scripts/self-test.sh` binary version gate. |

### High: CI Did Not Match Release-Gate Coverage

| Field | Detail |
| --- | --- |
| File/path | `.github/workflows/ci.yml` |
| Reason | CI missed `go test -race ./...`, full shell syntax coverage, Web UI JS syntax, docs consistency and self-test diagnostics. |
| Impact | Security-sensitive regressions could pass PR CI while failing release-gate or production rollout gates. |
| Fix | CI now has separate Go, race, shell/Web UI, self-test diagnostic and manual/scheduled release-gate jobs. |
| Validation | Local equivalents passed: `go test -race ./...`, full `bash -n`, bundled Node `--check`, `scripts/docs-consistency.sh`, `scripts/self-test.sh`. |

### Medium: Go Toolchain Below Requested Security Baseline

| Field | Detail |
| --- | --- |
| File/path | `go.mod` |
| Reason | Repository used `go 1.26.4`; audit prompt requested `1.26.5` due security fixes. |
| Impact | Builds could miss standard library fixes in `crypto/tls`, `os` or other runtime packages. |
| Fix | `go.mod` now uses `go 1.26.5`; `GOTOOLCHAIN=auto` downloaded and used `go1.26.5` successfully. |
| Validation | `go env GOVERSION`; `go test ./...`; `go test -race ./...`; `make build`; `scripts/build.sh`. |

### Medium: Startup Seed Error Was Silently Ignored

| Field | Detail |
| --- | --- |
| File/path | `cmd/api/main.go` |
| Reason | `store.SeedLocalInventory` failure was ignored during API startup. |
| Impact | In production, schema/config/DB write problems could be hidden behind a running API process with incomplete local inventory state. |
| Fix | API now logs seed failure and fails fast in `MEGAVPN_PRODUCTION_MODE=true`; non-production keeps running with an explicit warning. |
| Validation | `go test ./...`; `go test -race ./...`; `go vet ./...`. |

### Medium: Remaining Ignored Error Backlog

| Field | Detail |
| --- | --- |
| File/path | `cmd`, `internal` |
| Reason | `rg "_\\s*=\\s*[^=]" cmd internal --glob '*.go'` still reports 450 ignored-error sites after the startup seed fix. Many are expected best-effort audit/log/write-close/cleanup paths, but they are not yet classified. |
| Impact | A subset may hide failed audit writes, failed state cleanup, failed runtime rollback or failed agent telemetry submission. |
| Fix | This changeset fixes the highest-signal startup seed case. Remaining sites need a scoped classification pass by subsystem: worker runtime apply, agent cleanup/install, API audit/logging, store cascade cleanup, cryptographic hash writes and HTTP response writes. |
| Validation | Repeat `rg "_\\s*=\\s*[^=]" cmd internal --glob '*.go'` and require each subsystem PR to reduce or justify its bucket. |

### Medium: Camouflage Header Leakage Regression Coverage Needed gRPC Parity

| Field | Detail |
| --- | --- |
| File/path | `internal/infra/postgres/instance_drivers_test.go` |
| Reason | WebSocket camouflage had fallback identity header tests, but gRPC camouflage did not explicitly assert the same privacy boundary. |
| Impact | A future Nginx gRPC edge change could reintroduce `X-Forwarded-For` / `X-Real-IP` leakage to fallback upstreams. |
| Fix | Added gRPC camouflage regression coverage for clearing `X-Real-IP`, `X-Forwarded-For` and `Forwarded` on primary and fallback locations. |
| Validation | `go test ./internal/infra/postgres`; `go test ./...`; `go test -race ./...`. |

### Low: Secret Scan Needs False-Positive-Tuned Automation

| Field | Detail |
| --- | --- |
| File/path | repository-wide |
| Reason | Broad keyword scan for `token`, `password`, `secret`, `DSN`, `bearer` is intentionally noisy because these are first-class product concepts. Focused marker scan found only tests/placeholders/code literals, not committed production private-key material. |
| Impact | Without a tuned scanner, real accidental secrets could be hidden in large false-positive output. |
| Fix | Add a dedicated secret scanning gate with allowlisted test fixtures and generated-placeholder patterns. |
| Validation | Current focused scan: `rg -n "BEGIN (RSA \|EC \|OPENSSH \|DSA \|)PRIVATE KEY\|AKIA[0-9A-Z]{16}\|ghp_[A-Za-z0-9_]{36}\|xox[baprs]-\|-----BEGIN" --glob '!tmp/**' --glob '!bin/**' .`. |

## Deferred P1/P2 Backlog

- Move one bounded use-case at a time from `internal/api/http` to `internal/app/*`.
- Split scripts into `scripts/ci`, `scripts/smoke`, `scripts/ops`, `scripts/lib`
  only after `rg`/CI reference checks.
- Move long release notes out of README into `CHANGELOG.md` or
  `docs/releases/<version>.md`.
- Pin GitHub Actions by commit SHA for stricter supply-chain mode.
- Add negative tests for path traversal, symlink escape, replay signatures,
  stale leases, forged job results, invalid RBAC job type, revoked public tokens,
  expired share tokens, changed SSH host keys, Nginx directive injection and
  fallback header leakage across all profiles.
- Add disposable PostgreSQL migration/backup/restore CI job when DSNs are
  available.
