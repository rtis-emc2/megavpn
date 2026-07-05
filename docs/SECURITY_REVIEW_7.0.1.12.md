# Security and Release Review: 7.0.1.12

**Release:** `7.0.1.12`

## Scope

- Traffic-camouflage fallback loop guard for Xray/VLESS + Nginx ingress packs.
- Web UI and service-pack smoke preflight for fallback websites that point back
  to the public ingress endpoint.
- Documentation update for the fallback URL/Host/SNI trust boundary.
- Release baseline and web asset cache-busting were updated to `7.0.1.12`.
- No database migration, RBAC model, authentication flow, secret persistence,
  VLESS route-policy rendering or firewall enforcement behavior was changed in
  this release.

## Changes Reviewed

- Service-pack API normalization rejects `fallback_upstream_url`,
  `fallback_host_header` and `fallback_sni` values that match the requested
  public `endpoint_host` for traffic-camouflage packs.
- Nginx config rendering rejects the same loop condition for managed
  `ws_camouflage_edge` and `grpc_edge` profiles, and for explicit
  `traffic_camouflage` / `fallback_loop_guard` specs.
- `Instances -> Create from pack` now warns that the fallback website must be a
  separate site and blocks obvious same-host loop cases before the API call.
- `scripts/service-pack-smoke.sh` now fails before creating a camouflage pack
  when the fallback URL, fallback Host header or fallback SNI points back to the
  endpoint host.
- English and Russian user guides and release gates document the same
  operator-facing rule.

## Security Assessment

- Attack surface change: reduced operational exposure. No new public endpoint,
  privileged job type or node-side command surface was added.
- SSRF/proxy-loop risk reduced: the platform now rejects the most common
  self-referential fallback configuration before an ingress edge can proxy
  browser traffic back into itself.
- Data exposure risk: unchanged. The release does not render secrets, generated
  config content or new node inventory fields in public UI responses.
- Availability risk reduced: a misconfigured camouflage fallback no longer
  creates a persistent reverse-proxy loop through the public ingress host.

## Verification Evidence

- `go test ./...`: passed before release tagging.
- Targeted tests for `internal/api/http` and `internal/infra/postgres`: passed.
- Frontend JavaScript syntax check with bundled Node.js: passed.
- CSS brace balance check: passed.
- `bash -n` for touched shell scripts: passed.
- `scripts/service-pack-smoke.sh` same-host fallback preflight: rejected before
  API call as expected.
- `scripts/self-test.sh`: final tagged run passed with `15` passed, `0` failed
  and `7` skipped on this workstation. The `frontend-js-syntax` gate is skipped
  by the script because system `node` is not on `PATH`; the same check passed
  manually with the bundled Node.js runtime.
- `MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh`: final tagged run
  passed with `10` passed, `0` failed and `6` skipped on this workstation.

## Residual Risk

- Same-host loop prevention cannot prove that an arbitrary upstream IP, CDN or
  load balancer does not route back to the ingress host. Production validation
  still needs live fallback-site smoke from outside the node and operator review
  of DNS/LB topology.
- This release does not add Nginx config preview/diff UI, live `nginx -t`
  evidence surfacing or reusable Nginx edge profile catalog extraction; those
  remain the next traffic-camouflage hardening steps.
