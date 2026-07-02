-- Release: 7.0.1.2
-- Ensure existing installations receive firewall RBAC permissions after baseline consolidation.

insert into permissions(id, code, name, scope_type, created_at)
values
  (gen_random_uuid(), 'firewall.read', 'Read firewall policies', 'node', now()),
  (gen_random_uuid(), 'firewall.manage', 'Manage firewall policies', 'node', now()),
  (gen_random_uuid(), 'firewall.apply', 'Apply firewall policies', 'node', now())
on conflict (code) do nothing;

insert into role_permissions(role_id, permission_id)
select r.id, p.id
from roles r
join permissions p on p.code in ('firewall.read')
where r.code in ('readonly','engineer','admin','superadmin')
on conflict do nothing;

insert into role_permissions(role_id, permission_id)
select r.id, p.id
from roles r
join permissions p on p.code in ('firewall.manage','firewall.apply')
where r.code in ('admin','superadmin')
on conflict do nothing;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
values (gen_random_uuid(), 'system', 'migration.firewall_rbac_repair', 'firewall', 'firewall RBAC permissions repaired', '{}'::jsonb, now());
