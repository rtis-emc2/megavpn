# FE8 Evidence Doc Writing Rules

Branch: `release/8.0.0-frontend-console`

Status: active for all 8.0.0 release evidence and readiness documents.

## Rules

- Release evidence docs must be written as LF-only Markdown.
- Do not use CR-only line endings.
- Do not use CRLF when editing guarded evidence files.
- Do not use tools that collapse headings, paragraphs, bullets or tables into single long lines.
- Do not use markdown reflow tools for FE8 evidence docs unless the byte-level LF shape is validated afterward.
- Do not self-embed the final commit SHA in a document that is part of the same commit.
- Keep one heading per line.
- Keep one table row per line.
- Keep one bullet item per line.
- Keep guarded evidence lines at or below 240 characters.
- Write files with explicit LF bytes when recovering evidence docs.
- Run `scripts/ci/text-lf-guard.sh` before committing.
- Run `scripts/ci/docs-markdown-shape.sh` before committing FE8 acceptance changes.
- Run `scripts/ci/docs-consistency.sh` before pushing release evidence changes.
- Validate the committed object with `git show HEAD:<file>` when recovering line endings.
- Validate GitHub raw for the final pushed SHA when a task is about evidence formatting.
- Treat GitHub raw view as the final source of truth for multiline LF acceptance.

## Byte-Level Recovery Pattern

Use byte-level normalization when a guarded text file may contain CR bytes:

```python
from pathlib import Path

path = Path("docs/FRONTEND_ACCEPTANCE_8.0.0.md")
data = path.read_bytes().replace(b"\r\n", b"\n").replace(b"\r", b"\n")
text = data.decode("utf-8")
lines = text.split("\n")
if lines and lines[-1] == "":
    lines = lines[:-1]
path.write_bytes(("\n".join(lines) + "\n").encode("utf-8"))
```

## Guarded Files

The text LF guard protects the CI workflow, guard scripts and FE8 release evidence docs.
Any new FE8 release evidence document that affects readiness must be added to `scripts/ci/text-lf-guard.sh`.
