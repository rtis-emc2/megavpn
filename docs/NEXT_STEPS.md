# Next Steps

**Release:** `7.1.1.16`

Current roadmap: [`ROADMAP_V1_AND_TZ.md`](../ROADMAP_V1_AND_TZ.md).
Russian companion: [`NEXT_STEPS_RU.md`](NEXT_STEPS_RU.md).
Canonical repository: `github.com/rtis-emc2/megavpn`.

## Immediate Engineering Queue

1. Validate traffic accounting collectors on live nodes after re-applying
   managed Xray, OpenVPN and WireGuard instances: Xray Stats API, WireGuard
   `wg show <interface> transfer`, OpenVPN status files, attribution to
   `service_accesses`, Traffic Accounting `Collector status` active/degraded/
   missing/inactive freshness, expected/observed/missing instance coverage,
   reconnect/restart behavior and measured real cardinality before deciding
   whether table partitioning or cold archive storage is needed. Then add an
   explicit collector heartbeat if operators need collector health independent
   from user traffic deltas.
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
   `scripts/ops/control-plane-install.sh`, generated environment, master key,
   admin credentials, Nginx edge and systemd units.
7. Validate service-pack and runtime paths on a test server: IPsec/L2TP, Xray
   Reality, Xray+Nginx gRPC, Xray WebSocket camouflage, OpenVPN TCP/UDP,
   WireGuard, HTTP Proxy, MTProto and Shadowsocks. For camouflage matrix runs,
   run `scripts/smoke/service-pack-smoke.sh --matrix ... --plan` first to preview
   selected packs, endpoint hosts, certificate/fallback requirements and port
   overlaps without creating instances. Set `MEGAVPN_FALLBACK_UPSTREAM_URL` to
   the real fallback website; otherwise those packs are intentionally skipped.
   Store staged batch evidence in `MEGAVPN_SMOKE_EVIDENCE_DIR` and validate
   `_matrix-summary.json` with `scripts/ci/service-pack-evidence-report.js`
   before accepting the batch. Prefer `scripts/smoke/service-pack-staged-smoke.sh`
   for operator runs because it groups protocols into conflict-aware batches
   and validates evidence after every batch. Preserve its top-level
   `_staged-summary.json` together with per-batch evidence so final acceptance
   can trace each protocol group to its `_matrix-summary.json`. Run all
   443-based batches with `--cleanup` for diagnostics or on isolated
   disposable nodes for final release evidence.
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
