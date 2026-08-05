# Security and Release Review: 7.1.1.4

**Release:** `7.1.1.4`

## Decision

The patch keeps agent communication fail-closed. Unsigned success and error
responses remain rejected; no compatibility bypass was introduced.

## Security Changes

- Agent API errors are signed before leaving the control plane, including
  failures caused by stale or revoked agent identity.
- The response signature uses the bearer token already presented over the
  authenticated TLS channel. Tokens are not returned in response bodies or
  logs.
- Unsigned-response diagnostics include only status, content type and a
  whitespace-normalized response preview capped at 256 bytes.
- Automatic re-enrollment is intentionally not attempted. Identity recovery
  requires the existing SSH trust path and a new one-time enrollment token.
- A stale database `agent_status=online` value no longer overrides heartbeat
  freshness in operator-visible node and job diagnostics.

## Residual Risk

Reverse proxies must preserve the `X-MegaVPN-Agent-*` response headers. A proxy
that strips them is rejected by the agent and now produces an actionable status
diagnostic. Recovery of an invalid agent identity still requires an available,
host-key-pinned SSH bootstrap channel.

## Verification

Regression coverage verifies signed `401` responses, rejection of unsigned
responses with bounded diagnostics, signed no-content responses, replay
rejection and stale-heartbeat UI projection.
