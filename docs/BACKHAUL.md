# Managed Backhaul

**Release:** `7.0.1.19`

Managed backhaul connects an ingress node to an egress node so client access routes can target a remote exit without hardcoding ad-hoc next-hop values in every policy.

## Architecture

```text
Control Plane
  -> backhaul_links / backhaul_transports
  -> node.backhaul.apply jobs
  -> node.backhaul.probe jobs
  -> node.backhaul.cleanup jobs
  -> ingress agent writes local tunnel config
  -> egress agent writes listener/peer config
  -> route projection can use active managed interface
  -> node.route_policy.apply installs conservative kernel policy routing
```

Core database objects:

- `backhaul_links`: ingress node, egress node, selected transport, desired driver, route table and lifecycle status.
- `backhaul_transports`: driver-specific profile, endpoint, tunnel addresses, interface name, secret refs, health and applied timestamps.

Core API/UI:

- `GET /api/v1/backhaul/drivers`
- `GET /api/v1/backhaul-links`
- `POST /api/v1/backhaul-links`
- `POST /api/v1/backhaul-links/{id}/apply`
- `POST /api/v1/backhaul-links/{id}/probe`
- `POST /api/v1/backhaul-links/{id}/promote`
- `DELETE /api/v1/backhaul-links/{id}`
- Backhaul page in the Control Plane UI.

## Driver Status

| Driver | Layer | Apply behavior | Notes |
| --- | --- | --- | --- |
| `wireguard` | L3 | Verifies/installs `wireguard-tools`, `iproute2` and `nftables`, writes config, writes systemd unit, enables unit | Primary low-overhead transport. Ingress applies managed SNAT on `oifname <mgbh*>`; egress applies managed masquerade on `iifname <mgbh*>` so return traffic has a deterministic tunnel path. |
| `openvpn_udp` | L3 | Verifies/installs `openvpn`, `iproute2` and `nftables`, writes static-key P2P config/profile, writes systemd unit, enables unit | UDP fallback when WireGuard is unsuitable. Uses compatible AES-256-CBC + HMAC-SHA256 static-key mode and avoids version-sensitive compression directives until cert-mode backhaul is split into a separate driver. |
| `openvpn_tcp_443` | L3 | Verifies/installs `openvpn`, `iproute2` and `nftables`, writes config/static key, writes systemd unit, enables unit | TCP fallback; avoid as default due TCP-over-TCP behavior. Uses the same compatibility profile as OpenVPN UDP. |
| `ipsec_l2tp` | L3 | Writes strongSwan profile and PSK file only | Manual activation until full host profile validation exists. |
| `ikev2` | L3 | Writes strongSwan profile and PSK file only | Manual activation until full host profile validation exists. |
| `xray_vless_ws_tls` | Proxy | Writes Xray client/server profile and unit only | Requires TLS edge review before activation. |
| `xray_vless_grpc_tls` | Proxy | Writes Xray client/server profile and unit only | Requires TLS edge review before activation. |
| `xray_vless_reality` | Proxy | Writes Xray client/server Reality profile and unit only | Camouflage transport, not direct kernel routing. |
| `xray_tun_vless` | L3 over proxy | Writes Xray profile and unit only | Requires policy routing and loop protection before activation. |

## UI Selection Semantics

The Backhaul create form has two related controls:

| Control | Meaning | Operational effect |
| --- | --- | --- |
| `Active backhaul transport` | The single active ingress-to-egress transport path. | Stored as `desired_driver`, rendered as the selected transport, used by apply/probe gating and route-policy projection. It is always included and cannot be unchecked in the create form. |
| `Optional standby transports` | Extra internal profiles generated for this backhaul link. | Stored as `drivers` during create only when explicitly checked. They are generated backup profiles for controlled fallback, diagnostics or later promotion, but they are not active after create. |
| `Promote to active` | Explicitly changes the selected active transport to a healthy standby transport. | Updates `selected_transport_id` and `desired_driver`, sets the link active when the promoted transport is active on both sides, and queues an ingress route-policy refresh. |

These controls do not select client-facing VPN protocols. They define the
internal node-to-node transport between ingress and egress. Client access
instances keep their own service drivers, client configs and route policies.

## Data Flow

