-- Service discovery keeps observed service candidates separate from managed instances.
-- A discovered service is not automatically managed by the control plane.

create table if not exists node_service_discoveries(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete cascade,
  service_code text not null references service_definitions(code),
  name text not null,
  systemd_unit text null,
  config_path text null,
  status text not null check(status in ('discovered','available','missing','ignored','imported')),
  source text not null check(source in ('inventory','manual','import')),
  payload_json jsonb not null default '{}'::jsonb,
  detected_at timestamptz not null default now(),
  unique(node_id, service_code, name)
);

create index if not exists idx_node_service_discoveries_node on node_service_discoveries(node_id, service_code);
create index if not exists idx_node_service_discoveries_status on node_service_discoveries(status);

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.service_discovery','platform','service discovery schema installed','{}'::jsonb,now())
on conflict do nothing;
