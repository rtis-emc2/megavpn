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
