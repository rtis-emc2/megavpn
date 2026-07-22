-- External provider egress profiles are global policy objects. Deployments bind
-- a profile to a concrete node without changing the node's main routing table.

create table if not exists external_egress_profiles(
  id uuid primary key,
  profile_key text not null,
  display_name text not null,
  description text not null default '',
  protocol text not null check(protocol in ('openvpn','wireguard','socks5','http_connect','shadowsocks','vless','l2tp','l2tp_ipsec','ikev2','trojan','hysteria2')),
  transport text not null default '',
  status text not null default 'draft' check(status in ('draft','active','disabled','deleted')),
  import_format text not null default 'structured',
  endpoint_host text not null default '',
  endpoint_port integer null check(endpoint_port is null or endpoint_port between 1 and 65535),
  config_json jsonb not null default '{}'::jsonb,
  created_by uuid null references platform_users(id) on delete set null,
  updated_by uuid null references platform_users(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz null
);

create unique index if not exists external_egress_profiles_key_active_uq
  on external_egress_profiles(profile_key) where status <> 'deleted';
create index if not exists external_egress_profiles_status_protocol_idx
  on external_egress_profiles(status, protocol, created_at desc);

create table if not exists external_egress_profile_secrets(
  profile_id uuid not null references external_egress_profiles(id) on delete cascade,
  purpose text not null check(purpose in ('config','username','password','private_key','public_key','certificate','ca_certificate','preshared_key','tls_auth_key','tls_crypt_key','pkcs12','tls_crypt_v2_key','static_key','uuid')),
  secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key(profile_id, purpose)
);

create table if not exists external_egress_deployments(
  id uuid primary key,
  profile_id uuid not null references external_egress_profiles(id) on delete cascade,
  node_id uuid not null references nodes(id) on delete restrict,
  desired_status text not null default 'active' check(desired_status in ('active','inactive','deleted')),
  status text not null default 'pending' check(status in ('pending','queued','applying','active','degraded','failed','inactive','deleted')),
  interface_name text not null,
  routing_table text not null check(
    case when routing_table ~ '^[0-9]+$'
      then routing_table::integer between 40000 and 48999
      else false
    end
  ),
  fwmark integer not null check(fwmark between 1297678336 and 1297743871),
  route_metric integer not null default 100 check(route_metric between 1 and 32767),
  config_json jsonb not null default '{}'::jsonb,
  health_json jsonb not null default '{}'::jsonb,
  last_job_id uuid null references jobs(id) on delete set null,
  last_error text not null default '',
  applied_at timestamptz null,
  observed_at timestamptz null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(profile_id, node_id),
  unique(node_id, interface_name),
  unique(node_id, routing_table),
  unique(node_id, fwmark)
);

create index if not exists external_egress_deployments_node_status_idx
  on external_egress_deployments(node_id, status, updated_at desc);

alter table client_access_groups
  add column if not exists external_egress_profile_id uuid null references external_egress_profiles(id) on delete restrict;

create index if not exists client_access_groups_external_egress_idx
  on client_access_groups(external_egress_profile_id) where external_egress_profile_id is not null and deleted_at is null;

create or replace function megavpn_validate_external_egress_reference()
returns trigger
language plpgsql
as $$
declare
  provider_status text;
  provider_protocol text;
begin
  if tg_table_name = 'client_access_groups' then
    if new.external_egress_profile_id is null then
      return new;
    end if;
    if lower(new.service_code) <> 'vless' then
      raise exception using errcode='23514', message='external egress profiles can only be assigned to VLESS client access groups';
    end if;
    select status, protocol into provider_status, provider_protocol
      from external_egress_profiles
      where id=new.external_egress_profile_id
      for update;
  else
    if new.desired_status <> 'active' or new.status in ('inactive','deleted') then
      return new;
    end if;
    select status, protocol into provider_status, provider_protocol
      from external_egress_profiles
      where id=new.profile_id
      for update;
  end if;
  if provider_status is null then
    raise exception using errcode='23503', message='external egress profile does not exist';
  end if;
  if provider_status <> 'active' then
    raise exception using errcode='23514', message='external egress profile must be active';
  end if;
  if provider_protocol not in ('openvpn','wireguard') then
    raise exception using errcode='23514', message='external egress profile runtime is not available';
  end if;
  return new;
end;
$$;

drop trigger if exists client_access_groups_external_egress_guard on client_access_groups;
create trigger client_access_groups_external_egress_guard
before insert or update of external_egress_profile_id, service_code
on client_access_groups
for each row execute function megavpn_validate_external_egress_reference();

drop trigger if exists external_egress_deployments_profile_guard on external_egress_deployments;
create trigger external_egress_deployments_profile_guard
before insert or update of profile_id, desired_status, status
on external_egress_deployments
for each row execute function megavpn_validate_external_egress_reference();

create or replace function megavpn_guard_external_egress_profile_lifecycle()
returns trigger
language plpgsql
as $$
begin
  if old.protocol <> new.protocol then
    raise exception using errcode='23514', message='external egress profile protocol is immutable';
  end if;
  if old.status <> new.status and new.status <> 'active' then
    if exists (
      select 1 from client_access_groups
      where external_egress_profile_id=old.id and deleted_at is null and status <> 'deleted'
    ) then
      raise exception using errcode='23503', message='external egress profile is still assigned to a client access group';
    end if;
    if exists (
      select 1 from external_egress_deployments
      where profile_id=old.id and status not in ('inactive','deleted')
    ) then
      raise exception using errcode='23503', message='external egress profile still has a live deployment';
    end if;
  end if;
  return new;
end;
$$;

drop trigger if exists external_egress_profiles_lifecycle_guard on external_egress_profiles;
create trigger external_egress_profiles_lifecycle_guard
before update of protocol, status
on external_egress_profiles
for each row execute function megavpn_guard_external_egress_profile_lifecycle();
