alter table instance_runtime_states
  add column if not exists enabled_state text not null default '',
  add column if not exists config_hash text not null default '',
  add column if not exists listening_ports_json jsonb not null default '[]'::jsonb,
  add column if not exists agent_reported_at timestamptz null;

alter table node_agents
  add column if not exists last_runtime_sync_at timestamptz null;

create index if not exists idx_instance_runtime_states_agent_reported_at
  on instance_runtime_states(agent_reported_at desc);
