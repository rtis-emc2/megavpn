-- Service discovery global hardening.
-- Extends discovered services into importable candidates for managed instances.

alter table node_service_discoveries
  add column if not exists confidence int not null default 50,
  add column if not exists endpoint_host text null,
  add column if not exists endpoint_port int null,
  add column if not exists managed_instance_id uuid null references instances(id) on delete set null;

create index if not exists idx_node_service_discoveries_importable
  on node_service_discoveries(node_id, status, service_code)
  where status in ('available','discovered');

create index if not exists idx_node_service_discoveries_managed_instance
  on node_service_discoveries(managed_instance_id)
  where managed_instance_id is not null;

create table if not exists node_service_discovery_events(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete cascade,
  discovery_id uuid null references node_service_discoveries(id) on delete set null,
  event_type text not null check(event_type in ('detected','updated','imported','ignored','unignored','failed')),
  summary text not null,
  payload_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);

create index if not exists idx_node_service_discovery_events_node_created
  on node_service_discovery_events(node_id, created_at desc);

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.service_discovery_global','platform','service discovery global schema installed','{}'::jsonb,now())
on conflict do nothing;
