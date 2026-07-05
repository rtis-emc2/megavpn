-- Release: 7.0.1.19
-- Repair default service-pack rows that were duplicated under different keys
-- on existing installations.

with raw_default_pack_signatures as (
  select
    t.ctid,
    t.key,
    t.version,
    t.updated_at,
    t.display_order,
    lower(trim(t.label)) as label_key,
    lower(trim(t.base_name_template)) as base_name_key,
    lower(trim(t.endpoint_hint)) as endpoint_hint_key,
    t.requires_endpoint_host,
    coalesce((
      select string_agg(
        concat_ws(':',
          lower(trim(coalesce(component->>'service_code', ''))),
          lower(trim(coalesce(component->>'preset_key', ''))),
          lower(trim(coalesce(component->>'name_suffix', ''))),
          lower(trim(coalesce(component->>'slug_suffix', ''))),
          coalesce(component->>'endpoint_port', ''),
          lower(trim(coalesce(component->'spec'->>'service_profile', '')))
        ),
        '|' order by ordinality
      )
      from jsonb_array_elements(t.components_json) with ordinality as c(component, ordinality)
    ), '') as component_signature
  from service_pack_templates t
  where t.source = 'default'
    and t.status = 'active'
),
default_pack_signatures as (
  select
    *,
    row_number() over (
      partition by
        label_key,
        base_name_key,
        endpoint_hint_key,
        requires_endpoint_host,
        component_signature
      order by
        version desc,
        updated_at desc,
        display_order asc,
        key asc
    ) as rn
  from raw_default_pack_signatures
),
duplicate_default_packs as (
  select ctid, key
  from default_pack_signatures
  where rn > 1
)
update service_pack_templates target
set
  key = left(target.key || '-duplicate-' || substr(md5(target.ctid::text), 1, 8), 96),
  status = 'deleted',
  updated_at = now()
from duplicate_default_packs duplicate
where target.ctid = duplicate.ctid;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
select gen_random_uuid(), 'system', 'migration.service_pack_semantic_dedup', 'service_pack', 'duplicate default service pack templates archived', '{}'::jsonb, now()
where not exists(select 1 from audit_events where action='migration.service_pack_semantic_dedup');
