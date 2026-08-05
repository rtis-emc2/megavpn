create table if not exists client_service_identities(
  id uuid primary key,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  service_code text not null,
  profile_key text not null,
  credential_json jsonb not null default '{}'::jsonb,
  status text not null default 'active' check(status in ('active','rotated','revoked')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(client_account_id, service_code, profile_key)
);

create index if not exists idx_client_service_identities_lookup
on client_service_identities(service_code, profile_key, status, updated_at desc);

insert into client_service_identities(
  id,
  client_account_id,
  service_code,
  profile_key,
  credential_json,
  status,
  created_at,
  updated_at
)
select
  gen_random_uuid(),
  existing.client_account_id,
  'xray-core',
  existing.profile_key,
  jsonb_build_object('xray_uuid', existing.xray_uuid),
  'active',
  now(),
  now()
from (
  select distinct on (sa.client_account_id, coalesce(nullif(sa.metadata_json->>'xray_identity_key',''), 'vless'))
    sa.client_account_id,
    coalesce(nullif(sa.metadata_json->>'xray_identity_key',''), 'vless') as profile_key,
    coalesce(nullif(sa.metadata_json->>'xray_uuid',''), nullif(sa.metadata_json->>'uuid','')) as xray_uuid,
    sa.status,
    sa.updated_at
  from service_accesses sa
  join instances i on i.id = sa.instance_id
  join service_definitions sd on sd.id = i.service_definition_id
  where sd.code in ('xray-core','xray','xray_core')
    and sa.status in ('active','pending')
    and coalesce(nullif(sa.metadata_json->>'xray_uuid',''), nullif(sa.metadata_json->>'uuid','')) is not null
  order by
    sa.client_account_id,
    coalesce(nullif(sa.metadata_json->>'xray_identity_key',''), 'vless'),
    case sa.status when 'active' then 0 when 'pending' then 1 else 2 end,
    sa.updated_at desc
) existing
on conflict(client_account_id, service_code, profile_key) do nothing;

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
select gen_random_uuid(), 'system', 'migration.client_service_identities', 'client', 'client service identity registry created', '{}'::jsonb, now()
where not exists(select 1 from audit_events where action='migration.client_service_identities');
