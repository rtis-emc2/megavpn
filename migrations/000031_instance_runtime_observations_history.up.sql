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
