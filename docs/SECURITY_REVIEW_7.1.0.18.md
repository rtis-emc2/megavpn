# Security and Release Review: 7.1.0.18

**Release:** `7.1.0.18`

## Scope

This review covers the Firewall operator UI hardening after `7.1.0.17`:

- Firewall tab panels now use the full workspace width instead of inheriting the
  narrower bounded stack layout;
- active Firewall tabs use one clear selected style;
- Address Lists operator wording is simplified into Address groups and entries;
- Address groups now show group count in the tab and entry count in the view;
- Node Apply now shows managed node count and uses row-scoped preview/apply
  actions instead of duplicate top-level apply buttons;
- address group and entry tables have wider action columns and clearer
  sectioning;
- Web UI asset cache-key bump to `7.1.0.18`.

## Security Notes

- No firewall backend API, authorization, policy validation, preview/apply job
  schema or nftables rendering behavior changed in this release.
- The UI still requires `firewall.manage` for catalog mutations and
  `firewall.apply` for node preview/apply actions.
- The row-scoped Node Apply actions reduce accidental application to the wrong
  node by keeping the action next to the target node.
- Address group wording is presentation-only; existing API/database names remain
  stable for compatibility.

## Validation

- `node --check web/assets/firewall-page.js`
- `node --check web/assets/app.js`
- `scripts/docs-consistency.sh`
- `go test ./...`
- `test -z "$(gofmt -l cmd internal)"`
- static multi-command SQL scan for production Go runtime paths
- `git diff --check`

## Residual Risk

- Final acceptance still needs visual verification in the deployed browser,
  especially wide desktop layouts with long address values and row actions.
