-- MegaVPN MVP identity and access foundation.

alter table audit_events
  add column if not exists actor_user_id uuid null;

create table if not exists platform_users(
  id uuid primary key,
  email text not null unique,
  display_name text not null,
  status text not null check(status in ('active','disabled','locked')),
  password_hash text not null,
  auth_source text not null default 'local' check(auth_source in ('local')),
  last_login_at timestamptz null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_platform_users_status on platform_users(status);

create table if not exists roles(
  id uuid primary key,
  code text not null unique,
  name text not null,
  is_system boolean not null default true,
  created_at timestamptz not null default now()
);

create table if not exists permissions(
  id uuid primary key,
  code text not null unique,
  name text not null,
  scope_type text not null check(scope_type in ('global','node','instance','client','artifact','secret','job','audit','endpoint')),
  created_at timestamptz not null default now()
);

alter table permissions
  drop constraint if exists permissions_scope_type_check;

alter table permissions
  add constraint permissions_scope_type_check
  check(scope_type in ('global','node','instance','client','artifact','secret','job','audit','endpoint'));

create table if not exists role_permissions(
  role_id uuid not null references roles(id) on delete cascade,
  permission_id uuid not null references permissions(id) on delete cascade,
  primary key(role_id, permission_id)
);

create table if not exists platform_user_roles(
  user_id uuid not null references platform_users(id) on delete cascade,
  role_id uuid not null references roles(id) on delete cascade,
  assigned_by uuid null references platform_users(id) on delete set null,
  created_at timestamptz not null default now(),
  primary key(user_id, role_id)
);

create table if not exists user_sessions(
  id uuid primary key,
  user_id uuid not null references platform_users(id) on delete cascade,
  session_token_hash text not null unique,
  ip inet null,
  user_agent text null,
  expires_at timestamptz not null,
  revoked_at timestamptz null,
  created_at timestamptz not null default now()
);

create index if not exists idx_user_sessions_user_expires on user_sessions(user_id, expires_at);
create index if not exists idx_user_sessions_revoked on user_sessions(revoked_at);

insert into roles(id, code, name, is_system, created_at)
values
  (gen_random_uuid(), 'superadmin', 'Superadmin', true, now()),
  (gen_random_uuid(), 'admin', 'Admin', true, now()),
  (gen_random_uuid(), 'engineer', 'Engineer', true, now()),
  (gen_random_uuid(), 'readonly', 'Readonly', true, now())
on conflict (code) do nothing;

insert into permissions(id, code, name, scope_type, created_at)
values
  (gen_random_uuid(), 'dashboard.read', 'Read dashboard', 'global', now()),
  (gen_random_uuid(), 'service.read', 'Read services', 'global', now()),
  (gen_random_uuid(), 'node.read', 'Read nodes', 'node', now()),
  (gen_random_uuid(), 'node.write', 'Manage nodes', 'node', now()),
  (gen_random_uuid(), 'node.bootstrap', 'Bootstrap nodes', 'node', now()),
  (gen_random_uuid(), 'instance.read', 'Read instances', 'instance', now()),
  (gen_random_uuid(), 'instance.write', 'Manage instances', 'instance', now()),
  (gen_random_uuid(), 'instance.apply', 'Apply instance revisions', 'instance', now()),
  (gen_random_uuid(), 'client.read', 'Read clients', 'client', now()),
  (gen_random_uuid(), 'client.write', 'Manage clients', 'client', now()),
  (gen_random_uuid(), 'client.provision', 'Provision client access', 'client', now()),
  (gen_random_uuid(), 'artifact.read', 'Read artifacts', 'artifact', now()),
  (gen_random_uuid(), 'artifact.export', 'Export artifacts', 'artifact', now()),
  (gen_random_uuid(), 'share_link.manage', 'Manage share links', 'artifact', now()),
  (gen_random_uuid(), 'job.read', 'Read jobs', 'job', now()),
  (gen_random_uuid(), 'job.write', 'Create jobs', 'job', now()),
  (gen_random_uuid(), 'job.cancel', 'Cancel jobs', 'job', now()),
  (gen_random_uuid(), 'audit.read', 'Read audit logs', 'audit', now()),
  (gen_random_uuid(), 'secret.reveal', 'Reveal secrets', 'secret', now()),
  (gen_random_uuid(), 'settings.manage', 'Manage platform settings', 'global', now()),
  (gen_random_uuid(), 'auth.manage', 'Manage auth and roles', 'global', now()),
  (gen_random_uuid(), 'endpoint.read', 'Read virtual endpoints', 'endpoint', now()),
  (gen_random_uuid(), 'endpoint.write', 'Manage virtual endpoints', 'endpoint', now())
on conflict (code) do nothing;

insert into role_permissions(role_id, permission_id)
select r.id, p.id
from roles r
join permissions p on p.code in (
  'dashboard.read',
  'service.read',
  'node.read',
  'instance.read',
  'client.read',
  'artifact.read',
  'job.read',
  'audit.read',
  'endpoint.read'
)
where r.code = 'readonly'
on conflict do nothing;

insert into role_permissions(role_id, permission_id)
select r.id, p.id
from roles r
join permissions p on p.code in (
  'dashboard.read',
  'service.read',
  'node.read',
  'instance.read',
  'client.read',
  'client.write',
  'client.provision',
  'artifact.read',
  'artifact.export',
  'share_link.manage',
  'job.read',
  'audit.read',
  'endpoint.read'
)
where r.code = 'engineer'
on conflict do nothing;

insert into role_permissions(role_id, permission_id)
select r.id, p.id
from roles r
join permissions p on p.code in (
  'dashboard.read',
  'service.read',
  'node.read',
  'node.write',
  'node.bootstrap',
  'instance.read',
  'instance.write',
  'instance.apply',
  'client.read',
  'client.write',
  'client.provision',
  'artifact.read',
  'artifact.export',
  'share_link.manage',
  'job.read',
  'job.write',
  'job.cancel',
  'audit.read',
  'settings.manage',
  'endpoint.read',
  'endpoint.write'
)
where r.code = 'admin'
on conflict do nothing;

insert into role_permissions(role_id, permission_id)
select r.id, p.id
from roles r
join permissions p on true
where r.code = 'superadmin'
on conflict do nothing;

insert into audit_events(id, actor_user_id, actor_type, action, resource_type, summary, payload_json, created_at)
values(
  gen_random_uuid(),
  null,
  'system',
  'migration.identity_access_foundation',
  'platform',
  'identity and access foundation installed',
  '{}'::jsonb,
  now()
)
on conflict do nothing;
