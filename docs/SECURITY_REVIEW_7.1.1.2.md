# Security and Release Review: 7.1.1.2

**Release:** `7.1.1.2`

## Scope

- L2TP/IPsec, VLESS and Shadowsocks external provider import, encrypted-secret,
  deployment, apply, probe, cleanup and selected-group routing paths.
- Initial authenticated UI loading and request timeout behavior.
- Agent-managed paths, systemd units, generated Xray/strongSwan/xl2tpd/pppd
  configuration and fail-closed routing ownership.

## Findings Closed

1. Core API requests are individually abortable; the initial populated
   dashboard waits only for its five critical datasets, then secondary
   inventory starts in parallel without blocking that render or competing for
   the initial request budget.
2. VLESS requires TLS or REALITY and rejects `allowInsecure`, unsupported
   transports, invalid REALITY material, unknown URL/JSON fields and control
   characters.
3. Shadowsocks accepts supported SIP002/JSON AEAD profiles and rejects plugins,
   legacy ciphers, unknown fields and malformed ports.
4. L2TP/IPsec accepts only a strict key/value or JSON field set, rejects
   duplicate/unknown fields and requires PSK plus username/password.
5. L2TP/IPsec uses an isolated managed PPP interface, dedicated mark/table and
   unreachable fallback; the node main routing table is not replaced.
6. VLESS/Shadowsocks run in a deployment-specific Xray process whose SOCKS
   listener binds only to `127.0.0.1`; no wildcard provider listener is
   accepted as healthy.
7. Provider runtime files use explicit path and mode allowlists. Xray configs
   are tested by the installed binary before systemd activation.
8. Cleanup is protocol-specific and removes exact managed units, files and L3
   policy state. L2TP cleanup reloads strongSwan after fragment removal.
9. A node advisory lock and lifecycle checks prevent competing external
   L2TP/IPsec deployments or a managed XL2TPD server from claiming node-scoped
   resources. PostgreSQL serializes both reservation paths with a node-row lock,
   closing the check-then-write race and protecting direct database writes as
   well as API operations. Agent preflight rejects an unmanaged UDP/1701
   listener.
10. External provider selection remains global access-group policy and does not
    route unselected client or ordinary node traffic through the provider.

## Trust And Secret Model

- The Control Plane stores provider configuration and credential values in
  encrypted `secret_refs`; list and read APIs expose only purpose names.
- Apply jobs contain secret references until claimed by the assigned node.
  Probe and cleanup payloads contain no provider plaintext.
- Agent jobs validate node ownership, deployment identifiers, resource ranges,
  protocol support and deterministic proxy ports before writing runtime state.
- The node agent is a privileged trust boundary. Root compromise can read
  materialized provider configuration and intentionally use loopback proxy
  listeners; this release does not claim isolation from node root.

## Residual Risk And Production Evidence

- macOS unit tests cannot prove Linux strongSwan/xl2tpd/pppd negotiation,
  systemd restart behavior or policy routing. Run apply, probe, traffic and
  cleanup on a disposable Ubuntu node with real provider credentials.
- Provider MTU, DNS, latency and availability are third-party properties.
- L2TP/IPsec supports PSK with username/password and one deployment per node;
  certificate-authenticated variants are not runtime-ready.
- External provider routing is IPv4-only in this release.
- The loopback Xray listener prevents remote access and accidental node-wide
  capture, but privileged local processes remain inside the node trust domain.

## Required Gates

```bash
make build
go test ./...
go test -race ./...
go vet ./...
scripts/ci/docs-consistency.sh
scripts/ci/self-test.sh
scripts/ci/release-gate.sh
```
