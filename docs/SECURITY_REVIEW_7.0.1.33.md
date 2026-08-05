# Security and Release Review: 7.0.1.33

**Release:** `7.0.1.33`

## Scope

- Nginx HTTP-to-HTTPS redirect generation for TLS edge profiles.
- Shared Nginx runtime cleanup after instance deletion and node emergency
  cleanup.
- Service-pack component selection layout hardening.

## Changes

- Generated TLS-enabled Nginx configs now render a dedicated `listen 80`
  redirect server block before the HTTPS edge server block.
- Non-standard HTTPS ports are preserved in redirect targets.
- Nginx instance deletion and node emergency cleanup now run a shared Nginx
  finalizer after removing MegaVPN-managed conf files.
- The finalizer validates and reloads Nginx when other
  `/etc/nginx/conf.d/megavpn-*.conf` files remain, and stops the shared
  `nginx` unit when no MegaVPN-managed Nginx configs remain.
- Service-pack component cards no longer render numbered circles; the selectable
  row is aligned as checkbox plus component content.

## Security Assessment

- The redirect server block is generated only from already validated
  `server_name` and numeric port inputs. It does not accept raw directive text.
- The HTTP listener redirects to HTTPS before fallback-site routing is evaluated,
  reducing accidental plain-HTTP exposure for camouflage endpoints.
- The cleanup change does not add `nginx.service` to the generic managed unit
  allowlist. Nginx is handled by a dedicated shared-runtime finalizer after
  MegaVPN-owned config removal.
- The finalizer only uses the fixed `/etc/nginx/conf.d/megavpn-*.conf` glob to
  decide whether to reload or stop shared Nginx runtime. It does not delete
  non-MegaVPN Nginx files.

## Verification Evidence

- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh` passed
  (`passed=12 skipped=6`).
- `govulncheck ./...` completed with no vulnerabilities found.
- `go test ./...` passed.
- `go test -race ./...` passed.
- `go vet ./...` passed during pre-release verification.
- `scripts/docs-consistency.sh` passed for release `7.0.1.33`.
- `node --check web/assets/instances-page.js` and
  `node --check web/assets/node-workflows.js` passed with the bundled Node.js
  runtime during pre-release verification.
- `git diff --check` passed during pre-release verification.

## Residual Risk

- Live verification still requires a real Nginx host to confirm `nginx -t`,
  reload/stop behavior and public HTTP-to-HTTPS behavior.
- On a host where non-MegaVPN sites share the same Nginx service, stopping Nginx
  after the last MegaVPN config is removed can affect those sites. Production
  nodes should remain dedicated to MegaVPN-managed edge workloads, or operators
  should keep a separate Nginx service boundary for unrelated sites.
