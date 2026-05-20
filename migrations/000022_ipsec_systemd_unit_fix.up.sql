-- Normalize IPsec instances to the strongswan-starter systemd unit.

update instances
set systemd_unit = 'strongswan-starter',
    updated_at = now()
where service_definition_id = (select id from service_definitions where code = 'ipsec')
  and coalesce(systemd_unit, '') in ('', 'strongswan');

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.ipsec_systemd_unit_fix','platform','ipsec systemd unit normalized to strongswan-starter','{}'::jsonb,now())
on conflict do nothing;
