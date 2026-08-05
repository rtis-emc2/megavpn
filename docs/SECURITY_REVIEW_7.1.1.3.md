# Security and Release Review: 7.1.1.3

**Release:** `7.1.1.3`

## Scope

- External egress profile identifier lifecycle and operator catalog exposure.
- Protocol-specific external egress forms and secret submission.
- L2TP/IPsec PSK and certificate authentication parsing, validation, storage,
  agent materialization and cleanup.
- Regression review for selected-group routing and node main-table isolation.

## Findings Closed

1. Operators no longer choose internal profile identifiers. The control plane
   generates a normalized protocol-prefixed key from the profile UUID and
   rejects attempts to mutate it.
2. The operator catalog returns only protocols with a runtime-ready driver.
   Planned and preview definitions cannot be selected or activated through the
   normal UI.
3. Protocol changes rebuild the relevant form. Hidden or disabled controls from
   another protocol cannot submit stale credential values.
4. L2TP/IPsec provider endpoints accept only validated DNS names, IP addresses
   or single-host CIDRs. Multi-host CIDRs and unsafe values are rejected.
5. L2TP/IPsec authentication is explicit: PSK or client certificate. Switching
   mode removes obsolete encrypted secret references in the same transaction.
6. Certificate mode requires a CA certificate, client certificate and matching
   RSA or ECDSA private key. Chain validation and validity checks run before
   storage and again on the node.
7. Agent certificate paths are derived from the validated deployment ID,
   constrained by an exact path/mode allowlist and written with mode `0600`.
8. Runtime cleanup removes exact deployment certificate files and reloads
   strongSwan without wildcard deletion.
9. Editing profile metadata does not replace the encrypted provider connection
   configuration. Runtime changes still require a validated replacement
   configuration.
10. External egress remains group-scoped. No provider default route is
    installed in the node main table.

## Trust And Secret Model

- Provider configuration, passwords, PSKs and private keys are stored in
  encrypted `secret_refs`; read APIs expose purpose names only.
- Secrets are hydrated only into an apply job claimed by the owning node.
- Probe and cleanup jobs do not require provider plaintext.
- The node agent remains a privileged trust boundary. Root compromise can read
  materialized credentials and is outside the isolation claim.

## Residual Risk And Required Evidence

- macOS unit tests cannot prove Linux strongSwan, `xl2tpd`, `pppd`, systemd or
  policy-routing behavior.
- Production promotion of certificate-authenticated L2TP/IPsec requires apply,
  negotiation, selected-client traffic, probe, restart and cleanup evidence on
  a disposable Ubuntu node with real provider credentials.
- Provider MTU, DNS, certificate policy and availability remain third-party
  properties.
- External provider IPv6 default routing remains outside this release scope.

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
