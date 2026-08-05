# Security and Release Review: 7.1.1.18

**Release:** `7.1.1.18`

## Decision

The patch makes L2TP over IPsec external-egress lifecycle convergence stricter
without weakening process ownership checks or the fail-closed routing
boundary. No unresolved critical or high-severity finding was identified in
the changed surface.

## Reviewed Surface

- Managed L2TP systemd lifecycle and PPP interface ownership.
- UDP/1701 listener inspection and termination.
- Cleanup ordering and artifact retention on failure.
- PPP interface and route convergence.
- L2TP/IPsec runtime health classification.
- Apply, Probe and Cleanup idempotency.

## Security Properties

- Cleanup terminates a UDP/1701 listener only after validating that the process
  belongs to the managed deployment.
- Managed artifacts are retained when teardown ownership or completion cannot
  be established.
- A stale PPP interface must be removed before Apply can replace its runtime.
- The provider default route is installed only after the managed interface has
  an IPv4 address.
- L2TP health requires both `ESTABLISHED` and `INSTALLED` IPsec state in
  addition to the unit, interface, address, route and policy rule.
- Unknown teardown, route and process-inspection failures remain fatal.
- The node main routing table is not modified.
- Routing table number, fwmark and rule priority remain constrained to the
  deployment-managed ranges validated by job decoding.

## Residual Risk

L2TP process ownership is inferred from managed systemd state, process
arguments and managed paths. A host with manually modified units or wrapper
processes may require operator cleanup because unknown ownership fails closed.
The exact provider handshake, credentials, NAT behavior and peer-side policy
cannot be proven by repository tests and remain live deployment evidence.

## Verification

Unit tests cover stale interface release, ordered cleanup, ownership-safe
listener handling, route convergence, complete and incomplete probe state and
stable-probe requirements. Full Go tests and `go vet` pass, the changed agent
surface passes the race detector, and script/documentation self-tests pass.
Publication still requires the normal production release evidence for a live
provider handshake and disposable infrastructure.
