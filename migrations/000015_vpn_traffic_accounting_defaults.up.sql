-- Enable OpenVPN/WireGuard traffic accounting on default service-pack templates.
-- Existing instances still need apply/re-apply so managed OpenVPN status files
-- and WireGuard peer attribution comments are rendered on the node.

with updated as (
  select
    s.key,
    jsonb_agg(
      case
        when component.value->>'service_code' in ('openvpn','wireguard') then
          jsonb_set(component.value, '{spec,traffic_accounting_enabled}', 'true'::jsonb, true)
        else component.value
      end
      order by component.ordinality
    ) as components_json
  from service_pack_templates s
  cross join lateral jsonb_array_elements(s.components_json) with ordinality as component(value, ordinality)
  where s.source = 'default'
    and s.status <> 'deleted'
    and s.key in (
      'default_access_suite',
      'openvpn_tcp_11994',
      'openvpn_udp_1194',
      'wireguard_roadwarrior'
    )
  group by s.key
)
update service_pack_templates target
set components_json = updated.components_json,
    version = target.version + 1,
    updated_at = now()
from updated
where target.key = updated.key
  and target.components_json is distinct from updated.components_json;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
select gen_random_uuid(), 'system', 'migration.vpn_traffic_accounting_defaults', 'service_pack',
       'openvpn and wireguard traffic accounting enabled on default service-pack templates',
       '{}'::jsonb, now()
where not exists(select 1 from audit_events where action='migration.vpn_traffic_accounting_defaults');
