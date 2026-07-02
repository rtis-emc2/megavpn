# Next Steps

**Release:** `7.0.1.2`

Current roadmap: [`ROADMAP_V1_AND_TZ.md`](../ROADMAP_V1_AND_TZ.md).
Russian companion: [`NEXT_STEPS_RU.md`](NEXT_STEPS_RU.md).
Canonical repository: `github.com/rtis-emc2/megavpn`.

## Immediate Engineering Queue

1. Re-run managed backhaul apply on real ingress/egress nodes after updating API,
   UI and agents. Re-apply must stop obsolete managed units, remove stale
   managed interfaces, remove conflicting managed WireGuard listeners, create
   the nft NAT rule, converge runtime state and show the same `/30` profile on
   both sides.
2. Run the PostgreSQL integration suite with `MEGAVPN_TEST_DATABASE_DSN`. The
   suite must create a temporary schema, apply all migrations and verify jobs,
   locks, provisioning and baseline access routes.
3. Verify runtime-state APIs after a real `instance.apply`:
   `/api/v1/service-drivers`, `/api/v1/instances/runtime-states`,
   `/api/v1/instances/{id}/runtime-state`,
   `/api/v1/instances/{id}/runtime-observations` and
   `/agent/runtime/instances`.
4. Verify the node bootstrap console on a remote control plane: setup tabs,
   onboarding explanation, agent-channel next step, public base URL display,
   Control Plane TLS apply, SSH access method creation, enrollment-token
   rotation, bootstrap queueing, update buttons and heartbeat transition.
5. Validate clean deployment on a fresh server with
   `scripts/control-plane-install.sh`, generated environment, master key,
   admin credentials, Nginx edge and systemd units.
6. Validate service-pack and runtime paths on a test server: IPsec/L2TP, Xray
   Reality, Xray+Nginx gRPC, Xray WebSocket camouflage, OpenVPN TCP/UDP,
   WireGuard, HTTP Proxy, MTProto and Shadowsocks.
7. Validate and harden the topology workspace: local static world map, GeoIP
   node placement, node owner metadata, role/health badges, backhaul edges,
   route toggles, failed-hop diagnostics and per-node workload drill-down.
8. Validate VLESS access groups end to end: default route, local breakout,
   selected egress node, target-only access, blocked access, ad-block rule and
   provisioning selection.
9. Design the VLESS subscription endpoint: per-client token, selected inbound
   services, rotation, cache headers, QR/text export and provisioning status.
10. Formalize traffic-camouflage profiles: Xray WebSocket/gRPC edge, hidden
   path, fallback upstream, SNI/TLS binding, Nginx preview, validation and
   rollback.
11. Extract the Nginx edge profile catalog: reusable definitions, certificate
    binding, generated config diff, atomic apply and operator-visible failure.
12. Keep ACME automation paused until a canonical challenge strategy is chosen:
    `HTTP-01`, `DNS-01` or delegated external ACME.
13. Run API only with explicit bootstrap credentials:
    `MEGAVPN_BOOTSTRAP_ADMIN_USERNAME` and
    `MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD`.
14. Create OpenAPI/public API and internal agent API contracts.
15. Formalize typed job payload schemas on top of `internal/jobschema`.
16. Decide the v1.0 agent-transport security target for mandatory mTLS versus
    signed HTTP messages over HTTPS.
17. Continue UI split by moving node management, bootstrap diagnostics and
    workload views out of `app.js`.
18. Complete revision workflow: candidate, validated, applied, rollback, apply
    history and safe rollback engine.
19. Continue routing hardening: rollback/remove stage for retired policies,
    conntrack visibility, MTU/MSS clamp and route-policy telemetry.
20. Complete managed backhaul multi-driver enforcement: controlled Xray TUN,
    strongSwan/IKEv2, OpenVPN certificate-mode P2P and health probes.
21. When the maintenance window is approved, rewrite Git history for sensitive
    historical commits/tags and update deployed checkouts with the documented
    history-rewrite procedure.
