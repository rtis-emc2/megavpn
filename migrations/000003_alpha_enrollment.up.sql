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
