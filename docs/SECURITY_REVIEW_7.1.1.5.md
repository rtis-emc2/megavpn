# Security and Release Review: 7.1.1.5

**Release:** `7.1.1.5`

## Decision

The patch preserves the five-minute bidirectional HMAC timestamp window and
adds an earlier two-minute SSH bootstrap guard. No replay, signature or token
validation control is relaxed.

## Security Changes

- Valid node identities receive actionable clock-skew diagnostics when their
  signed requests fall outside the accepted timestamp window.
- Failed requests with an invalid bearer token are logged and rejected but do
  not mutate the claimed node's operator-visible authentication state.
- The worker reads remote UTC through the existing host-key-pinned SSH channel
  before issuing a one-time enrollment token.
- Enrollment material is not created for an SSH bootstrap that is already known
  to fail the clock-synchronization precondition.
- The agent treats a timestamp from a response with failed verification as
  diagnostic evidence only. It never adjusts local time or accepts the response.

## Residual Risk

Manual bundle installation cannot run the worker-side SSH clock preflight.
Operators must maintain NTP on those nodes. Worker and API hosts must also share
reliable time synchronization when deployed separately. A compromised host can
still report arbitrary local time, but it cannot bypass HMAC verification or
the server timestamp window.

## Verification

Go regression tests cover unsafe and synchronized clock offsets, invalid remote
timestamps, request and response diagnostics, and protection against invalid
tokens changing node diagnostics. The standard self-test and release gates
remain required before production promotion.