1. Operator creates a backhaul link and selects ingress node, egress node, active backhaul transport and any optional standby transports.
2. Control Plane validates node roles and driver support.
3. Control Plane generates driver material and stores secrets as encrypted secret refs.
4. Operator applies the backhaul link.
5. Control Plane queues `node.backhaul.apply` jobs for every selected transport profile: one ingress job and one egress job per profile.
6. Before writing files for managed-systemd drivers, each agent verifies runtime capability and installs the missing Ubuntu package when needed. Both ingress and egress apply require `iproute2` and `nftables` before managed NAT is enabled.
7. Each agent validates its own `node_id`, validates managed paths and writes only allowed files.
8. For managed-systemd drivers, the agent reads the previous managed manifest, stops/disables obsolete managed units when present, resets failed state, removes obsolete generated files, removes previous/current managed `mgbh*` interfaces when present, reloads systemd, enables the generated unit and records local service readiness only: systemd `active` and tunnel interface presence. Apply intentionally does not ping the peer because the opposite side may still be starting. WireGuard configs use the local tunnel host with the transport `/30` prefix so a connected route to the peer tunnel IP exists even while `Table=off` prevents wg-quick from installing broad routes. Apply fails when runtime install fails, the generated unit is not `active`, or the tunnel interface is not present.
9. When both sides succeed, managed-systemd transports become `active`; profile-only transports become `materialized` and never produce a false active route. Failed apply results are stored per side in `health_json.ingress` or `health_json.egress` so a partial apply shows the missing/failing side explicitly, with the root cause shown before generic failure text.
10. Every L3 transport profile gets its own `/30`; duplicate failed profiles are normalized to a unique CIDR during the next apply.
11. Route-policy projection can use the active managed backhaul interface for remote egress routes.
12. If the selected active transport fails but a standby L3 transport is active
    on both sides, the operator can promote the standby transport. Promotion is
    explicit: the system does not silently move production traffic to a standby
    path without operator intent.
13. `node.route_policy.apply` installs policy routing for IPv4 L3/L4 `allow`
    candidates and Xray/VLESS system source-routes. For VLESS remote egress,
    the source route is derived from Xray `sendThrough` and points
    `from <ingress_backhaul_ip>/32` to the selected backhaul routing table.
14. Operator can run `probe` from the Backhaul UI after the selected transport is `active` and both ingress/egress sides have applied timestamps. The Control Plane queues two `node.backhaul.probe` jobs, one per side.
15. Each probe waits for systemd active state, local interface presence, route lookup to the peer tunnel address through the expected backhaul interface and ICMP reachability with retries.
16. Probe results are stored in `backhaul_transports.health_json.ingress` and `.egress`, including peer route lookup, peer address, packet loss, min/avg/max/stddev latency and exact agent reason. A failed probe preserves `degraded`/`unhealthy` health instead of replacing it with a generic error.
17. Delete is a managed cleanup flow, not only a database soft-delete. The Control Plane queues `node.backhaul.cleanup` for every materialized transport on both nodes; missing units/files/directories/interfaces are reported as `not found - skip`, and only after the cleanup batch succeeds does the link move to `deleted`.
18. Before queueing a new cleanup batch and before Jobs API reads, the backend recovers stale `running` jobs whose lease has expired back to `retrying`. This prevents a dead agent request or interrupted process from blocking backhaul deletion indefinitely.
19. Deleted links are excluded from active Backhaul operations but remain visible for a short operator-review window in the Backhaul UI `Recently Deleted Backhaul` table with per-transport ingress/egress cleanup summaries.

## Security Model

- Secrets are generated server-side and stored through secret refs, not in public UI responses.
- Agent writes are restricted to `/etc/megavpn/backhaul/` and `megavpn-backhaul-*.service`.
- Agent cleanup removes only validated managed systemd units, generated files, one-level managed directories under `/etc/megavpn/backhaul/` and validated managed `mgbh*` interfaces; missing units/files/directories/interfaces are treated as already-cleaned idempotent state and reported as `not found - skip`.
- Agent result manifests redact file content before persistence.
- Backhaul jobs are agent-only; the worker refuses to execute `node.backhaul.apply`, `node.backhaul.probe` and `node.backhaul.cleanup`.
- Backhaul activation and route policy enforcement are separate jobs with separate audit/job results.
- Managed L3 backhaul uses double NAT by default: ingress SNATs selected client traffic to the ingress tunnel address before it enters the backhaul, and egress masquerades traffic leaving the backhaul to the public/default route. This avoids unsafe broad reverse routes on egress nodes and keeps node-side return routing deterministic.
- WireGuard/OpenVPN managed units are role-specific (`...-ingress.service` / `...-egress.service`) and bounded in length for systemd compatibility. Re-applying a profile after upgrade stops/disables the older common unit name from the previous manifest when present.
- Agent apply results include systemd activation preflight, unit file path, `daemon-reload` output, `LoadState`, `ActiveState` and first useful `systemctl status` lines. `unit unknown` should be treated as a node-side systemd/load-state problem, not as a generic backhaul status.
- Route policy enforcement is conservative: client policies require IPv4,
  `allow`, L3/L4 source identity, CIDR/IPv4 endpoint destination and explicit
  non-main route table. Xray/VLESS system routes require a valid IPv4
  `sendThrough` source, explicit managed backhaul interface and explicit
  non-main route table.
- Xray/IPsec profiles are not auto-enabled until transport-specific safety gates are implemented.

## Deployment Model

Minimum production path for the first ingress/egress pair:

