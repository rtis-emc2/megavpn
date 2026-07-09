#!/usr/bin/env bash
set -euo pipefail

python3 - <<'PY'
from pathlib import Path

checks = {
    "docs/FE8_MORNING_AUDIT_8.0.0.md": {
        "min_lines": 120,
        "first3": [
            "# FE8 8.0.0 Morning Audit",
            "",
            "Branch: `release/8.0.0-frontend-console`",
        ],
    },
    "docs/FE8_REMAINING_DEBT_8.0.0.md": {
        "min_lines": 100,
        "first3": [
            "# FE8 Remaining Debt For 8.0.0",
            "",
            "Branch: `release/8.0.0-frontend-console`",
        ],
    },
    "docs/FRONTEND_ACCEPTANCE_8.0.0.md": {
        "min_lines": 200,
        "first3": [
            "# RTIS MegaVPN Frontend Acceptance 8.0.0",
            "",
            "Branch: `release/8.0.0-frontend-console`",
        ],
    },
}

forbidden_acceptance = [
    "git rev-parse HEAD",
    "pending final evidence",
    "plus the normalizing commit",
    "reported by",
    "pending at handoff",
    "<new final commit SHA after this task>",
]

failed = False

for path, cfg in checks.items():
    p = Path(path)
    if not p.exists():
        print(f"FAIL: missing {path}")
        failed = True
        continue

    data = p.read_bytes()

    if b"\r" in data:
        print(f"FAIL: {path} contains CR bytes; use LF-only line endings")
        failed = True

    if not data.endswith(b"\n"):
        print(f"FAIL: {path} does not end with LF newline")
        failed = True

    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError as exc:
        print(f"FAIL: {path} is not valid UTF-8: {exc}")
        failed = True
        continue

    raw_lines = text.split("\n")
    if raw_lines and raw_lines[-1] == "":
        raw_lines = raw_lines[:-1]

    print(f"{path}: {len(raw_lines)} LF lines")

    if len(raw_lines) < cfg["min_lines"]:
        print(f"FAIL: {path} has too few LF lines; expected >= {cfg['min_lines']}")
        failed = True

    if raw_lines[:3] != cfg["first3"]:
        print(f"FAIL: {path} first three LF lines wrong: {raw_lines[:3]!r}")
        failed = True

    for i, line in enumerate(raw_lines, 1):
        if len(line) > 240:
            print(f"FAIL: {path}:{i}: line too long ({len(line)} chars)")
            failed = True

    if path.endswith("FRONTEND_ACCEPTANCE_8.0.0.md"):
        for token in forbidden_acceptance:
            if token in text:
                print(f"FAIL: {path} contains forbidden placeholder: {token!r}")
                failed = True

if failed:
    raise SystemExit(1)

print("docs-markdown-shape LF-only PASS")
PY
