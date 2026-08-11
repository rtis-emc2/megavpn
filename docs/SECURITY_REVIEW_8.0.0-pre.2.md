# Security and Release Review: 8.0.0-pre.2

**Release:** `8.0.0-pre.2`

## Decision

The repository is suitable for controlled pre-release testing. It must not be
promoted to stable `8.0.0` without disposable PostgreSQL, backup/restore and
real node/data-plane evidence. No known critical vulnerability is accepted in
the changed surface.

## Reviewed Surface

- Browser/API session and CSRF boundaries.
- Signed agent requests, response verification and transport configuration.
- Job lease/result ownership and agent inventory reporting.
- Bootstrap secret reveal and browser SSH terminal audit behavior.
- SMTP transport, generated MIME messages and public mail action URLs.
- PostgreSQL normalized relationships and retained-history constraints.
- External provider editor, protocol readiness filtering and compact-screen
  modal behavior.
- Client access group materialization and bounded apply-job fan-out across
  supported inbound protocols.
- Address-pool mutation authorization, validation and allocated-pool deletion
  protection.
- External-provider list pagination and deployment/profile ownership checks.
- Agent installed/target version reporting and update-workflow handoff.
- Operator invitation and client access email rendering on desktop/mobile.

## Added Controls

- Non-loopback agent traffic requires HTTPS unless a named compatibility
  override is explicitly enabled.
- Agent TLS supports a private CA and a paired client certificate/private key;
  TLS is limited to version 1.2 or newer.
- Login success, bootstrap secret reveal and terminal ticket/start abort when
  required audit persistence fails.
- Agent inventory submission errors are exposed in job evidence.
- SMTP requires TLS for authenticated delivery, rejects header injection,
  bounds recipients and attachment sizes, and applies network deadlines.
- Mail action URLs are restricted to absolute HTTPS URLs without embedded
  credentials; loopback HTTP is retained for local testing and public tokens
  are path escaped.
- The API test suite rejects newly introduced fully ignored store calls in HTTP
  handlers.
- External-provider forms expose only runtime-ready protocols and use stable,
  responsive layout primitives.
- Group membership changes use one durable provisioning job per group
  operation, preserve existing client credentials where applicable and reject
  disabled or unsupported service families before materialization.
- Address-pool writes remain permission-gated and reject destructive structural
  changes or deletion while active allocations reference the pool.
- External-provider paging inputs are bounded server-side; deployment actions
  remain scoped to the selected profile and node.

## Residual Risk

React Router `7.18.2` is affected by GHSA-qwww-vcr4-c8h2 only when its RSC mode
is used. MegaVPN is a client-only declarative `BrowserRouter` SPA and does not
expose RSC or server actions. `npm run audit:ci` permits only this exact
advisory/version combination and fails if RSC, server-router or data-router APIs
enter the source tree. Remove the exception immediately when an upstream stable
patched release is available.

| Risk | Severity | Required action |
| --- | --- | --- |
| Some persistence-layer audit writes are not in the same transaction as the state mutation | Medium | Introduce a transactional audit/outbox incrementally before stable promotion |
| The HTTP store interface and PostgreSQL store remain large | Medium maintainability | Extract one bounded use case per change; do not combine with release hot fixes |
| mTLS is optional rather than globally mandatory | Deployment dependent | Keep HMAC mandatory, use HTTPS, private CA and client certificates where the trust model requires mTLS |
| Provider and VPN behavior depends on kernel, packages, peer policy and network path | Operational | Complete live staged smoke and retain evidence |
| Email rendering varies across legacy mail clients | Low | Keep table-based layout, inline-compatible CSS and test representative production clients |

## Verification Policy

Local release evidence includes Go unit/race tests, `go vet`, all production
binary builds, frontend typecheck/lint/tests/build, shell/JavaScript syntax,
documentation consistency and security regression tests. PostgreSQL migration,
backup/restore, systemd, Nginx and data-plane gates fail closed when their
external environment or evidence is absent.

## Stable Promotion Rule

Stable `8.0.0` requires all gates in
[`docs/releases/8.0.0-pre.2.md`](releases/8.0.0-pre.2.md) and
[`docs/RELEASE_GATES.md`](RELEASE_GATES.md). A green repository-only pipeline
is necessary but insufficient for production promotion.
