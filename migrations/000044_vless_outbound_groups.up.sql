with defaults as (
  select '[{"key":"default","label":"Default direct","outbound_tag":"direct"}]'::jsonb as groups
),
updated as (
  select
    s.key,
    jsonb_agg(
      case
        when component.value->>'service_code' = 'xray-core' then
          jsonb_set(
            jsonb_set(component.value, '{spec,default_vless_group}', '"default"'::jsonb, true),
            '{spec,vless_groups}',
            defaults.groups,
            true
          )
        else component.value
      end
      order by component.ordinality
    ) as components_json
  from service_pack_templates s
  cross join defaults
  cross join lateral jsonb_array_elements(s.components_json) with ordinality as component(value, ordinality)
  where s.key in (
    'default_access_suite',
    'xray_vless_reality',
    'xray_nginx_grpc_edge',
    'xray_nginx_http_edge'
  )
  group by s.key
)
update service_pack_templates target
set components_json = updated.components_json,
    version = target.version + 1,
    updated_at = now()
from updated
where target.key = updated.key;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
values (gen_random_uuid(), 'system', 'migration.vless_outbound_groups', 'service_pack', 'vless outbound groups added to xray service pack templates', '{}'::jsonb, now());
