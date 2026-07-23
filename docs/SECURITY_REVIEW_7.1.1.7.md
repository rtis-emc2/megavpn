# Security and Release Review: 7.1.1.7

**Release:** `7.1.1.7`

## Decision

This release is a required production hotfix for `7.1.1.6`. It restores
heartbeat persistence without weakening authentication.

## Security Changes

- Successful heartbeats still clear stale authorization diagnostics only after
  token and HMAC verification.
- Database failures are no longer mislabeled as missing node identities.
- Internal database errors are logged server-side but are not exposed in signed
  API responses.
- Invalid bearer tokens remain unable to mutate node diagnostics.

## Residual Risk

Production release evidence still requires disposable PostgreSQL integration
and live agent-channel smoke testing. Existing agents retry heartbeat requests
with bounded backoff while the API is being upgraded.

## Verification

Regression tests cover the production non-null schema invariant, missing-node
handling and generic persistence failures. The full Go test suite and
documentation consistency checks pass.
