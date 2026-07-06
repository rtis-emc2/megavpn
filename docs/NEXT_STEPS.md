# Next Steps

**Release:** `7.1.0.1`

Current roadmap: [`ROADMAP_V1_AND_TZ.md`](../ROADMAP_V1_AND_TZ.md).
Russian companion: [`NEXT_STEPS_RU.md`](NEXT_STEPS_RU.md).
Canonical repository: `github.com/rtis-emc2/megavpn`.

## Immediate Engineering Queue

1. Design and implement user traffic accounting with at least 180 days of
   retention: define the event schema, aggregation granularity, privacy
   boundary, storage partitioning, retention cleanup, RBAC access and export
   audit trail before collecting production traffic data.
2. Re-run managed backhaul apply on real ingress/egress nodes after updating API,
   UI and agents. Re-apply must stop obsolete managed units, remove stale
   managed interfaces, remove conflicting managed WireGuard listeners, create
   the nft NAT rule, converge runtime state and show the same `/30` profile on
   both sides.
3. Run the PostgreSQL integration suite with `MEGAVPN_TEST_DATABASE_DSN`. The
   suite must create a temporary schema, apply all migrations and verify jobs,
   locks, provisioning and baseline access routes.
4. Verify runtime-state APIs after a real `instance.apply`:
   `/api/v1/service-drivers`, `/api/v1/instances/runtime-states`,
   `/api/v1/instances/{id}/runtime-state`,
   `/api/v1/instances/{id}/runtime-observations` and
   `/agent/runtime/instances`.
5. Verify the node bootstrap console on a remote control plane: setup tabs,
   onboarding explanation, agent-channel next step, public base URL display,
   Control Plane TLS apply, SSH access method creation, enrollment-token
   rotation, bootstrap queueing, update buttons and heartbeat transition.
6. Validate clean deployment on a fresh server with
   `scripts/control-plane-install.sh`, generated environment, master key,
   admin credentials, Nginx edge and systemd units.
7. Validate service-pack and runtime paths on a test server: IPsec/L2TP, Xray
   Reality, Xray+Nginx gRPC, Xray WebSocket camouflage, OpenVPN TCP/UDP,
   WireGuard, HTTP Proxy, MTProto and Shadowsocks. For camouflage matrix runs,
   set `MEGAVPN_FALLBACK_UPSTREAM_URL` to the real fallback website; otherwise
   those packs are intentionally skipped.
8. Validate and harden the topology workspace: local static world map, GeoIP
   node placement, node owner metadata, role/health badges, backhaul edges,
   operator-facing route-toggle UX on real nodes, failed-hop diagnostics and
   per-node workload drill-down. Backend route-toggle schema, cleanup batch
   metadata and route-policy refresh regression coverage are in place.
9. Validate VLESS access groups end to end: default route, local breakout,
   selected egress node, target-only access, blocked access, ad-block rule and
   provisioning selection, including on-demand catalog sync for freshly created
   active groups.
10. Validate VLESS subscriptions end to end: rotate/revoke token, one-time URL
   display, public `Cache-Control: no-store` feed, active-access filtering,
   QR/text export and provisioning-result visibility.
11. Finish traffic-camouflage hardening after the HTTP-to-HTTPS redirect and
   shared Nginx cleanup baseline: Nginx config preview, `nginx -t` evidence
   surface, live fallback-site smoke and generated VLESS subscription
   verification.
12. Extract the Nginx edge profile catalog: reusable definitions, certificate
    binding, generated config diff, atomic apply and operator-visible failure.
13. Keep ACME automation paused until a canonical challenge strategy is chosen:
    `HTTP-01`, `DNS-01` or delegated external ACME.
14. Run API only with explicit bootstrap credentials:
    `MEGAVPN_BOOTSTRAP_ADMIN_USERNAME` and
    `MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD`.
15. Create OpenAPI/public API and internal agent API contracts.
16. Formalize typed job payload schemas on top of `internal/jobschema`.
17. Decide the v1.0 agent-transport security target for mandatory mTLS versus
    signed HTTP messages over HTTPS.
18. Continue UI split by moving node management, bootstrap diagnostics and
    workload views out of `app.js`.
19. Complete revision workflow: candidate, validated, applied, rollback, apply
    history and safe rollback engine.
20. Continue routing hardening after the node route-policy preview, apply
    telemetry and explicit cleanup baseline: conntrack visibility, MTU/MSS
    clamp and live telemetry validation on real nodes.
21. Complete managed backhaul multi-driver enforcement: controlled Xray TUN,
    strongSwan/IKEv2, OpenVPN certificate-mode P2P and health probes.
22. When the maintenance window is approved, rewrite Git history for sensitive
    historical commits/tags and update deployed checkouts with the documented
    history-rewrite procedure.
