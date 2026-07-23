# VLESS Access Groups

**Release:** `7.1.1.16`

Russian companion: [VLESS_GROUPS_RU.md](VLESS_GROUPS_RU.md).

## Purpose

VLESS access groups are reusable client access groups for client routing. The
source of truth is `Clients -> Groups`: a group is assigned to a client once,
and runtime instances receive materialized `service_accesses` as a projection.
This keeps VLESS client policy out of individual instance forms and makes
routing behavior auditable.

A VLESS group is global. Client membership is stored once and is materialized
into every active Xray/VLESS instance that exposes that group. A VLESS instance
still owns the listener, certificate/Reality settings and default egress path;
the group decides what a client is allowed to do across the fleet.

## Architecture

```text
Operator
  -> Clients / Groups
  -> client_access_groups table
  -> client_access_group_memberships table
  -> materialized service_accesses per Xray/VLESS instance
  -> Xray instance revision renderer
  -> node agent apply
  -> generated Xray routing rules
```

The Control Plane stores group policy and membership centrally. Legacy
`vless_group_templates` remains as a compatibility catalog for one transition
release, but new membership operations use generic `client_access_groups`. When
an Xray/VLESS instance revision is created or updated, the active group catalog
is embedded into the rendered instance spec. During apply, the driver converts
groups into Xray routing rules scoped by client user/email.

Saving, disabling or deleting a global VLESS group also runs a catalog sync for
existing Xray/VLESS instances. The sync creates a validated instance revision
with the current catalog and queues `instance.apply` for active instances, so
remote nodes receive the new routing config without a manual instance edit.

## Modes

| Mode | Result |
| --- | --- |
| `Instance default route` | Client follows the instance-level default outbound. Use this for standard remote egress through managed backhaul. |
| `Current node exit` | Client exits from the node that accepted the VLESS connection. Use only when local breakout is intentional. |
| `Selected egress node` | Group requires a specific egress node. Apply must resolve that choice through managed routing/backhaul. |
| `Only selected instance` | Client may reach only the selected service instance endpoint. A managed allow rule is generated and the remaining traffic is blocked. |
| `Block all traffic` | Client traffic is denied. Use for quarantine, suspension or staged provisioning. |

`Block managed ad domains` adds an Xray rule for the managed advertising domain
set before the final group outbound. The Xray runtime must have compatible
domain data installed.

## Operator Flow

### Group catalog

1. Open `Clients`.
2. Select `Groups`.
3. Create or edit reusable VLESS access groups.
4. For `Selected egress node`, choose an egress node and ensure a working
   backhaul route exists.
5. For `Only selected instance`, choose a target service instance with a valid
   endpoint host and port.
6. Save the group.
7. Review the sync result. Any failed instance is reported with the stage and
   validation error.
8. Open the target VLESS instance in `Instances -> Manage` only when you need
   to inspect runtime materialization or re-apply the instance.
9. Select `Default VLESS group` if an instance-level fallback other than
   `Default access` is required.
10. Client provisioning can still select a group for an inbound, but the primary
    bulk workflow should use `Clients -> Groups`.

### Bulk membership

For fleet operations, operators do not need to open every client:

1. Open `Clients -> Groups`.
2. Click `Members` on the target group.
3. Search clients with server-side pagination. Page size is operator
   controlled; the UI no longer assumes a hidden fixed limit.
4. Choose the membership scope:
   - selected clients on the current page;
   - all clients matching the current filter;
   - pasted usernames, emails or client IDs;
   - any combination of the above.
5. Choose `Add only` when existing members must stay in their current group, or
   `Add or move` when existing members may be moved into the selected group.
6. Run `Preview changes` and review the dry-run result:
   `will create`, `will move`, `will skip`, `will fail` and `apply job count`.
7. Run `Apply previewed changes`.

The Control Plane writes the desired membership to
`client_access_group_memberships`, then creates or updates the materialized
`service_accesses` for every active Xray/VLESS instance whose catalog exposes
the selected group. Adding the same client to the same group is idempotent,
moving a client between groups preserves the VLESS UUID, and the bulk update
queues bounded `instance.apply` jobs by affected instance, not one apply job per
client.

The legacy `Instances -> VLESS groups` and `Instances -> Manage -> Applied
client access groups` sections remain read-only/compatibility entry points.
They show catalog/materialization and link to `Clients -> Groups`; this does
not mean membership belongs to a concrete instance.

Client configs and artifacts are not rebuilt one-by-one from this bulk flow.
After membership changes, use the normal build/subscription workflow when
delivery artifacts must be refreshed.

