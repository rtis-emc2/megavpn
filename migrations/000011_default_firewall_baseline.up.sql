-- Release: 7.0.1.28
-- Seed the managed default node firewall baseline and allow IPv6 ICMP rules.

alter table firewall_rules drop constraint if exists firewall_rules_protocol_check;
alter table firewall_rules
  add constraint firewall_rules_protocol_check check(protocol in ('any','tcp','udp','icmp','icmpv6'));

insert into firewall_address_lists(id, key, label, description, scope, status, created_at, updated_at)
values
  (gen_random_uuid(), 'vpn_client_sources', 'VPN client source ranges', 'Default source ranges used by managed VPN clients and private overlay networks.', 'global', 'active', now(), now())
on conflict(key) do update
set label=excluded.label,
    description=excluded.description,
    scope=excluded.scope,
    status=case when firewall_address_lists.status='deleted' then firewall_address_lists.status else excluded.status end,
    updated_at=now();

with target as (
  select id from firewall_address_lists where key='vpn_client_sources' and status <> 'deleted'
), seed(value,value_type,label) as (
  values
    ('10.0.0.0/8','cidr','RFC1918 private client range'),
    ('172.16.0.0/12','cidr','RFC1918 private client range'),
    ('192.168.0.0/16','cidr','RFC1918 private client range'),
    ('100.64.0.0/10','cidr','CGNAT client range'),
    ('fd00::/8','cidr','IPv6 unique local client range')
)
insert into firewall_address_entries(id,list_id,value,value_type,label,status,created_at,updated_at)
select gen_random_uuid(), target.id, seed.value, seed.value_type, seed.label, 'active', now(), now()
from target
cross join seed
on conflict(list_id,value) where status <> 'deleted' do nothing;

update firewall_policies
set label='Default node firewall',
    description='Default node firewall: deny unsolicited input and forwarding when strict mode is selected, keep node egress open, and allow common edge entrypoints plus managed VPN client forwarding ranges.',
    default_input_policy='drop',
    default_forward_policy='drop',
    default_output_policy='accept',
    status=case when status='deleted' then status else 'active' end,
    updated_at=now()
where key='node_base';

with policy as (
  select id from firewall_policies where key='node_base' and status <> 'deleted'
), trusted_operators as (
  select id from firewall_address_lists where key='trusted_operators' and status <> 'deleted'
), vpn_client_sources as (
  select id from firewall_address_lists where key='vpn_client_sources' and status <> 'deleted'
), seed(priority,chain,action,direction,protocol,src_list_key,dst_ports,state_match,comment,enabled,baseline_key) as (
  values
    (50,'input','drop','in','any','', '', ARRAY['invalid']::text[], 'Drop invalid input packets.', true, 'drop_invalid_input'),
    (55,'forward','drop','forward','any','', '', ARRAY['invalid']::text[], 'Drop invalid forwarded packets.', true, 'drop_invalid_forward'),
    (100,'input','accept','in','icmp','', '', ARRAY[]::text[], 'Allow IPv4 ICMP diagnostics.', true, 'allow_icmp_v4'),
    (105,'input','accept','in','icmpv6','', '', ARRAY[]::text[], 'Allow IPv6 ICMP diagnostics.', true, 'allow_icmp_v6'),
    (120,'input','accept','in','tcp','', '80,443', ARRAY['new','established']::text[], 'Allow public HTTP and HTTPS edge entrypoints.', true, 'allow_edge_http_https'),
    (200,'input','accept','in','tcp','trusted_operators', '22', ARRAY['new','established']::text[], 'Allow SSH management from trusted operators after the address list is populated.', false, 'allow_ssh_trusted_operators'),
    (300,'forward','accept','forward','any','vpn_client_sources', '', ARRAY['new','established']::text[], 'Allow managed VPN client source ranges to forward through the node.', true, 'allow_vpn_client_forward')
)
insert into firewall_rules(
  id,policy_id,priority,chain,action,direction,protocol,src_list_id,dst_list_id,src_cidr,dst_cidr,src_ports,dst_ports,state_match,comment,enabled,log,status,metadata_json,created_at,updated_at
)
select
  gen_random_uuid(), policy.id, seed.priority, seed.chain, seed.action, seed.direction, seed.protocol,
  case seed.src_list_key
    when 'trusted_operators' then trusted_operators.id
    when 'vpn_client_sources' then vpn_client_sources.id
    else null
  end,
  null, '', '', '', seed.dst_ports, seed.state_match, seed.comment, seed.enabled, false, 'active',
  jsonb_build_object('baseline','default_firewall','baseline_key',seed.baseline_key),
  now(), now()
from policy
cross join seed
left join trusted_operators on seed.src_list_key='trusted_operators'
left join vpn_client_sources on seed.src_list_key='vpn_client_sources'
where not exists (
  select 1
  from firewall_rules r
  where r.policy_id=policy.id
    and r.status <> 'deleted'
    and (r.metadata_json->>'baseline_key'=seed.baseline_key or r.comment=seed.comment)
);

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
select gen_random_uuid(), 'system', 'migration.default_firewall_baseline', 'firewall', 'default firewall baseline seeded', '{}'::jsonb, now()
where not exists(select 1 from audit_events where action='migration.default_firewall_baseline');
