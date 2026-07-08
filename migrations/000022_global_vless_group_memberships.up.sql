-- Release: 7.1.1.0
-- Global desired VLESS group membership. Runtime service_accesses remain the
-- per-instance materialized projection used by Xray config generation.

create table if not exists vless_group_memberships (
  id uuid primary key,
  group_key text not null references vless_group_templates(key) on update cascade on delete restrict,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  status text not null default 'active' check(status in ('active','disabled','revoked')),
  metadata_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(client_account_id)
);

create index if not exists idx_vless_group_memberships_group_status
on vless_group_memberships(group_key, status, updated_at desc);

create index if not exists idx_vless_group_memberships_client_status
on vless_group_memberships(client_account_id, status, updated_at desc);

with candidates as (
  select
    sa.client_account_id,
    coalesce(
      nullif(sa.metadata_json->>'vless_group',''),
      nullif(sa.metadata_json->>'xray_group',''),
      nullif(sa.metadata_json->>'outbound_group',''),
      nullif(sa.metadata_json->'inbound_service'->>'vless_group','')
    ) as group_key,
    sa.metadata_json,
    sa.created_at,
    sa.updated_at,
    row_number() over(partition by sa.client_account_id order by sa.updated_at desc, sa.created_at desc) as rn
  from service_accesses sa
  join vless_group_templates vgt on vgt.key = coalesce(
    nullif(sa.metadata_json->>'vless_group',''),
    nullif(sa.metadata_json->>'xray_group',''),
    nullif(sa.metadata_json->>'outbound_group',''),
    nullif(sa.metadata_json->'inbound_service'->>'vless_group','')
  )
  where sa.status in ('pending','active','disabled')
    and vgt.status = 'active'
)
insert into vless_group_memberships(id, group_key, client_account_id, status, metadata_json, created_at, updated_at)
select gen_random_uuid(), group_key, client_account_id, 'active',
       jsonb_build_object('source','migration:service_accesses','legacy_metadata',metadata_json),
       created_at, updated_at
from candidates
where rn = 1
on conflict(client_account_id) do update set
  group_key = excluded.group_key,
  status = 'active',
  metadata_json = vless_group_memberships.metadata_json || excluded.metadata_json,
  updated_at = greatest(vless_group_memberships.updated_at, excluded.updated_at);

insert into audit_events(id,actor_type,action,resource_type,resource_id,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.vless_group_memberships','vless_group',null,'global VLESS group membership registry installed','{}'::jsonb,now())
on conflict do nothing;
