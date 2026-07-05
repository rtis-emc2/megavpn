# Security and Release Review: 7.0.1.7

**Release:** `7.0.1.7`

## Scope

- Delta review for traffic-camouflage hardening after `7.0.1.6`.
- Nginx reverse-proxy fallback validation and WebSocket edge rendering.
- Service-pack smoke flow for Xray+Nginx gRPC/WebSocket camouflage packs.
- VLESS provisioning and subscription public endpoint regression coverage.

This is a targeted release review. It does not replace an independent
repository-wide security scan before a stable production claim.

## Security Model

| Area | Control |
| --- | --- |
| Fallback website | API, smoke and renderer require absolute `http://` or `https://` fallback URLs without credentials |
| Public camouflage path | API rejects `/`, query, fragment and unsafe directive characters |
| Xray backend exposure | Default camouflage packs bind Xray to `127.0.0.1`; public traffic enters through Nginx |
| WebSocket upgrade | Nginx renderer uses structured `websocket_upgrade` instead of raw pack-owned directive text |
| Client profiles | VLESS artifacts and subscriptions use public Nginx endpoint metadata and reject backend-port leakage in tests |
| Failed apply | Agent still validates Xray/Nginx configs and rolls managed files back on failed validation/apply |

## Changes Reviewed

- `buildNginxServerConfig` now validates fallback upstreams as HTTP(S) URLs and
  rejects credentials, relative URLs and unsupported schemes.
- WebSocket camouflage uses a structured `websocket_upgrade` flag that renders
  Upgrade/Connection headers, long read/send timeouts and disabled proxy
  buffering.
- `scripts/service-pack-smoke.sh` now accepts
  `MEGAVPN_FALLBACK_UPSTREAM_URL`, `MEGAVPN_CAMOUFLAGE_PATH`,
  `MEGAVPN_FALLBACK_HOST_HEADER` and `MEGAVPN_FALLBACK_SNI`. Single-pack smoke
  fails before API calls when a camouflage pack has no fallback URL; matrix
  smoke reports those packs as skipped.
- Regression tests assert that VLESS artifacts and subscriptions for
  camouflage profiles expose `:443` public WebSocket settings and do not expose
  the `127.0.0.1:7080` backend profile.

## Automated Evidence

```bash
go test -count=1 ./internal/infra/postgres -run 'Test(BuildNginx|ClientVLESSSubscription|BuildXrayServerConfig)'
go test -count=1 ./internal/api/http -run 'Test(DefaultWebSocketCamouflage|NormalizeServicePackCamouflage|DefaultServicePacks)'
go test -count=1 ./cmd/worker -run 'TestBuildXrayArtifacts'
bash -n scripts/service-pack-smoke.sh
```

Result: all targeted checks passed.

Final release evidence:

```bash
go test -count=1 ./...
PATH=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH find web/assets -maxdepth 1 -name "*.js" -print0 | xargs -0 -n1 node --check
scripts/self-test.sh
MEGAVPN_RELEASE_ALLOW_SKIPS=1 PATH=/Users/netwizd/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH scripts/release-gate.sh
```

Result:

- `go test -count=1 ./...`: passed.
- Frontend JS syntax check: passed.
- `scripts/self-test.sh`: `16 passed`, `0 failed`, `6 skipped`;
  report `tmp/self-test/self-test-20260705T142711Z.md`.
- `scripts/release-gate.sh` with local skips allowed: `10 passed`,
  `0 failed`, `6 skipped`.

## Residual Risks

1. Live camouflage smoke still requires a disposable node, real certificate and
   operator-selected fallback site.
2. Nginx config preview/diff before apply is still a product backlog item.
3. Full independent security scan remains required before a stable production
   release claim.
