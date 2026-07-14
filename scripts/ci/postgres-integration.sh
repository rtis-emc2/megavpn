#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

if [[ -z "${MEGAVPN_TEST_DATABASE_DSN:-}" ]]; then
  printf 'postgres integration failed: MEGAVPN_TEST_DATABASE_DSN is required for this gate\n' >&2
  exit 1
fi

tmp_files=()
cleanup() {
  if [[ "${#tmp_files[@]}" -gt 0 ]]; then
    rm -f "${tmp_files[@]}"
  fi
}
trap cleanup EXIT

printf 'PostgreSQL integration DSN is set (redacted).\n'

run_json_checked() {
  local package="$1"
  local pattern="$2"
  local label="$3"
  shift 3
  local -a required_tests=("$@")

  local log_file
  log_file="$(mktemp "${TMPDIR:-/tmp}/megavpn-${label}.XXXXXX.json")"
  tmp_files+=("$log_file")

  printf 'Running focused PostgreSQL integration: package=%s pattern=%s\n' "$package" "$pattern"
  go test -json -count=1 -timeout=5m "$package" -run "$pattern" | tee "$log_file"

  python3 - "$log_file" "$label" "${required_tests[@]}" <<'PY'
import json
import sys

path = sys.argv[1]
label = sys.argv[2]
required = sys.argv[3:]
state = {name: {"run": False, "pass": False, "skip": False, "fail": False} for name in required}
dsn_skip_outputs = []

with open(path, "r", encoding="utf-8") as fh:
    for line_no, line in enumerate(fh, 1):
        if not line.strip():
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError as exc:
            raise SystemExit(f"{label}: invalid go test -json line {line_no}: {exc}") from exc
        output = event.get("Output") or ""
        if "set MEGAVPN_TEST_DATABASE_DSN" in output:
            dsn_skip_outputs.append((line_no, event.get("Test") or "package"))
        test = event.get("Test")
        action = event.get("Action")
        if test not in state:
            continue
        if action == "run":
            state[test]["run"] = True
        elif action == "pass":
            state[test]["pass"] = True
        elif action == "skip":
            state[test]["skip"] = True
        elif action == "fail":
            state[test]["fail"] = True

missing = [name for name, item in state.items() if not item["run"]]
skipped = [name for name, item in state.items() if item["skip"]]
failed = [name for name, item in state.items() if item["fail"]]
not_passed = [name for name, item in state.items() if not item["pass"]]

errors = []
if dsn_skip_outputs:
    locations = ", ".join(f"{test}@line{line_no}" for line_no, test in dsn_skip_outputs)
    errors.append(f"DSN skip output detected: {locations}")
if missing:
    errors.append("missing required tests: " + ", ".join(missing))
if skipped:
    errors.append("required tests skipped: " + ", ".join(skipped))
if failed:
    errors.append("required tests failed: " + ", ".join(failed))
if not_passed:
    errors.append("required tests did not pass: " + ", ".join(not_passed))
if errors:
    raise SystemExit(label + " failed: " + "; ".join(errors))

print(label + ": required PostgreSQL tests passed without skips: " + ", ".join(required))
PY
}

printf 'Running full PostgreSQL infra integration suite.\n'
go test -count=1 -v -timeout=15m ./internal/infra/postgres

run_json_checked \
  ./internal/infra/postgres \
  'TestPostgresIntegrationCreateNodeSSHAccessMethod' \
  'postgres-ssh-infra' \
  TestPostgresIntegrationCreateNodeSSHAccessMethodAtomic \
  TestPostgresIntegrationCreateNodeSSHAccessMethodRejectsRetiredNode \
  TestPostgresIntegrationCreateNodeSSHAccessMethodConcurrentDuplicate

run_json_checked \
  ./internal/api/http \
  'TestPostgresIntegrationCreateNodeSSHAccessMethodHTTP' \
  'postgres-ssh-http' \
  TestPostgresIntegrationCreateNodeSSHAccessMethodHTTP
