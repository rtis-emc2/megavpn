-- Release: 7.0.1.18
-- Repair existing installations where the consolidated baseline was already
-- marked as applied before the firewall catalog tables were introduced.

create extension if not exists pgcrypto;

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
select gen_random_uuid(), 'system', 'migration.firewall_schema_repair', 'firewall', 'firewall schema repaired for existing installation', '{}'::jsonb, now()
where not exists(select 1 from audit_events where action='migration.firewall_schema_repair');
