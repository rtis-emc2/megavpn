-- Node bootstrap foundation.

alter table nodes
  add column if not exists role text not null default 'egress';

alter table nodes
  drop constraint if exists nodes_role_check;

alter table nodes
  add constraint nodes_role_check
  check(role in ('ingress','egress'));

create table if not exists node_access_methods(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete cascade,
  method text not null,
  is_enabled boolean not null default true,
  ssh_host text null,
  ssh_port int null,
  ssh_user text null,
  auth_type text null,
  secret_ref_id uuid null references secret_refs(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check(method in ('local','ssh','manual_bundle','agent')),
  check(auth_type in ('ssh_key','password','token','none') or auth_type is null)
);

create index if not exists idx_node_access_methods_node on node_access_methods(node_id, method);

create table if not exists node_bootstrap_runs(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete cascade,
  job_id uuid null references jobs(id) on delete set null,
  status text not null,
  bootstrap_mode text not null,
  request_payload_json jsonb not null default '{}'::jsonb,
  result_payload_json jsonb null,
  started_at timestamptz null,
  finished_at timestamptz null,
  created_by uuid null references platform_users(id) on delete set null,
  created_at timestamptz not null default now(),
  check(status in ('queued','running','succeeded','failed','cancelled')),
  check(bootstrap_mode in ('ssh_bootstrap','manual_bundle'))
);

create index if not exists idx_node_bootstrap_runs_node_created on node_bootstrap_runs(node_id, created_at desc);
create index if not exists idx_node_bootstrap_runs_job on node_bootstrap_runs(job_id) where job_id is not null;

insert into audit_events(id, actor_user_id, actor_type, action, resource_type, summary, payload_json, created_at)
values(
  gen_random_uuid(),
  null,
  'system',
  'migration.node_bootstrap_foundation',
  'platform',
  'node bootstrap foundation installed',
  '{}'::jsonb,
  now()
)
on conflict do nothing;
