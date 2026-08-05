drop trigger if exists instances_l2tp_runtime_guard on instances;
drop trigger if exists external_egress_deployments_l2tp_runtime_guard on external_egress_deployments;
drop function if exists megavpn_guard_l2tp_runtime_exclusivity();

-- Restore the runtime allowlist installed by migration 000024.
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

delete from schema_migrations where version='000025_external_egress_runtime_guards';
