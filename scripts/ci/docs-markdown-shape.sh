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
]

failed = False

for path, cfg in checks.items():
    p = Path(path)
    if not p.exists():
        print(f"FAIL: missing {path}")
        failed = True
        continue

    text = p.read_text(encoding="utf-8")
    lines = text.splitlines()
    print(f"{path}: {len(lines)} lines")

    if len(lines) < cfg["min_lines"]:
        print(f"FAIL: {path} has too few lines; expected >= {cfg['min_lines']}")
        failed = True

    if lines[:3] != cfg["first3"]:
        print(f"FAIL: {path} first three lines wrong: {lines[:3]!r}")
        failed = True

    for i, line in enumerate(lines, 1):
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

print("docs-markdown-shape PASS")
PY
