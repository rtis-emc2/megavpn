-- Require explicit SSH host-key pinning for automated bootstrap.

alter table node_access_methods
  add column if not exists ssh_host_key_sha256 text null;

insert into audit_events(id, actor_user_id, actor_type, action, resource_type, summary, payload_json, created_at)
values(
  gen_random_uuid(),
  null,
  'system',
  'migration.ssh_bootstrap_host_key_pin',
  'platform',
  'ssh bootstrap host key pinning installed',
  '{}'::jsonb,
  now()
)
on conflict do nothing;
