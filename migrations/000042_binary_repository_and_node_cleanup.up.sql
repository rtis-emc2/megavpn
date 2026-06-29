create table if not exists binary_artifacts(
  id uuid primary key,
  name text not null,
  kind text not null,
  service_code text not null default '',
  version text not null,
  os_family text not null default 'linux',
  os_version text not null default '',
  architecture text not null check(architecture in ('amd64','arm64')),
  storage_path text not null,
  size_bytes bigint not null default 0 check(size_bytes >= 0),
  sha256 text not null check(sha256 ~ '^[a-f0-9]{64}$'),
  signature text not null default '',
  status text not null default 'active' check(status in ('active','disabled','deleted')),
  metadata_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint binary_artifacts_kind_check check(kind in ('agent','runtime','package','script','bundle'))
);

create unique index if not exists idx_binary_artifacts_unique_active
  on binary_artifacts(name, kind, service_code, version, os_family, os_version, architecture)
  where status <> 'deleted';

create index if not exists idx_binary_artifacts_lookup
  on binary_artifacts(kind, service_code, architecture, status, created_at desc);

create table if not exists binary_manifests(
  id uuid primary key,
  artifact_id uuid not null references binary_artifacts(id) on delete cascade,
  channel text not null default 'stable',
  manifest_json jsonb not null default '{}'::jsonb,
  status text not null default 'active' check(status in ('active','disabled','deleted')),
  created_at timestamptz not null default now()
);

create unique index if not exists idx_binary_manifests_unique_active
  on binary_manifests(artifact_id, channel)
  where status = 'active';

create table if not exists binary_download_tickets(
  id uuid primary key,
  artifact_id uuid not null references binary_artifacts(id) on delete cascade,
  node_id uuid null references nodes(id) on delete cascade,
  job_id uuid null references jobs(id) on delete set null,
  token_hash text not null unique,
  token_hint text not null default '',
  status text not null default 'active' check(status in ('active','used','revoked','expired')),
  expires_at timestamptz not null,
  used_at timestamptz null,
  created_at timestamptz not null default now()
);

create index if not exists idx_binary_download_tickets_artifact
  on binary_download_tickets(artifact_id, status, expires_at);

insert into permissions(id, code, name, scope_type, created_at)
values
  (gen_random_uuid(), 'binary_repository.read', 'Read binary repository', 'global', now()),
  (gen_random_uuid(), 'binary_repository.manage', 'Manage binary repository', 'global', now())
on conflict (code) do nothing;

insert into role_permissions(role_id, permission_id)
select r.id, p.id
from roles r
join permissions p on p.code in ('binary_repository.read')
where r.code in ('admin', 'superadmin')
on conflict do nothing;

insert into role_permissions(role_id, permission_id)
select r.id, p.id
from roles r
join permissions p on p.code in ('binary_repository.read', 'binary_repository.manage')
where r.code = 'superadmin'
on conflict do nothing;
