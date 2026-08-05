-- Remove the superseded VLESS template and membership registries. The
-- client_access_group* tables are the only desired-state source of truth;
-- service_accesses remains the per-instance runtime projection.

insert into client_access_groups(
  id, service_code, group_key, display_name, description, status, policy_json,
  scope_mode, auto_apply_new_instances, created_at, updated_at, deleted_at
)
select
  gen_random_uuid(),
  'vless',
  vgt.key,
  vgt.label,
  coalesce(vgt.description, ''),
  case when vgt.status = 'disabled' then 'disabled' else 'active' end,
  jsonb_build_object(
    'access_mode', vgt.access_mode,
    'egress_mode', vgt.egress_mode,
    'egress_node_id', coalesce(vgt.egress_node_id::text, ''),
    'target_instance_id', coalesce(vgt.target_instance_id::text, ''),
    'outbound_tag', vgt.outbound_tag,
    'ad_block', vgt.ad_block,
    'rules', vgt.rules_json,
    'extra_rules', vgt.extra_rules_json
  ),
  'all_active_instances',
  true,
  vgt.created_at,
  vgt.updated_at,
  null
from vless_group_templates vgt
where vgt.status in ('active', 'disabled')
on conflict(service_code, group_key) where deleted_at is null do nothing;

-- Preserve a legacy membership only when the canonical registry does not
-- already own an active VLESS assignment. A current canonical choice always
-- wins over an older materialized service-access projection.
with legacy_candidates as (
  select
    vgm.client_account_id,
    vgm.group_key,
    'active'::text as status,
    vgm.metadata_json || jsonb_build_object('retired_source', 'vless_group_memberships') as metadata_json,
    vgm.created_at,
    vgm.updated_at,
    0 as source_priority
  from vless_group_memberships vgm
  where vgm.status = 'active'
  union all
  select
    sa.client_account_id,
    coalesce(
      nullif(sa.metadata_json->>'vless_group',''),
      nullif(sa.metadata_json->>'xray_group',''),
      nullif(sa.metadata_json->>'outbound_group',''),
      nullif(sa.metadata_json->'inbound_service'->>'vless_group','')
    ) as group_key,
    'active'::text,
    jsonb_build_object('retired_source', 'service_accesses', 'service_access_id', sa.id::text),
    sa.created_at,
    sa.updated_at,
    1 as source_priority
  from service_accesses sa
  where sa.status in ('pending', 'active', 'disabled')
),
ranked as (
  select lc.*,
    row_number() over(
      partition by lc.client_account_id
      order by lc.updated_at desc, lc.source_priority asc, lc.created_at desc, lc.group_key asc
    ) as rn
  from legacy_candidates lc
  join client_access_groups cag
    on cag.service_code = 'vless'
   and cag.group_key = lc.group_key
   and cag.deleted_at is null
  where nullif(lc.group_key, '') is not null
    and not exists (
      select 1
      from client_access_group_memberships current_membership
      where current_membership.client_account_id = lc.client_account_id
        and current_membership.service_code = 'vless'
        and current_membership.status = 'active'
    )
)
insert into client_access_group_memberships(
  id, client_account_id, service_code, group_id, status, source,
  metadata_json, created_at, updated_at, removed_at
)
select
  gen_random_uuid(),
  ranked.client_account_id,
  'vless',
  cag.id,
  ranked.status,
  'migration:legacy_vless_retirement',
  ranked.metadata_json,
  ranked.created_at,
  ranked.updated_at,
  null
from ranked
join client_access_groups cag
  on cag.service_code = 'vless'
 and cag.group_key = ranked.group_key
 and cag.deleted_at is null
where ranked.rn = 1
on conflict(client_account_id, service_code) where status = 'active' do nothing;

-- The old compatibility marker must not survive as part of the public policy.
update client_access_groups
set policy_json = policy_json - 'legacy_vless_template',
    updated_at = now()
where service_code = 'vless'
  and policy_json ? 'legacy_vless_template';

insert into audit_events(
  id, actor_type, action, resource_type, summary, payload_json, created_at
)
values(
  gen_random_uuid(),
  'system',
  'migration.legacy_vless_registry_retired',
  'access_group',
  'legacy VLESS registries consolidated into client access groups',
  jsonb_build_object(
    'legacy_templates', (select count(*) from vless_group_templates),
    'legacy_memberships', (select count(*) from vless_group_memberships),
    'recorded_conflicts', (select count(*) from client_access_group_migration_conflicts),
    'conflict_sample', coalesce((
      select jsonb_agg(sample_row)
      from (
        select jsonb_build_object(
          'client_account_id', client_account_id,
          'instance_id', instance_id,
          'service_access_id', service_access_id,
          'service_code', service_code,
          'group_key', group_key,
          'reason', reason
        ) as sample_row
        from client_access_group_migration_conflicts
        order by created_at asc, id asc
        limit 100
      ) conflicts
    ), '[]'::jsonb)
  ),
  now()
);

drop table if exists client_access_group_migration_conflicts;
drop table if exists vless_group_memberships;
drop table if exists vless_group_templates;

-- Logical access protocols evolve independently from runtime service
-- definitions. Enforce a stable identifier shape instead of a stale enum.
alter table client_access_groups
  drop constraint if exists client_access_groups_service_code_check;
alter table client_access_groups
  add constraint client_access_groups_service_code_check
  check(service_code ~ '^[a-z][a-z0-9_-]{0,63}$');
