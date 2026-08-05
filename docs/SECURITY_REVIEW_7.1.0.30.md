# Security and Release Review: 7.1.0.30

**Release:** `7.1.0.30`

## Scope

- Hardens managed Nginx camouflage edge rendering for VLESS/WebSocket and gRPC
  service packs.
- Camouflage edge profiles now default to `forward_client_ip=false`.
- Generated Nginx locations explicitly clear inbound identity forwarding
  headers before proxying traffic:
  `X-Real-IP`, `X-Forwarded-For`, `X-Forwarded-Host`,
  `X-Forwarded-Port` and `Forwarded`.
- Operators can still opt into classic reverse-proxy behavior with
  `forward_client_ip=true` for cases where preserving the client address is
  required.
- Fallback proxy locations now set conservative upstream timeouts and clear
  hop-by-hop `Connection` forwarding.

## Security Notes

- This release does not add a database-level uniqueness constraint for Xray or
  Nginx instances. Multiple Xray instances remain valid when their endpoint,
  path, host, purpose or rollout lifecycle is intentionally separate.
- The change reduces accidental disclosure of client/source proxy chains to
  third-party fallback websites.
- Existing authorization boundaries are unchanged; service-pack rollout and
  instance revision apply remain protected by the existing permissions.
- Secrets, client identities and generated client artifacts are not modified.

## Validation

- `go test ./internal/infra/postgres ./internal/api/http`
- `git diff --check`

## Residual Risk

- A third-party fallback website or its CDN may still append its own proxy hop
  to diagnostic pages. The control plane can prevent forwarding our incoming
  chain, but it cannot control headers added after traffic leaves the managed
  node.
- Full browser compatibility depends on the fallback website. Sites with strict
  CSP, absolute asset URLs, complex cookies, heavy client-side JavaScript or
  redirect-heavy flows may still be slow or visually incomplete when reverse
  proxied.
