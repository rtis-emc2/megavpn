# Managed Backhaul

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
- `DELETE /api/v1/backhaul-links/{id}`
- Backhaul page in the Control Plane UI.

## Driver Status

| Driver | Layer | Apply behavior | Notes |
| --- | --- | --- | --- |
| `wireguard` | L3 | Writes config, writes systemd unit, enables unit | Primary low-overhead transport. |
| `openvpn_udp` | L3 | Writes hardened static-key P2P config/profile, writes systemd unit, enables unit | UDP fallback when WireGuard is unsuitable. Uses compatible AES-256-CBC + HMAC-SHA256 static-key mode until cert-mode backhaul is split into a separate driver. |
| `openvpn_tcp_443` | L3 | Writes config/static key, writes systemd unit, enables unit | TCP fallback; avoid as default due TCP-over-TCP behavior. |
| `ipsec_l2tp` | L3 | Writes strongSwan profile and PSK file only | Manual activation until full host profile validation exists. |
| `ikev2` | L3 | Writes strongSwan profile and PSK file only | Manual activation until full host profile validation exists. |
| `xray_vless_ws_tls` | Proxy | Writes Xray client/server profile and unit only | Requires TLS edge review before activation. |
| `xray_vless_grpc_tls` | Proxy | Writes Xray client/server profile and unit only | Requires TLS edge review before activation. |
| `xray_vless_reality` | Proxy | Writes Xray client/server Reality profile and unit only | Camouflage transport, not direct kernel routing. |
| `xray_tun_vless` | L3 over proxy | Writes Xray profile and unit only | Requires policy routing and loop protection before activation. |

## Data Flow

1. Operator creates a backhaul link and selects ingress node, egress node and preferred driver.
2. Control Plane validates node roles and driver support.
3. Control Plane generates driver material and stores secrets as encrypted secret refs.
4. Operator applies the backhaul link.
5. Control Plane queues two `node.backhaul.apply` jobs: one for ingress, one for egress.
6. Each agent validates its own `node_id`, validates managed paths and writes only allowed files.
7. For managed-systemd drivers, the agent reloads systemd, enables the generated unit and records unit/interface health. Apply fails when the generated unit is not `active` or the tunnel interface is not present.
8. When both sides succeed, the selected transport and link become `active`.
9. Route-policy projection can use the active managed backhaul interface for remote egress routes.
10. `node.route_policy.apply` installs policy routing for IPv4 L3/L4 `allow` candidates only.
11. Operator can run `probe` from the Backhaul UI after the selected transport is `active`. The Control Plane queues two `node.backhaul.probe` jobs, one per side.
12. Each probe validates systemd active state, local interface presence and ICMP reachability to the peer tunnel address.
13. Probe results are stored in `backhaul_transports.health_json.ingress` and `.egress`, including packet loss and min/avg/max/stddev latency when Linux ping reports RTT data.
14. Delete is a managed cleanup flow, not only a database soft-delete. The Control Plane queues `node.backhaul.cleanup` for every materialized transport on both nodes; missing units/files/directories are reported as `not found - skip`, and only after the cleanup batch succeeds does the link move to `deleted`.

## Security Model

- Secrets are generated server-side and stored through secret refs, not in public UI responses.
- Agent writes are restricted to `/etc/megavpn/backhaul/` and `megavpn-backhaul-*.service`.
- Agent cleanup removes only validated managed systemd units and one-level managed directories under `/etc/megavpn/backhaul/`; missing units/files/directories are treated as already-cleaned idempotent state and reported as `not found - skip`.
- Agent result manifests redact file content before persistence.
- Backhaul jobs are agent-only; the worker refuses to execute `node.backhaul.apply`, `node.backhaul.probe` and `node.backhaul.cleanup`.
- Backhaul activation and route policy enforcement are separate jobs with separate audit/job results.
- Route policy enforcement is conservative: IPv4, `allow`, L3/L4 source identity, CIDR/IPv4 endpoint destination and explicit non-main route table are required.
- Xray/IPsec profiles are not auto-enabled until transport-specific safety gates are implemented.

## Deployment Model

Minimum production path for the first ingress/egress pair:

1. Add ingress node and egress node.
2. Verify both agents are `online`.
3. Install required runtime capability on both nodes:
   - WireGuard: `wireguard`
   - OpenVPN: `openvpn`
   - IPsec: `ipsec` and optionally `xl2tpd`
   - Xray: `xray-core`
4. Create Backhaul link.
5. Apply selected driver.
6. Confirm both generated jobs are `succeeded`.
7. Run Backhaul `Test` and verify both ingress and egress probe jobs are `succeeded`.
8. Check health summary: both sides should report `healthy`, packet loss should be `0`, latency should be visible as average RTT.
9. Create client access route with remote egress node.
10. Queue route policy sync for the ingress node.
11. Verify route projection uses `managed_backhaul` and route policy job reports `enforced=true`.

## Failure Scenarios

- One side fails apply: transport and link move to `failed`; inspect job logs and agent result.
- Unit/interface missing after apply: apply job fails; install/verify the runtime capability on that node before applying again.
- Agent offline: jobs remain queued/running until the agent polls.
- Endpoint unreachable: tunnel unit may start but health reports `degraded`; inspect agent job result and transport health.
- Cleanup failed: link remains `failed`; inspect the cleanup job result. The agent will not remove paths outside the managed backhaul directory or managed systemd unit prefix.
- Xray/IPsec selected: config is written but not enabled automatically; manual transport activation is required.
- Duplicate ingress/egress/name: database constraint blocks active duplicate links.
- Route table is `main`: kernel enforcement skips the route; managed backhaul links allocate a dedicated table automatically.

## Next Engineering Steps

- Add throughput and MTU/MSS probes on top of the current RTT/packet-loss probe.
- Add rollback/removal command for disabled route policies.
- Add controlled Xray TUN activation with loop prevention.
- Add strongSwan profile validation and activation for IPsec/IKEv2.
- Add PostgreSQL integration tests covering backhaul creation, apply jobs and route projection.
