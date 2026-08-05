-- Expand the database-level external egress runtime allowlist and make the
-- L2TP/IPsec client/server port reservation race-free. Both trigger paths lock
-- the node row before checking the opposite runtime table.

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
  if provider_protocol not in ('openvpn','wireguard','l2tp_ipsec','vless','shadowsocks') then
    raise exception using errcode='23514', message='external egress profile runtime is not available';
  end if;
  return new;
end;
$$;

create or replace function megavpn_guard_l2tp_runtime_exclusivity()
returns trigger
language plpgsql
as $$
declare
  runtime_protocol text;
  service_code text;
begin
  -- The node lock serializes instance and external deployment reservations.
  perform 1 from nodes where id=new.node_id for update;

  if tg_table_name = 'instances' then
    select code into service_code
      from service_definitions
      where id=new.service_definition_id;
    if service_code <> 'xl2tpd' or not new.enabled or new.status='deleted' then
      return new;
    end if;
    if exists (
      select 1
      from external_egress_deployments d
      join external_egress_profiles p on p.id=d.profile_id
      where d.node_id=new.node_id
        and d.desired_status='active'
        and d.status<>'deleted'
        and p.protocol='l2tp_ipsec'
        and p.status<>'deleted'
    ) then
      raise exception using errcode='23514', message='XL2TPD server instance cannot share a node with an active L2TP/IPsec external egress deployment';
    end if;
    return new;
  end if;

  if new.desired_status <> 'active' or new.status='deleted' then
    return new;
  end if;
  select protocol into runtime_protocol
    from external_egress_profiles
    where id=new.profile_id;
  if runtime_protocol <> 'l2tp_ipsec' then
    return new;
  end if;
  if exists (
    select 1
    from instances i
    join service_definitions sd on sd.id=i.service_definition_id
    where i.node_id=new.node_id
      and i.enabled=true
      and i.status<>'deleted'
      and sd.code='xl2tpd'
  ) then
    raise exception using errcode='23514', message='L2TP/IPsec external egress cannot share a node with a managed XL2TPD server instance';
  end if;
  if exists (
    select 1
    from external_egress_deployments d
    join external_egress_profiles p on p.id=d.profile_id
    where d.node_id=new.node_id
      and d.id<>new.id
      and d.desired_status='active'
      and d.status<>'deleted'
      and p.protocol='l2tp_ipsec'
      and p.status<>'deleted'
  ) then
    raise exception using errcode='23514', message='only one active L2TP/IPsec external egress deployment is supported per node';
  end if;
  return new;
end;
$$;

drop trigger if exists instances_l2tp_runtime_guard on instances;
create trigger instances_l2tp_runtime_guard
before insert or update of node_id, service_definition_id, enabled, status
on instances
for each row execute function megavpn_guard_l2tp_runtime_exclusivity();

drop trigger if exists external_egress_deployments_l2tp_runtime_guard on external_egress_deployments;
create trigger external_egress_deployments_l2tp_runtime_guard
before insert or update of profile_id, node_id, desired_status, status
on external_egress_deployments
for each row execute function megavpn_guard_l2tp_runtime_exclusivity();
