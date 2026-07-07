# Security and Release Review: 7.1.0.29

**Release:** `7.1.0.29`

## Scope

- Makes service-pack instance rollout idempotent for repeated operator submit.
- If a component already exists for the same node, endpoint host, service code
  and endpoint port, the API reuses that instance instead of creating a suffixed
  duplicate.
- Reused rows are returned as `existing_instances`.
- A fully existing pack returns `status=already_exists` with HTTP 200.
- New tests cover canonical Xray WebSocket component reuse when a duplicate
  `-2` style instance is also present.

## Security Notes

- Prevents accidental duplicate runtime listeners on the same endpoint, which
  could otherwise create port conflicts, ambiguous client provisioning targets
  and unsafe operational drift.
- Does not grant additional permissions; the existing `instance.write`
  permission still gates service-pack rollout.
- Does not mutate existing instances or secrets when a component is reused.
- Existing API redaction semantics are unchanged.

## Validation

- `go test ./internal/api/http -count=1`
- `go test ./...`
- `scripts/docs-consistency.sh`
- `git diff --check`
- SQL prepared-statement guard over `cmd` and `internal` remained clean.

## Residual Risk

- Existing duplicate instances created before this release are not deleted
  automatically; operators should remove the accidental suffixed duplicate
  through normal delete/force-delete lifecycle after confirming it is not used.
- If two intentionally separate endpoints share a port but use different
  endpoint hosts, this fix does not collapse them.
