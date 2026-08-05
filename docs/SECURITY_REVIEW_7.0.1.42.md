# Security and Release Review: 7.0.1.42

**Release:** `7.0.1.42`

## Scope

- Create-from-pack Web UI state handling after successful service-pack creation.
- Operator-facing duplicate-submit prevention for service-pack rollout.
- Release documentation, Web UI asset cache keys and version metadata.

## Changes

- The Create from pack page now stores the submitted draft before queueing the
  API request and reuses it after `refresh()`.
- Submitted node, endpoint, routing, traffic-camouflage and per-component
  settings are preserved in the rendered form after success or validation
  failure.
- A successful create shows a completion banner above the form with created
  instance count and queued apply/runtime-install job counts.
- The submitted form and service-pack selector are disabled after success. The
  only forward actions are opening the instance list or explicitly starting
  another create flow.
- The page-level submit guard rejects duplicate submits while the create flow is
  running or already completed.

## Security Assessment

- No unauthenticated endpoint, new API route or new privileged job type was
  added.
- No database migration, secret format change, node file write path or runtime
  service renderer changed.
- Duplicate-submit prevention reduces accidental duplicate instance creation
  from operator UI interaction, but backend uniqueness and idempotency checks
  remain the authoritative safety boundary.
- The draft state is held only in the in-memory Web UI state. It is not persisted
  to local storage/session storage and is cleared when leaving the create-pack
  page, opening the instance list, switching to manual creation or choosing
  "Create another".
- User-controlled values rendered in the success state continue to pass through
  existing HTML escaping helpers.

## Verification Evidence

- Bundled Node.js syntax check passed for changed Web UI assets:
  `instances-page.js`, `app-state.js`.
- `go test ./...` passed.
- `go vet ./...` passed.
- Full release gate evidence is tracked by `scripts/release-gate.sh` for the
  tagged release commit.