1. Add ingress node and egress node.
2. Verify both agents are `online`.
3. For WireGuard/OpenVPN, runtime packages are installed by `Apply profiles` if missing. `nftables` is required on both ingress and egress nodes for managed NAT lifecycle. For profile-only drivers, install runtime capability before manual activation:
   - IPsec: `ipsec` and optionally `xl2tpd`
   - Xray: `xray-core`
4. Create Backhaul link.
5. Apply profiles.
6. Confirm both generated jobs are `succeeded`.
7. Run Backhaul `Test` and verify both ingress and egress probe jobs are `succeeded`.
8. Check health summary: both sides should report `healthy`, packet loss should be `0`, latency should be visible as average RTT. If a service is active but the test is `failed/degraded`, use the shown reason, route lookup, peer address and packet loss to distinguish missing connected route, firewall/UDP reachability, tunnel handshake and route table problems.
9. Create client access route with remote egress node.
10. Queue route policy sync for the ingress node.
11. Verify route projection uses `managed_backhaul` for the primary candidate,
    `managed_backhauls` for the failover set, and route policy job reports
    `enforced=true`.
12. For VLESS remote egress, verify the route-policy job result includes an
    active `xray_vless_remote_egress` system route and the ingress node has
    `ip rule from <sendThrough>/32 table <backhaul_table>` in the kernel rule
    set.
13. When disabling a mapped backhaul route, the control plane marks the link and
    transports `disabled`, queues scoped cleanup jobs with
    `route_disable_batch_id`, and queues a mandatory ingress route-policy
    refresh. The disabled link remains visible in topology but is excluded from
    `managed_backhaul` route selection until enabled again.

## Failure Scenarios

- One side fails apply: transport and link move to `failed`; Backhaul UI shows `partial`, the applied side, the missing/failing side and the per-side health/error saved from the failed job. Root-cause readiness reasons such as `systemd unit is not active`, `interface is not present`, `active_state=failed` and `unit_status_output` are preserved for operator diagnostics.
- Selected transport fails while a standby is healthy: traffic and route
  projection still use the selected active transport until an operator promotes
  the healthy standby. Use `Backhaul -> Manage -> Promote to active`.
- OpenVPN unit fails on start: check the Jobs or Backhaul modal result summary first; it includes the unit name, active state and first useful `systemctl status`/OpenVPN error line before manual SSH inspection is needed.
- Different `mgbh*` interface names or different tunnel CIDRs on ingress and egress for the same selected transport indicate stale runtime state or different transport profiles, not a healthy single tunnel. Re-apply removes interfaces recorded in the previous managed manifest and the target managed interface before recreating it; unrelated stale interfaces from older/deleted links must be removed by managed Backhaul delete or a controlled one-time cleanup after verifying the owning unit is obsolete.
- Unit/interface missing after apply: apply job fails; install/verify the runtime capability on that node before applying again.
- Agent offline: jobs remain queued until the agent polls. If an agent claimed a job and died, the backend returns the expired `running` lease to `retrying`.
- Endpoint unreachable: tunnel unit may start but health reports `degraded`; inspect agent job result and transport health.
- Client traffic reaches egress but replies do not return: verify both managed NAT comments exist in `nft list chain ip megavpn_backhaul postrouting`: `ingress-snat` on the ingress node and `egress-masquerade` on the egress node. Re-apply the selected backhaul transport if either rule is missing.
- VLESS clients connect but traffic exits from ingress or does not leave the
  node: verify the rendered Xray outbound has `sendThrough`, then run
  `node.route_policy.apply` on the ingress node and confirm the job produced an
  active `xray_vless_remote_egress` system route. If the route is blocked, fix
  the selected managed backhaul table/interface before re-applying the Xray
  instance.
- Cleanup failed: link remains `failed`; inspect the cleanup job result. Stale `running` cleanup jobs are recovered automatically after lease expiry, but real failed/cancelled jobs still require operator retry. The agent will not remove paths outside the managed backhaul directory or managed systemd unit prefix.
- Cleanup succeeded and link disappeared from the active list: expected lifecycle behavior. The link is now `deleted`; use the `Recently Deleted Backhaul` table and Jobs/Audit views for cleanup confirmation.
- Xray/IPsec selected: config is written and status becomes `materialized`, but it is not enabled automatically; manual transport activation is required until the driver-specific safety gate exists.
- Duplicate ingress/egress/name: database constraint blocks active duplicate links.
- Route table is `main`: kernel enforcement skips the route; managed backhaul links allocate a dedicated table automatically.

## Next Engineering Steps

- Add throughput and MTU/MSS probes on top of the current RTT/packet-loss probe.
- Add rollback/removal command for disabled route policies.
- Add controlled Xray TUN activation with loop prevention.
- Add strongSwan profile validation and activation for IPsec/IKEv2.
- Add PostgreSQL integration tests covering backhaul creation, apply jobs and route projection.
