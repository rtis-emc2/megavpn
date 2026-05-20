-- Expose xl2tpd as a first-class service definition for runtime/apply flows.

insert into service_definitions (
  id,
  code,
  name,
  category,
  tier,
  supports_accounts,
  supports_artifacts,
  enabled,
  supports_install,
  supports_instances,
  created_at
)
values (
  gen_random_uuid(),
  'xl2tpd',
  'XL2TPD',
  'vpn',
  'A',
  false,
  false,
  true,
  true,
  true,
  now()
)
on conflict (code) do update set
  name = excluded.name,
  category = excluded.category,
  tier = excluded.tier,
  supports_accounts = excluded.supports_accounts,
  supports_artifacts = excluded.supports_artifacts,
  enabled = excluded.enabled,
  supports_install = excluded.supports_install,
  supports_instances = excluded.supports_instances;

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.xl2tpd_service_definition','platform','xl2tpd service definition upserted','{}'::jsonb,now())
on conflict do nothing;
