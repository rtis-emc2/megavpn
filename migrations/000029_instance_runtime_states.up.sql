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
