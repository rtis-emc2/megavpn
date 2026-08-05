-- Release: 7.1.1.0
-- Align persisted firewall node state with the disable lifecycle supported by
-- the API, UI and agent.

alter table firewall_node_state drop constraint if exists firewall_node_state_status_check;
alter table firewall_node_state
  add constraint firewall_node_state_status_check
  check(status in ('unknown','pending','pending_disable','applied','disabled','failed','drifted','stale'));

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
select
  gen_random_uuid(),
  'system',
  'migration.firewall_node_state_disable_status',
  'firewall',
  'firewall node state disable statuses enabled',
  '{"release":"7.1.1.0"}'::jsonb,
  now()
where not exists(select 1 from audit_events where action='migration.firewall_node_state_disable_status');
