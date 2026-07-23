# Security and Release Review: 7.1.1.6

**Release:** `7.1.1.6`

## Decision

The recovery change is safe for production promotion. It does not relax token,
request-signature, replay or timestamp validation.

## Security Changes

- Authorization failure state is cleared only inside the successful heartbeat
  path, after token and HMAC signature verification.
- Invalid-token requests remain unable to mutate the claimed node diagnostics.
- Heartbeat and auth diagnostic timestamps now use the same PostgreSQL clock.
- Node and agent recovery state is committed atomically.

## Residual Risk

A holder of a valid node token can still create authorization failures by
sending malformed signed requests. A subsequent valid heartbeat clears that
transient state. Operators must rotate the node identity if token compromise is
suspected.

Reliable NTP remains required on agent, API and PostgreSQL hosts for signed
request validation and accurate operational timestamps.

## Verification

The full Go test suite passes. The PostgreSQL integration regression records an
authorization failure, submits a successful heartbeat and verifies that both
failure fields are cleared.
