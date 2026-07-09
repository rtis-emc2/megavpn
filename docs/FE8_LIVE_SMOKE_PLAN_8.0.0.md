# FE8 8.0.0 Live Smoke Plan

Branch: `release/8.0.0-frontend-console`

Generated UTC: `2026-07-09T09:57:57Z`

Status: **OPEN**.

Final cutover impact: live disposable API/DB/node evidence is required before
8.0.0 can move from NO-GO to GO. This document is a plan, not execution
evidence.

## Required Environment

| Variable or resource | Required value |
| --- | --- |
| `MEGAVPN_RELEASE_BASE_URL` or `MEGAVPN_PUBLIC_BASE_URL` | Disposable control-plane base URL reachable from the runner. |
| `MEGAVPN_RELEASE_DATABASE_DSN` | Disposable primary PostgreSQL DSN for migrations and integration checks. |
| `MEGAVPN_RELEASE_RESTORE_DATABASE_DSN` | Disposable restore PostgreSQL DSN for backup/restore drill. |
| `MEGAVPN_RELEASE_NODE_ID` | Disposable node id safe for firewall, route and service operations. |
| `MEGAVPN_RELEASE_ENDPOINT_DOMAIN` | Disposable endpoint domain for service-pack and VPN smoke. |
| `MEGAVPN_RELEASE_CERTIFICATE_ID` | Disposable certificate id when service smoke requires TLS material. |
| `MEGAVPN_RELEASE_RUN_SERVICE_MATRIX=1` | Enables the VPN/service matrix gate. |
| `MEGAVPN_RELEASE_REQUIRE_AGENT_REPORT=1` | Requires agent evidence in service-pack smoke. |

## Disposable Data Requirements

- Test users must have RBAC permissions for all migrated workflows.
- The database must be disposable and restorable from backup.
- The node must be isolated from production traffic and safe for firewall apply,
  emergency disable, route policy apply and runtime service jobs.
- Endpoint domains, certificates, enrollment tokens, share links and
  subscription URLs must be disposable and revocable.
- Any generated client configs, private keys, invite secrets, terminal tickets
  and one-time URLs must be treated as secrets and removed after the run.

## Command Sequence

1. Export the required environment variables listed above.
2. Confirm the release SHA is pushed and selected for the smoke run.
3. Run:

```bash
MEGAVPN_RELEASE_RUN_SERVICE_MATRIX=1 \
MEGAVPN_RELEASE_REQUIRE_AGENT_REPORT=1 \
scripts/ci/release-gate.sh
```

4. Run focused smoke scripts where the release gate does not cover browser
   workflow evidence:

```bash
MEGAVPN_PUBLIC_BASE_URL="$MEGAVPN_RELEASE_BASE_URL" scripts/smoke/api-smoke.sh
MEGAVPN_PUBLIC_BASE_URL="$MEGAVPN_RELEASE_BASE_URL" scripts/smoke/vless-client-access-groups-smoke.sh
MEGAVPN_PUBLIC_BASE_URL="$MEGAVPN_RELEASE_BASE_URL" scripts/smoke/node-inventory-smoke.sh "$MEGAVPN_RELEASE_NODE_ID"
MEGAVPN_PUBLIC_BASE_URL="$MEGAVPN_RELEASE_BASE_URL" scripts/smoke/service-discovery-smoke.sh "$MEGAVPN_RELEASE_NODE_ID"
MEGAVPN_PUBLIC_BASE_URL="$MEGAVPN_RELEASE_BASE_URL" scripts/smoke/capability-install-smoke.sh "$MEGAVPN_RELEASE_NODE_ID"
MEGAVPN_PUBLIC_BASE_URL="$MEGAVPN_RELEASE_BASE_URL" scripts/smoke/agent-identity-smoke.sh "$MEGAVPN_RELEASE_NODE_ID"
MEGAVPN_PUBLIC_BASE_URL="$MEGAVPN_RELEASE_BASE_URL" scripts/smoke/service-pack-smoke.sh --matrix "$MEGAVPN_RELEASE_NODE_ID" "$MEGAVPN_RELEASE_ENDPOINT_DOMAIN" "$MEGAVPN_RELEASE_CERTIFICATE_ID"
```

5. Capture browser evidence for each migrated workflow against the same
   disposable environment.
6. Archive command logs, CI links, screenshots, rollback notes and cleanup
   evidence with the final release candidate.

## Workflow Coverage

- Login/session and `/` plus `/legacy/` serving.
- VLESS group create/edit/scope/member preview/apply/remove/sync.
- Firewall address group, policy and rule CRUD.
- Firewall preview, apply, node state and emergency disable on disposable node.
- Client create/status/revoke/delete.
- Client artifact build/download/delete.
- Client share, subscription and email delivery.
- Client route create/delete, access rotation/delete and config cleanup.
- Instance lifecycle, apply/reapply, rollback, diagnostics and delete.
- Service pack create/update/enable/disable/delete and create instance from
  pack.
- Manual instance create and instance spec replace.
- Runtime artifact URL import.
- Node diagnostics, inventory, capabilities, discovery, bootstrap, security and
  control.
- Certificates/PKI import preview/apply, self-signed, managed CA,
  issue-from-CA, default, revoke/delete and PKI root create.
- Platform settings save/apply, mail settings/test, users, invites and
  sessions.
- Backhaul apply/probe/promote/route projection.
- Route Policy preview/apply/cleanup on disposable topology.

## Rollback And Cleanup

1. Keep `/legacy/` available throughout smoke.
2. Revoke all test sessions, invites, enrollment tokens, share links and
   subscriptions.
3. Delete or revoke disposable certificates and client artifacts.
4. Return firewall, route policy and service runtime state to the pre-smoke
   baseline.
5. Destroy disposable databases and nodes after evidence is archived.

## Current Blocker

This workstation session did not provide the required disposable API, DB, node,
endpoint domain or certificate inputs. Live smoke remains OPEN and final 8.0.0
cutover remains NO-GO.
