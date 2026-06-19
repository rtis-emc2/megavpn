alter table node_agents add column if not exists agent_token_hash text null;
alter table node_agents add column if not exists token_hint text null;
create index if not exists idx_node_agents_token_hash on node_agents(agent_token_hash) where agent_token_hash is not null;
