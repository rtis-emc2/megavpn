# Client Access Groups

**Release:** `7.1.1.8`

Russian companion: [ACCESS_GROUPS_RU.md](ACCESS_GROUPS_RU.md).

## Purpose

Client access groups are the source of truth for assigning clients to access
policies. Operators manage them only in `Clients -> Groups`.

`Instances` are not the place to manage group members. An instance is a runtime
target: it receives materialized `service_accesses`, applies a revision and
deploys config on the node.

## Data Model

```text
Clients -> Groups
  -> client_access_groups
  -> client_access_group_memberships
  -> service_accesses as runtime projection
  -> instance revision
  -> agent apply
```

| Object | Purpose |
| --- | --- |
| `client_access_groups` | Global access policy: service, key, route/policy, scope. |
| `client_access_group_memberships` | Desired client membership in a group. |
| `service_accesses` | Runtime projection for concrete service instances. |
| `instances` | Listener/runtime that receives materialization through revision/apply. |

## Service Catalog

The group creation form uses `GET /api/v1/client-access-services`. The catalog
shows every known client access service:

| Service | Status |
| --- | --- |
| VLESS / Xray | active; supports groups, membership and materialization. |
| OpenVPN | coming soon/catalog-only in groups UI until materialization is enabled. |
| WireGuard | coming soon/catalog-only in groups UI until materialization is enabled. |
| L2TP/IPsec | coming soon/catalog-only in groups UI until materialization is enabled. |
| HTTP Proxy | coming soon/catalog-only. |
| SOCKS Proxy | planned/catalog-only. |
| Shadowsocks | coming soon/catalog-only. |
| MTProto | coming soon/catalog-only. |

Unsupported services are visible to the operator, but they are never silently
applied. If a service cannot safely support groups, the UI disables creation
and the backend returns a validation error.

## Operator Flow

1. Open `Clients -> Groups`.
2. Pick a service filter or keep `All services`.
3. Create a group only for a service that supports groups.
4. Open `Members`.
5. Load clients with search/status/assignment filters and pagination.
6. Select visible clients, all filtered clients or pasted usernames/emails/client
   IDs.
7. Run `Preview`.
8. Review create/move/skip/fail counts and affected instances.
9. Run `Apply changes`.

`Apply` stays disabled until a successful preview exists. Any selection, paste,
filter or mode change invalidates the preview.

## Runtime Behavior

- VLESS memberships are materialized into every active Xray/VLESS instance
  whose catalog contains the selected group.
- Bulk assignment queues bounded apply jobs by affected instance, not one job
  per client.
- VLESS UUIDs are preserved when moving a client between groups because
  credential identity is stored separately from runtime projection.
- New matching VLESS instances receive existing memberships through group
  materialization/sync.
- Instance detail may show applied access groups, materialized members,
  sync/apply state and links to `Clients -> Groups`, but it must not add
  members directly.

## Security And Audit

- Group create/update/member changes require the relevant RBAC permissions.
- Preview must not mutate PostgreSQL.
- Apply revalidates the payload server-side.
- Unsupported services fail closed.
- Duplicate `service_accesses` are not created.
- Audit trail must answer who changed a group, who changed membership, which
  instances were affected and which jobs applied runtime state.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Service is visible, but a group cannot be created | The service is catalog-only or planned. Use VLESS or wait for materialization. |
| Apply is unavailable | Run Preview first, then keep selection/paste/filter unchanged before Apply. |
| Client is not present in runtime | Check affected instances in preview/apply result, sync state and `instance.apply` jobs. |
| Client kept the old UUID | Expected: VLESS UUID is a stable service identity and is preserved during moves. |
