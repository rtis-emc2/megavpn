create extension if not exists pgcrypto;

create table if not exists nodes(
  id uuid primary key,
  name text not null unique,
  kind text not null check(kind in ('local','remote')),
  status text not null check(status in ('draft','bootstrapping','online','degraded','offline','maintenance','draining','retired')),
  address text not null,
  os_family text not null,
  os_version text not null,
  architecture text not null check(architecture in ('amd64','arm64')),
  execution_mode text not null check(execution_mode in ('local_managed','agent_managed','ssh_bootstrap','manual_bundle')),
  agent_status text not null default 'unknown',
  last_heartbeat_at timestamptz null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists node_agents(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete cascade,
  status text not null,
  agent_version text null,
  protocol_version text null,
  fingerprint text not null unique,
  registered_at timestamptz not null default now(),
  last_seen_at timestamptz null,
  revoked_at timestamptz null,
  unique(node_id)
);

create table if not exists service_definitions(
  id uuid primary key,
  code text not null unique,
  name text not null,
  category text not null,
  tier text not null check(tier in ('A','B','C')),
  supports_accounts boolean not null default false,
  supports_artifacts boolean not null default false,
  enabled boolean not null default true,
  created_at timestamptz not null default now()
);

create table if not exists instances(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete restrict,
  service_definition_id uuid not null references service_definitions(id),
  name text not null,
  slug text not null unique,
  systemd_unit text null,
  status text not null check(status in ('draft','provisioning','active','degraded','disabled','failed','deleting','deleted')),
  enabled boolean not null default true,
  endpoint_host text null,
  endpoint_port int null,
  current_revision_id uuid null,
  last_applied_revision_id uuid null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(node_id,name)
);

create table if not exists instance_revisions(
  id uuid primary key,
  instance_id uuid not null references instances(id) on delete cascade,
  revision_no int not null,
  source text not null default 'manual',
  status text not null check(status in ('draft','validated','applied','failed','rolled_back','superseded')),
  spec_json jsonb not null default '{}'::jsonb,
  rendered_hash text null,
  validation_errors_json jsonb not null default '[]'::jsonb,
  created_at timestamptz not null default now(),
  applied_at timestamptz null,
  unique(instance_id, revision_no)
);

create table if not exists client_accounts(
  id uuid primary key,
  username text not null unique,
  display_name text null,
  email text null,
  status text not null check(status in ('draft','active','suspended','revoked','expired','deleted')),
  expires_at timestamptz null,
  notes text null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists service_accesses(
  id uuid primary key,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  instance_id uuid not null references instances(id) on delete restrict,
  status text not null check(status in ('pending','active','disabled','revoked','failed')),
  provision_mode text not null default 'manual',
  policy_json jsonb not null default '{}'::jsonb,
  metadata_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(client_account_id, instance_id)
);

create table if not exists artifacts(
  id uuid primary key,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  service_access_id uuid null references service_accesses(id) on delete set null,
  artifact_type text not null,
  storage_path text not null,
  content_hash text null,
  size_bytes bigint null,
  status text not null default 'ready',
  created_at timestamptz not null default now()
);

create table if not exists share_links(
  id uuid primary key,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  target_type text not null check(target_type in ('artifact','bundle')),
  target_id uuid not null,
  token text not null unique,
  status text not null check(status in ('active','expired','revoked','consumed')),
  expires_at timestamptz not null,
  download_count bigint not null default 0,
  created_at timestamptz not null default now()
);

create table if not exists jobs(
  id uuid primary key,
  type text not null,
  scope_type text not null,
  scope_id uuid null,
  node_id uuid null references nodes(id) on delete set null,
  instance_id uuid null references instances(id) on delete set null,
  status text not null check(status in ('queued','running','succeeded','failed','cancelled','retrying')),
  priority int not null default 100,
  payload_json jsonb not null default '{}'::jsonb,
  result_json jsonb null,
  created_at timestamptz not null default now(),
  started_at timestamptz null,
  finished_at timestamptz null
);

create table if not exists job_logs(
  id uuid primary key,
  job_id uuid not null references jobs(id) on delete cascade,
  level text not null check(level in ('debug','info','warn','error')),
  message text not null,
  payload_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);

create table if not exists audit_events(
  id uuid primary key,
  actor_type text not null check(actor_type in ('platform_user','system','agent','importer')),
  action text not null,
  resource_type text not null,
  resource_id uuid null,
  summary text not null,
  payload_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);

create index if not exists idx_jobs_status_priority on jobs(status,priority,created_at);
create index if not exists idx_audit_created_at on audit_events(created_at desc);
create index if not exists idx_nodes_status on nodes(status);
