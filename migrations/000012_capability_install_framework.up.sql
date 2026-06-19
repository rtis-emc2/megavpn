-- MegaVPN 0.6.10.4-alpha: capability installation framework.
-- Adds install job metadata/events and normalizes service definitions for nginx and xray-core.

alter table service_definitions
  add column if not exists supports_install boolean not null default false,
  add column if not exists supports_instances boolean not null default true;

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
values
  (gen_random_uuid(), 'xray-core', 'Xray-core', 'vpn', 'A', true, true, true, true, true, now()),
  (gen_random_uuid(), 'nginx', 'Nginx', 'edge', 'A', false, false, true, true, true, now())
on conflict (code) do update set
  name = excluded.name,
  category = excluded.category,
  tier = excluded.tier,
  supports_accounts = excluded.supports_accounts,
  supports_artifacts = excluded.supports_artifacts,
  enabled = excluded.enabled,
  supports_install = excluded.supports_install,
  supports_instances = excluded.supports_instances;

update service_definitions
set supports_install = true, supports_instances = true
where code in ('nginx','xray-core','xray','openvpn','wireguard','ipsec','shadowsocks');

create table if not exists node_capability_install_events(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete cascade,
  job_id uuid null references jobs(id) on delete set null,
  capability_code text not null,
  strategy text not null,
  status text not null check(status in ('queued','running','succeeded','failed','verified')),
  summary text not null,
  payload_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);

create index if not exists idx_node_capability_install_events_node_created
  on node_capability_install_events(node_id, created_at desc);

create index if not exists idx_node_capability_install_events_capability
  on node_capability_install_events(node_id, capability_code, created_at desc);

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.capability_install_framework','platform','capability installation framework schema installed','{}'::jsonb,now())
on conflict do nothing;
