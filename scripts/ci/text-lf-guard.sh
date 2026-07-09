#!/usr/bin/env bash
set -euo pipefail

python3 - <<'PY'
from pathlib import Path

checks = {
    ".gitattributes": {
        "min_lines": 8,
        "first3": [
            "*.md text eol=lf",
            "*.sh text eol=lf",
            "*.ts text eol=lf",
        ],
    },
    ".github/workflows/ci.yml": {
        "min_lines": 80,
        "first3": [
            "name: CI",
            "",
            "on:",
        ],
    },
    "scripts/ci/docs-markdown-shape.sh": {
        "min_lines": 80,
        "first3": [
            "#!/usr/bin/env bash",
            "set -euo pipefail",
            "",
        ],
    },
    "scripts/ci/docs-consistency.sh": {
        "min_lines": 10,
        "first3": [
            "#!/usr/bin/env bash",
            "set -euo pipefail",
            "",
        ],
    },
    "scripts/ci/text-lf-guard.sh": {
        "min_lines": 80,
        "first3": [
            "#!/usr/bin/env bash",
            "set -euo pipefail",
            "",
        ],
    },
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

failed = False

for path, cfg in checks.items():
    p = Path(path)
    if not p.exists():
        print(f"FAIL: missing {path}")
        failed = True
        continue

    data = p.read_bytes()
    cr_count = data.count(b"\r")
    lf_count = data.count(b"\n")

    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError as exc:
        print(f"FAIL: {path} is not valid UTF-8: {exc}")
        failed = True
        continue

    lines = text.split("\n")
    if lines and lines[-1] == "":
        lines = lines[:-1]

    print(f"{path}: LF={lf_count} CR={cr_count} lines={len(lines)}")

    if cr_count != 0:
        print(f"FAIL: {path} contains CR bytes")
        failed = True

    if not data.endswith(b"\n"):
        print(f"FAIL: {path} does not end with LF")
        failed = True

    if len(lines) < cfg["min_lines"]:
        print(f"FAIL: {path} has {len(lines)} lines; expected >= {cfg['min_lines']}")
        failed = True

    if lines[:3] != cfg["first3"]:
        print(f"FAIL: {path} first three lines mismatch: {lines[:3]!r}")
        failed = True

if failed:
    raise SystemExit(1)

print("text-lf-guard PASS")
PY