## Validation Rules

- Group keys are stable identifiers and must remain lowercase-safe.
- `Selected egress node` requires an egress node.
- `Only selected instance` requires a target instance or explicit advanced
  rules.
- Deleted groups are removed from future rendered revisions and the catalog
  sync queues apply for active Xray/VLESS instances.
- Disabled groups remain in the catalog for audit and rollback context, but are
  not offered as active provisioning choices.
- Advanced route rules must be a JSON array of Xray-compatible field rules.

## Runtime Behavior

- Group data is copied into an instance revision at instance save/create time
  during global catalog sync and on demand during client provisioning. This
  prevents a freshly created active group from being visible in the provisioning
  UI while missing from the selected Xray instance revision.
- Bulk membership stores one global desired group per client in
  `client_access_group_memberships`. The Control Plane materializes that
  desired state into `service_accesses` for all matching active Xray/VLESS
  instances and for new matching instances created later.
- Client provisioning can still create a direct VLESS access binding for the
  selected inbound. If a global membership exists, materialization keeps the
  instance bindings aligned with that global group.
- `VLESS group members` does not create duplicate rows. The client VLESS
  identity is kept in `client_service_identities` and reused when ingress nodes
  change or when the client is moved between groups.
- Reprovisioning preserves the existing client binding group. Xray UUID
  rotation is profile-wide: rotating one VLESS access updates the shared
  `client_service_identities` UUID and marks all active/pending Xray accesses
  for the same client identity profile as `pending`, so every affected ingress
  is re-applied with the new credential. Empty group input never becomes a
  synthetic `route` group; stale implicit metadata falls back to an active
  catalog/default group, while an explicitly selected invalid group remains a
  validation error.
- Provisioning validates the chosen group after catalog sync. If a group is not
  active or selected egress cannot be resolved through active backhaul, the API
  returns the available group keys and the blocking resolution error.
- Apply renders Xray routing rules per client user/email.
- When instance or group remote egress resolves to a managed backhaul, the
  driver writes Xray `freedom.sendThrough` with the ingress-side backhaul
  address. `node.route_policy.apply` also publishes a system route for that
  source address so locally generated Xray traffic leaves through the selected
  egress node instead of the ingress node default route.
- When the active managed backhaul transport changes, the Control Plane
  refreshes affected Xray instance revisions before route-policy apply. This
  keeps the Xray outbound `sendThrough`, the selected backhaul interface and
  the kernel policy route in the same convergence cycle.
- Before applying route policy, open the ingress node diagnostics and run
  `Inspect route policy`. The preview shows whether the VLESS/Xray system
  route is active, which managed backhaul table/interface will be used, and any
  blocked reason. VLESS UUID-like source identities are redacted in this preview
  because they are credential-like values.
- A client binding that references a deleted or unknown group falls back to the
  instance default group during render.
- `Only selected instance` generates an allow rule for the target endpoint and
  a final block rule for all other traffic in that group.
- Instance-level egress still decides the default route for `Default access`.

## Risks

| Risk | Control |
| --- | --- |
| Client unexpectedly exits from the ingress node | Use `Instance default route` plus instance-level remote egress, or force `Selected egress node`. |
| Stale group after edit | Save/status/delete runs catalog sync and queues apply; reprovision or rotate preserves the client binding group when it still exists and falls back only for stale implicit metadata. |
| Broken target-only group | Target instance must have endpoint host and port; otherwise revision validation fails. |
| Missing ad-block data | Keep Xray geosite data installed with the runtime artifact/package. |
| Overly broad advanced JSON | Keep advanced rules collapsed by default and reserve them for reviewed exceptions. |

## Audit Evidence

Operators should be able to answer:

- who created, edited, disabled or deleted a group;
- which clients are globally assigned to a group;
- which VLESS instance revision contains the group catalog;
- which materialized client bindings use a given group key;
- which apply job rendered and deployed the effective Xray config.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Group is not visible during provisioning | Confirm it is active and refresh core data. |
| Changed group has no runtime effect | Check the group mutation response for sync failures, then inspect the queued `instance.apply` job for that instance. |
| Target-only group fails validation | Verify the target instance endpoint host and port. |
| Remote egress is not used | Verify instance egress mode, selected egress node, active backhaul and route-policy sync. The ingress node should have a `node.route_policy.apply` result with an active `xray_vless_remote_egress` system route, an `inet megavpn route_policy_output` mark rule and an `ip rule fwmark <mark> table <backhaul_table>` kernel rule. |
| Ad blocking has no effect | Verify Xray geosite data and generated routing rules. |
