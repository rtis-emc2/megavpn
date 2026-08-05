-- Rollback support recreates compatibility projections from the canonical
-- registry. New code never writes these tables.

create table vless_group_templates (
  key text primary key,
  label text not null,
  description text not null default '',
  access_mode text not null default 'instance_default',
  egress_mode text not null default 'default',
  egress_node_id uuid null references nodes(id) on delete set null,
  target_instance_id uuid null references instances(id) on delete set null,
  outbound_tag text not null default 'direct',
  ad_block boolean not null default false,
  rules_json jsonb not null default '[]'::jsonb,
  extra_rules_json jsonb not null default '[]'::jsonb,
  status text not null default 'active',
  source text not null default 'canonical_rollback',
  version integer not null default 1,
  display_order integer not null default 1000,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint vless_group_templates_key_check check(key ~ '^[a-z0-9][a-z0-9_.:-]{0,63}$'),
	constraint vless_group_templates_access_mode_check
	  check(access_mode in ('instance_default','local_breakout','egress_node','instance_only','block')),
	constraint vless_group_templates_egress_mode_check
	  check(egress_mode in ('default','local_breakout','egress_node','instance_only','block')),
	constraint vless_group_templates_outbound_tag_check
	  check(outbound_tag ~ '^[A-Za-z0-9_.:-]{1,64}$'),
	constraint vless_group_templates_rules_array_check check(jsonb_typeof(rules_json) = 'array'),
	constraint vless_group_templates_extra_rules_array_check check(jsonb_typeof(extra_rules_json) = 'array'),
  constraint vless_group_templates_status_check check(status in ('active','disabled','deleted'))
);

create index vless_group_templates_status_order_idx
  on vless_group_templates(status, display_order, label, key);

insert into vless_group_templates(
  key, label, description, access_mode, egress_mode, egress_node_id,
  target_instance_id, outbound_tag, ad_block, rules_json, extra_rules_json,
  status, source, version, display_order, created_at, updated_at
)
select
  group_key,
  display_name,
  description,
  coalesce(nullif(policy_json->>'access_mode',''), 'instance_default'),
  coalesce(nullif(policy_json->>'egress_mode',''), 'default'),
  nullif(policy_json->>'egress_node_id','')::uuid,
  nullif(policy_json->>'target_instance_id','')::uuid,
  coalesce(nullif(policy_json->>'outbound_tag',''), 'direct'),
  coalesce((policy_json->>'ad_block')::boolean, false),
  coalesce(policy_json->'rules', '[]'::jsonb),
  coalesce(policy_json->'extra_rules', '[]'::jsonb),
  status,
  'canonical_rollback',
  1,
  100,
  created_at,
  updated_at
from client_access_groups
where service_code = 'vless'
  and deleted_at is null;

create table vless_group_memberships (
  id uuid primary key,
  group_key text not null references vless_group_templates(key) on update cascade on delete restrict,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  status text not null default 'active' check(status in ('active','disabled','revoked')),
  metadata_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(client_account_id)
);

create index idx_vless_group_memberships_group_status
  on vless_group_memberships(group_key, status, updated_at desc);
create index idx_vless_group_memberships_client_status
  on vless_group_memberships(client_account_id, status, updated_at desc);

insert into vless_group_memberships(
  id, group_key, client_account_id, status, metadata_json, created_at, updated_at
)
select
  gen_random_uuid(),
  selected.group_key,
  selected.client_account_id,
  selected.status,
  selected.metadata_json || jsonb_build_object('source', 'canonical_rollback'),
  selected.created_at,
  selected.updated_at
from (
  select
    cag.group_key,
    membership.client_account_id,
    case when membership.status in ('active','disabled','revoked') then membership.status else 'revoked' end as status,
    membership.metadata_json,
    membership.created_at,
    membership.updated_at,
    row_number() over(
      partition by membership.client_account_id
      order by
        case membership.status when 'active' then 0 when 'disabled' then 1 else 2 end,
        membership.updated_at desc,
        membership.created_at desc,
        membership.id
    ) as rn
  from client_access_group_memberships membership
  join client_access_groups cag on cag.id = membership.group_id
  where membership.service_code = 'vless'
    and membership.status in ('active','disabled','revoked')
    and cag.deleted_at is null
) selected
where selected.rn = 1;

create table client_access_group_migration_conflicts (
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

create index idx_client_access_group_migration_conflicts_client
  on client_access_group_migration_conflicts(client_account_id, service_code, created_at desc);

alter table client_access_groups
  drop constraint if exists client_access_groups_service_code_check;
alter table client_access_groups
  add constraint client_access_groups_service_code_check
  check(service_code in ('vless','openvpn','l2tp','wireguard','shadowsocks','http_proxy','mtproto'));
