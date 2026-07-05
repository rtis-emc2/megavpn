# Security and Release Review: 7.0.1.3

**Release:** `7.0.1.3`

Date: 2026-07-05

Scope:

- Control Plane API, worker, agent and PostgreSQL persistence paths relevant to release blockers.
- Agent transport signing, privileged job creation, job lease completion,
  managed file writes, managed-file rollback, systemd unit validation, SSH
  bootstrap, Nginx server-name validation and traffic-camouflage ingress
  profiles.
- Documentation, release metadata and release roadmap.

## Result

No new P0/P1 security defect was found in the reviewed release-critical paths.

This review is not a full independent delegated repository scan. The delegated-worker
preflight was unavailable in the local environment, so this document records a
parent-agent targeted review plus automated tests. A production release still needs
the full release gates listed below.

## Reviewed Controls

| Control | Result |
| --- | --- |
| Release version and tag consistency | Pass after `v7.0.1.3` tag on reviewed commit |
| Agent unsigned 200/204 response rejection tests | Present and passing |
| Signed empty 204 response path | Present and passing |
| Generic privileged job API restriction | Privileged apply, cleanup, capability, route-policy and emergency-cleanup job types require typed endpoints |
| Job completion lease enforcement | Agent completion requires `running` status, valid owner and non-expired lease |
| Agent managed file writes | Absolute path, root allowlist, unsafe-token rejection and symlink rejection are present |
| Generated systemd unit validation | Managed unit allowlists and shell-exec rejection are present for instance/backhaul paths |
| SSH bootstrap host/user hardening | Strict user validation, host-key fingerprint pinning and `StrictHostKeyChecking=yes` are present |
| Nginx server-name validation | Whitespace, directive characters, malformed IP literals and invalid names are rejected |
| Traffic camouflage API contract | Fallback website is mandatory; hidden path and fallback directive values reject unsafe characters, credentials and invalid schemes |
| Xray/Nginx camouflage isolation | Xray WebSocket/gRPC backends bind loopback; public root traffic is routed through Nginx fallback website |
| Nginx managed-file rollback | Instance apply snapshots managed files and restores/removes them on validation, network-policy or systemd failure |
| OpenVPN full-tunnel NAT | Default full-tunnel packs materialize managed `nat_rules` for client-pool masquerade |
| Forbidden deployment-domain string grep | Clean in the current tree |

## Automated Checks

Passed:

```bash
go test -count=1 ./...
go test -race ./...
go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate
scripts/self-test.sh
MEGAVPN_RELEASE_ALLOW_SKIPS=1 scripts/release-gate.sh
```

The normal `scripts/release-gate.sh` run fails closed locally because production
evidence gates are intentionally missing in this workstation environment.

Skipped production evidence:

- PostgreSQL migrations and integration test on disposable database.
- Backup and restore drill.
- `systemd-analyze verify` on a Linux/systemd host.
- `nginx -t` on a target edge host.
- API smoke against deployed Control Plane.
- VPN/service smoke matrix.

## Remaining Release Blockers

1. Run `scripts/release-gate.sh` on a release host with the required environment:
   `MEGAVPN_RELEASE_DATABASE_DSN`, `MEGAVPN_RELEASE_RESTORE_DATABASE_DSN`,
   `MEGAVPN_RELEASE_BASE_URL` and `MEGAVPN_RELEASE_RUN_SERVICE_MATRIX=1`.
2. Verify clean install on a new Ubuntu host, including migrations, API, worker,
   nginx edge, agent enrollment and node cleanup.
3. Verify service smoke matrix: OpenVPN TCP/UDP full tunnel, WireGuard,
   Xray/VLESS Reality, Xray+Nginx gRPC, Xray WebSocket camouflage fallback,
   Shadowsocks, Nginx edge and managed Backhaul.
4. Verify VLESS instance egress through managed ingress-to-egress backhaul, not
   direct breakout from the ingress node.
5. Complete delegated repository-wide security scan before any stable release
   claim.
6. Execute repository history rewrite only during an approved maintenance window
   following `docs/OPERATIONS_RUNBOOK.md`.

## Product Follow-Up

Planned next work:

- Backhaul graph with per-edge health, route disable/enable evidence and
  failed-hop diagnostics.
- VLESS subscription endpoint with per-client token rotation and selected inbound
  service visibility.
- Nginx edge profile catalog with generated config preview and operator-visible
  failed-apply evidence.
- Live traffic-camouflage smoke that checks fallback website behavior and
  generated VLESS subscription connectivity.
