-- Release 7.1.0.14: harden firewall apply upgrade path.
-- Keep repaired installations aligned with the runtime firewall catalog model.

alter table firewall_address_lists drop constraint if exists firewall_address_lists_scope_check;
alter table firewall_address_lists
  add constraint firewall_address_lists_scope_check
  check(scope in ('global','control_plane','node','service','client','template'));

alter table firewall_rules drop constraint if exists firewall_rules_protocol_check;
alter table firewall_rules
  add constraint firewall_rules_protocol_check
  check(protocol in ('any','tcp','udp','icmp','icmpv6'));

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
select
  gen_random_uuid(),
  'system',
  'migration.firewall_apply_schema_hardening',
  'firewall',
  'firewall apply schema constraints aligned with runtime catalog',
  '{"release":"7.1.0.14"}'::jsonb,
  now()
where not exists(select 1 from audit_events where action='migration.firewall_apply_schema_hardening');
