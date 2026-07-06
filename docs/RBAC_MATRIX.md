# RBAC Matrix

**Release:** `7.1.0.1`

## Seeded Roles

| Role | Intent |
| --- | --- |
| `readonly` | Read operational state and audit logs. |
| `engineer` | Manage clients, artifacts and share links; no node/bootstrap/apply authority. |
| `admin` | Operate nodes, instances, jobs and platform settings except unrestricted auth/secret reveal. |
| `superadmin` | Full permission set. |

## Permissions

| Permission | Scope | Typical API Surface |
| --- | --- | --- |
| `dashboard.read` | Global | Dashboard summary |
| `service.read` | Global | Service catalog |
| `node.read` | Node | Nodes, diagnostics, inventory, route-policy preview |
| `node.write` | Node | Node metadata, capability install/verify, backhaul, route-policy apply |
| `node.bootstrap` | Node | SSH/manual bootstrap, agent token rotation |
| `instance.read` | Instance | Instance list/detail/revisions |
| `instance.write` | Instance | Desired spec/revision edits |
| `instance.apply` | Instance | Apply/restart/start/stop/enable/disable |
| `client.read` | Client | Client accounts/access |
| `client.write` | Client | Client account mutation |
| `client.provision` | Client | Provision/revoke access and rotate/revoke VLESS subscriptions |
| `artifact.read` | Artifact | Artifact list/download/preview |
| `artifact.export` | Artifact | Artifact build jobs |
| `share_link.manage` | Artifact | Publish/revoke public share links |
| `job.read` | Job | Job list/detail/logs |
| `job.write` | Job | Non-privileged direct jobs only |
| `job.cancel` | Job | Cancel pending/running jobs |
| `audit.read` | Audit | Audit event list |
| `secret.reveal` | Secret | Explicit secret reveal operations |
| `settings.manage` | Global | Platform TLS/mail/settings |
| `auth.manage` | Global | Users, roles, invites |
| `endpoint.read` | Endpoint | Virtual endpoint read |
| `endpoint.write` | Endpoint | Virtual endpoint mutation |

## Privileged Job Rules

The generic `POST /api/v1/jobs` endpoint is not an operator escape hatch. These job types must be created through typed endpoints:

| Job Type | Required Typed Authority |
| --- | --- |
| `platform.control_plane_tls.apply` | `settings.manage` |
| `node.bootstrap` | `node.bootstrap` |
| `node.agent.rotate_token` | `node.bootstrap` |
| `node.backhaul.apply` / `probe` / `cleanup` | `node.write` |
| `node.route_policy.apply` / `cleanup` | `node.write` |
| `node.capability.install` / `verify` | `node.write` |
| `instance.apply` / lifecycle actions | `instance.apply` |

Remaining direct job types are mapped by job type before enqueue. A role with `job.write` alone must not be able to operate nodes, instances, route policy or capability installation.

## Release Review Checklist

- Confirm no production role grants `secret.reveal` except break-glass `superadmin`.
- Confirm `engineer` cannot bootstrap nodes, install capabilities or apply instance revisions.
- Confirm `readonly` can read audit logs but cannot create jobs.
- Review audit logs after every RBAC migration.
