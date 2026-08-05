-- Release: 7.1.1.0
-- Generic client-centric access groups. Service-specific runtime rows in
-- service_accesses remain materialized projections, not the source of truth.

alter table permissions
  drop constraint if exists permissions_scope_type_check;

alter table permissions
  add constraint permissions_scope_type_check
  check(scope_type in ('global','node','instance','client','artifact','secret','job','audit','endpoint','traffic','access_group'));

create table if not exists client_access_groups (
  id uuid primary key,
  service_code text not null,
  group_key text not null,
  display_name text not null,
  description text not null default '',
  status text not null default 'active',
  policy_json jsonb not null default '{}'::jsonb,
  scope_mode text not null default 'all_active_instances',
  auto_apply_new_instances boolean not null default true,
  created_by uuid null references platform_users(id) on delete set null,
  updated_by uuid null references platform_users(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz null,
  constraint client_access_groups_service_code_check
    check(service_code in ('vless','openvpn','l2tp','wireguard','shadowsocks','http_proxy','mtproto')),
  constraint client_access_groups_group_key_check
    check(group_key ~ '^[a-z0-9][a-z0-9_.:-]{0,63}$'),
  constraint client_access_groups_status_check
    check(status in ('active','disabled','deleted')),
  constraint client_access_groups_scope_mode_check
    check(scope_mode in ('all_active_instances','selected_instances','all_except_selected')),
  constraint client_access_groups_policy_object_check
    check(jsonb_typeof(policy_json) = 'object')
);

create unique index if not exists idx_client_access_groups_service_key_active
  on client_access_groups(service_code, group_key)
  where deleted_at is null;

create index if not exists idx_client_access_groups_service_status
  on client_access_groups(service_code, status, updated_at desc);

create table if not exists client_access_group_memberships (
  id uuid primary key,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  service_code text not null,
  group_id uuid not null references client_access_groups(id) on delete restrict,
  status text not null default 'active',
  source text not null default 'manual',
  metadata_json jsonb not null default '{}'::jsonb,
  created_by uuid null references platform_users(id) on delete set null,
  updated_by uuid null references platform_users(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  removed_at timestamptz null,
  constraint client_access_group_memberships_service_code_check
    check(service_code in ('vless','openvpn','l2tp','wireguard','shadowsocks','http_proxy','mtproto')),
  constraint client_access_group_memberships_status_check
    check(status in ('active','disabled','revoked','removed'))
);

create unique index if not exists idx_client_access_group_memberships_one_active_per_service
  on client_access_group_memberships(client_account_id, service_code)
  where status = 'active';

create index if not exists idx_client_access_group_memberships_group_status
  on client_access_group_memberships(group_id, status, updated_at desc);

create index if not exists idx_client_access_group_memberships_client_status
  on client_access_group_memberships(client_account_id, service_code, status);

create table if not exists client_access_group_instance_scopes (
  group_id uuid not null references client_access_groups(id) on delete cascade,
  instance_id uuid not null references instances(id) on delete cascade,
  mode text not null check(mode in ('include','exclude')),
  created_at timestamptz not null default now(),
  primary key(group_id, instance_id, mode)
);

create index if not exists idx_client_access_group_instance_scopes_instance
  on client_access_group_instance_scopes(instance_id, mode);

create table if not exists client_access_group_sync_state (
  group_id uuid not null references client_access_groups(id) on delete cascade,
  instance_id uuid not null references instances(id) on delete cascade,
  desired_hash text not null,
  last_applied_hash text null,
  status text not null default 'pending',
  last_job_id uuid null references jobs(id) on delete set null,
  last_error text not null default '',
  updated_at timestamptz not null default now(),
  primary key(group_id, instance_id),
  constraint client_access_group_sync_state_status_check
    check(status in ('pending','queued','applied','failed','skipped'))
);

create table if not exists client_access_group_migration_conflicts (
  id uuid primary key,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  instance_id uuid null references instances(id) on delete set null,
  service_access_id uuid null references service_accesses(id) on delete set null,
  service_code text not null,
  group_key text not null,
  reason text not null,
  metadata_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);

create index if not exists idx_client_access_group_migration_conflicts_client
  on client_access_group_migration_conflicts(client_account_id, service_code, created_at desc);

insert into permissions(id, code, name, scope_type, created_at)
values
  (gen_random_uuid(), 'access_group.read', 'Read client access groups', 'access_group', now()),
  (gen_random_uuid(), 'access_group.member.write', 'Manage client access group members', 'access_group', now()),
  (gen_random_uuid(), 'access_group.policy.write', 'Manage client access group policy', 'access_group', now()),
  (gen_random_uuid(), 'access_group.scope.write', 'Manage client access group scope', 'access_group', now()),
  (gen_random_uuid(), 'access_group.sync', 'Sync client access groups to runtime instances', 'access_group', now())
on conflict(code) do nothing;

insert into role_permissions(role_id, permission_id)
select r.id, p.id
from roles r
join permissions p on p.code in ('access_group.read')
where r.code in ('readonly','engineer','admin','superadmin')
on conflict do nothing;

insert into role_permissions(role_id, permission_id)
select r.id, p.id
from roles r
join permissions p on p.code in (
  'access_group.member.write',
  'access_group.policy.write',
  'access_group.scope.write',
  'access_group.sync'
)
where r.code in ('admin','superadmin')
on conflict do nothing;

insert into role_permissions(role_id, permission_id)
select r.id, p.id
from roles r
join permissions p on p.code in ('access_group.member.write')
where r.code = 'engineer'
on conflict do nothing;

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
  case when vgt.status in ('active','disabled','deleted') then vgt.status else 'active' end,
  jsonb_build_object(
    'access_mode', vgt.access_mode,
    'egress_mode', vgt.egress_mode,
    'egress_node_id', coalesce(vgt.egress_node_id::text, ''),
    'target_instance_id', coalesce(vgt.target_instance_id::text, ''),
    'outbound_tag', vgt.outbound_tag,
    'ad_block', vgt.ad_block,
    'rules', vgt.rules_json,
    'extra_rules', vgt.extra_rules_json,
    'legacy_vless_template', true
  ),
  'all_active_instances',
  true,
  vgt.created_at,
  vgt.updated_at,
  case when vgt.status = 'deleted' then now() else null end
from vless_group_templates vgt
on conflict(service_code, group_key) where deleted_at is null do update set
  display_name = excluded.display_name,
  description = excluded.description,
  status = excluded.status,
  policy_json = excluded.policy_json,
  updated_at = greatest(client_access_groups.updated_at, excluded.updated_at);

with legacy_vless as (
  select
    sa.id as service_access_id,
    sa.client_account_id,
    sa.instance_id,
    coalesce(
      nullif(sa.metadata_json->>'vless_group',''),
      nullif(sa.metadata_json->>'xray_group',''),
      nullif(sa.metadata_json->>'outbound_group',''),
      nullif(sa.metadata_json->'inbound_service'->>'vless_group','')
    ) as group_key,
    sa.metadata_json,
    sa.created_at,
    sa.updated_at
  from service_accesses sa
  where sa.status in ('pending','active','disabled')
    and coalesce(
      nullif(sa.metadata_json->>'vless_group',''),
      nullif(sa.metadata_json->>'xray_group',''),
      nullif(sa.metadata_json->>'outbound_group',''),
      nullif(sa.metadata_json->'inbound_service'->>'vless_group','')
    ) is not null
),
conflicted_clients as (
  select client_account_id
  from legacy_vless
  group by client_account_id
  having count(distinct group_key) > 1
),
conflict_rows as (
  select lv.*
  from legacy_vless lv
  join conflicted_clients cc on cc.client_account_id = lv.client_account_id
)
insert into client_access_group_migration_conflicts(
  id, client_account_id, instance_id, service_access_id, service_code, group_key,
  reason, metadata_json, created_at
)
select
  gen_random_uuid(),
  client_account_id,
  instance_id,
  service_access_id,
  'vless',
  group_key,
  'client has different VLESS groups on different materialized service accesses',
  jsonb_build_object('legacy_metadata', metadata_json),
  now()
from conflict_rows
on conflict do nothing;

with legacy_vless as (
  select
    sa.client_account_id,
    coalesce(
      nullif(sa.metadata_json->>'vless_group',''),
      nullif(sa.metadata_json->>'xray_group',''),
      nullif(sa.metadata_json->>'outbound_group',''),
      nullif(sa.metadata_json->'inbound_service'->>'vless_group','')
    ) as group_key,
    sa.metadata_json,
    min(sa.created_at) as created_at,
    max(sa.updated_at) as updated_at
  from service_accesses sa
  where sa.status in ('pending','active','disabled')
    and coalesce(
      nullif(sa.metadata_json->>'vless_group',''),
      nullif(sa.metadata_json->>'xray_group',''),
      nullif(sa.metadata_json->>'outbound_group',''),
      nullif(sa.metadata_json->'inbound_service'->>'vless_group','')
    ) is not null
  group by sa.client_account_id, group_key, sa.metadata_json
),
non_conflict as (
  select
    client_account_id,
    min(group_key) as group_key,
    jsonb_build_object('source','migration:service_accesses') as metadata_json,
    min(created_at) as created_at,
    max(updated_at) as updated_at
  from legacy_vless
  group by client_account_id
  having count(distinct group_key) = 1
),
from_global as (
  select
    vgm.client_account_id,
    vgm.group_key,
    vgm.status,
    jsonb_build_object('source','migration:vless_group_memberships','legacy_metadata',vgm.metadata_json) as metadata_json,
    vgm.created_at,
    vgm.updated_at
  from vless_group_memberships vgm
),
candidates as (
  select client_account_id, group_key, 'active'::text as status, metadata_json, created_at, updated_at
  from non_conflict
  union all
  select client_account_id, group_key, status, metadata_json, created_at, updated_at
  from from_global
),
ranked as (
  select *,
    row_number() over(partition by client_account_id order by updated_at desc, created_at desc) as rn
  from candidates
)
insert into client_access_group_memberships(
  id, client_account_id, service_code, group_id, status, source,
  metadata_json, created_at, updated_at
)
select
  gen_random_uuid(),
  ranked.client_account_id,
  'vless',
  cag.id,
  case when ranked.status in ('active','disabled','revoked') then ranked.status else 'active' end,
  'migration',
  ranked.metadata_json,
  ranked.created_at,
  ranked.updated_at
from ranked
join client_access_groups cag on cag.service_code = 'vless'
  and cag.group_key = ranked.group_key
  and cag.deleted_at is null
where ranked.rn = 1
on conflict(client_account_id, service_code) where status = 'active' do update set
  group_id = excluded.group_id,
  metadata_json = client_access_group_memberships.metadata_json || excluded.metadata_json,
  updated_at = greatest(client_access_group_memberships.updated_at, excluded.updated_at);

update service_accesses sa
set metadata_json = sa.metadata_json || jsonb_build_object(
  'source','legacy_instance_binding',
  'legacy_binding_reason','conflicting VLESS group membership during client access group migration'
),
updated_at = now()
where exists (
  select 1
  from client_access_group_migration_conflicts c
  where c.service_access_id = sa.id
);

update service_accesses sa
set metadata_json = sa.metadata_json || jsonb_build_object(
  'source','client_access_group',
  'access_group_id', cag.id::text,
  'access_group_key', cag.group_key
),
updated_at = now()
from client_access_group_memberships m
join client_access_groups cag on cag.id = m.group_id
where m.service_code = 'vless'
  and m.status = 'active'
  and sa.client_account_id = m.client_account_id
  and coalesce(
    nullif(sa.metadata_json->>'vless_group',''),
    nullif(sa.metadata_json->>'xray_group',''),
    nullif(sa.metadata_json->>'outbound_group',''),
    nullif(sa.metadata_json->'inbound_service'->>'vless_group','')
  ) = cag.group_key
  and not exists (
    select 1 from client_access_group_migration_conflicts c
    where c.service_access_id = sa.id
  );

insert into audit_events(id,actor_type,action,resource_type,resource_id,summary,payload_json,created_at)
values(
  gen_random_uuid(),
  'system',
  'migration.client_access_groups',
  'access_group',
  null,
  'generic client access group registry installed',
  '{}'::jsonb,
  now()
)
on conflict do nothing;
