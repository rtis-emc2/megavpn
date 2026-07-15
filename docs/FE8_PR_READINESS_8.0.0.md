# FE8 8.0.0 PR Readiness Package

Branch: `release/8.0.0-frontend-console`

Generated UTC: `2026-07-15T05:31:52Z`

Status: **READY FOR PR REVIEW, NOT READY FOR FINAL CUTOVER**.

Current accepted evidence HEAD inspected:
`3abc200c3d7c5525eaded994244af488d0728b41`.

Latest accepted CI:
GitHub Actions run `29391281058` PASS for
`3abc200c3d7c5525eaded994244af488d0728b41`.

## Ready For PR Review

- The branch is pushed and CI-covered for `release/8.0.0-frontend-console` and
  `release/**`.
- `/legacy/` rollback remains present and documented.
- Package manager policy is npm-only:
  `frontend/package-lock.json` exists and `frontend/pnpm-lock.yaml` is absent.
- VLESS, Firewall, Clients, Instances, Service Packs, Nodes, Certificates/PKI,
  Platform Settings/Mail/Access, Backhaul and Route Policy migrated workflows
  are documented as connected where backend endpoints exist.
- Secure SSH access-method creation is connected where implemented, with
  PostgreSQL-backed integration evidence for the atomic secret/access-method
  path.
- Manual bootstrap bundle reveal/download is connected through the new React
  console using primary POST reveal/download endpoints only. Sensitive bundle
  handling is transient and explicitly confirmed, and PostgreSQL-backed plus
  real HTTP/router integration evidence exists with required bundle tests
  executed without skips.
- `/legacy/` remains rollback for other incomplete workflows.
- Missing exact sub-actions remain disabled or documented with backend-missing
  reasons.
- Static guards, docs guards, Go checks and frontend checks pass locally, with
  clean npm install covered by GitHub CI because this workstation has no npm.
- GitHub Actions pins were updated to upstream node24 actions while preserving
  commit-SHA pinning.
- `.gitattributes`, CI workflow, documentation guards and evidence docs are
  LF-only multiline files in GitHub raw view; `scripts/ci/text-lf-guard.sh`
  is wired into CI and `scripts/ci/docs-consistency.sh`.

## Ready For Staging Validation

- Operator workflow review can start on a disposable or staging environment.
- Live smoke must use disposable data and must not mutate production nodes,
  certificates, route policy or firewall state.
- `/legacy/` should stay enabled during staging validation as rollback.

## Final Cutover Blockers

- Backend binary/version metadata still reports `7.1.1.0`.
- Version tag and release metadata are not synchronized to final `8.0.0`.
- Full production release gate has not passed without skips.
- Live disposable API/DB/node smoke has not run.
- Live node/bootstrap and agent onboarding validation has not run.
- Disposable PostgreSQL integration evidence exists for the tested backend
  suites, including SSH access-method creation and manual bootstrap bundle
  reveal/download.
- Backup/restore evidence remains missing.
- Full live disposable API/node/service smoke remains missing.
- Responsive desktop/tablet/phone workflow screenshots are missing.
- Human English/Russian i18n wording review remains open.
- Backend-missing and future-scope sub-actions remain documented.

## Suggested PR Title

```text
MegaVPN Console 8.0.0 release frontend migration evidence
```

## Suggested PR Body

```markdown
## Summary

- Migrates accepted 8.0.0 frontend workflows to the new React console where
  backend endpoints exist.
- Keeps `/legacy/` as rollback.
- Updates CI coverage, frontend/static guards, evidence docs and release debt.
- Updates pinned GitHub Actions to node24 runtime major versions while keeping
  commit SHA pinning.

## Review Notes

- Final 8.0.0 cutover is still NO-GO.
- Backend version metadata remains `7.1.1.0` until the separate version sync.
- Live disposable smoke, full release gate without skips, responsive evidence
  and human i18n wording review are still required.
- Backend-missing sub-actions are documented in
  `docs/FE8_REMAINING_DEBT_8.0.0.md`.

## Rollback

- `/legacy/` remains available and must not be removed before final cutover
  approval.
```
