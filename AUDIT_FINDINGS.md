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

### High: Runtime and Database Side Effects Ignored Failures

| Field | Detail |
| --- | --- |
| File/path | `cmd/agent`, `cmd/worker`, `internal/infra/postgres` |
| Reason | Runtime apply/delete paths, route-policy/netpolicy enforcement, client provisioning, backhaul state transitions and several DB-side lifecycle updates ignored errors after a job appeared successful. |
| Impact | Operators could see `queued`, `applied` or `ok` state while systemd reload, service enable, route-policy queueing, artifact generation, JSON decoding or DB cleanup actually failed. This directly affects instance deletion, emergency cleanup, client config propagation, VLESS routing and backhaul convergence. |
| Fix | Runtime/systemd reloads now fail closed and are included in job evidence; JSON DB fields use a shared fail-closed decoder; provisioning/status/route-policy/backhaul side effects return errors; bootstrap secret zeroization is checked; zip bundle writer close errors are preserved. |
| Validation | `go test ./cmd/agent ./cmd/worker ./internal/infra/postgres`; full validation matrix below. |

### Medium: Classified Ignored Error Backlog

| Field | Detail |
| --- | --- |
| File/path | `cmd`, `internal` |
| Reason | The remaining ignored errors are intentionally limited to best-effort audit/event inserts, response writes after headers are committed, cleanup of temporary files, websocket shutdown, telemetry/inventory submission retries and test cleanup. |
| Impact | These paths should not change product state-machine decisions. Failed audit/event writes remain observable through primary job/result errors where they are security or lifecycle relevant. |
| Fix | State-changing ignored errors in P0 scope were converted to explicit errors. Three direct `s.db.Exec` best-effort event inserts remain in discovery/capability event logging and are intentionally non-blocking. |
| Validation | `rg "_ = json\\.Unmarshal" cmd internal` returns no matches; targeted grep for ignored state-changing calls returns no matches; remaining `_ =` sites are cleanup/audit/telemetry classes. |

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
