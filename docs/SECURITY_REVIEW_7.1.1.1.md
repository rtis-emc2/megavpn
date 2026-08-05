# Security and Release Review: 7.1.1.1

**Release:** `7.1.1.1`

## Scope

- Reviews the managed external provider egress profile, deployment, job and
  selected-client routing paths.
- Reviews automatic UI refresh behavior around operator forms and modals.
- Reviews preview/dry-run purity, job-result ordering, runtime cleanup and
  fail-closed route health.
- Reviews encrypted secret references, imported configuration handling,
  managed artifact paths and agent-side command construction.
- Reviews reachable Go dependency vulnerabilities through the pinned release
  `govulncheck` gate.

## Findings Closed

1. Global auto-refresh now pauses while a modal is open or an editable control
   has focus.
2. Client access-group preview and dry-run resolve existing instances without
   persisting Xray catalog revisions.
3. External-egress deployment state updates require the completing job to match
   `last_job_id`, preventing stale completion races.
4. Route health rejects an unreachable-only table as a healthy provider path.
5. External-egress cleanup fails when policy-script execution, route flush or
   policy-rule verification fails; emergency node cleanup removes managed
   external-egress rules and tables even when the provider unit is already gone.
6. WireGuard provider imports reject multiple peers and ambiguous ownership.
7. Imported OpenVPN/WireGuard profiles require successful parsing and preview
   before activation.
8. Provider selection and normal egress controls are mutually exclusive in the
   client routing form.
9. Profile deletion and migration rollback remove obsolete deployment and
   secret-reference state.
10. `golang.org/x/text` is updated to `v0.39.0`, closing reachable
    `GO-2026-5970` reported through the PostgreSQL/pgx call path.

## Security Model

- External provider profiles are global policy objects; deployments are
  node-scoped runtime materializations.
- Only explicitly assigned client access groups receive the provider fwmark.
- Every deployment receives an isolated managed interface, routing table,
  fwmark and policy-rule priority.
- A high-metric unreachable default remains installed in the provider table so
  marked traffic does not escape through the main table when the provider
  tunnel is unavailable.
- Runtime artifacts use a deployment-specific directory and systemd unit. The
  agent validates identifiers, paths, modes, protocol support and node
  ownership before applying them.
- Secret values remain encrypted at rest and redacted from profile responses.

## Validation

- Unit and package tests for agent external-egress routing and cleanup helpers.
- Parser tests for OpenVPN and WireGuard provider imports.
- PostgreSQL integration coverage for bounded VLESS group materialization,
  stable UUIDs, duplicate prevention, preview purity and stale job results.
- Full Go test, race, vet, build, JavaScript syntax, shell syntax,
  documentation and release gates.

## Residual Risk

- Provider availability, MTU, DNS behavior and throughput depend on the third
  party and require live disposable-node evidence.
- Kernel policy routing and systemd validation require Linux and cannot be
  proven by macOS unit tests alone.
- PostgreSQL migration and backup/restore evidence requires configured
  disposable source and restore databases.
- Catalogued protocols other than OpenVPN and WireGuard are intentionally not
  runtime-ready and must stay unavailable for activation.
