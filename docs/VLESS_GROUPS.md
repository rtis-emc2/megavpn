# VLESS Access Groups

**Release:** `7.1.0.12`

Russian companion: [VLESS_GROUPS_RU.md](VLESS_GROUPS_RU.md).

## Purpose

VLESS access groups are reusable client-routing templates. They are configured
once under `Instances -> VLESS groups` and then selected during client
provisioning. This keeps VLESS client policy out of individual instance forms
and makes routing behavior auditable.

A VLESS instance still owns the listener, certificate/Reality settings and
default egress path. A VLESS group decides what a specific client binding is
allowed to do on top of that instance.

## Architecture

```text
Operator
  -> Instances / VLESS groups
  -> vless_group_templates table
  -> Xray instance revision renderer
  -> node agent apply
  -> generated Xray routing rules
  -> client provisioning bindings
```

The Control Plane stores group templates centrally. When an Xray/VLESS instance
revision is created or updated, the active group catalog is embedded into the
rendered instance spec. During apply, the driver converts groups into Xray
routing rules scoped by client user/email.

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

1. Open `Instances`.
2. Select `VLESS groups`.
3. Create or edit reusable groups.
4. For `Selected egress node`, choose an egress node and ensure a working
   backhaul route exists.
5. For `Only selected instance`, choose a target service instance with a valid
   endpoint host and port.
6. Save the group.
7. Review the sync result. Any failed instance is reported with the stage and
   validation error.
8. Open the target VLESS instance in `Instances -> Manage`.
9. Select `Default VLESS group` if a default other than `Default access` is
   required.
10. During client provisioning, select the appropriate group for each VLESS
    inbound.

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
- Client provisioning stores the selected group key on the client access
  binding.
- Reprovisioning and Xray UUID rotation preserve the existing client binding
  group. Empty group input never becomes a synthetic `route` group; stale
  implicit metadata falls back to an active catalog/default group, while an
  explicitly selected invalid group remains a validation error.
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
- which VLESS instance revision contains the group catalog;
- which client bindings use a given group key;
- which apply job rendered and deployed the effective Xray config.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Group is not visible during provisioning | Confirm it is active and refresh core data. |
| Changed group has no runtime effect | Check the group mutation response for sync failures, then inspect the queued `instance.apply` job for that instance. |
| Target-only group fails validation | Verify the target instance endpoint host and port. |
| Remote egress is not used | Verify instance egress mode, selected egress node, active backhaul and route-policy sync. The ingress node should have a `node.route_policy.apply` result with an active `xray_vless_remote_egress` system route, an `inet megavpn route_policy_output` mark rule and an `ip rule fwmark <mark> table <backhaul_table>` kernel rule. |
| Ad blocking has no effect | Verify Xray geosite data and generated routing rules. |
