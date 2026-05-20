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
