# Security and Release Review: 7.1.1.19

**Release:** `7.1.1.19`

## Decision

The patch corrects L2TP runtime generation and adds failure evidence without
weakening external-egress isolation, credential handling or SSH host-key
pinning. No unresolved critical or high-severity finding was identified in the
changed surface.

## Reviewed Surface

- Generated xl2tpd client configuration and managed systemd restart policy.
- Failed L2TP Apply and Probe evidence collection.
- Provider credential redaction in agent job results.
- Control Plane browser SSH runtime dependencies.
- Browser terminal WebSocket lifecycle and reverse-proxy timeouts.

## Security Properties

- Provider passwords, pre-shared keys, private keys and usernames are redacted
  from collected diagnostic output before it is submitted to the Control
  Plane.
- Diagnostic command output is bounded to prevent unbounded job-result growth.
- Evidence collection is read-only and does not alter provider or node state.
- L2TP failure does not change the node main routing table or relax the
  fail-closed provider routing policy.
- Failed managed units are rate-limited instead of consuming resources in a
  tight restart loop.
- Browser SSH still requires an enabled node access method, a one-time
  authenticated session ticket and a pinned SSH host-key fingerprint.
- SSH commands continue to use argv construction and strict known-host
  verification; the patch does not add shell interpolation.
- WebSocket proxy lifetime is extended only for authenticated API traffic and
  does not change public authorization policy.

## Residual Risk

Provider reachability, IKE policy, credentials, NAT behavior and peer-side
allowlists cannot be proven by repository tests. Diagnostic redaction is based
on the encrypted values supplied in the claimed Apply job; operators must still
avoid storing unrelated secrets in provider endpoint or descriptive fields.
Password-based browser SSH uses `sshpass` and should be treated as a migration
path; SSH private-key authentication with least-privilege operator accounts is
preferred.

## Verification

Unit tests cover xl2tpd retry rendering, restart controls, failure
classification and redaction. Full Go tests pass, the changed agent and HTTP
packages pass the race detector, JavaScript and shell syntax checks pass, and
the documentation consistency gate recognizes all `7.1.1.19` release
artifacts. Publication still requires live SSH reachability and provider IKE/
PPP/data-plane evidence on disposable or controlled nodes.
