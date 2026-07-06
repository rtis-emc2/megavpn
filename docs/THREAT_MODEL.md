# Threat Model

**Release:** `7.0.1.36`

## Scope

MegaVPN consists of:

- Control-plane API and Web UI.
- PostgreSQL state store.
- Worker process for queued jobs, bootstrap and control-plane TLS apply.
- Node agent for inventory, service materialization and runtime actions.
- Nginx public edge.
- Generated VPN/proxy service configs and client artifacts.
- Runtime binary repository and pinned node downloads.
- Managed ingress-to-egress backhaul route projection.
- Node topology map with GeoIP metadata and managed backhaul overlay.
- VLESS access groups for client routing policy and public per-client VLESS
  subscription endpoints.

## Trust Boundaries

| Boundary | Trusted Side | Untrusted Side | Controls |
| --- | --- | --- | --- |
| Browser to API | API session middleware | Browser/client network | HttpOnly cookie, CSRF header, rate limits, strict JSON decode |
| Public share link | Artifact download handler | Anyone with token URL | High-entropy token, `token_hash`, expiry, revocation, artifact-root containment |
| API to PostgreSQL | API/worker store code | SQL data and concurrent workers | Transactions, row locks, lease owner checks |
| API to agent | Signed agent channel | Network and reverse proxies | HMAC request/response signatures, timestamp window, nonce/body hash |
| Worker to SSH node | Bootstrap worker | Remote host/network | Strict user/host validation, host-key SHA256 pinning, `--` target separator |
| Agent to host OS | Managed allowlists | Job payload/spec data | Path canonicalization, symlink-safe writes, unit allowlists, direct systemd argv |
| Generated artifacts | Artifact root | Filesystem path data | Artifact-root symlink containment before serving |
| Runtime binary repository | Artifact root and signed agent download | External release URLs and uploaded files | HTTPS import, SHA-256 pinning, artifact-root containment, service-specific install path allowlists |
| VLESS ingress to egress | Xray instance route config | Client traffic and public network | Instance-level default outbound, managed backhaul source routing, explicit access groups |
| VLESS access groups | Group catalog and client bindings | Operator mistakes and client traffic | Central catalog, apply-time rendering, target-only allow rules, final block fallback |
| VLESS subscriptions | Token registry and public subscription feed | Anyone with a live bearer URL | Token hashing, one-time plaintext display, expiry, revocation, active-access filtering, `Cache-Control: no-store` |

## Primary Assets

- Platform user credentials and sessions.
- Node enrollment tokens and persistent agent tokens.
- Secret master key and encrypted `secret_refs`.
- Platform certificates and service PKI roots.
- VPN service private keys, PSKs, account passwords and generated client bundles.
- VLESS subscription tokens and generated client profile URLs.
- Job/audit history.
- PostgreSQL database and artifact root backups.

## Key Threats And Mitigations

| Threat | Mitigation |
| --- | --- |
| Agent command tampering or replay | Bidirectional HMAC signatures, timestamp window, nonce/body hash, signed empty `204` job poll |
| Stale or stolen job result | Agent completion requires current `locked_by`, `running` status and non-expired lease |
| Privilege escalation through generic jobs | Privileged job types must use typed APIs; remaining direct jobs require job-type permission matrix |
| Filesystem escape from agent-managed files | Canonical absolute paths, root allowlists, whitespace/control rejection, symlink-safe parent/target checks |
| Arbitrary systemd unit injection | Strict managed unit allowlist and direct ExecStart validation |
| SSH bootstrap MITM or option injection | `ssh_host_key_sha256`, safe user/host regex plus IP parser, strict known_hosts, `--` before target |
| Nginx directive injection | DNS/IP/wildcard-only `server_name`, directive character rejection |
| Public share token database disclosure | Store `token_hash` only, expose plaintext once, use `token_hint` for operations |
| Supply-chain compromise of remote installer | Xray remote install requires pinned script SHA-256; otherwise fail closed |
| Runtime binary substitution | Runtime artifacts are stored under a control-plane artifact root and installed only after hash verification and service/path allowlist checks |
| Accidental direct breakout from ingress VLESS | Xray egress is resolved at instance level; generated configs add a default outbound via managed backhaul when the node role requires remote egress |
| Unsafe node cleanup | Cleanup jobs must remain scoped to managed instance/backhaul units and operator confirmation must name the target node |
| Subscription URL leakage | Treat subscription tokens as bearer credentials; use per-client rotation, expiry, audit and `Cache-Control: no-store` |
| Secret loss | Backup DB/artifacts separately from master key; master key rotation and sealed copy required |

## Residual Risks

- A root-running node agent is intentionally privileged. Production deployments must restrict which operators can queue apply/capability jobs.
- Package-manager capability installs write broadly to the node OS. Use manual-present strategy or pre-baked images where policy forbids runtime package installation.
- IPsec/L2TP and some Xray/backhaul profiles can be materialize-only until their full runtime validation is completed.
- Public share links are bearer URLs. Hashing protects database disclosure, not recipients forwarding a live URL.
- The topology map exposes operational metadata derived from public node IPs. External GeoIP providers will see those public IPs; regulated deployments should use an internal GeoIP endpoint or disable lookup with `MEGAVPN_GEOIP_LOOKUP_URL_TEMPLATE=disabled`.
- A full repository-wide security scan requires delegated worker coverage. Parent-agent-only scans are useful but must not be treated as exhaustive release evidence.
