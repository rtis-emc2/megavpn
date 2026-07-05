-- Release: 7.0.1.3
-- Keep default OpenVPN service-pack templates full-tunnel by default.

with updated as (
  select
    s.key,
    jsonb_agg(
      case
        when component.value->>'service_code' = 'openvpn'
          then jsonb_set(
            component.value,
            '{spec,server_extra_lines}',
            $json$[
              "push \"redirect-gateway def1 bypass-dhcp\"",
              "push \"dhcp-option DNS 1.1.1.1\"",
              "push \"dhcp-option DNS 1.0.0.1\""
            ]$json$::jsonb,
            true
          )
        else component.value
      end
      order by component.ordinality
    ) as components_json
  from service_pack_templates s
  cross join lateral jsonb_array_elements(s.components_json) with ordinality as component(value, ordinality)
  where s.key in ('default_access_suite', 'openvpn_tcp_11994', 'openvpn_udp_1194')
  group by s.key
)
update service_pack_templates target
set
  components_json = updated.components_json,
  version = target.version + 1,
  updated_at = now()
from updated
where target.key = updated.key
  and target.components_json is distinct from updated.components_json;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
values (
  gen_random_uuid(),
  'system',
  'migration.openvpn_full_tunnel_defaults',
  'service_pack',
  'OpenVPN service-pack defaults updated to full-tunnel',
  '{}'::jsonb,
  now()
);
