# Security and Release Review: 7.1.0.1

**Release:** `7.1.0.1`

## Scope

- Firewall operator model and Web UI clarity.
- Firewall documentation and default baseline explanation.
- Release roadmap update for user traffic accounting with 180-day retention.
- Release metadata, Web UI asset cache keys and documentation consistency.

## Changes

- Firewall Overview now renders a catalog-to-apply workflow diagram:
  address lists -> rules -> policy -> apply job -> node state.
- Firewall Rules now renders a compact rule-anatomy guide that shows the order
  in which operators should reason about a rule.
- Firewall documentation now includes a Mermaid model diagram and an explicit
  default node baseline table for seeded production rules.
- The documented next development path now puts audited user traffic accounting
  and retention design first.
- Version metadata, Web UI cache keys, release banners and the documentation
  index security-review links were advanced to `7.1.0.1`.

## Security Assessment

- No unauthenticated endpoint, privileged job type, database migration or node
  nftables runtime behavior was added in this release.
- Existing RBAC boundaries remain unchanged: `firewall.read`,
  `firewall.manage` and `firewall.apply` still separate inspection, catalog
  mutation and node apply.
- The UI change is read-only guidance around existing catalog objects and does
  not bypass policy validation or node-side render validation.
- The documented traffic-accounting direction is intentionally design-first:
  schema, privacy boundary, retention cleanup, RBAC and export audit must be
  specified before collecting production traffic data.
- Default firewall behavior remains governed by the existing seeded `node_base`
  policy, integration tests and agent renderer tests.

## Verification Evidence

- `node --check web/assets/firewall-page.js` is required before tagging.
- `go test ./...` is required before tagging.
- `go vet ./...` is required before tagging.
- `scripts/docs-consistency.sh` is required before tagging.
- Full release gate evidence is tracked by `scripts/release-gate.sh` for the
  tagged release commit.
