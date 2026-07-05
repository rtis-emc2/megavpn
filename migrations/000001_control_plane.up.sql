-- RTIS MegaVPN consolidated baseline migration.
-- Release: 7.0.1.3
-- This file squashes the historical migration chain into a single
-- fresh-install baseline. Existing databases that already recorded
-- 000001_control_plane in schema_migrations will skip this file.


-- -----------------------------------------------------------------------------
-- Section: control_plane
-- -----------------------------------------------------------------------------
create extension if not exists pgcrypto;

create table if not exists nodes(
  id uuid primary key,
  name text not null unique,
  kind text not null check(kind in ('local','remote')),
  status text not null check(status in ('draft','bootstrapping','online','degraded','offline','maintenance','draining','retired')),
  address text not null,
  location_label text not null default '',
  latitude double precision null,
  longitude double precision null,
  accuracy_radius_km double precision null,
  geoip_provider text not null default '',
  geoip_status text not null default 'pending',
  geoip_ip text not null default '',
  geoip_country_code text not null default '',
  geoip_country_name text not null default '',
  geoip_region text not null default '',
  geoip_city text not null default '',
  geoip_org text not null default '',
  geoip_asn text not null default '',
  geoip_resolved_at timestamptz null,
  geoip_error text not null default '',
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

-- -----------------------------------------------------------------------------
-- Section: seed
-- -----------------------------------------------------------------------------
insert into service_definitions(id,code,name,category,tier,supports_accounts,supports_artifacts,enabled)
values
  (gen_random_uuid(),'openvpn','OpenVPN','vpn','A',true,true,true),
  (gen_random_uuid(),'xray','Xray','vpn','A',true,true,true),
  (gen_random_uuid(),'nginx','Nginx','edge','A',false,false,true),
  (gen_random_uuid(),'ipsec','IPsec / L2TP / IKE','vpn','A',true,true,true),
  (gen_random_uuid(),'wireguard','WireGuard','vpn','B',true,true,true),
  (gen_random_uuid(),'shadowsocks','Shadowsocks','proxy','B',true,true,true),
  (gen_random_uuid(),'mtproto','MTProto','proxy','C',true,true,true),
  (gen_random_uuid(),'http_proxy','HTTP Proxy','proxy','C',true,false,true)
on conflict(code) do nothing;

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.seed','platform','initial service catalog seeded','{}'::jsonb,now())
on conflict do nothing;

-- -----------------------------------------------------------------------------
-- Section: alpha_enrollment
-- -----------------------------------------------------------------------------
create table if not exists node_enrollment_tokens(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete cascade,
  token_hash text not null unique,
  token_hint text not null,
  status text not null check(status in ('active','used','revoked','expired')),
  expires_at timestamptz not null,
  used_at timestamptz null,
  created_at timestamptz not null default now()
);

create index if not exists idx_node_enrollment_tokens_node on node_enrollment_tokens(node_id,status,expires_at);

alter table jobs add column if not exists locked_by text null;
alter table jobs add column if not exists locked_until timestamptz null;

create index if not exists idx_jobs_node_status on jobs(node_id,status,created_at);

-- -----------------------------------------------------------------------------
-- Section: agent_identity
-- -----------------------------------------------------------------------------
alter table node_agents add column if not exists agent_token_hash text null;
alter table node_agents add column if not exists token_hint text null;
create index if not exists idx_node_agents_token_hash on node_agents(agent_token_hash) where agent_token_hash is not null;

-- -----------------------------------------------------------------------------
-- Section: agent_identity_hardening
-- -----------------------------------------------------------------------------
-- Final alpha hardening for agent identity lifecycle.
-- Keep at most one active enrollment token per node.
update node_enrollment_tokens
set status='expired'
where status='active' and expires_at <= now();

with ranked as (
  select id,
         row_number() over (partition by node_id order by created_at desc) as rn
  from node_enrollment_tokens
  where status='active'
)
update node_enrollment_tokens t
set status='revoked'
from ranked r
where t.id = r.id and r.rn > 1;

create unique index if not exists ux_node_enrollment_tokens_one_active
  on node_enrollment_tokens(node_id)
  where status='active';

create index if not exists idx_node_agents_active_node
  on node_agents(node_id, status)
  where revoked_at is null;

-- -----------------------------------------------------------------------------
-- Section: jobs_hardening
-- -----------------------------------------------------------------------------
-- Jobs subsystem hardening:
-- - DB-backed resource locks for mutating operations
-- - job lease indexes
-- - job_logs lookup indexes

create table if not exists resource_locks(
  id uuid primary key,
  resource_type text not null,
  resource_id uuid not null,
  lock_kind text not null,
  job_id uuid not null references jobs(id) on delete cascade,
  acquired_at timestamptz not null default now(),
  expires_at timestamptz not null,
  unique(resource_type, resource_id, lock_kind),
  check(lock_kind in ('mutate','delete','bootstrap','apply','provision'))
);

create index if not exists idx_resource_locks_job on resource_locks(job_id);
create index if not exists idx_resource_locks_expires_at on resource_locks(expires_at);
create index if not exists idx_job_logs_job_created on job_logs(job_id, created_at asc);
create index if not exists idx_jobs_locked_until on jobs(locked_until) where locked_until is not null;
create index if not exists idx_jobs_scope on jobs(scope_type, scope_id);

update jobs
set status='retrying', locked_by=null, locked_until=null
where status='running' and locked_until is not null and locked_until < now();

-- -----------------------------------------------------------------------------
-- Section: node_inventory
-- -----------------------------------------------------------------------------
-- Node inventory and capability detection.
-- This keeps actual node observations in PostgreSQL while preserving nodes as source-of-truth records.

create table if not exists node_inventory_snapshots(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete cascade,
  payload_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now()
);

create table if not exists node_capabilities(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete cascade,
  capability_code text not null,
  version text null,
  status text not null check(status in ('available','missing','broken','disabled')),
  detected_at timestamptz not null default now(),
  source text not null check(source in ('inventory','manual','bootstrap')),
  unique(node_id, capability_code)
);

create index if not exists idx_node_inventory_snapshots_node_created on node_inventory_snapshots(node_id, created_at desc);
create index if not exists idx_node_capabilities_node_code on node_capabilities(node_id, capability_code);
create index if not exists idx_node_capabilities_status on node_capabilities(status);

-- -----------------------------------------------------------------------------
-- Section: agent_inventory_claim_fix
-- -----------------------------------------------------------------------------
-- Agent inventory claim fix:
-- inventory jobs are read-only collection jobs and must not be blocked by
-- stale node bootstrap locks left by older alpha builds.

delete from resource_locks rl
using jobs j
where rl.job_id = j.id
  and j.type in ('node.inventory', 'node.inventory.sync')
  and j.status in ('queued', 'retrying');

update jobs
set locked_by = null,
    locked_until = null
where type in ('node.inventory', 'node.inventory.sync')
  and status in ('queued', 'retrying');

-- -----------------------------------------------------------------------------
-- Section: claim_conn_busy_fix
-- -----------------------------------------------------------------------------
-- Code-only migration marker retained for ordered migration compatibility in release 7.0.1.2.
-- Fixes pgx "conn busy" in job claiming by closing candidate rows before issuing lock/update commands.
select 1;

-- -----------------------------------------------------------------------------
-- Section: service_discovery
-- -----------------------------------------------------------------------------
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

-- -----------------------------------------------------------------------------
-- Section: service_discovery_global
-- -----------------------------------------------------------------------------
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

-- -----------------------------------------------------------------------------
-- Section: capability_install_framework
-- -----------------------------------------------------------------------------
-- Capability installation framework retained for release 7.0.1.2.
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

-- -----------------------------------------------------------------------------
-- Section: capability_install_hardening
-- -----------------------------------------------------------------------------
-- Capability install hardening retained for release 7.0.1.2.
-- Makes capability result storage tolerant to failed installers and installer-owned states.

alter table node_capabilities
  drop constraint if exists node_capabilities_status_check;

alter table node_capabilities
  add constraint node_capabilities_status_check
  check(status in ('available','missing','broken','disabled','installing','failed','degraded','unknown'));

alter table node_capabilities
  drop constraint if exists node_capabilities_source_check;

alter table node_capabilities
  add constraint node_capabilities_source_check
  check(source in ('inventory','manual','bootstrap','installer','verification','system'));

alter table node_capability_install_events
  drop constraint if exists node_capability_install_events_status_check;

alter table node_capability_install_events
  add constraint node_capability_install_events_status_check
  check(status in ('queued','running','succeeded','failed','verified','preflight_failed','fallback_used','cancelled'));

create index if not exists idx_node_capability_install_events_job
  on node_capability_install_events(job_id, created_at desc);

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.capability_install_hardening','platform','capability install hardening schema installed','{}'::jsonb,now())
on conflict do nothing;

-- -----------------------------------------------------------------------------
-- Section: nginx_install_cleanup_fix
-- -----------------------------------------------------------------------------
-- Nginx installer cleanup / fallback hardening marker.
-- Runtime behavior is implemented in the agent installer. This migration records the build-level fix in audit.

insert into audit_events (
  id,
  actor_type,
  action,
  resource_type,
  summary,
  payload_json,
  created_at
)
values (
  gen_random_uuid(),
  'system',
  'migration.nginx_install_cleanup_fix',
  'platform',
  'nginx install cleanup and fallback hardening installed',
  '{"scope":["nginx_org_repo_noninteractive_gpg","ubuntu_repo_clean_fallback","ubuntu_26_04_resolute_preflight","apt_policy_guard"]}'::jsonb,
  now()
)
on conflict do nothing;

-- -----------------------------------------------------------------------------
-- Section: identity_access_foundation
-- -----------------------------------------------------------------------------
-- MegaVPN identity and access foundation.

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

-- -----------------------------------------------------------------------------
-- Section: secret_refs_and_agent_trust
-- -----------------------------------------------------------------------------
-- MegaVPN secret refs and agent trust foundation.

create table if not exists secret_refs(
  id uuid primary key,
  secret_type text not null check(secret_type in ('password','uuid','private_key','public_key','certificate','psk','ssh_key','api_token','opaque')),
  ciphertext bytea not null,
  key_version text not null,
  nonce bytea null,
  meta_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  rotated_at timestamptz null
);

create table if not exists agent_trust_roots(
  id uuid primary key,
  name text not null,
  status text not null check(status in ('active','rotated','revoked')),
  ca_cert_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  ca_key_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  created_by uuid null references platform_users(id) on delete set null,
  created_at timestamptz not null default now(),
  rotated_at timestamptz null
);

create table if not exists node_agent_certificates(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete cascade,
  trust_root_id uuid not null references agent_trust_roots(id) on delete restrict,
  serial_no text not null unique,
  cert_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  key_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  status text not null check(status in ('issued','active','rotated','revoked','expired')),
  issued_at timestamptz not null,
  expires_at timestamptz not null,
  revoked_at timestamptz null
);

create index if not exists idx_node_agent_certificates_node on node_agent_certificates(node_id, status);
create index if not exists idx_node_agent_certificates_expires on node_agent_certificates(expires_at);

insert into audit_events(id, actor_user_id, actor_type, action, resource_type, summary, payload_json, created_at)
values(
  gen_random_uuid(),
  null,
  'system',
  'migration.secret_refs_and_agent_trust',
  'platform',
  'secret refs and agent trust foundation installed',
  '{}'::jsonb,
  now()
)
on conflict do nothing;

-- -----------------------------------------------------------------------------
-- Section: node_bootstrap_foundation
-- -----------------------------------------------------------------------------
-- Node bootstrap foundation.

alter table nodes
  add column if not exists role text not null default 'egress';

alter table nodes
  add column if not exists location_label text not null default '';

alter table nodes
  add column if not exists latitude double precision null;

alter table nodes
  add column if not exists longitude double precision null;

alter table nodes
  add column if not exists accuracy_radius_km double precision null;

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

-- -----------------------------------------------------------------------------
-- Section: platform_user_usernames
-- -----------------------------------------------------------------------------
-- Platform user usernames and bootstrap login support.

alter table platform_users
  add column if not exists username text;

update platform_users
set username = lower(
  case
    when position('@' in email) > 0 then split_part(email, '@', 1)
    else email
  end
)
where username is null or btrim(username) = '';

alter table platform_users
  alter column username set not null;

create unique index if not exists idx_platform_users_username on platform_users(username);

insert into audit_events(id, actor_user_id, actor_type, action, resource_type, summary, payload_json, created_at)
values(
  gen_random_uuid(),
  null,
  'system',
  'migration.platform_user_usernames',
  'platform',
  'platform user usernames installed',
  '{}'::jsonb,
  now()
)
on conflict do nothing;

-- -----------------------------------------------------------------------------
-- Section: mail_and_invites_foundation
-- -----------------------------------------------------------------------------
create table if not exists platform_mail_settings(
  id boolean primary key default true,
  enabled boolean not null default false,
  provider text not null default 'smtp',
  smtp_host text not null default '',
  smtp_port integer not null default 587,
  smtp_username text not null default '',
  smtp_password_secret_ref_id uuid null references secret_refs(id) on delete set null,
  smtp_auth_mode text not null default 'plain',
  smtp_tls_mode text not null default 'starttls',
  from_email text not null default '',
  from_name text not null default '',
  reply_to_email text not null default '',
  invite_url_base text not null default '',
  last_test_at timestamptz null,
  last_error text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint platform_mail_settings_singleton check (id = true),
  constraint platform_mail_settings_provider_check check (provider in ('smtp')),
  constraint platform_mail_settings_auth_mode_check check (smtp_auth_mode in ('none','plain')),
  constraint platform_mail_settings_tls_mode_check check (smtp_tls_mode in ('none','starttls','starttls_required'))
);

insert into platform_mail_settings(id, created_at, updated_at)
values (true, now(), now())
on conflict (id) do nothing;

create table if not exists platform_user_invites(
  id uuid primary key,
  user_id uuid not null references platform_users(id) on delete cascade,
  email text not null,
  token_hash text not null unique,
  token_hint text not null default '',
  status text not null default 'pending',
  expires_at timestamptz not null,
  sent_at timestamptz null,
  accepted_at timestamptz null,
  delivery_error text not null default '',
  created_by uuid null references platform_users(id) on delete set null,
  created_at timestamptz not null default now(),
  constraint platform_user_invites_status_check check (status in ('pending','sent','accepted','revoked','expired','delivery_failed'))
);

create index if not exists idx_platform_user_invites_user_status on platform_user_invites(user_id, status);
create index if not exists idx_platform_user_invites_expires on platform_user_invites(expires_at);

create table if not exists client_email_deliveries(
  id uuid primary key,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  email text not null,
  subject text not null,
  status text not null default 'queued',
  artifact_ids jsonb not null default '[]'::jsonb,
  share_link_ids jsonb not null default '[]'::jsonb,
  payload_json jsonb not null default '{}'::jsonb,
  error_text text not null default '',
  created_by uuid null references platform_users(id) on delete set null,
  sent_at timestamptz null,
  created_at timestamptz not null default now(),
  constraint client_email_deliveries_status_check check (status in ('queued','sent','failed'))
);

create index if not exists idx_client_email_deliveries_client_created on client_email_deliveries(client_account_id, created_at desc);

-- -----------------------------------------------------------------------------
-- Section: agent_communication_diagnostics
-- -----------------------------------------------------------------------------
alter table node_agents add column if not exists last_auth_failure_at timestamptz null;
alter table node_agents add column if not exists last_auth_failure_reason text not null default '';
alter table node_agents add column if not exists last_job_poll_at timestamptz null;
alter table node_agents add column if not exists last_job_claim_at timestamptz null;
alter table node_agents add column if not exists last_job_claim_job_id uuid null references jobs(id) on delete set null;
alter table node_agents add column if not exists last_job_claim_type text not null default '';
alter table node_agents add column if not exists last_job_result_at timestamptz null;
alter table node_agents add column if not exists last_job_result_job_id uuid null references jobs(id) on delete set null;
alter table node_agents add column if not exists last_job_result_type text not null default '';
alter table node_agents add column if not exists last_job_result_status text not null default '';
alter table node_agents add column if not exists last_inventory_sync_at timestamptz null;
alter table node_agents add column if not exists last_discovery_sync_at timestamptz null;

create index if not exists idx_node_agents_last_job_claim_at on node_agents(last_job_claim_at desc);
create index if not exists idx_node_agents_last_job_result_at on node_agents(last_job_result_at desc);

-- -----------------------------------------------------------------------------
-- Section: xl2tpd_service_definition
-- -----------------------------------------------------------------------------
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

-- -----------------------------------------------------------------------------
-- Section: ipsec_systemd_unit_fix
-- -----------------------------------------------------------------------------
-- Normalize IPsec instances to the strongswan-starter systemd unit.

update instances
set systemd_unit = 'strongswan-starter',
    updated_at = now()
where service_definition_id = (select id from service_definitions where code = 'ipsec')
  and coalesce(systemd_unit, '') in ('', 'strongswan');

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.ipsec_systemd_unit_fix','platform','ipsec systemd unit normalized to strongswan-starter','{}'::jsonb,now())
on conflict do nothing;

-- -----------------------------------------------------------------------------
-- Section: platform_service_pki_roots
-- -----------------------------------------------------------------------------
-- Platform service PKI roots.

create table if not exists platform_service_pki_roots(
  id uuid primary key,
  service_code text not null,
  pki_profile text not null default 'default',
  status text not null check(status in ('active','rotated','revoked')),
  ca_cert_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  ca_key_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  common_name text not null,
  created_at timestamptz not null default now(),
  rotated_at timestamptz null
);

create unique index if not exists idx_platform_service_pki_roots_active
on platform_service_pki_roots(service_code, pki_profile)
where status = 'active';

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.platform_service_pki_roots','platform','platform service pki roots installed','{}'::jsonb,now())
on conflict do nothing;

-- -----------------------------------------------------------------------------
-- Section: platform_certificates
-- -----------------------------------------------------------------------------
create table if not exists platform_certificates(
  id uuid primary key,
  name text not null,
  description text not null default '',
  source text not null check(source in ('imported','self_signed','managed_ca','ca_issued','letsencrypt')),
  kind text not null check(kind in ('leaf','ca')),
  status text not null default 'active',
  common_name text not null default '',
  san_json jsonb not null default '[]'::jsonb,
  issuer_name text not null default '',
  parent_certificate_id uuid null references platform_certificates(id) on delete set null,
  cert_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  key_secret_ref_id uuid null references secret_refs(id) on delete restrict,
  chain_secret_ref_id uuid null references secret_refs(id) on delete restrict,
  not_before timestamptz null,
  not_after timestamptz null,
  is_default boolean not null default false,
  meta_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_platform_certificates_kind_status
on platform_certificates(kind,status,created_at desc);

create index if not exists idx_platform_certificates_default
on platform_certificates(is_default,kind,status);

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.platform_certificates','platform','platform certificates installed','{}'::jsonb,now())
on conflict do nothing;

-- -----------------------------------------------------------------------------
-- Section: active_node_name_uniqueness
-- -----------------------------------------------------------------------------
alter table nodes drop constraint if exists nodes_name_key;

create unique index if not exists nodes_name_active_key
  on nodes(name)
  where status <> 'retired';

-- -----------------------------------------------------------------------------
-- Section: control_plane_tls_settings
-- -----------------------------------------------------------------------------
create table if not exists platform_control_plane_tls_settings(
  id boolean primary key default true,
  enabled boolean not null default true,
  mode text not null default 'managed_certificate',
  public_base_url text not null default '',
  server_name text not null default '',
  listen_port integer not null default 443,
  upstream_url text not null default 'http://127.0.0.1:8080',
  certificate_id uuid null references platform_certificates(id) on delete set null,
  self_signed_common_name text not null default '',
  self_signed_san_json jsonb not null default '[]'::jsonb,
  last_applied_at timestamptz null,
  last_error text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint platform_control_plane_tls_settings_singleton check (id = true),
  constraint platform_control_plane_tls_settings_mode_check check (mode in ('managed_certificate','self_signed_fallback')),
  constraint platform_control_plane_tls_settings_port_check check (listen_port between 1 and 65535)
);

insert into platform_control_plane_tls_settings(id, created_at, updated_at)
values(true, now(), now())
on conflict(id) do nothing;

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.control_plane_tls_settings','platform','control plane tls settings installed','{}'::jsonb,now())
on conflict do nothing;

-- -----------------------------------------------------------------------------
-- Section: secret_redaction_hardening
-- -----------------------------------------------------------------------------
-- Remove historical plaintext bootstrap and agent-rotation secrets from JSON payload history.

update jobs
set payload_json = jsonb_set(
  payload_json - 'new_agent_token',
  '{new_agent_token_hash}',
  to_jsonb(encode(digest(coalesce(payload_json->>'new_agent_token',''), 'sha256'), 'hex')),
  true
)
where type = 'node.agent.rotate_token'
  and payload_json ? 'new_agent_token';

update jobs
set status = 'cancelled',
    finished_at = coalesce(finished_at, now()),
    locked_by = null,
    locked_until = null,
    result_json = coalesce(result_json, '{}'::jsonb) ||
      jsonb_build_object(
        'message', 'token rotation cancelled by migration because plaintext payload was redacted; queue a fresh rotation',
        'redacted_by_migration', '000027_secret_redaction_hardening'
      )
where type = 'node.agent.rotate_token'
  and (
    status in ('queued','retrying')
    or (status = 'running' and locked_until is not null and locked_until < now())
  )
  and not (payload_json ? 'new_agent_token_secret_ref_id');

delete from resource_locks
where job_id in (
  select id
  from jobs
  where type = 'node.agent.rotate_token'
    and status = 'cancelled'
    and coalesce(result_json->>'redacted_by_migration','') = '000027_secret_redaction_hardening'
);

update jobs
set result_json = (coalesce(result_json, '{}'::jsonb) - 'agent_bootstrapenv') ||
  jsonb_build_object(
    'agent_bootstrapenv_redacted', true,
    'redacted_by_migration', '000027_secret_redaction_hardening'
  )
where coalesce(result_json, '{}'::jsonb) ? 'agent_bootstrapenv';

update node_bootstrap_runs
set result_payload_json = (coalesce(result_payload_json, '{}'::jsonb) - 'agent_bootstrapenv') ||
  jsonb_build_object(
    'agent_bootstrapenv_redacted', true,
    'redacted_by_migration', '000027_secret_redaction_hardening'
  )
where coalesce(result_payload_json, '{}'::jsonb) ? 'agent_bootstrapenv';

update job_logs
set payload_json = (coalesce(payload_json, '{}'::jsonb) - 'new_agent_token' - 'agent_bootstrapenv') ||
  jsonb_build_object(
    'secrets_redacted', true,
    'redacted_by_migration', '000027_secret_redaction_hardening'
  )
where coalesce(payload_json, '{}'::jsonb) ?| array['new_agent_token','agent_bootstrapenv'];

insert into audit_events(id, actor_user_id, actor_type, action, resource_type, summary, payload_json, created_at)
values(
  gen_random_uuid(),
  null,
  'system',
  'migration.secret_redaction_hardening',
  'platform',
  'historical plaintext job/bootstrap secrets redacted',
  '{}'::jsonb,
  now()
)
on conflict do nothing;

-- -----------------------------------------------------------------------------
-- Section: client_access_routes
-- -----------------------------------------------------------------------------
create table if not exists client_access_routes(
  id uuid primary key,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  service_access_id uuid null references service_accesses(id) on delete cascade,
  instance_id uuid null references instances(id) on delete set null,
  node_id uuid null references nodes(id) on delete set null,
  name text not null,
  status text not null check(status in ('pending','active','disabled','revoked')),
  action text not null default 'allow' check(action in ('allow','deny')),
  destination_type text not null check(destination_type in ('endpoint','cidr','dns','service')),
  destination text not null,
  protocol text not null default 'any' check(protocol in ('any','tcp','udp','icmp')),
  ports text not null default '*',
  description text null,
  policy_json jsonb not null default '{}'::jsonb,
  metadata_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_client_access_routes_client_status
on client_access_routes(client_account_id,status,created_at desc);

create index if not exists idx_client_access_routes_node_status
on client_access_routes(node_id,status,created_at desc);

create index if not exists idx_client_access_routes_access
on client_access_routes(service_access_id);

create unique index if not exists idx_client_access_routes_baseline_access
on client_access_routes(service_access_id)
where service_access_id is not null and (metadata_json->>'baseline') = 'true';

insert into audit_events(id,actor_type,action,resource_type,resource_id,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.client_access_routes','routing',null,'client access routing registry installed','{}'::jsonb,now())
on conflict do nothing;

-- -----------------------------------------------------------------------------
-- Section: instance_runtime_states
-- -----------------------------------------------------------------------------
create table if not exists instance_runtime_states(
  id uuid primary key,
  instance_id uuid not null references instances(id) on delete cascade,
  node_id uuid null references nodes(id) on delete set null,
  service_code text not null default '',
  systemd_unit text not null default '',
  desired_status text not null default 'unknown',
  runtime_status text not null default 'unknown',
  health_status text not null default 'unknown',
  drift_status text not null default 'unknown',
  active_state text not null default '',
  last_job_id uuid null references jobs(id) on delete set null,
  last_job_type text not null default '',
  last_job_status text not null default '',
  applied_revision_id uuid null references instance_revisions(id) on delete set null,
  observed_revision_id uuid null references instance_revisions(id) on delete set null,
  endpoint_host text not null default '',
  endpoint_port integer null,
  result_json jsonb not null default '{}'::jsonb,
  error_text text not null default '',
  checked_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(instance_id)
);

create index if not exists idx_instance_runtime_states_node_health
  on instance_runtime_states(node_id, health_status);

create index if not exists idx_instance_runtime_states_drift
  on instance_runtime_states(drift_status);

-- -----------------------------------------------------------------------------
-- Section: agent_runtime_observations
-- -----------------------------------------------------------------------------
alter table instance_runtime_states
  add column if not exists enabled_state text not null default '',
  add column if not exists config_hash text not null default '',
  add column if not exists listening_ports_json jsonb not null default '[]'::jsonb,
  add column if not exists agent_reported_at timestamptz null;

alter table node_agents
  add column if not exists last_runtime_sync_at timestamptz null;

create index if not exists idx_instance_runtime_states_agent_reported_at
  on instance_runtime_states(agent_reported_at desc);

-- -----------------------------------------------------------------------------
-- Section: instance_runtime_observations_history
-- -----------------------------------------------------------------------------
create table if not exists instance_runtime_observations(
  id uuid primary key,
  instance_id uuid not null references instances(id) on delete cascade,
  node_id uuid null references nodes(id) on delete set null,
  source text not null default '',
  service_code text not null default '',
  systemd_unit text not null default '',
  desired_status text not null default 'unknown',
  runtime_status text not null default 'unknown',
  health_status text not null default 'unknown',
  drift_status text not null default 'unknown',
  active_state text not null default '',
  enabled_state text not null default '',
  config_hash text not null default '',
  last_job_id uuid null references jobs(id) on delete set null,
  last_job_type text not null default '',
  last_job_status text not null default '',
  applied_revision_id uuid null references instance_revisions(id) on delete set null,
  observed_revision_id uuid null references instance_revisions(id) on delete set null,
  endpoint_host text not null default '',
  endpoint_port integer null,
  listening_ports_json jsonb not null default '[]'::jsonb,
  result_json jsonb not null default '{}'::jsonb,
  error_text text not null default '',
  observed_at timestamptz not null default now(),
  received_at timestamptz not null default now()
);

create index if not exists idx_instance_runtime_observations_instance_time
  on instance_runtime_observations(instance_id, observed_at desc, received_at desc);

create index if not exists idx_instance_runtime_observations_node_time
  on instance_runtime_observations(node_id, observed_at desc, received_at desc);

create index if not exists idx_instance_runtime_observations_health_drift
  on instance_runtime_observations(health_status, drift_status, observed_at desc);

create index if not exists idx_instance_runtime_observations_received_at
  on instance_runtime_observations(received_at);

-- -----------------------------------------------------------------------------
-- Section: managed_backhaul
-- -----------------------------------------------------------------------------
create table if not exists backhaul_links(
  id uuid primary key,
  name text not null,
  ingress_node_id uuid not null references nodes(id) on delete restrict,
  egress_node_id uuid not null references nodes(id) on delete restrict,
  status text not null default 'planned',
  selected_transport_id uuid null,
  desired_driver text not null default 'wireguard',
  routing_table text not null default 'main',
  route_metric integer not null default 50,
  failover_policy_json jsonb not null default '{}'::jsonb,
  metadata_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint backhaul_links_distinct_nodes check (ingress_node_id <> egress_node_id),
  constraint backhaul_links_status_check check (status in ('planned','pending_apply','active','disabled','failed','deleted'))
);

create unique index if not exists backhaul_links_active_pair_idx
on backhaul_links(ingress_node_id, egress_node_id, name)
where status <> 'deleted';

create index if not exists backhaul_links_ingress_idx
on backhaul_links(ingress_node_id)
where status <> 'deleted';

create index if not exists backhaul_links_egress_idx
on backhaul_links(egress_node_id)
where status <> 'deleted';

create table if not exists backhaul_transports(
  id uuid primary key,
  link_id uuid not null references backhaul_links(id) on delete cascade,
  driver text not null,
  priority integer not null default 100,
  status text not null default 'planned',
  endpoint_host text not null default '',
  endpoint_port integer not null default 0,
  protocol text not null default '',
  interface_name text not null default '',
  tunnel_cidr text not null default '',
  ingress_address text not null default '',
  egress_address text not null default '',
  config_json jsonb not null default '{}'::jsonb,
  secret_refs_json jsonb not null default '{}'::jsonb,
  health_json jsonb not null default '{}'::jsonb,
  applied_ingress_at timestamptz null,
  applied_egress_at timestamptz null,
  last_error text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint backhaul_transports_status_check check (status in ('planned','pending_apply','active','disabled','failed'))
);

create unique index if not exists backhaul_transports_link_driver_idx
on backhaul_transports(link_id, driver);

create index if not exists backhaul_transports_active_driver_idx
on backhaul_transports(driver, status);

alter table backhaul_links
  drop constraint if exists backhaul_links_selected_transport_fk;

alter table backhaul_links
  add constraint backhaul_links_selected_transport_fk
  foreign key(selected_transport_id) references backhaul_transports(id) on delete set null;

-- -----------------------------------------------------------------------------
-- Section: job_lease_recovery
-- -----------------------------------------------------------------------------
-- Code-only migration marker retained for ordered migration compatibility in release 7.0.1.2.
-- Adds backend stale job lease recovery used by Jobs API and managed backhaul cleanup/delete flows.
select 1;

-- -----------------------------------------------------------------------------
-- Section: agent_version_visibility
-- -----------------------------------------------------------------------------
-- Code-only migration marker retained for ordered migration compatibility in release 7.0.1.2.
-- Agent register/heartbeat now refresh agent_version/protocol_version and Nodes UI exposes per-node/bulk agent update actions.
select 1;

-- -----------------------------------------------------------------------------
-- Section: backhaul_profile_apply_semantics
-- -----------------------------------------------------------------------------
-- Code-only migration marker retained for ordered migration compatibility in release 7.0.1.2.
-- Backhaul apply now queues all selected transport profiles and treats
-- materialize-only drivers as materialized, not active.

-- -----------------------------------------------------------------------------
-- Section: backhaul_apply_runtime_dependencies
-- -----------------------------------------------------------------------------
-- Code-only migration marker retained for ordered migration compatibility in release 7.0.1.2.
-- Backhaul apply now verifies/installs WireGuard/OpenVPN runtime dependencies,
-- enforces unique L3 transport CIDRs and uses node-scoped locks for node jobs.

-- -----------------------------------------------------------------------------
-- Section: ssh_bootstrap_host_key_pin
-- -----------------------------------------------------------------------------
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

-- -----------------------------------------------------------------------------
-- Section: share_link_token_hash
-- -----------------------------------------------------------------------------
create extension if not exists pgcrypto;

alter table share_links
  add column if not exists token_hash text null;

alter table share_links
  add column if not exists token_hint text not null default '';

update share_links
set token_hash = encode(digest(token, 'sha256'), 'hex'),
    token_hint = case
      when length(token) <= 14 then token
      else substring(token from 1 for 8) || '...' || substring(token from length(token) - 5 for 6)
    end
where token_hash is null
  and token is not null
  and token <> '';

update share_links
set token_hash = encode(digest(id::text || ':' || created_at::text, 'sha256'), 'hex'),
    token_hint = 'revoked',
    status = 'revoked'
where token_hash is null;

alter table share_links
  alter column token_hash set not null;

alter table share_links
  alter column token drop not null;

alter table share_links
  drop constraint if exists share_links_token_key;

create unique index if not exists idx_share_links_token_hash
  on share_links(token_hash);

update share_links
set token = null
where token is not null;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
values (gen_random_uuid(), 'system', 'migration.share_link_token_hash', 'security', 'share link plaintext tokens migrated to token_hash/token_hint', '{}'::jsonb, now());

-- -----------------------------------------------------------------------------
-- Section: service_pack_catalog
-- -----------------------------------------------------------------------------
create extension if not exists pgcrypto;

create table if not exists service_pack_templates (
  key text primary key,
  label text not null,
  description text not null default '',
  base_name_template text not null default '',
  endpoint_hint text not null default '',
  requires_endpoint_host boolean not null default true,
  platform_notes_json jsonb not null default '[]'::jsonb,
  recommendations_json jsonb not null default '[]'::jsonb,
  components_json jsonb not null default '[]'::jsonb,
  status text not null default 'active',
  source text not null default 'default',
  version integer not null default 1,
  display_order integer not null default 1000,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint service_pack_templates_key_check check (key ~ '^[a-z0-9][a-z0-9_-]{1,95}$'),
  constraint service_pack_templates_status_check check (status in ('active','disabled','deleted')),
  constraint service_pack_templates_version_check check (version > 0),
  constraint service_pack_templates_components_array_check check (jsonb_typeof(components_json) = 'array'),
  constraint service_pack_templates_platform_notes_array_check check (jsonb_typeof(platform_notes_json) = 'array'),
  constraint service_pack_templates_recommendations_array_check check (jsonb_typeof(recommendations_json) = 'array')
);

create index if not exists idx_service_pack_templates_status_order
  on service_pack_templates(status, display_order, label);

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
values (gen_random_uuid(), 'system', 'migration.service_pack_catalog', 'service_pack', 'service pack template catalog created', '{}'::jsonb, now());

-- -----------------------------------------------------------------------------
-- Section: default_access_suite_pack
-- -----------------------------------------------------------------------------
insert into service_pack_templates(
  key,
  label,
  description,
  base_name_template,
  endpoint_hint,
  requires_endpoint_host,
  platform_notes_json,
  recommendations_json,
  components_json,
  status,
  source,
  version,
  display_order,
  created_at,
  updated_at
) values (
  'default_access_suite',
  'Default Remote Access Suite',
  'Creates a baseline multi-protocol remote-access suite: VLESS + OpenVPN TCP pair, standalone VLESS, OpenVPN UDP, Shadowsocks and WireGuard.',
  'edge-access',
  'access.example.com',
  true,
  $json$[
    "Template does not store runtime secrets: Reality keys, WireGuard private key and Shadowsocks password are generated during revision/apply and stored as secret refs.",
    "Components use distinct listen ports so the pack can be created on one node and one endpoint host."
  ]$json$::jsonb,
  $json$[
    "Verify DNS, firewall/NAT and conflict-free ports on the selected node before production rollout.",
    "WireGuard/OpenVPN address pools are allocated automatically from Address Pools catalog.",
    "Use a valid endpoint host/SNI for the public VLESS listener on 443; the second VLESS listener uses 8443 to avoid a port conflict."
  ]$json$::jsonb,
  $json$[
    {
      "label": "VLESS TCP Edge",
      "description": "VLESS/Reality component for the VLESS + OpenVPN TCP pair.",
      "service_code": "xray-core",
      "preset_key": "reality_tcp",
      "name_suffix": "vless-tcp-edge",
      "slug_suffix": "vless-tcp-edge",
      "endpoint_port": 443,
      "requires_endpoint_host": true,
      "spec": {
        "service_profile": "reality_tcp",
        "security": "reality",
        "network": "tcp",
        "dest": "www.cloudflare.com:443",
        "fingerprint": "chrome",
        "auto_generate_reality_keys": true,
        "config_mode": "0640"
      }
    },
    {
      "label": "OpenVPN TCP Companion",
      "description": "OpenVPN TCP component for the VLESS + OpenVPN TCP pair.",
      "service_code": "openvpn",
      "preset_key": "tcp_11994",
      "name_suffix": "openvpn-tcp",
      "slug_suffix": "openvpn-tcp",
      "endpoint_port": 11994,
      "requires_endpoint_host": true,
      "spec": {
        "service_profile": "tcp_11994",
        "pki_scope": "platform",
        "pki_profile": "default",
        "proto": "tcp",
        "dev": "tun",
        "address_pool_mode": "auto",
        "server_extra_lines": [
          "push \"redirect-gateway def1 bypass-dhcp\"",
          "push \"dhcp-option DNS 1.1.1.1\"",
          "push \"dhcp-option DNS 1.0.0.1\""
        ],
        "config_mode": "0644"
      }
    },
    {
      "label": "VLESS Standalone",
      "description": "Standalone VLESS/Reality instance on an alternative TCP port.",
      "service_code": "xray-core",
      "preset_key": "reality_tcp",
      "name_suffix": "vless",
      "slug_suffix": "vless",
      "endpoint_port": 8443,
      "requires_endpoint_host": true,
      "spec": {
        "service_profile": "reality_tcp",
        "security": "reality",
        "network": "tcp",
        "dest": "www.cloudflare.com:443",
        "fingerprint": "chrome",
        "auto_generate_reality_keys": true,
        "config_mode": "0640"
      }
    },
    {
      "label": "OpenVPN UDP",
      "description": "Classic OpenVPN UDP baseline.",
      "service_code": "openvpn",
      "preset_key": "udp_1194",
      "name_suffix": "openvpn-udp",
      "slug_suffix": "openvpn-udp",
      "endpoint_port": 1194,
      "requires_endpoint_host": true,
      "spec": {
        "service_profile": "udp_1194",
        "pki_scope": "platform",
        "pki_profile": "default",
        "proto": "udp",
        "dev": "tun",
        "address_pool_mode": "auto",
        "server_extra_lines": [
          "push \"redirect-gateway def1 bypass-dhcp\"",
          "push \"dhcp-option DNS 1.1.1.1\"",
          "push \"dhcp-option DNS 1.0.0.1\""
        ],
        "config_mode": "0644"
      }
    },
    {
      "label": "Shadowsocks",
      "description": "Standalone Shadowsocks chacha20-ietf-poly1305 baseline.",
      "service_code": "shadowsocks",
      "preset_key": "chacha_full",
      "name_suffix": "shadowsocks",
      "slug_suffix": "shadowsocks",
      "endpoint_port": 8388,
      "requires_endpoint_host": true,
      "spec": {
        "service_profile": "chacha_full",
        "method": "chacha20-ietf-poly1305",
        "mode": "tcp_and_udp",
        "timeout": 300,
        "auto_generate_server_password": true,
        "config_mode": "0640"
      }
    },
    {
      "label": "WireGuard",
      "description": "Standalone WireGuard road-warrior baseline.",
      "service_code": "wireguard",
      "preset_key": "roadwarrior",
      "name_suffix": "wireguard",
      "slug_suffix": "wireguard",
      "endpoint_port": 51820,
      "requires_endpoint_host": true,
      "spec": {
        "service_profile": "roadwarrior",
        "address_pool_mode": "auto",
        "client_allowed_ips": "0.0.0.0/0, ::/0",
        "client_dns": "1.1.1.1, 1.0.0.1",
        "persistent_keepalive": 25,
        "auto_generate_server_key": true,
        "config_mode": "0600"
      }
    }
  ]$json$::jsonb,
  'active',
  'default',
  1,
  5,
  now(),
  now()
)
on conflict(key) do nothing;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
values (gen_random_uuid(), 'system', 'migration.default_access_suite_pack', 'service_pack', 'default access suite service pack seeded', '{}'::jsonb, now());

-- -----------------------------------------------------------------------------
-- Section: address_pool_catalog
-- -----------------------------------------------------------------------------
create extension if not exists pgcrypto;

create table if not exists address_pool_spaces (
  id uuid primary key default gen_random_uuid(),
  key text not null unique,
  label text not null,
  description text not null default '',
  family text not null default 'ipv4',
  base_cidr cidr not null,
  start_cidr cidr not null,
  allocation_prefix integer not null,
  service_scope text not null default 'remote_access',
  routing_enabled boolean not null default false,
  status text not null default 'active',
  display_order integer not null default 1000,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint address_pool_spaces_key_check check (key ~ '^[a-z0-9][a-z0-9_-]{1,95}$'),
  constraint address_pool_spaces_family_check check (family in ('ipv4')),
  constraint address_pool_spaces_status_check check (status in ('active','disabled','deleted')),
  constraint address_pool_spaces_prefix_check check (allocation_prefix >= masklen(base_cidr) and allocation_prefix <= 32),
  constraint address_pool_spaces_start_check check (start_cidr <<= base_cidr)
);

create table if not exists address_pool_allocations (
  id uuid primary key default gen_random_uuid(),
  pool_space_id uuid not null references address_pool_spaces(id) on delete restrict,
  cidr cidr not null,
  node_id uuid references nodes(id) on delete set null,
  instance_id uuid references instances(id) on delete set null,
  service_code text not null default '',
  purpose text not null default 'remote_access',
  status text not null default 'active',
  route_export boolean not null default false,
  metadata_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint address_pool_allocations_status_check check (status in ('reserved','active','released')),
  constraint address_pool_allocations_metadata_object_check check (jsonb_typeof(metadata_json) = 'object')
);

create unique index if not exists uq_address_pool_allocations_active_cidr
  on address_pool_allocations(pool_space_id, cidr)
  where status in ('reserved','active');

create unique index if not exists uq_address_pool_allocations_instance_purpose
  on address_pool_allocations(instance_id, purpose)
  where instance_id is not null and status in ('reserved','active');

create index if not exists idx_address_pool_allocations_node
  on address_pool_allocations(node_id, status);

create index if not exists idx_address_pool_allocations_instance
  on address_pool_allocations(instance_id, status);

insert into address_pool_spaces(
  key,label,description,family,base_cidr,start_cidr,allocation_prefix,service_scope,routing_enabled,status,display_order,created_at,updated_at
) values (
  'remote_access_v4',
  'Remote Access IPv4',
  'Default IPv4 supernet for WireGuard, OpenVPN and L2TP client pools.',
  'ipv4',
  '172.16.0.0/12',
  '172.16.112.0/24',
  24,
  'remote_access',
  false,
  'active',
  10,
  now(),
  now()
) on conflict(key) do nothing;

insert into address_pool_spaces(
  key,label,description,family,base_cidr,start_cidr,allocation_prefix,service_scope,routing_enabled,status,display_order,created_at,updated_at
) values (
  'imported_remote_access_v4',
  'Imported Remote Access IPv4',
  'Observed pools imported from existing instance revisions. The allocator does not use this space for new automatic allocations.',
  'ipv4',
  '10.0.0.0/8',
  '10.0.0.0/24',
  24,
  'imported',
  false,
  'active',
  900,
  now(),
  now()
) on conflict(key) do nothing;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
values (gen_random_uuid(), 'system', 'migration.address_pool_catalog', 'address_pool', 'address pool catalog created', '{}'::jsonb, now());

-- -----------------------------------------------------------------------------
-- Section: binary_repository_and_node_cleanup
-- -----------------------------------------------------------------------------
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

-- -----------------------------------------------------------------------------
-- Section: active_instance_uniqueness
-- -----------------------------------------------------------------------------
alter table instances drop constraint if exists instances_slug_key;
alter table instances drop constraint if exists instances_node_id_name_key;

create unique index if not exists instances_active_slug_idx
  on instances(slug)
  where status <> 'deleted';

create unique index if not exists instances_active_node_name_idx
  on instances(node_id, name)
  where status <> 'deleted';

-- -----------------------------------------------------------------------------
-- Section: vless_outbound_groups
-- -----------------------------------------------------------------------------
with defaults as (
  select '[{"key":"default","label":"Default direct","outbound_tag":"direct"}]'::jsonb as groups
),
updated as (
  select
    s.key,
    jsonb_agg(
      case
        when component.value->>'service_code' = 'xray-core' then
          jsonb_set(
            jsonb_set(component.value, '{spec,default_vless_group}', '"default"'::jsonb, true),
            '{spec,vless_groups}',
            defaults.groups,
            true
          )
        else component.value
      end
      order by component.ordinality
    ) as components_json
  from service_pack_templates s
  cross join defaults
  cross join lateral jsonb_array_elements(s.components_json) with ordinality as component(value, ordinality)
  where s.key in (
    'default_access_suite',
    'xray_vless_reality',
    'xray_nginx_grpc_edge',
    'xray_nginx_http_edge'
  )
  group by s.key
)
update service_pack_templates target
set components_json = updated.components_json,
    version = target.version + 1,
    updated_at = now()
from updated
where target.key = updated.key;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
values (gen_random_uuid(), 'system', 'migration.vless_outbound_groups', 'service_pack', 'vless outbound groups added to xray service pack templates', '{}'::jsonb, now());

-- -----------------------------------------------------------------------------
-- Section: firewall_policy_catalog
-- -----------------------------------------------------------------------------
create table if not exists firewall_address_lists(
  id uuid primary key,
  key text not null unique,
  label text not null,
  description text not null default '',
  scope text not null default 'global' check(scope in ('global','control_plane','node','service','client')),
  status text not null default 'active' check(status in ('active','disabled','deleted')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists firewall_address_entries(
  id uuid primary key,
  list_id uuid not null references firewall_address_lists(id) on delete cascade,
  value text not null,
  value_type text not null check(value_type in ('cidr','address','range','dns')),
  label text not null default '',
  status text not null default 'active' check(status in ('active','disabled','deleted')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index if not exists firewall_address_entries_active_value_idx
  on firewall_address_entries(list_id, value)
  where status <> 'deleted';

create table if not exists firewall_policies(
  id uuid primary key,
  key text not null unique,
  label text not null,
  description text not null default '',
  scope text not null default 'node' check(scope in ('control_plane','node','template')),
  node_id uuid null references nodes(id) on delete cascade,
  default_input_policy text not null default 'accept' check(default_input_policy in ('accept','drop','reject')),
  default_forward_policy text not null default 'accept' check(default_forward_policy in ('accept','drop','reject')),
  default_output_policy text not null default 'accept' check(default_output_policy in ('accept','drop','reject')),
  status text not null default 'active' check(status in ('active','disabled','deleted')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists firewall_policies_node_idx
  on firewall_policies(node_id, status);

create table if not exists firewall_rules(
  id uuid primary key,
  policy_id uuid not null references firewall_policies(id) on delete cascade,
  priority integer not null default 1000 check(priority between 1 and 65000),
  chain text not null default 'input' check(chain in ('input','forward','output')),
  action text not null check(action in ('accept','drop','reject')),
  direction text not null default 'in' check(direction in ('in','out','forward')),
  protocol text not null default 'any' check(protocol in ('any','tcp','udp','icmp')),
  src_list_id uuid null references firewall_address_lists(id) on delete set null,
  dst_list_id uuid null references firewall_address_lists(id) on delete set null,
  src_cidr text not null default '',
  dst_cidr text not null default '',
  src_ports text not null default '',
  dst_ports text not null default '',
  state_match text[] not null default '{}'::text[],
  comment text not null default '',
  enabled boolean not null default true,
  log boolean not null default false,
  status text not null default 'active' check(status in ('active','disabled','deleted')),
  metadata_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists firewall_rules_policy_priority_idx
  on firewall_rules(policy_id, priority, created_at)
  where status <> 'deleted';

create table if not exists firewall_revisions(
  id uuid primary key,
  policy_id uuid not null references firewall_policies(id) on delete cascade,
  revision_no integer not null,
  rendered_hash text not null default '',
  rules_json jsonb not null default '[]'::jsonb,
  status text not null default 'active' check(status in ('active','superseded','failed')),
  created_by uuid null references platform_users(id) on delete set null,
  created_at timestamptz not null default now(),
  unique(policy_id, revision_no)
);

create table if not exists firewall_node_state(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete cascade,
  policy_id uuid null references firewall_policies(id) on delete set null,
  revision_id uuid null references firewall_revisions(id) on delete set null,
  desired_revision_id uuid null references firewall_revisions(id) on delete set null,
  status text not null default 'unknown' check(status in ('unknown','pending','applied','failed','drifted')),
  observed_json jsonb not null default '{}'::jsonb,
  last_job_id uuid null references jobs(id) on delete set null,
  updated_at timestamptz not null default now(),
  unique(node_id)
);

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

insert into firewall_address_lists(id, key, label, description, scope, status, created_at, updated_at)
values
  (gen_random_uuid(), 'trusted_control_plane', 'Trusted control plane', 'Control-plane hosts allowed to manage node agent and SSH access.', 'control_plane', 'active', now(), now()),
  (gen_random_uuid(), 'trusted_operators', 'Trusted operators', 'Operator source networks for privileged access.', 'global', 'active', now(), now())
on conflict(key) do nothing;

insert into firewall_policies(id, key, label, description, scope, default_input_policy, default_forward_policy, default_output_policy, status, created_at, updated_at)
values
  (gen_random_uuid(), 'control_plane_default', 'Control plane baseline', 'Baseline policy for control-plane host exposure.', 'control_plane', 'accept', 'accept', 'accept', 'active', now(), now()),
  (gen_random_uuid(), 'node_base', 'Node baseline', 'Baseline node firewall policy. Default accept remains until explicit enforcement is enabled.', 'node', 'accept', 'accept', 'accept', 'active', now(), now())
on conflict(key) do nothing;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
values (gen_random_uuid(), 'system', 'migration.firewall_policy_catalog', 'firewall', 'firewall policy catalog created', '{}'::jsonb, now());

-- -----------------------------------------------------------------------------
-- Section: client_vless_subscriptions
-- -----------------------------------------------------------------------------
create extension if not exists pgcrypto;

create table if not exists client_subscriptions(
  id uuid primary key,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  token_hash text not null,
  token_hint text not null default '',
  status text not null default 'active' check(status in ('active','expired','revoked')),
  expires_at timestamptz not null,
  download_count bigint not null default 0,
  last_used_at timestamptz null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index if not exists client_subscriptions_token_hash_idx
  on client_subscriptions(token_hash);

create index if not exists client_subscriptions_client_status_idx
  on client_subscriptions(client_account_id, status, expires_at desc);

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
select gen_random_uuid(), 'system', 'migration.client_vless_subscriptions', 'client', 'client VLESS subscription token registry created', '{}'::jsonb, now()
where not exists(select 1 from audit_events where action='migration.client_vless_subscriptions');
